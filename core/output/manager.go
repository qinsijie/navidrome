package output

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

var (
	managerInstance *Manager
	once            sync.Once
)

type StoredState struct {
	LocalName      string                `json:"localName"`
	LocalEnabled   bool                  `json:"localEnabled"`
	HQConfig       HQPlayerConfig        `json:"hqConfig"`
	DLNADevices    map[string]DLNADevice `json:"dlnaDevices"`
	ActiveDeviceID string                `json:"activeDeviceId"`
}

type NowPlayingInfo struct {
	DeviceID   string        `json:"deviceId"`
	Track      TrackMetadata `json:"track"`
	State      string        `json:"state"` // playing, paused, stopped
	TimeOffset float64       `json:"timeOffset"`
	StartedAt  int64         `json:"startedAt,omitempty"`
}

type Manager struct {
	mu                sync.RWMutex
	localName         string
	localEnabled      bool
	activeDevID       string
	dlnaScanner       *DLNAScanner
	hqClient          *HQPlayerClient
	hqConfig          HQPlayerConfig
	httpClient        *http.Client
	lastScan          time.Time
	currentTrack      TrackMetadata
	playbackState     string
	currentTimeOffset float64
	playbackStartTime int64
}

func GetManager() *Manager {
	once.Do(func() {
		managerInstance = &Manager{
			localName:    "本机播放",
			localEnabled: true,
			activeDevID:  string(DeviceTypeLocal),
			dlnaScanner:  NewDLNAScanner(),
			hqClient:     NewHQPlayerClient(),
			httpClient:   &http.Client{Timeout: 5 * time.Second},
			hqConfig: HQPlayerConfig{
				Name:    "HQPlayer - 客厅",
				Host:    "",
				Port:    DefaultHQPlayerPort,
				Enabled: false,
			},
		}
		managerInstance.loadState()
		go managerInstance.refreshDevices(context.Background())
	})
	return managerInstance
}

func (m *Manager) stateFilePath() string {
	dir := conf.Server.DataFolder.String()
	if dir == "" {
		pkgVar := os.Getenv("TRIM_PKGVAR")
		if pkgVar != "" {
			dir = filepath.Join(pkgVar, "data")
		} else {
			dir = "./var/data"
		}
	}
	return filepath.Join(dir, "audio_output.json")
}

func (m *Manager) loadState() {
	path := m.stateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var state StoredState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if state.LocalName != "" {
		m.localName = state.LocalName
	}
	m.localEnabled = state.LocalEnabled
	if state.ActiveDeviceID != "" {
		m.activeDevID = state.ActiveDeviceID
	}
	if state.HQConfig.Host != "" || state.HQConfig.Name != "" {
		m.hqConfig = state.HQConfig
		if m.hqConfig.Port <= 0 || m.hqConfig.Port == 32400 {
			m.hqConfig.Port = DefaultHQPlayerPort
		}
	}
	for k, dev := range state.DLNADevices {
		d := dev
		m.dlnaScanner.devices[k] = &d
	}
}

