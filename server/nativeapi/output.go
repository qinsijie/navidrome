package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/musicengine"
	"github.com/navidrome/navidrome/core/output"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/events"
)

type selectDeviceRequest struct {
	DeviceID      string `json:"deviceId"`
	ForceTakeover bool   `json:"forceTakeover,omitempty"`
}

type updateDeviceRequest struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
}

type testHQPlayerRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type addDLNARequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type playbackControlRequest struct {
	Channel       string  `json:"channel,omitempty"` // "local" or "remote"
	DeviceID      string  `json:"deviceId,omitempty"`
	Action        string  `json:"action"` // play, pause, stop, next, prev, jump, seek, volume, playMode, clear
	Index         int     `json:"index,omitempty"`
	TimeOffset    float64 `json:"timeOffset,omitempty"`
	Volume        int     `json:"volume,omitempty"` // 0-100
	Mode          string  `json:"mode,omitempty"`
	ForceTakeover bool    `json:"forceTakeover,omitempty"`
}

type playStreamRequest struct {
	Channel       string  `json:"channel,omitempty"` // "local" or "remote"
	DeviceID      string  `json:"deviceId,omitempty"`
	TrackID       string  `json:"trackId"`
	Title         string  `json:"title"`
	Artist        string  `json:"artist"`
	Album         string  `json:"album"`
	StreamURL     string  `json:"streamUrl"`
	Duration      float64 `json:"duration,omitempty"`
	TimeOffset    float64 `json:"timeOffset,omitempty"`
	ForceTakeover bool    `json:"forceTakeover,omitempty"`
}

type setQueueRequest struct {
	Channel       string                 `json:"channel,omitempty"` // "local" or "remote"
	DeviceID      string                 `json:"deviceId,omitempty"`
	Tracks        []output.TrackMetadata `json:"tracks"`
	StartIndex    int                    `json:"startIndex"`
	TimeOffset    float64                `json:"timeOffset,omitempty"`
	PlayMode      string                 `json:"playMode,omitempty"`
	AutoPlay      bool                   `json:"autoPlay,omitempty"`
	ForceTakeover bool                   `json:"forceTakeover,omitempty"`
}

func resolveOutboundURL(r *http.Request, rawURL string, targetHost string) string {
	return output.ResolveOutboundURL(r, rawURL, conf.Server.Port, targetHost)
}

func appendPCMModeIfHQPlayer(streamURL string, isHQPlayer bool, mgr *output.Manager) string {
	if !isHQPlayer || streamURL == "" || mgr.GetHQPlayerConfig().StreamMode == "raw" {
		return streamURL
	}
	if !strings.Contains(streamURL, "/api/output/stream/") || strings.Contains(streamURL, "mode=") {
		return streamURL
	}
	pathPart := streamURL
	queryPart := ""
	if idx := strings.Index(streamURL, "?"); idx != -1 {
		pathPart = streamURL[:idx]
		queryPart = streamURL[idx+1:]
	}
	if !strings.HasSuffix(strings.ToLower(pathPart), ".wav") {
		pathPart += ".wav"
	}
	if queryPart == "" {
		return pathPart + "?mode=pcm"
	}
	return pathPart + "?" + queryPart + "&mode=pcm"
}

