package musicengine

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestParseUniversalID(t *testing.T) {
	source, id, ok := ParseUniversalID("online:wy:1859245776")
	if !ok || source != "wy" || id != "1859245776" {
		t.Fatalf("ParseUniversalID failed: got %s, %s, %v", source, id, ok)
	}

	_, _, ok = ParseUniversalID("local:123")
	if ok {
		t.Fatalf("ParseUniversalID should fail for local id")
	}
}

func TestParseProtocolPath(t *testing.T) {
	src, id, q, ok := ParseProtocolPath("online://wy/1859245776?quality=flac")
	if !ok || src != "wy" || id != "1859245776" || q != "flac" {
		t.Fatalf("ParseProtocolPath failed: %s, %s, %s, %v", src, id, q, ok)
	}
}

func TestPolyfillsCrypto(t *testing.T) {
	vm := goja.New()
	if err := InjectPolyfills(vm); err != nil {
		t.Fatalf("InjectPolyfills failed: %v", err)
	}

	val, err := vm.RunString(`
		crypto.md5("hello")
	`)
	if err != nil || val.String() != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("md5 polyfill failed: %v, %v", val, err)
	}
}

func TestSlabRingBuffer(t *testing.T) {
	buf := NewSlabRingBuffer(0)
	defer buf.Close()

	data := []byte("fLaC\x00\x00\x00\x22hello_world_flac_test_stream")
	n, err := buf.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("Buffer write failed: %v, %d", err, n)
	}
	buf.MarkDone(nil)

	sniff := buf.SniffedResult()
	if sniff.Format != "flac" {
		t.Fatalf("Expected format flac, got %s", sniff.Format)
	}

	out := make([]byte, len(data))
	rn, err := buf.ReadAt(out, 0)
	if rn != len(data) || string(out) != string(data) {
		t.Fatalf("Buffer read failed: %v, read %d bytes", err, rn)
	}
}

func TestSlabRingBufferTimeout(t *testing.T) {
	buf := NewSlabRingBuffer(0)
	defer buf.Close()

	start := time.Now()
	out := make([]byte, 100)
	// Reading when empty should time out safely after 2.5s and return EOF
	_, err := buf.ReadAt(out, 0)
	elapsed := time.Since(start)

	if elapsed < 2*time.Second || elapsed > 4*time.Second {
		t.Logf("Buffer read timeout elapsed: %v", elapsed)
	}
	if err == nil {
		t.Fatalf("Expected timeout EOF, got nil error")
	}
}

func TestBuildDefensiveShadowTrack(t *testing.T) {
	onlineTrack := &OnlineTrack{
		UniversalID:  "online:wy:12345",
		ProtocolPath: "online://wy/12345?quality=flac",
		Source:       "wy",
		SongID:       "12345",
		Title:        "Test Title",
		Artist:       "Test Artist",
		Album:        "Test Album",
		Duration:     180.0,
		Quality:      QualityFlac,
	}

	mf := BuildDefensiveShadowTrack(onlineTrack, 1)
	if mf.ID != "online:wy:12345" {
		t.Errorf("ID mismatch: %s", mf.ID)
	}
	if !strings.HasPrefix(mf.Path, "online://") {
		t.Errorf("Path invalid: %s", mf.Path)
	}
	if mf.BitRate <= 0 || mf.Size <= 0 {
		t.Errorf("Defensive bitRate or size invalid: %d, %d", mf.BitRate, mf.Size)
	}
	if mf.Missing {
		t.Errorf("Shadow track should not be marked missing")
	}
}