func (m *Manager) saveState() {
	m.mu.RLock()
	state := StoredState{
		LocalName:      m.localName,
		LocalEnabled:   m.localEnabled,
		HQConfig:       m.hqConfig,
		DLNADevices:    make(map[string]DLNADevice),
		ActiveDeviceID: m.activeDevID,
	}
	m.dlnaScanner.mu.RLock()
	for k, v := range m.dlnaScanner.devices {
		state.DLNADevices[k] = *v
	}
	m.dlnaScanner.mu.RUnlock()
	m.mu.RUnlock()

	path := m.stateFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func (m *Manager) Scan(ctx context.Context) {
	m.refreshDevices(ctx)
}

func (m *Manager) refreshDevices(ctx context.Context) {
	_, _ = m.dlnaScanner.Discover(ctx, 3*time.Second)
	m.mu.Lock()
	m.lastScan = time.Now()
	m.mu.Unlock()
	m.saveState()
}

// GetDevices returns audio output destinations. If onlyEnabled is true, only returns active/enabled devices.
func (m *Manager) GetDevices(ctx context.Context, onlyEnabled bool) []Device {
	m.mu.RLock()
	stale := time.Since(m.lastScan) > 45*time.Second
	activeID := m.activeDevID
	locName := m.localName
	locEnabled := m.localEnabled
	hqCfg := m.hqConfig
	m.mu.RUnlock()

	if stale {
		go m.refreshDevices(context.Background())
	}

	var list []Device

	// 1. Local / Browser
	if !onlyEnabled || locEnabled {
		list = append(list, Device{
			ID:          string(DeviceTypeLocal),
			Name:        locName,
			Type:        DeviceTypeLocal,
			Enabled:     locEnabled,
			Active:      activeID == string(DeviceTypeLocal) || activeID == "",
			Status:      "ready",
			Description: "当前浏览器 / App 内置音频播放引擎",
		})
	}

	// 2. HQPlayer Node
	hqStatus := "unconfigured"
	if hqCfg.Host != "" {
		if m.hqClient.CheckStatus(ctx, hqCfg) {
			hqStatus = "connected"
		} else {
			hqStatus = "offline"
		}
	}
	hqDisplayName := hqCfg.Name
	if hqDisplayName == "" {
		if hqCfg.Host != "" {
			hqDisplayName = fmt.Sprintf("HQPlayer (%s:%d)", hqCfg.Host, hqCfg.Port)
		} else {
			hqDisplayName = "HQPlayer Hi-Fi 节点"
		}
	}

	if !onlyEnabled || hqCfg.Enabled {
		list = append(list, Device{
			ID:          "hqplayer",
			Name:        hqDisplayName,
			Type:        DeviceTypeHQPlayer,
			URL:         fmt.Sprintf("http://%s:%d", hqCfg.Host, hqCfg.Port),
			Enabled:     hqCfg.Enabled,
			Active:      activeID == "hqplayer",
			Status:      hqStatus,
			Description: "Hi-Res / DSD 源码直推端点 (旁路服务端重采样与转码)",
		})
	}

	// 3. DLNA Renderers
	m.dlnaScanner.mu.RLock()
	for _, dev := range m.dlnaScanner.devices {
		if onlyEnabled && !dev.Enabled {
			continue
		}
		status := "online"
		if time.Since(dev.LastSeen) > 10*time.Minute {
			status = "offline"
		}
		list = append(list, Device{
			ID:          dev.UDN,
			Name:        dev.Name,
			Type:        DeviceTypeDLNA,
			URL:         dev.Location,
			ControlURL:  dev.ControlURL,
			Enabled:     dev.Enabled,
			Active:      activeID == dev.UDN,
			Status:      status,
			Description: "局域网 DLNA / UPnP AVTransport 串流播放器",
		})
	}
	m.dlnaScanner.mu.RUnlock()

	return list
}

func (m *Manager) SetActiveDevice(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify target device is enabled
	if deviceID == string(DeviceTypeLocal) {
		if !m.localEnabled {
			return fmt.Errorf("本机播放已被禁用")
		}
	} else if deviceID == "hqplayer" {
		if !m.hqConfig.Enabled || m.hqConfig.Host == "" {
			return fmt.Errorf("HQPlayer 节点未启用或未配置 IP")
		}
	} else {
		m.dlnaScanner.mu.RLock()
		dev, ok := m.dlnaScanner.devices[deviceID]
		m.dlnaScanner.mu.RUnlock()
		if !ok || !dev.Enabled {
			return fmt.Errorf("DLNA 目标设备未找到或已被禁用")
		}
	}

	m.activeDevID = deviceID
	log.Info(fmt.Sprintf("Audio output destination switched to: %s", deviceID))
	go m.saveState()
	return nil
}

func (m *Manager) GetActiveDevice() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeDevID == "" {
		return string(DeviceTypeLocal)
	}
	return m.activeDevID
}

func (m *Manager) UpdateDevice(deviceID, name string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if deviceID == string(DeviceTypeLocal) {
		if name != "" {
			m.localName = name
		}
		m.localEnabled = enabled
	} else if deviceID == "hqplayer" {
		if name != "" {
			m.hqConfig.Name = name
		}
		m.hqConfig.Enabled = enabled
	} else {
		m.dlnaScanner.mu.Lock()
		if dev, ok := m.dlnaScanner.devices[deviceID]; ok {
			if name != "" {
				dev.Name = name
			}
			dev.Enabled = enabled
		}
		m.dlnaScanner.mu.Unlock()
	}

	go m.saveState()
	return nil
}

