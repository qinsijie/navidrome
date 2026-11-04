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
	Action        string  `json:"action"` // play, pause, stop, next, prev, jump, seek, volume, playMode
	Index         int     `json:"index,omitempty"`
	TimeOffset    float64 `json:"timeOffset,omitempty"`
	Volume        int     `json:"volume,omitempty"` // 0-100
	Mode          string  `json:"mode,omitempty"`
	ForceTakeover bool    `json:"forceTakeover,omitempty"`
}

type playStreamRequest struct {
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

// addPublicOutputRoutes mounts unauthenticated endpoints for remote DACs / players (HQPlayer, DLNA)
// to fetch raw physical audio streams without token expiration limitations.
func (api *Router) addPublicOutputRoutes(r chi.Router) {
	streamHandler := func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, "missing track id", http.StatusBadRequest)
			return
		}

		mf, err := api.ds.MediaFile().Get(r.Context(), id)
		if err != nil || mf == nil {
			http.NotFound(w, r)
			return
		}

		if strings.HasPrefix(mf.Path, "online://") {
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
				relay := musicengine.NewStreamRelay(resolved, 0, int64(mf.Size))
				defer relay.Close()
				if err := relay.StartPull(r.Context(), 0); err != nil {
					log.Error(r.Context(), "Audio Stream: Failed to connect to online audio CDN", "id", id, err)
					http.Error(w, "stream relay pull error: "+err.Error(), http.StatusBadGateway)
					return
				}
				pacingRate := stream.CalculateAudioPacingRate(int64(mf.Size), mf.Duration, mf.Suffix)
				pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, 384*1024)
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

		pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, 384*1024)
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
			CurrentIndex: state.CurrentIndex,
			SourceClient: state.SourceClient,
			Queue:        state.Queue,
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
		} else if req.DeviceID == string(output.DeviceTypeLocal) {
			mgr.ReleaseHQPlayerLease(user.ID)
		}

		if err := mgr.SetActiveDevice(req.DeviceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())
		sess.UpdatePlayback(r.Context(), sess.Snapshot().State, sess.Snapshot().Track, sess.Snapshot().CurrentIndex, sess.Snapshot().TimeOffset, sess.Snapshot().PlayMode, req.DeviceID, clientUID)

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

	r.Get("/output/queue", func(w http.ResponseWriter, r *http.Request) {
		user, _ := request.UserFrom(r.Context())
		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sess.Snapshot())
	})

	r.Post("/output/queue", func(w http.ResponseWriter, r *http.Request) {
		var req setQueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		targetHost := mgr.GetActiveTargetHost()
		outboundIP := output.DetectOutboundIP(targetHost)
		port := conf.Server.Port

		for i := range req.Tracks {
			t := &req.Tracks[i]
			if t.Stream == "" && t.TrackID != "" {
				t.Stream = fmt.Sprintf("http://%s:%d/api/output/stream/%s", outboundIP, port, t.TrackID)
			} else if strings.HasPrefix(t.Stream, "/") {
				t.Stream = fmt.Sprintf("http://%s:%d%s", outboundIP, port, t.Stream)
			}
			if t.Duration <= 0 && t.TrackID != "" {
				if mf, err := api.ds.MediaFile().Get(r.Context(), t.TrackID); err == nil && mf != nil {
					t.Duration = float64(mf.Duration)
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
		}

		user, _ := request.UserFrom(r.Context())
		if req.AutoPlay && mgr.GetActiveDevice() == "hqplayer" {
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

		if mgr.GetActiveDevice() != string(output.DeviceTypeLocal) {
			if err := mgr.GetSession().SetQueue(req.Tracks, req.StartIndex, req.TimeOffset, req.PlayMode, req.AutoPlay); err != nil {
				log.Error(r.Context(), "Failed to set output queue", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())
		userSnap := sess.SetQueue(r.Context(), req.Tracks, req.StartIndex, req.TimeOffset, req.PlayMode, req.AutoPlay, mgr.GetActiveDevice(), clientUID)

		// When output device is HQPlayer, report playback on behalf of the user to scrobbler
		if req.AutoPlay && mgr.GetActiveDevice() == "hqplayer" && userSnap.Track.TrackID != "" {
			if tracker := scrobbler.GetActivePlayTracker(); tracker != nil {
				_ = tracker.ReportPlayback(r.Context(), scrobbler.ReportPlaybackParams{
					MediaId:      userSnap.Track.TrackID,
					PositionMs:   int64(req.TimeOffset * 1000),
					State:        scrobbler.StatePlaying,
					PlaybackRate: 1.0,
					ClientId:     "user:" + user.ID,
					ClientName:   "HQPlayer (" + user.UserName + ")",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"userSession": userSnap,
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
		if mgr.GetActiveDevice() == "hqplayer" {
			mgr.AcquireHQPlayerLease(user.ID, user.UserName, meta)
		}
		if err := mgr.PlayStream(r.Context(), resolvedURL, meta, req.TimeOffset); err != nil {
			log.Error(r.Context(), "Failed to play stream on output device", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())
		sess.UpdatePlayback(r.Context(), "playing", meta, -1, req.TimeOffset, "", mgr.GetActiveDevice(), clientUID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "nowPlaying": mgr.GetNowPlaying()})
	})

	r.Post("/output/control", func(w http.ResponseWriter, r *http.Request) {
		var req playbackControlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		user, _ := request.UserFrom(r.Context())
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

		sess := output.GetUserSessionManager().GetSession(user.ID, user.UserName)
		clientUID, _ := request.ClientUniqueIdFrom(r.Context())

		stateStr := "playing"
		targetIndex := req.Index
		snap := sess.Snapshot()
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
			clearSnap := sess.ClearQueue(r.Context(), clientUID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "userSession": clearSnap, "nowPlaying": mgr.GetNowPlaying()})
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

		userSnap := sess.UpdatePlayback(r.Context(), stateStr, currentTrack, targetIndex, req.TimeOffset, req.Mode, "", clientUID)

		if mgr.GetActiveDevice() == "hqplayer" {
			if tracker := scrobbler.GetActivePlayTracker(); tracker != nil && currentTrack.TrackID != "" {
				_ = tracker.ReportPlayback(r.Context(), scrobbler.ReportPlaybackParams{
					MediaId:      currentTrack.TrackID,
					PositionMs:   int64(req.TimeOffset * 1000),
					State:        stateStr,
					PlaybackRate: 1.0,
					ClientId:     "user:" + user.ID,
					ClientName:   "HQPlayer (" + user.UserName + ")",
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "userSession": userSnap, "nowPlaying": mgr.GetNowPlaying()})
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
