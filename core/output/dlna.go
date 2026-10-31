package output

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
)

const ssdpMulticastAddr = "239.255.255.250:1900"

var ssdpQueries = []string{
	"M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n",
	"M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n",
	"M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: ssdp:all\r\n\r\n",
}

// DLNADevice represents a discovered or manually added DLNA MediaRenderer
type DLNADevice struct {
	UDN        string    `json:"udn"`
	Name       string    `json:"name"`
	Location   string    `json:"location"`
	ControlURL string    `json:"controlUrl"`
	Enabled    bool      `json:"enabled"`
	LastSeen   time.Time `json:"lastSeen"`
}

// DLNAScanner performs SSDP multicast discovery for DLNA DMR devices
type DLNAScanner struct {
	mu      sync.RWMutex
	devices map[string]*DLNADevice
	client  *http.Client
}

func NewDLNAScanner() *DLNAScanner {
	return &DLNAScanner{
		devices: make(map[string]*DLNADevice),
		client:  &http.Client{Timeout: 4 * time.Second},
	}
}

// Discover sends SSDP M-SEARCH broadcasts across multicast and collects responses
func (s *DLNAScanner) Discover(ctx context.Context, timeout time.Duration) ([]DLNADevice, error) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve multicast addr: %w", err)
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	for _, msg := range ssdpQueries {
		_, _ = conn.WriteTo([]byte(msg), addr)
	}

	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)

	buf := make([]byte, 4096)
	locations := make(map[string]bool)

	for {
		if ctx.Err() != nil {
			break
		}
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // timeout or closed
		}

		resp := string(buf[:n])
		lines := strings.Split(resp, "\r\n")
		for _, line := range lines {
			if strings.HasPrefix(strings.ToUpper(line), "LOCATION:") {
				loc := strings.TrimSpace(line[9:])
				if loc != "" && !locations[loc] {
					locations[loc] = true
					go s.fetchDeviceDesc(loc)
				}
			}
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]DLNADevice, 0, len(s.devices))
	for _, dev := range s.devices {
		res = append(res, *dev)
	}
	return res, nil
}

// FetchDeviceFromURL allows manual addition of a DLNA DMR device
func (s *DLNAScanner) FetchDeviceFromURL(ctx context.Context, loc string, customName string) (*DLNADevice, error) {
	if !strings.HasPrefix(loc, "http://") && !strings.HasPrefix(loc, "https://") {
		loc = "http://" + loc
	}

	// Try fetching desc XML
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接到该设备 URL: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var root rootXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("解析 UPnP 设备描述 XML 失败: %w", err)
	}

	var avTransportControlURL string
	for _, svc := range root.Device.ServiceList.Services {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			avTransportControlURL = svc.ControlURL
			break
		}
	}

	if avTransportControlURL == "" {
		return nil, fmt.Errorf("该设备未声明 AVTransport 串流播放服务")
	}

	baseURL, err := url.Parse(loc)
	if err == nil {
		ctrlURL, err := url.Parse(avTransportControlURL)
		if err == nil {
			avTransportControlURL = baseURL.ResolveReference(ctrlURL).String()
		}
	}

	name := root.Device.FriendlyName
	if customName != "" {
		name = customName
	}
	udn := root.Device.UDN
	if udn == "" {
		udn = loc
	}

	dev := &DLNADevice{
		UDN:        udn,
		Name:       name,
		Location:   loc,
		ControlURL: avTransportControlURL,
		Enabled:    true,
		LastSeen:   time.Now(),
	}

	s.mu.Lock()
	s.devices[udn] = dev
	s.mu.Unlock()

	return dev, nil
}

// XML structures for UPnP device description
type rootXML struct {
	Device deviceXML `xml:"device"`
}

type deviceXML struct {
	DeviceType   string         `xml:"deviceType"`
	FriendlyName string         `xml:"friendlyName"`
	UDN          string         `xml:"UDN"`
	ServiceList  serviceListXML `xml:"serviceList"`
}

type serviceListXML struct {
	Services []serviceXML `xml:"service"`
}

type serviceXML struct {
	ServiceType string `xml:"serviceType"`
	ServiceId   string `xml:"serviceId"`
	ControlURL  string `xml:"controlURL"`
}

func (s *DLNAScanner) fetchDeviceDesc(location string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, location, nil)
	if err != nil {
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var root rootXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return
	}

	var avTransportControlURL string
	for _, svc := range root.Device.ServiceList.Services {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			avTransportControlURL = svc.ControlURL
			break
		}
	}

	if avTransportControlURL == "" {
		return
	}

	baseURL, err := url.Parse(location)
	if err == nil {
		ctrlURL, err := url.Parse(avTransportControlURL)
		if err == nil {
			avTransportControlURL = baseURL.ResolveReference(ctrlURL).String()
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	udn := root.Device.UDN
	if udn == "" {
		udn = location
	}

	enabled := true
	if old, exists := s.devices[udn]; exists {
		enabled = old.Enabled
	}

	s.devices[udn] = &DLNADevice{
		UDN:        udn,
		Name:       root.Device.FriendlyName,
		Location:   location,
		ControlURL: avTransportControlURL,
		Enabled:    enabled,
		LastSeen:   time.Now(),
	}
	log.Info("Discovered DLNA DMR: " + root.Device.FriendlyName + " (" + avTransportControlURL + ")")
}

func sendSOAP(ctx context.Context, client *http.Client, controlURL, action, body string) error {
	soapBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/" xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <u:%s xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      %s
    </u:%s>
  </s:Body>
</s:Envelope>`, action, body, action)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, bytes.NewBufferString(soapBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"urn:schemas-upnp-org:service:AVTransport:1#`+action+`"`)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dlna soap %s: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dlna soap %s failed (%d): %s", action, resp.StatusCode, string(respBody))
	}
	return nil
}

func SetAVTransportURI(ctx context.Context, client *http.Client, controlURL, streamURI, title, artist string) error {
	didlLite := fmt.Sprintf(`&lt;DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item id="0" parentID="-1" restricted="1"&gt;&lt;dc:title&gt;%s&lt;/dc:title&gt;&lt;dc:creator&gt;%s&lt;/dc:creator&gt;&lt;upnp:class&gt;object.item.audioItem.musicTrack&lt;/upnp:class&gt;&lt;res&gt;%s&lt;/res&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;`,
		escapeXML(title), escapeXML(artist), escapeXML(streamURI))

	body := fmt.Sprintf(`<InstanceID>0</InstanceID><CurrentURI>%s</CurrentURI><CurrentURIMetaData>%s</CurrentURIMetaData>`,
		escapeXML(streamURI), didlLite)

	return sendSOAP(ctx, client, controlURL, "SetAVTransportURI", body)
}

func Play(ctx context.Context, client *http.Client, controlURL string) error {
	body := `<InstanceID>0</InstanceID><Speed>1</Speed>`
	return sendSOAP(ctx, client, controlURL, "Play", body)
}

func Pause(ctx context.Context, client *http.Client, controlURL string) error {
	body := `<InstanceID>0</InstanceID>`
	return sendSOAP(ctx, client, controlURL, "Pause", body)
}

func Stop(ctx context.Context, client *http.Client, controlURL string) error {
	body := `<InstanceID>0</InstanceID>`
	return sendSOAP(ctx, client, controlURL, "Stop", body)
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
