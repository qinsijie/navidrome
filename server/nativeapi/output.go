package nativeapi

import (
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
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
)

type selectDeviceRequest struct {
	DeviceID string `json:"deviceId"`
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
	Action     string  `json:"action"` // play, pause, stop, next, prev, jump, seek, volume, playMode
	Index      int     `json:"index,omitempty"`
	TimeOffset float64 `json:"timeOffset,omitempty"`
	Volume     int     `json:"volume,omitempty"` // 0-100
	Mode       string  `json:"mode,omitempty"`
}

type playStreamRequest struct {
	TrackID    string  `json:"trackId"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	StreamURL  string  `json:"streamUrl"`
	Duration   float64 `json:"duration,omitempty"`
	TimeOffset float64 `json:"timeOffset,omitempty"`
}

type setQueueRequest struct {
	Tracks     []output.TrackMetadata `json:"tracks"`
	StartIndex int                    `json:"startIndex"`
	TimeOffset float64                `json:"timeOffset,omitempty"`
	PlayMode   string                 `json:"playMode,omitempty"`
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
				relay.ServeHTTP(w, r)
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
			)
		}

		bw := stream.NewBurstResponseWriter(w)
		http.ServeContent(bw, r, fi.Name(), fi.ModTime(), f)
	}

	r.Get("/output/stream/{id}", streamHandler)
	r.Head("/output/stream/{id}", streamHandler)
}

func (api *Router) addOutputRoutes(r chi.Router) {
	mgr := output.GetManager()

	r.Get("/output/devices", func(w http.ResponseWriter, r *http.Request) {
		onlyEnabled := r.URL.Query().Get("enabled_only") == "true"
		devices := mgr.GetDevices(r.Context(), onlyEnabled)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"devices":    devices,
			"activeId":   mgr.GetActiveDevice(),
			"hqConfig":   mgr.GetHQPlayerConfig(),
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
		if err := mgr.SetActiveDevice(req.DeviceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

		if err := mgr.GetSession().SetQueue(req.Tracks, req.StartIndex, req.TimeOffset, req.PlayMode); err != nil {
			log.Error(r.Context(), "Failed to set output queue", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"nowPlaying": mgr.GetNowPlaying(),
		})
	})

	r.Post("/output/play", func(w http.ResponseWriter, r *http.Request) {
		var req playStreamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
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
		if err := mgr.PlayStream(r.Context(), resolvedURL, meta, req.TimeOffset); err != nil {
			log.Error(r.Context(), "Failed to play stream on output device", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		if req.Action == "seek" && req.TimeOffset < 1.0 {
			req.TimeOffset = 0
		}
		if err := mgr.ControlPlaybackWithDetails(r.Context(), req.Action, req.Index, req.TimeOffset, req.Volume, req.Mode); err != nil {
			log.Error(r.Context(), "Failed to control output device", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "nowPlaying": mgr.GetNowPlaying()})
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
