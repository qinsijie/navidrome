package output

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
)

var (
	managerInstance *Manager
	once            sync.Once
)

type Manager struct {
	mu            sync.RWMutex
	activeDevID   string
	dlnaScanner   *DLNAScanner
	hqClient      *HQPlayerClient
	hqConfig      HQPlayerConfig
	httpClient    *http.Client
	cachedDevices []Device
	lastScan      time.Time
}

func GetManager() *Manager {
	once.Do(func() {
		managerInstance = &Manager{
			activeDevID: string(DeviceTypeLocal),
			dlnaScanner: NewDLNAScanner(),
			hqClient:    NewHQPlayerClient(),
			httpClient:  &http.Client{Timeout: 5 * time.Second},
		}
		// Initial background scan
		go managerInstance.refreshDevices(context.Background())
	})
	return managerInstance
}

func (m *Manager) refreshDevices(ctx context.Context) {
	_, _ = m.dlnaScanner.Discover(ctx, 3*time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastScan = time.Now()
}

// GetDevices returns all available audio output destinations
func (m *Manager) GetDevices(ctx context.Context) []Device {
	// Trigger scan if older than 30s
	m.mu.RLock()
	stale := time.Since(m.lastScan) > 30*time.Second
	activeID := m.activeDevID
	hqCfg := m.hqConfig
	m.mu.RUnlock()

	if stale {
		go m.refreshDevices(context.Background())
	}

	var list []Device

	// 1. Local / Browser
	list = append(list, Device{
		ID:          string(DeviceTypeLocal),
		Name:        "本机 / 浏览器播放",
		Type:        DeviceTypeLocal,
		Active:      activeID == string(DeviceTypeLocal) || activeID == "",
		Description: "当前浏览器内置音频播放引擎",
	})

	// 2. Discovered DLNA Renderers
	dlnaList, _ := m.dlnaScanner.Discover(ctx, 1*time.Second)
	for _, dev := range dlnaList {
		list = append(list, Device{
			ID:          dev.UDN,
			Name:        "DLNA: " + dev.Name,
			Type:        DeviceTypeDLNA,
			URL:         dev.Location,
			ControlURL:  dev.ControlURL,
			Active:      activeID == dev.UDN,
			Description: "局域网 DLNA / UPnP 渲染器",
		})
	}

	// 3. HQPlayer Node if enabled
	if hqCfg.Enabled && hqCfg.Host != "" {
		list = append(list, Device{
			ID:          "hqplayer",
			Name:        fmt.Sprintf("HQPlayer (%s:%d)", hqCfg.Host, hqCfg.Port),
			Type:        DeviceTypeHQPlayer,
			URL:         fmt.Sprintf("http://%s:%d", hqCfg.Host, hqCfg.Port),
			Active:      activeID == "hqplayer",
			Description: "Hi-Res / DSD 源码直推端点",
		})
	}

	return list
}

// SetActiveDevice changes the current active output device
func (m *Manager) SetActiveDevice(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeDevID = deviceID
	log.Info(fmt.Sprintf("Audio output destination switched to: %s", deviceID))
	return nil
}

// GetActiveDevice returns the ID of current active device
func (m *Manager) GetActiveDevice() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeDevID == "" {
		return string(DeviceTypeLocal)
	}
	return m.activeDevID
}

// SetHQPlayerConfig saves HQPlayer configuration
func (m *Manager) SetHQPlayerConfig(cfg HQPlayerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hqConfig = cfg
	log.Info(fmt.Sprintf("Updated HQPlayer configuration: %s:%d (enabled=%v)", cfg.Host, cfg.Port, cfg.Enabled))
	return nil
}

// GetHQPlayerConfig returns the current HQPlayer configuration
func (m *Manager) GetHQPlayerConfig() HQPlayerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hqConfig
}

// PlayStream dispatches stream playback to the currently active output device
func (m *Manager) PlayStream(ctx context.Context, streamURL string, meta TrackMetadata) error {
	m.mu.RLock()
	activeID := m.activeDevID
	hqCfg := m.hqConfig
	m.mu.RUnlock()

	if activeID == "" || activeID == string(DeviceTypeLocal) {
		// Handled by client browser
		return nil
	}

	if activeID == "hqplayer" {
		return m.hqClient.PlayRawStream(ctx, hqCfg, streamURL, meta)
	}

	// DLNA device
	m.dlnaScanner.mu.RLock()
	dev, ok := m.dlnaScanner.devices[activeID]
	m.dlnaScanner.mu.RUnlock()

	if !ok || dev.ControlURL == "" {
		return fmt.Errorf("DLNA device %s not found or offline", activeID)
	}

	if err := SetAVTransportURI(ctx, m.httpClient, dev.ControlURL, streamURL, meta.Title, meta.Artist); err != nil {
		return fmt.Errorf("set av transport uri: %w", err)
	}
	return Play(ctx, m.httpClient, dev.ControlURL)
}

// ControlPlayback dispatches pause/resume/stop to active DLNA device
func (m *Manager) ControlPlayback(ctx context.Context, action string) error {
	m.mu.RLock()
	activeID := m.activeDevID
	m.mu.RUnlock()

	if activeID == "" || activeID == string(DeviceTypeLocal) || activeID == "hqplayer" {
		return nil
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
	case "pause":
		return Pause(ctx, m.httpClient, dev.ControlURL)
	case "stop":
		return Stop(ctx, m.httpClient, dev.ControlURL)
	}
	return nil
}
