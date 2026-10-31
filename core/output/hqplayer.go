package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
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

// SendHQPlayerCommand sends line-based commands to HQPlayer via TCP port 32400
func (c *HQPlayerClient) SendHQPlayerCommand(ctx context.Context, cfg HQPlayerConfig, cmds ...string) error {
	if cfg.Host == "" {
		return fmt.Errorf("HQPlayer 主机未配置")
	}
	port := cfg.Port
	if port <= 0 {
		port = 32400
	}
	target := fmt.Sprintf("%s:%d", cfg.Host, port)

	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return fmt.Errorf("hqplayer tcp dial %s: %w", target, err)
	}
	defer conn.Close()

	for _, cmd := range cmds {
		line := strings.TrimSpace(cmd) + "\n"
		if _, err := conn.Write([]byte(line)); err != nil {
			return fmt.Errorf("hqplayer write command %q: %w", cmd, err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// PlayRawStream pushes the direct raw audio stream URL to HQPlayer via TCP command protocol or HTTP REST
func (c *HQPlayerClient) PlayRawStream(ctx context.Context, cfg HQPlayerConfig, streamURL string, meta TrackMetadata, timeOffset float64) error {
	if cfg.Host == "" {
		return fmt.Errorf("HQPlayer 主机未配置")
	}
	port := cfg.Port
	if port <= 0 {
		port = 32400
	}

	log.Info(fmt.Sprintf("Sending raw audio stream to HQPlayer node (%s:%d): %s (offset: %.1fs)", cfg.Host, port, streamURL, timeOffset))

	// 1. First try line-based TCP command (HQPlayer standard control protocol on 32400)
	cmds := []string{
		"playlist-clear",
		fmt.Sprintf("playlist-add %s", streamURL),
		"play",
	}
	if timeOffset > 0 {
		cmds = append(cmds, fmt.Sprintf("seek %d", int(timeOffset)))
	}

	tcpErr := c.SendHQPlayerCommand(ctx, cfg, cmds...)
	if tcpErr == nil {
		log.Info(fmt.Sprintf("HQPlayer TCP: successfully queued and triggered playback for %s", streamURL))
		return nil
	}

	log.Warn(fmt.Sprintf("HQPlayer TCP command returned: %v, trying HTTP REST fallback...", tcpErr))

	// 2. Fallback to HTTP REST API
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, port)
	payload, _ := json.Marshal(map[string]interface{}{
		"url":    streamURL,
		"title":  meta.Title,
		"artist": meta.Artist,
		"album":  meta.Album,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/playlist/add", bytes.NewReader(payload))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		if resp, err := c.client.Do(req); err == nil {
			resp.Body.Close()
			playReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/player/play", nil)
			if pResp, pErr := c.client.Do(playReq); pErr == nil {
				pResp.Body.Close()
				return nil
			}
		}
	}

	// 3. Simple URL play fallback
	playURL := fmt.Sprintf("%s/api/player/play?url=%s", baseURL, streamURL)
	req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, playURL, nil)
	if err2 == nil {
		if resp2, err2 := c.client.Do(req2); err2 == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode < 400 {
				return nil
			}
			body, _ := io.ReadAll(resp2.Body)
			return fmt.Errorf("hqplayer returned status %d: %s", resp2.StatusCode, string(body))
		}
	}

	return fmt.Errorf("hqplayer push failed: %w", tcpErr)
}

// Control sends playback control action to HQPlayer
func (c *HQPlayerClient) Control(ctx context.Context, cfg HQPlayerConfig, action string, timeOffset float64, volume int) error {
	switch action {
	case "play":
		if err := c.SendHQPlayerCommand(ctx, cfg, "play"); err == nil {
			return nil
		}
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/player/play", nil)
		_, _ = c.client.Do(req)
	case "pause":
		if err := c.SendHQPlayerCommand(ctx, cfg, "pause"); err == nil {
			return nil
		}
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/player/pause", nil)
		_, _ = c.client.Do(req)
	case "stop":
		if err := c.SendHQPlayerCommand(ctx, cfg, "stop"); err == nil {
			return nil
		}
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/player/stop", nil)
		_, _ = c.client.Do(req)
	case "seek":
		if err := c.SendHQPlayerCommand(ctx, cfg, fmt.Sprintf("seek %d", int(timeOffset))); err == nil {
			return nil
		}
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/player/seek?offset=%d", baseURL, int(timeOffset)), nil)
		_, _ = c.client.Do(req)
	case "volume":
		volVal := volume
		if volVal < 0 {
			volVal = 0
		}
		if volVal > 100 {
			volVal = 100
		}
		if err := c.SendHQPlayerCommand(ctx, cfg, fmt.Sprintf("set-volume %d", volVal)); err == nil {
			return nil
		}
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/player/volume?volume=%d", baseURL, volVal), nil)
		_, _ = c.client.Do(req)
	}
	return nil
}

// CheckStatus verifies if the HQPlayer node is reachable
func (c *HQPlayerClient) CheckStatus(ctx context.Context, cfg HQPlayerConfig) bool {
	if cfg.Host == "" {
		return false
	}
	port := cfg.Port
	if port <= 0 {
		port = 32400
	}
	target := fmt.Sprintf("%s:%d", cfg.Host, port)
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