// addPublicOutputRoutes mounts unauthenticated endpoints for remote DACs / players (HQPlayer, DLNA)
// to fetch raw physical audio streams without token expiration limitations.
func (api *Router) addPublicOutputRoutes(r chi.Router) {
	streamHandler := func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(chi.URLParam(r, "id"), ".wav")
		if id == "" {
			http.Error(w, "missing track id", http.StatusBadRequest)
			return
		}

		mf, err := api.ds.MediaFile().Get(r.Context(), id)
		if err != nil || mf == nil {
			http.NotFound(w, r)
			return
		}

		if r.URL.Query().Get("mode") == "pcm" && !output.IsDSDMedia(mf.Suffix, mf.Path) {
			output.ServePCMStream(w, r, mf, api.ds)
			return
		}

		if strings.HasPrefix(mf.Path, "online://") {
			// 0. Cache hit: direct disk stream with pacer
			pacingRate := stream.CalculateAudioPacingRate(int64(mf.Size), mf.Duration, mf.Suffix)
			pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, 1024*1024)
			if musicengine.GetCacheManager().ServeCachedTrack(pw, r, mf.ID) {
				return
			}

			source, songID, quality, ok := musicengine.ParseProtocolPath(mf.Path)
			if !ok {
				source, songID, ok = musicengine.ParseUniversalID(mf.ID)
			}
			if ok {
				engine := musicengine.GetEngine(api.ds)
				resolved, err := engine.ResolveStreamURL(r.Context(), source, songID, quality)
				if err != nil {
					log.Error(r.Context(), "Audio Stream: Failed to resolve online stream URL", "id", id, err)
					http.Error(w, "stream resolve error: "+err.Error(), http.StatusBadGateway)
					return
				}
				relay := musicengine.NewStreamRelay(resolved, 0, int64(mf.Size), mf.ID)
				defer relay.Close()
				if err := relay.StartPull(r.Context(), 0); err != nil {
					log.Error(r.Context(), "Audio Stream: Failed to connect to online audio CDN", "id", id, err)
					http.Error(w, "stream relay pull error: "+err.Error(), http.StatusBadGateway)
					return
				}
				relay.ServeHTTP(pw, r)
				return
			}
		}

		filePath := mf.AbsolutePath()
		f, err := os.Open(filePath)
		if err != nil {
			http.Error(w, "file not found: "+err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "stat error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ext := strings.ToLower(strings.TrimPrefix(mf.Suffix, "."))
		mimeType := "audio/flac"
		switch ext {
		case "flac":
			mimeType = "audio/flac"
		case "wav", "wave":
			mimeType = "audio/wav"
		case "dsf":
			mimeType = "audio/dsf"
		case "dff":
			mimeType = "audio/x-dff"
		case "ape":
			mimeType = "audio/x-ape"
		case "wv":
			mimeType = "audio/x-wavpack"
		case "mp3":
			mimeType = "audio/mpeg"
		case "m4a", "aac":
			mimeType = "audio/mp4"
		case "ogg", "oga":
			mimeType = "audio/ogg"
		}

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Content-Duration", strconv.FormatFloat(float64(mf.Duration), 'G', -1, 32))

		pacingRate := stream.CalculateAudioPacingRate(fi.Size(), mf.Duration, mf.Suffix)
		if stream.IsStreamDebug {
			log.Info(r.Context(), "Audio Stream Request Received",
				"trackId", id,
				"filePath", filePath,
				"fileSize", fi.Size(),
				"mimeType", mimeType,
				"rangeHeader", r.Header.Get("Range"),
				"method", r.Method,
				"remoteAddr", r.RemoteAddr,
				"isRawPassthrough", true,
				"pacingRateKBps", int(pacingRate/1024),
			)
		} else {
			log.Debug(r.Context(), "Audio Stream Request Received",
				"trackId", id,
				"filePath", filePath,
				"fileSize", fi.Size(),
				"mimeType", mimeType,
				"rangeHeader", r.Header.Get("Range"),
				"method", r.Method,
				"remoteAddr", r.RemoteAddr,
				"isRawPassthrough", true,
				"pacingRateKBps", int(pacingRate/1024),
			)
		}

		pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, 1024*1024)
		http.ServeContent(pw, r, fi.Name(), fi.ModTime(), f)
	}

	r.Get("/output/stream/{id}", streamHandler)
	r.Head("/output/stream/{id}", streamHandler)
}

