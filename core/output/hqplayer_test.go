package output

import (
	"bufio"
	"context"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
)

func startMockHQServer(t *testing.T, handler func(conn net.Conn)) (string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handler(conn)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, func() { _ = ln.Close() }
}

func TestHQPlayerClient_TestConnection(t *testing.T) {
	client := NewHQPlayerClient()

	// 1. Valid HQPlayer XML <State/> response
	host, port, cleanup := startMockHQServer(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimSpace(line) == "<State/>" {
			_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><State active_mode="1" active_rate="5644800" state="0" volume="-3.0"/>` + "\n"))
		}
	})
	defer cleanup()

	ok, msg := client.TestConnection(context.Background(), host, port)
	if !ok {
		t.Fatalf("expected TestConnection to succeed, got ok=false, msg=%s", msg)
	}

	// 2. Non-HQPlayer open port (e.g. HTTP or silent service) should fail (no false positive)
	badHost, badPort, cleanupBad := startMockHQServer(t, func(conn net.Conn) {
		defer conn.Close()
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
	})
	defer cleanupBad()

	okBad, _ := client.TestConnection(context.Background(), badHost, badPort)
	if okBad {
		t.Fatalf("expected TestConnection to reject non-HQPlayer server, got ok=true")
	}
}

func TestHQPlayerClient_PlayRawStreamAndControl(t *testing.T) {
	var received []string

	host, port, cleanup := startMockHQServer(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimSpace(line)
			received = append(received, cmd)

			switch {
			case strings.HasPrefix(cmd, "<PlaylistClear"):
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><PlaylistClear result="OK"/>` + "\n"))
			case strings.HasPrefix(cmd, "<PlaylistAdd"):
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><PlaylistAdd result="OK"/>` + "\n"))
			case strings.HasPrefix(cmd, "<Play"):
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><Play result="OK"/>` + "\n"))
			case strings.HasPrefix(cmd, "<Seek"):
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><Seek result="OK"/>` + "\n"))
			case strings.HasPrefix(cmd, "<Stop"):
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><Stop result="OK"/>` + "\n"))
			default:
				_, _ = conn.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><Unknown result="Error">Unknown command</Unknown>` + "\n"))
			}
		}
	})
	defer cleanup()

	client := NewHQPlayerClient()
	cfg := HQPlayerConfig{
		Name:    "HQPE",
		Host:    host,
		Port:    port,
		Enabled: true,
	}

	streamURL := "http://10.10.10.101:4533/rest/stream?u=admin&t=tok&s=salt&id=123&format=raw"
	err := client.PlayRawStream(context.Background(), cfg, streamURL, TrackMetadata{Title: "Test"}, 42.8)
	if err != nil {
		t.Fatalf("PlayRawStream failed: %v", err)
	}

	if len(received) != 5 {
		t.Fatalf("expected 5 XML commands (<Stop>, <PlaylistClear>, <PlaylistAdd>, <Play>, <Seek>), got %d: %v", len(received), received)
	}
	if received[0] != "<Stop/>" {
		t.Errorf("unexpected cmd[0]: %s", received[0])
	}
	if received[1] != "<PlaylistClear/>" {
		t.Errorf("unexpected cmd[1]: %s", received[1])
	}
	expectedAdd := `<PlaylistAdd uri="http://10.10.10.101:4533/rest/stream?u=admin&amp;t=tok&amp;s=salt&amp;id=123&amp;format=raw"/>`
	if received[2] != expectedAdd {
		t.Errorf("expected %s, got %s", expectedAdd, received[2])
	}
	if received[3] != "<Play/>" {
		t.Errorf("unexpected cmd[3]: %s", received[3])
	}
	if received[4] != `<Seek position="42"/>` {
		t.Errorf("unexpected cmd[4]: %s", received[4])
	}

	// Verify micro offset (< 1.0s) strictly suppresses <Seek> to avoid buffer underruns
	received = nil
	err = client.PlayRawStream(context.Background(), cfg, streamURL, TrackMetadata{Title: "Test"}, 0.4)
	if err != nil {
		t.Fatalf("PlayRawStream with micro offset failed: %v", err)
	}
	if len(received) != 4 {
		t.Fatalf("expected 4 XML commands without Seek, got %d: %v", len(received), received)
	}
	for _, cmd := range received {
		if strings.HasPrefix(cmd, "<Seek") {
			t.Errorf("micro offset (<1.0s) should not trigger <Seek>, found %s", cmd)
		}
	}

	// Verify Pause maps to <Stop/>
	received = nil
	if err := client.Control(context.Background(), cfg, "pause", 0, 0); err != nil {
		t.Fatalf("Control(pause) failed: %v", err)
	}
	if len(received) != 1 || received[0] != "<Stop/>" {
		t.Fatalf("expected pause to send <Stop/>, got %v", received)
	}
}

func TestDetectOutboundIP(t *testing.T) {
	ip := DetectOutboundIP("10.10.10.120")
	if ip == "" || ip == "localhost" {
		t.Fatalf("expected valid outbound IP, got %q", ip)
	}
}

func TestResolveOutboundURL(t *testing.T) {
	// 1. Relative URL with localhost Host header should be healed to non-loopback LAN IP
	req := httptest.NewRequest("POST", "http://localhost:4533/api/output/play", nil)
	req.Host = "localhost:4533"

	resolved := ResolveOutboundURL(req, "/rest/stream?u=admin&t=tok&s=salt&id=song1", 4533, "10.10.10.120")
	if strings.Contains(resolved, "localhost") || strings.Contains(resolved, "127.0.0.1") {
		t.Fatalf("expected localhost/127.0.0.1 to be replaced with LAN IP, got: %s", resolved)
	}
	if !strings.Contains(resolved, "format=raw") {
		t.Fatalf("expected format=raw to be appended for /rest/stream, got: %s", resolved)
	}

	// 2. Full URL containing 127.0.0.1 should also be healed to LAN IP
	req2 := httptest.NewRequest("POST", "http://127.0.0.1:4533/api/output/play", nil)
	req2.Host = "127.0.0.1:4533"
	resolved2 := ResolveOutboundURL(req2, "http://127.0.0.1:4533/rest/stream?id=song2", 4533, "10.10.10.120")
	if strings.Contains(resolved2, "127.0.0.1") {
		t.Fatalf("expected 127.0.0.1 in rawURL to be replaced with LAN IP, got: %s", resolved2)
	}
	if !strings.Contains(resolved2, "format=raw") {
		t.Fatalf("expected format=raw to be present, got: %s", resolved2)
	}

	// 3. Explicit LAN Host header should be preserved
	req3 := httptest.NewRequest("POST", "http://10.10.10.101:4533/api/output/play", nil)
	req3.Host = "10.10.10.101:4533"
	resolved3 := ResolveOutboundURL(req3, "/rest/stream?id=song3&format=raw", 4533, "10.10.10.120")
	if !strings.HasPrefix(resolved3, "http://10.10.10.101:4533/rest/stream?") {
		t.Fatalf("expected 10.10.10.101:4533 prefix, got: %s", resolved3)
	}
}
