package output

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/navidrome/navidrome/log"
)

const DefaultHQPlayerPort = 4321

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
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// TestConnection tests TCP 4321 reachability and verifies Signalyst HQPlayer XML <State/> response
func (c *HQPlayerClient) TestConnection(ctx context.Context, host string, port int) (bool, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return false, "请输入 HQPlayer 主机 IP 或主机名"
	}
	if port <= 0 {
		port = DefaultHQPlayerPort
	}
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false, fmt.Sprintf("无法连接到 %s: %v", target, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("<State/>\n")); err != nil {
		return false, fmt.Sprintf("发送 HQPlayer 状态探测指令失败 (%s): %v", target, err)
	}

	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Sprintf("端口 %s 已开放，但未收到 HQPlayer XML 响应: %v", target, err)
	}

	resp = strings.TrimSpace(resp)
	if strings.Contains(resp, "<State") && (strings.Contains(resp, "active_mode=") || strings.Contains(resp, "active_rate=") || strings.Contains(resp, "state=")) {
		return true, fmt.Sprintf("成功连接至 HQPlayer 节点 (%s，XML 状态校验通过)", target)
	}

	return false, fmt.Sprintf("端口 %s 响应异常，非有效的 HQPlayer 控制端口: %s", target, resp)
}

// SendHQPlayerXMLCommands sends XML-over-TCP commands sequentially on a single socket and validates result="OK"
func (c *HQPlayerClient) SendHQPlayerXMLCommands(ctx context.Context, cfg HQPlayerConfig, cmds ...string) ([]string, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, fmt.Errorf("HQPlayer 主机未配置")
	}
	port := cfg.Port
	if port <= 0 {
		port = DefaultHQPlayerPort
	}
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, fmt.Errorf("hqplayer tcp dial %s: %w", target, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	reader := bufio.NewReader(conn)
	var responses []string

	for _, cmd := range cmds {
		line := strings.TrimSpace(cmd) + "\n"
		if _, err := conn.Write([]byte(line)); err != nil {
			return responses, fmt.Errorf("hqplayer write xml command %q: %w", cmd, err)
		}

		resp, err := reader.ReadString('\n')
		if err != nil {
			return responses, fmt.Errorf("hqplayer read xml response for %q: %w", cmd, err)
		}
		resp = strings.TrimSpace(resp)
		responses = append(responses, resp)

		if strings.Contains(resp, `result="Error"`) {
			return responses, fmt.Errorf("hqplayer rejected command %q: %s", cmd, resp)
		}
		if !strings.Contains(cmd, "<State") && !strings.Contains(resp, `result="OK"`) {
			return responses, fmt.Errorf("hqplayer unexpected response for %q: %s", cmd, resp)
		}
	}

	return responses, nil
}

// PlayRawStream pushes the direct raw audio stream URL to HQPlayer via Signalyst XML-over-TCP protocol on port 4321
func (c *HQPlayerClient) PlayRawStream(ctx context.Context, cfg HQPlayerConfig, streamURL string, meta TrackMetadata, timeOffset float64) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("HQPlayer 主机未配置")
	}
	port := cfg.Port
	if port <= 0 {
		port = DefaultHQPlayerPort
	}

	escapedURI := escapeXML(streamURL)
	log.Info(fmt.Sprintf("Sending raw audio stream to HQPlayer node (%s:%d): %s (offset: %.1fs)", cfg.Host, port, streamURL, timeOffset))

	cmds := []string{
		"<PlaylistClear/>",
		fmt.Sprintf(`<PlaylistAdd uri="%s"/>`, escapedURI),
		"<Play/>",
	}
	if timeOffset > 0 {
		cmds = append(cmds, fmt.Sprintf(`<Seek position="%d"/>`, int(timeOffset)))
	}

	resps, err := c.SendHQPlayerXMLCommands(ctx, cfg, cmds...)
	if err != nil {
		log.Error(ctx, "HQPlayer XML playback failed", "host", cfg.Host, "port", port, "streamURL", streamURL, "responses", resps, err)
		return err
	}

	log.Info(fmt.Sprintf("HQPlayer XML: successfully queued and started playback for %s (responses: %v)", streamURL, resps))
	return nil
}

// Control sends playback control action to HQPlayer via XML-over-TCP protocol
// Note: "pause" is intentionally mapped to "<Stop/>" to cleanly release HQPlayer DSP pipelines and DAC clock locks.
func (c *HQPlayerClient) Control(ctx context.Context, cfg HQPlayerConfig, action string, timeOffset float64, volume int) error {
	switch action {
	case "play":
		_, err := c.SendHQPlayerXMLCommands(ctx, cfg, "<Play/>")
		return err
	case "pause", "stop":
		_, err := c.SendHQPlayerXMLCommands(ctx, cfg, "<Stop/>")
		return err
	case "seek":
		_, err := c.SendHQPlayerXMLCommands(ctx, cfg, fmt.Sprintf(`<Seek position="%d"/>`, int(timeOffset)))
		return err
	case "volume":
		// Convert 0-100 UI volume to dB attenuation (-60dB to 0dB); ignore error if HQPlayer has fixed volume enabled
		db := -60.0 + (float64(volume)/100.0)*60.0
		if db > 0 {
			db = 0
		}
		if db < -100 {
			db = -100
		}
		_, _ = c.SendHQPlayerXMLCommands(ctx, cfg, fmt.Sprintf(`<Volume value="%.1f"/>`, db))
		return nil
	}
	return nil
}

// CheckStatus verifies if the HQPlayer node is reachable and responds to <State/>
func (c *HQPlayerClient) CheckStatus(ctx context.Context, cfg HQPlayerConfig) bool {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return false
	}
	port := cfg.Port
	if port <= 0 {
		port = DefaultHQPlayerPort
	}
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	if _, err := conn.Write([]byte("<State/>\n")); err != nil {
		return false
	}
	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.Contains(resp, "<State")
}
