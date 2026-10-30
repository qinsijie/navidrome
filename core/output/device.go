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
	Active      bool       `json:"active"`
	Description string     `json:"description,omitempty"`
}

type TrackMetadata struct {
	TrackID string `json:"trackId"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Album   string `json:"album"`
	Stream  string `json:"streamUrl"`
}
