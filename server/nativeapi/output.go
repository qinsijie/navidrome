package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/output"
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
	Action     string  `json:"action"` // play, pause, stop, seek, volume
	TimeOffset float64 `json:"timeOffset,omitempty"`
	Volume     int     `json:"volume,omitempty"` // 0-100
}

type playStreamRequest struct {
	TrackID    string  `json:"trackId"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	StreamURL  string  `json:"streamUrl"`
	TimeOffset float64 `json:"timeOffset,omitempty"`
}

func resolveOutboundURL(r *http.Request, rawURL string) string {
	if strings.HasPrefix(rawURL, "/") {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		rawURL = fmt.Sprintf("%s://%s%s", scheme, r.Host, rawURL)
	}

	host := r.Host
	if host != "" && !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1") {
		rawURL = strings.Replace(rawURL, "localhost:4533", host, 1)
		rawURL = strings.Replace(rawURL, "127.0.0.1:4533", host, 1)
	}
	return rawURL
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

	r.Post("/output/play", func(w http.ResponseWriter, r *http.Request) {
		var req playStreamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		resolvedURL := resolveOutboundURL(r, req.StreamURL)
		meta := output.TrackMetadata{
			TrackID: req.TrackID,
			Title:   req.Title,
			Artist:  req.Artist,
			Album:   req.Album,
			Stream:  resolvedURL,
		}
		if err := mgr.PlayStream(r.Context(), resolvedURL, meta, req.TimeOffset); err != nil {
			log.Error(r.Context(), "Failed to play stream on output device", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/output/control", func(w http.ResponseWriter, r *http.Request) {
		var req playbackControlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := mgr.ControlPlayback(r.Context(), req.Action, req.TimeOffset, req.Volume); err != nil {
			log.Error(r.Context(), "Failed to control output device", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
