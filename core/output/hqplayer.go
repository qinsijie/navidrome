package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/navidrome/navidrome/log"
)

// HQPlayerConfig stores configuration for HQPlayer node
type HQPlayerConfig struct {
	Name    string `json:"name"`
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
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

// TestConnection tests TCP reachability and optional API response
func (c *HQPlayerClient) TestConnection(ctx context.Context, host string, port int) (bool, string) {
	if host == "" {
		return false, "请输入 HQPlayer 主机 IP 或主机名"
	}
	if port <= 0 {
		port = 32400
	}
	target := fmt.Sprintf("%s:%d", host, port)

	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false, fmt.Sprintf("无法连接到 %s: %v", target, err)
	}
	_ = conn.Close()

	// TCP connected successfully! Also check HTTP endpoint
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/api/player", target), nil)
	resp, err := c.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		return true, fmt.Sprintf("成功连接至 HQPlayer 节点 (%s)", target)
	}

	return true, fmt.Sprintf("TCP 握手成功 (%s: 端口开放已就绪)", target)
}

// PlayRawStream pushes the direct raw audio stream URL to HQPlayer
func (c *HQPlayerClient) PlayRawStream(ctx context.Context, cfg HQPlayerConfig, streamURL string, meta TrackMetadata) error {
	if cfg.Host == "" || cfg.Port <= 0 {
		return fmt.Errorf("invalid HQPlayer configuration: %s:%d", cfg.Host, cfg.Port)
	}

	baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	log.Info(fmt.Sprintf("Sending raw audio stream to HQPlayer node %s: %s", baseURL, streamURL))

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
	target := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
