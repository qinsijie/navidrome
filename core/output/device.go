package output

type DeviceType string

const (
	DeviceTypeLocal    DeviceType = "local"
	DeviceTypeDLNA     DeviceType = "dlna"
	DeviceTypeHQPlayer DeviceType = "hqplayer"
)

type Device struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        DeviceType `json:"type"`
	URL         string     `json:"url,omitempty"`
	ControlURL  string     `json:"controlUrl,omitempty"`
	Enabled     bool       `json:"enabled"`
	Active      bool       `json:"active"`
	Status      string     `json:"status"` // "ready", "connected", "online", "offline", "unconfigured"
	Description string     `json:"description,omitempty"`
}

type TrackMetadata struct {
	TrackID  string  `json:"trackId"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Stream   string  `json:"streamUrl"`
	Duration float64 `json:"duration,omitempty"`
	CoverURL string  `json:"coverUrl,omitempty"`
}