func (m *Manager) SetHQPlayerConfig(cfg HQPlayerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Port <= 0 {
		cfg.Port = DefaultHQPlayerPort
	}
	if cfg.Name == "" {
		cfg.Name = "HQPlayer - 客厅"
	}
	m.hqConfig = cfg
	log.Info(fmt.Sprintf("Updated HQPlayer configuration: %s (%s:%d, enabled=%v)", cfg.Name, cfg.Host, cfg.Port, cfg.Enabled))
	go m.saveState()
	return nil
}

func (m *Manager) GetHQPlayerConfig() HQPlayerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hqConfig
}

// GetActiveTargetHost returns the IP/Hostname of the currently active remote output device
func (m *Manager) GetActiveTargetHost() string {
	m.mu.RLock()
	activeID := m.activeDevID
	hqHost := strings.TrimSpace(m.hqConfig.Host)
	m.mu.RUnlock()

	if activeID == "hqplayer" && hqHost != "" {
		return hqHost
	}

	m.dlnaScanner.mu.RLock()
	dev, ok := m.dlnaScanner.devices[activeID]
	m.dlnaScanner.mu.RUnlock()
	if ok {
		for _, raw := range []string{dev.ControlURL, dev.Location} {
			if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
				return u.Hostname()
			}
		}
	}

	return hqHost
}

// DetectOutboundIP queries the kernel routing table (via UDP dial without sending packets)
// or enumerates physical interfaces to find the real LAN IP reachable by targetHost.
func DetectOutboundIP(targetHost string) string {
	candidates := []string{}
	targetHost = strings.TrimSpace(targetHost)
	if targetHost != "" && targetHost != "localhost" && targetHost != "127.0.0.1" {
		candidates = append(candidates, targetHost)
	}
	candidates = append(candidates, "10.255.255.255", "192.168.255.255", "8.8.8.8")

	for _, dst := range candidates {
		conn, err := net.DialTimeout("udp", net.JoinHostPort(dst, "80"), 500*time.Millisecond)
		if err == nil {
			localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
			_ = conn.Close()
			if ok && !localAddr.IP.IsLoopback() && localAddr.IP.To4() != nil {
				return localAddr.IP.String()
			}
		}
	}

	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					return ip4.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

func IsLoopbackHost(h string) bool {
	h = strings.ToLower(strings.TrimSpace(h))
	return h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]"
}

// ResolveOutboundURL converts relative or loopback stream URLs into a full LAN-reachable raw stream URL
func ResolveOutboundURL(r *http.Request, rawURL string, defaultPort int, targetHost string) string {
	reqHost, reqPort, err := net.SplitHostPort(r.Host)
	if err != nil {
		reqHost = r.Host
		if defaultPort > 0 {
			reqPort = fmt.Sprintf("%d", defaultPort)
		} else {
			reqPort = "4533"
		}
	}
	if reqPort == "" || reqPort == "0" {
		reqPort = "4533"
	}

	if IsLoopbackHost(reqHost) {
		reqHost = DetectOutboundIP(targetHost)
	}
	outboundHostPort := net.JoinHostPort(reqHost, reqPort)

	if strings.HasPrefix(rawURL, "/") {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		rawURL = fmt.Sprintf("%s://%s%s", scheme, outboundHostPort, rawURL)
	}

	if u, err := url.Parse(rawURL); err == nil {
		if IsLoopbackHost(u.Hostname()) {
			uPort := u.Port()
			if uPort == "" {
				uPort = reqPort
			}
			u.Host = net.JoinHostPort(DetectOutboundIP(targetHost), uPort)
		}
		if strings.Contains(u.Path, "/rest/stream") {
			q := u.Query()
			if q.Get("format") == "" {
				q.Set("format", "raw")
			}
			if targetHost != "" {
				q.Set("target", "hqplayer")
			}
			u.RawQuery = q.Encode()
		}
		rawURL = u.String()
	}

	return rawURL
}

func (m *Manager) TestHQPlayer(ctx context.Context, host string, port int) (bool, string) {
	return m.hqClient.TestConnection(ctx, host, port)
}

