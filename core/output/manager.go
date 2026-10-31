package output

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

type Manager struct {
	mu           sync.RWMutex
	localName    string
	localEnabled bool
	activeDevID  string
	dlnaScanner  *DLNAScanner
	hqClient     *HQPlayerClient
	hqConfig     HQPlayerConfig
	httpClient   *http.Client
	lastScan     time.Time
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
				Port:    32400,
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
		if m.hqConfig.Port <= 0 {
			m.hqConfig.Port = 32400
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
		cfg.Port = 32400
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

func (m *Manager) PlayStream(ctx context.Context, streamURL string, meta TrackMetadata) error {
	m.mu.RLock()
	activeID := m.activeDevID
	hqCfg := m.hqConfig
	m.mu.RUnlock()

	if activeID == "" || activeID == string(DeviceTypeLocal) {
		return nil
	}

	if activeID == "hqplayer" {
		return m.hqClient.PlayRawStream(ctx, hqCfg, streamURL, meta)
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
	return Play(ctx, m.httpClient, dev.ControlURL)
}

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
