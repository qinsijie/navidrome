package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/navidrome/navidrome/log"
)

// HQPlayerConfig stores configuration for HQPlayer node
type HQPlayerConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode,omitempty"` // PCM, DSD
}

type HQPlayerClient struct {
	client *http.Client
}

func NewHQPlayerClient() *HQPlayerClient {
	return &HQPlayerClient{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// PlayRawStream pushes the direct raw audio stream URL to HQPlayer
func (c *HQPlayerClient) PlayRawStream(ctx context.Context, cfg HQPlayerConfig, streamURL string, meta TrackMetadata) error {
	if cfg.Host == "" || cfg.Port <= 0 {
		return fmt.Errorf("invalid HQPlayer configuration: %s:%d", cfg.Host, cfg.Port)
	}

	baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	log.Info(fmt.Sprintf("Sending raw audio stream to HQPlayer node %s: %s", baseURL, streamURL))

	// HQPlayer Control API: POST /api/playlist/add or /api/player/play
	payload, _ := json.Marshal(map[string]interface{}{
		"url":    streamURL,
		"title":  meta.Title,
		"artist": meta.Artist,
		"album":  meta.Album,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/playlist/add", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Fallback to simple query string format if REST JSON fails
		playURL := fmt.Sprintf("%s/api/player/play?url=%s", baseURL, streamURL)
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, playURL, nil)
		if err2 != nil {
			return fmt.Errorf("hqplayer request error: %w", err)
		}
		resp2, err2 := c.client.Do(req2)
		if err2 != nil {
			return fmt.Errorf("hqplayer connection failed: %w", err)
		}
		defer resp2.Body.Close()
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hqplayer returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CheckStatus verifies if the HQPlayer node is reachable
func (c *HQPlayerClient) CheckStatus(ctx context.Context, cfg HQPlayerConfig) bool {
	if cfg.Host == "" || cfg.Port <= 0 {
		return false
	}
	url := fmt.Sprintf("http://%s:%d/api/player", cfg.Host, cfg.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}