func (api *Router) addOutputRoutes(r chi.Router) {
	mgr := output.GetManager()

	// Wire UserSession real-time state broadcast to SSE Broker
	output.GetUserSessionManager().SetBroadcaster(func(ctx context.Context, username string, state output.UserSessionState) {
		senderCtx := request.WithUsername(ctx, username)
		if state.SourceClient != "" {
			senderCtx = request.WithClientUniqueId(senderCtx, state.SourceClient)
		}
		evt := &events.UserPlaybackState{
			UserID:       state.UserID,
			UserName:     state.UserName,
			TrackID:      state.Track.TrackID,
			Title:        state.Track.Title,
			Artist:       state.Track.Artist,
			Album:        state.Track.Album,
			Duration:     state.Track.Duration,
			State:        state.State,
			TimeOffset:   state.TimeOffset,
			StartedAt:    state.StartedAt,
			Epoch:        state.Epoch,
			OutputTarget: state.OutputTarget,
			PlayMode:     state.PlayMode,
			CurrentIndex:   state.CurrentIndex,
			SourceClient:   state.SourceClient,
			ActiveClientID: state.ActiveClientID,
			Queue:          state.Queue,
			Channel:      state.ActiveEvent,
			ActiveEvent:  state.ActiveEvent,
			Local:        state.Local,
			Remote:       state.Remote,
		}
		events.GetBroker().SendMessage(senderCtx, evt)
	})

	r.Get("/output/devices", func(w http.ResponseWriter, r *http.Request) {
		onlyEnabled := r.URL.Query().Get("enabled_only") == "true"
		devices := mgr.GetDevices(r.Context(), onlyEnabled)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"devices":    devices,
			"activeId":   string(output.DeviceTypeLocal),
			"hqConfig":   mgr.GetHQPlayerConfig(),
			"hqLease":    mgr.GetHQPlayerLease(),
			"nowPlaying": mgr.GetNowPlaying(),
		})
	})

	r.Get("/output/status", func(w http.ResponseWriter, r *http.Request) {
		status := mgr.GetNowPlaying()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	r.Post("/output/device", func(w http.ResponseWriter, r *http.Request) {
		var req selectDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, _ := request.UserFrom(r.Context())
		if req.DeviceID == "hqplayer" {
			canAccess, owner := mgr.CheckHQPlayerAccess(user.ID, user.UserName, req.ForceTakeover)
			if !canAccess {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":     "conflict",
					"code":       "HQPLAYER_OCCUPIED",
					"occupiedBy": owner,
				})
				return
			}
			if err := mgr.SetActiveDevice(req.DeviceID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if req.DeviceID != string(output.DeviceTypeLocal) && req.DeviceID != "" {
			if err := mgr.SetActiveDevice(req.DeviceID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		// Note: When switching to "local", we do NOT release HQPlayer or stop remote hardware.
		// "Local" and "Remote" operate as independent dual tracks.

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "activeId": req.DeviceID})
	})

	r.Post("/output/device/update", func(w http.ResponseWriter, r *http.Request) {
		var req updateDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := mgr.UpdateDevice(req.DeviceID, req.Name, req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/output/hqplayer/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg output.HQPlayerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := mgr.SetHQPlayerConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/output/hqplayer/test", func(w http.ResponseWriter, r *http.Request) {
		var req testHQPlayerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		ok, msg := mgr.TestHQPlayer(r.Context(), req.Host, req.Port)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      ok,
			"message": msg,
		})
	})

	r.Post("/output/dlna/scan", func(w http.ResponseWriter, r *http.Request) {
		mgr.Scan(r.Context())
		devices := mgr.GetDevices(r.Context(), false)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"devices": devices,
		})
	})

	r.Post("/output/dlna/add", func(w http.ResponseWriter, r *http.Request) {
		var req addDLNARequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		dev, err := mgr.AddManualDLNA(r.Context(), req.Name, req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"device": dev,
		})
	})

	r.Delete("/output/dlna/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		mgr.DeleteDLNA(id)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Get("/player/terminal-queue", func(w http.ResponseWriter, r *http.Request) {
		clientID := r.URL.Query().Get("clientId")
		if clientID == "" {
			if id, ok := request.ClientUniqueIdFrom(r.Context()); ok {
				clientID = id
			}
		}
		store := output.GetTerminalStore()
		state, found := store.Get(clientID)
		w.Header().Set("Content-Type", "application/json")
		if !found {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"found": false,
				"queue": []any{},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"found":        true,
			"clientId":     state.ClientID,
			"clientName":   state.ClientName,
			"queue":        state.Queue,
			"currentIndex": state.CurrentIdx,
			"timeOffset":   state.TimeOffset,
			"updatedAt":    state.UpdatedAt,
		})
	})

	r.Post("/player/terminal-queue", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientID   string                 `json:"clientId"`
			ClientName string                 `json:"clientName"`
			Queue      []output.TrackMetadata `json:"queue"`
			CurrentIdx int                    `json:"currentIndex"`
			TimeOffset float64                `json:"timeOffset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.ClientID == "" {
			if id, ok := request.ClientUniqueIdFrom(r.Context()); ok {
				req.ClientID = id
			}
		}
		if req.ClientID != "" {
			store := output.GetTerminalStore()
			store.Save(req.ClientID, req.ClientName, req.Queue, req.CurrentIdx, req.TimeOffset)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Get("/output/queue", func(w http.ResponseWriter, r *http.Request) {
		user, _ := request.UserFrom(r.Context())
		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		channel := r.URL.Query().Get("channel")
		deviceID := r.URL.Query().Get("deviceId")
		w.Header().Set("Content-Type", "application/json")
		if channel == "local" || deviceID == "local" {
			_ = json.NewEncoder(w).Encode(sess.Snapshot("local"))
		} else if channel == "remote" || (deviceID != "" && deviceID != "local") {
			status := mgr.GetNowPlaying()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"channel":      "remote",
				"deviceId":     status.DeviceID,
				"queue":        status.Queue,
				"currentIndex": status.CurrentIndex,
				"track":        status.Track,
				"state":        status.State,
				"timeOffset":   status.TimeOffset,
				"startedAt":    status.StartedAt,
				"epoch":        status.Epoch,
				"playMode":     status.PlayMode,
			})
		} else {
			_ = json.NewEncoder(w).Encode(sess.DualSnapshot("local", ""))
		}
	})

	r.Post("/output/queue", func(w http.ResponseWriter, r *http.Request) {
		var req setQueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		targetChannel := output.NormalizeChannel(req.Channel, req.DeviceID)
		targetDev := req.DeviceID
		if targetDev == "" {
			if targetChannel == "remote" {
				targetDev = mgr.GetActiveDevice()
			} else {
				targetDev = string(output.DeviceTypeLocal)
			}
		}

		targetHost := mgr.GetActiveTargetHost()
		outboundIP := output.DetectOutboundIP(targetHost)
		port := conf.Server.Port
		isHQPlayer := targetChannel == "remote" && (targetDev == "hqplayer" || (targetDev == "" && mgr.GetActiveDevice() == "hqplayer"))

		for i := range req.Tracks {
			t := &req.Tracks[i]
			if t.Stream == "" && t.TrackID != "" {
				t.Stream = fmt.Sprintf("http://%s:%d/api/output/stream/%s", outboundIP, port, t.TrackID)
			} else if strings.HasPrefix(t.Stream, "/") {
				t.Stream = fmt.Sprintf("http://%s:%d%s", outboundIP, port, t.Stream)
			}
			isDSD := false
			if t.TrackID != "" {
				if mf, err := api.ds.MediaFile().Get(r.Context(), t.TrackID); err == nil && mf != nil {
					isDSD = output.IsDSDMedia(mf.Suffix, mf.Path)
					if t.Duration <= 0 {
						t.Duration = float64(mf.Duration)
					}
					if t.Title == "" {
						t.Title = mf.Title
					}
					if t.Artist == "" {
						t.Artist = mf.Artist
					}
					if t.Album == "" {
						t.Album = mf.Album
					}
				}
			}
			if !isDSD {
				t.Stream = appendPCMModeIfHQPlayer(t.Stream, isHQPlayer, mgr)
			}
		}

		user, _ := request.UserFrom(r.Context())
		if targetChannel == "remote" {
			if targetDev != "" && targetDev != string(output.DeviceTypeLocal) {
				if mgr.GetActiveDevice() != targetDev {
					_ = mgr.SetActiveDevice(targetDev)
				}
				if mgr.GetSession() != nil {
					mgr.GetSession().SetDeviceID(targetDev)
				}
			}
			if req.AutoPlay && (targetDev == "hqplayer" || mgr.GetActiveDevice() == "hqplayer") {
				canAccess, owner := mgr.CheckHQPlayerAccess(user.ID, user.UserName, req.ForceTakeover)
				if !canAccess {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"status":     "conflict",
						"code":       "HQPLAYER_OCCUPIED",
						"occupiedBy": owner,
					})
					return
				}
				if len(req.Tracks) > req.StartIndex && req.StartIndex >= 0 {
					mgr.AcquireHQPlayerLease(user.ID, user.UserName, req.Tracks[req.StartIndex])
				}
			}

			if err := mgr.GetSession().SetQueue(req.Tracks, req.StartIndex, req.TimeOffset, req.PlayMode, req.AutoPlay); err != nil {
				log.Error(r.Context(), "Failed to set output queue", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())
		dualSnap := sess.SetQueue(r.Context(), targetChannel, req.Tracks, req.StartIndex, req.TimeOffset, req.PlayMode, req.AutoPlay, targetDev, clientUID)

		// When output device is HQPlayer and remote channel is playing, report playback to scrobbler
		if targetChannel == "remote" && req.AutoPlay && (targetDev == "hqplayer" || mgr.GetActiveDevice() == "hqplayer") && dualSnap.Remote.Track.TrackID != "" {
			if tracker := scrobbler.GetActivePlayTracker(); tracker != nil {
				_ = tracker.ReportPlayback(r.Context(), scrobbler.ReportPlaybackParams{
					MediaId:      dualSnap.Remote.Track.TrackID,
					PositionMs:   int64(req.TimeOffset * 1000),
					State:        scrobbler.StatePlaying,
					PlaybackRate: 1.0,
					ClientId:     "user:" + user.ID + ":remote",
					ClientName:   "HQPlayer (" + user.UserName + ")",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"channel":     targetChannel,
			"userSession": dualSnap,
			"nowPlaying":  mgr.GetNowPlaying(),
		})
	})

	r.Post("/output/play", func(w http.ResponseWriter, r *http.Request) {
		var req playStreamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, _ := request.UserFrom(r.Context())
		if mgr.GetActiveDevice() == "hqplayer" {
			canAccess, owner := mgr.CheckHQPlayerAccess(user.ID, user.UserName, req.ForceTakeover)
			if !canAccess {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":     "conflict",
					"code":       "HQPLAYER_OCCUPIED",
					"occupiedBy": owner,
				})
				return
			}
		}

		if req.TimeOffset < 1.0 {
			req.TimeOffset = 0
		}

		if req.StreamURL == "" && req.TrackID != "" {
			targetHost := mgr.GetActiveTargetHost()
			outboundIP := output.DetectOutboundIP(targetHost)
			req.StreamURL = fmt.Sprintf("http://%s:%d/api/output/stream/%s", outboundIP, conf.Server.Port, req.TrackID)
		}

		resolvedURL := resolveOutboundURL(r, req.StreamURL, mgr.GetActiveTargetHost())
		duration := req.Duration
		if duration <= 0 && req.TrackID != "" {
			if mf, err := api.ds.MediaFile().Get(r.Context(), req.TrackID); err == nil && mf != nil {
				duration = float64(mf.Duration)
				if req.Title == "" {
					req.Title = mf.Title
				}
				if req.Artist == "" {
					req.Artist = mf.Artist
				}
				if req.Album == "" {
					req.Album = mf.Album
				}
			}
		}

		meta := output.TrackMetadata{
			TrackID:  req.TrackID,
			Title:    req.Title,
			Artist:   req.Artist,
			Album:    req.Album,
			Stream:   resolvedURL,
			Duration: duration,
		}
		targetChannel := output.NormalizeChannel(req.Channel, req.DeviceID)
		targetDev := req.DeviceID
		if targetDev == "" {
			if targetChannel == "remote" {
				targetDev = mgr.GetActiveDevice()
			} else {
				targetDev = string(output.DeviceTypeLocal)
			}
		}

		if targetChannel == "remote" {
			isHQPlayer := targetDev == "hqplayer" || mgr.GetActiveDevice() == "hqplayer"
			isDSD := false
			if req.TrackID != "" {
				if mf, err := api.ds.MediaFile().Get(r.Context(), req.TrackID); err == nil && mf != nil {
					isDSD = output.IsDSDMedia(mf.Suffix, mf.Path)
				}
			}
			if !isDSD {
				resolvedURL = appendPCMModeIfHQPlayer(resolvedURL, isHQPlayer, mgr)
				meta.Stream = resolvedURL
			}
			if isHQPlayer {
				canAccess, owner := mgr.CheckHQPlayerAccess(user.ID, user.UserName, req.ForceTakeover)
				if !canAccess {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"status":     "conflict",
						"code":       "HQPLAYER_OCCUPIED",
						"occupiedBy": owner,
					})
					return
				}
				mgr.AcquireHQPlayerLease(user.ID, user.UserName, meta)
			}
			if err := mgr.PlayStream(r.Context(), resolvedURL, meta, req.TimeOffset); err != nil {
				log.Error(r.Context(), "Failed to play stream on output device", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())
		dualSnap := sess.UpdatePlayback(r.Context(), targetChannel, "playing", meta, -1, req.TimeOffset, "", targetDev, clientUID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"channel":     targetChannel,
			"userSession": dualSnap,
			"nowPlaying":  mgr.GetNowPlaying(),
		})
	})

	r.Post("/output/control", func(w http.ResponseWriter, r *http.Request) {
		var req playbackControlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		targetChannel := output.NormalizeChannel(req.Channel, req.DeviceID)
		targetDev := req.DeviceID
		if targetDev == "" {
			if targetChannel == "remote" {
				targetDev = mgr.GetActiveDevice()
			} else {
				targetDev = string(output.DeviceTypeLocal)
			}
		}

		user, _ := request.UserFrom(r.Context())
		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())

		if targetChannel == "remote" {
			if req.Action == "pause" || req.Action == "stop" {
				mgr.ReleaseHQPlayerLease(user.ID)
			}

			if req.Action == "seek" && req.TimeOffset < 1.0 {
				req.TimeOffset = 0
			}
			if err := mgr.ControlPlaybackWithDetails(r.Context(), req.Action, req.Index, req.TimeOffset, req.Volume, req.Mode); err != nil {
				log.Error(r.Context(), "Failed to control output device", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		stateStr := "playing"
		targetIndex := req.Index
		snap := sess.Snapshot(targetChannel)
		currentTrack := snap.Track

		switch req.Action {
		case "pause":
			stateStr = "paused"
		case "stop":
			stateStr = "stopped"
		case "play":
			stateStr = "playing"
		case "seek":
			stateStr = snap.State
		case "clear":
			clearSnap := sess.ClearQueue(r.Context(), targetChannel, clientUID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":      "ok",
				"channel":     targetChannel,
				"userSession": clearSnap,
				"nowPlaying":  mgr.GetNowPlaying(),
			})
			return
		case "next":
			stateStr = "playing"
			if snap.CurrentIndex+1 < len(snap.Queue) {
				targetIndex = snap.CurrentIndex + 1
				currentTrack = snap.Queue[targetIndex]
			}
		case "prev":
			stateStr = "playing"
			if snap.CurrentIndex-1 >= 0 {
				targetIndex = snap.CurrentIndex - 1
				currentTrack = snap.Queue[targetIndex]
			}
		case "jump":
			stateStr = "playing"
			if req.Index >= 0 && req.Index < len(snap.Queue) {
				targetIndex = req.Index
				currentTrack = snap.Queue[targetIndex]
			}
		}

		dualSnap := sess.UpdatePlayback(r.Context(), targetChannel, stateStr, currentTrack, targetIndex, req.TimeOffset, req.Mode, targetDev, clientUID)

		if targetChannel == "remote" && (targetDev == "hqplayer" || mgr.GetActiveDevice() == "hqplayer") {
			if tracker := scrobbler.GetActivePlayTracker(); tracker != nil && currentTrack.TrackID != "" {
				_ = tracker.ReportPlayback(r.Context(), scrobbler.ReportPlaybackParams{
					MediaId:      currentTrack.TrackID,
					PositionMs:   int64(req.TimeOffset * 1000),
					State:        stateStr,
					PlaybackRate: 1.0,
					ClientId:     "user:" + user.ID + ":remote",
					ClientName:   "HQPlayer (" + user.UserName + ")",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"channel":     targetChannel,
			"userSession": dualSnap,
			"nowPlaying":  mgr.GetNowPlaying(),
		})
	})

	r.Post("/output/pause", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.ControlPlayback(r.Context(), "pause", 0, 0); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/output/seek", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TimeOffset float64 `json:"timeOffset"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.TimeOffset < 1.0 {
			req.TimeOffset = 0
		}
		if err := mgr.ControlPlayback(r.Context(), "seek", req.TimeOffset, 0); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/output/volume", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Volume int `json:"volume"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if err := mgr.ControlPlayback(r.Context(), "volume", 0, req.Volume); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}