func (m *Manager) AddManualDLNA(ctx context.Context, name, loc string) (*DLNADevice, error) {
	dev, err := m.dlnaScanner.FetchDeviceFromURL(ctx, loc, name)
	if err != nil {
		return nil, err
	}
	m.saveState()
	return dev, nil
}

func (m *Manager) DeleteDLNA(deviceID string) {
	m.dlnaScanner.mu.Lock()
	delete(m.dlnaScanner.devices, deviceID)
	m.dlnaScanner.mu.Unlock()
	m.saveState()
}

func (m *Manager) GetNowPlaying() NowPlayingInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	offset := m.currentTimeOffset
	if m.playbackState == "playing" && m.playbackStartTime > 0 {
		elapsed := float64(time.Now().UnixMilli()-m.playbackStartTime) / 1000.0
		offset += elapsed
	}
	return NowPlayingInfo{
		DeviceID:   m.activeDevID,
		Track:      m.currentTrack,
		State:      m.playbackState,
		TimeOffset: offset,
		StartedAt:  m.playbackStartTime,
	}
}

func (m *Manager) PlayStream(ctx context.Context, streamURL string, meta TrackMetadata, timeOffset float64) error {
	m.mu.Lock()
	m.currentTrack = meta
	m.playbackState = "playing"
	m.currentTimeOffset = timeOffset
	m.playbackStartTime = time.Now().UnixMilli()
	activeID := m.activeDevID
	hqCfg := m.hqConfig
	m.mu.Unlock()

	if activeID == "" || activeID == string(DeviceTypeLocal) {
		return nil
	}

	if activeID == "hqplayer" {
		return m.hqClient.PlayRawStream(ctx, hqCfg, streamURL, meta, timeOffset)
	}

	m.dlnaScanner.mu.RLock()
	dev, ok := m.dlnaScanner.devices[activeID]
	m.dlnaScanner.mu.RUnlock()

	if !ok || dev.ControlURL == "" {
		return fmt.Errorf("DLNA device %s not found or offline", activeID)
	}

	if err := SetAVTransportURI(ctx, m.httpClient, dev.ControlURL, streamURL, meta.Title, meta.Artist); err != nil {
		return fmt.Errorf("set av transport uri: %w", err)
	}
	if err := Play(ctx, m.httpClient, dev.ControlURL); err != nil {
		return err
	}
	if timeOffset > 0 {
		_ = Seek(ctx, m.httpClient, dev.ControlURL, timeOffset)
	}
	return nil
}

func (m *Manager) ControlPlayback(ctx context.Context, action string, timeOffset float64, volume int) error {
	m.mu.Lock()
	if action == "play" {
		m.playbackState = "playing"
		m.playbackStartTime = time.Now().UnixMilli()
	} else if action == "pause" || action == "stop" {
		if m.playbackState == "playing" && m.playbackStartTime > 0 {
			m.currentTimeOffset += float64(time.Now().UnixMilli()-m.playbackStartTime) / 1000.0
		}
		m.playbackState = action
		m.playbackStartTime = 0
	}
	if action == "seek" {
		m.currentTimeOffset = timeOffset
		m.playbackStartTime = time.Now().UnixMilli()
	}
	activeID := m.activeDevID
	hqCfg := m.hqConfig
	m.mu.Unlock()

	if activeID == "" || activeID == string(DeviceTypeLocal) {
		return nil
	}

	if activeID == "hqplayer" {
		return m.hqClient.Control(ctx, hqCfg, action, timeOffset, volume)
	}

	m.dlnaScanner.mu.RLock()
	dev, ok := m.dlnaScanner.devices[activeID]
	m.dlnaScanner.mu.RUnlock()

	if !ok || dev.ControlURL == "" {
		return nil
	}

	switch action {
	case "play":
		return Play(ctx, m.httpClient, dev.ControlURL)
	case "pause", "stop":
		// Map pause to UPnP AVTransport#Stop to avoid DLNA renderer 501/701 lockups and DAC FIFO underruns
		return Stop(ctx, m.httpClient, dev.ControlURL)
	case "seek":
		return Seek(ctx, m.httpClient, dev.ControlURL, timeOffset)
	case "volume":
		ctrl := dev.RenderingControlURL
		if ctrl == "" {
			ctrl = dev.ControlURL
		}
		return SetVolume(ctx, m.httpClient, ctrl, volume)
	}
	return nil
}
