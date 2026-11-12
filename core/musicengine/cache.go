package musicengine

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

const (
	DefaultMaxCacheBytes = 3 * 1024 * 1024 * 1024 // 3 GB
	TargetLowWatermark   = 2400 * 1024 * 1024      // 2.4 GB (80%)
)

type StreamCacheManager struct {
	mu           sync.Mutex
	cacheDir     string
	maxSizeBytes int64
}

var (
	cacheMgrInstance *StreamCacheManager
	cacheMgrOnce     sync.Once
)

func GetCacheManager() *StreamCacheManager {
	cacheMgrOnce.Do(func() {
		baseDir := conf.Server.CacheFolder.String()
		if baseDir == "" {
			pkgVar := os.Getenv("TRIM_PKGVAR")
			if pkgVar != "" {
				baseDir = filepath.Join(pkgVar, "data", "cache")
			} else {
				baseDir = "./var/data/cache"
			}
		}
		cDir := filepath.Join(baseDir, "online_music")
		_ = os.MkdirAll(cDir, 0755)
		cacheMgrInstance = &StreamCacheManager{
			cacheDir:     cDir,
			maxSizeBytes: DefaultMaxCacheBytes,
		}
	})
	return cacheMgrInstance
}

func (m *StreamCacheManager) keyForTrack(trackID string) string {
	h := md5.Sum([]byte(trackID))
	return hex.EncodeToString(h[:])
}

func (m *StreamCacheManager) finalPath(trackID string) string {
	return filepath.Join(m.cacheDir, m.keyForTrack(trackID)+".stream")
}

func (m *StreamCacheManager) tmpPath(trackID string) string {
	return filepath.Join(m.cacheDir, m.keyForTrack(trackID)+".tmp")
}

// ServeCachedTrack attempts to serve the track from disk with full HTTP Range support.
// Returns true if successfully served from local cache.
func (m *StreamCacheManager) ServeCachedTrack(w http.ResponseWriter, r *http.Request, trackID string) bool {
	p := m.finalPath(trackID)
	fi, err := os.Stat(p)
	if err != nil || fi.Size() == 0 {
		return false
	}

	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()

	// Update Access/Mod time for LRU
	now := time.Now()
	_ = os.Chtimes(p, now, now)

	// Sniff content type from first 512 bytes
	headerBuf := make([]byte, 512)
	n, _ := f.Read(headerBuf)
	_, _ = f.Seek(0, io.SeekStart)

	contentType := "audio/flac"
	if n >= 4 && string(headerBuf[:4]) == "fLaC" {
		contentType = "audio/flac"
	} else if n >= 3 && string(headerBuf[:3]) == "ID3" {
		contentType = "audio/mpeg"
	} else if strings.Contains(strings.ToLower(trackID), "128k") || strings.Contains(strings.ToLower(trackID), "320k") {
		contentType = "audio/mpeg"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Navidrome-Stream-Cache", "HIT")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	log.Debug(r.Context(), fmt.Sprintf("StreamCache: Cache HIT for track %s (%.2f MB), serving directly from local disk",
		trackID, float64(fi.Size())/(1024*1024)))

	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	return true
}

// CreateCacheWriter creates an asynchronous writer that tees downstream data to a .tmp file.
// When finished, call commit(true) to atomically promote to .stream, or commit(false) to discard.
func (m *StreamCacheManager) CreateCacheWriter(trackID string) (io.WriteCloser, func(success bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	finalP := m.finalPath(trackID)
	if fi, err := os.Stat(finalP); err == nil && fi.Size() > 0 {
		return nil, func(bool) {} // already cached
	}

	tmpP := m.tmpPath(trackID)
	f, err := os.OpenFile(tmpP, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, func(bool) {}
	}

	commit := func(success bool) {
		_ = f.Close()
		if success {
			if fi, err := os.Stat(tmpP); err == nil && fi.Size() > 64*1024 {
				if err := os.Rename(tmpP, finalP); err == nil {
					log.Info(fmt.Sprintf("StreamCache: Successfully cached online track %s (%.2f MB)",
						trackID, float64(fi.Size())/(1024*1024)))
					go m.evictOldFilesIfNeeded()
					return
				}
			}
		}
		_ = os.Remove(tmpP)
	}

	return f, commit
}

func (m *StreamCacheManager) evictOldFilesIfNeeded() {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return
	}

	type fileInfo struct {
		path    string
		size    int64
		modTime time.Time
	}

	var totalSize int64
	var files []fileInfo

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".stream") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
		files = append(files, fileInfo{
			path:    filepath.Join(m.cacheDir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	if totalSize <= m.maxSizeBytes {
		return
	}

	// Sort files by ModTime ascending (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for _, f := range files {
		if totalSize <= TargetLowWatermark {
			break
		}
		if err := os.Remove(f.path); err == nil {
			totalSize -= f.size
			log.Debug(fmt.Sprintf("StreamCache: Evicted old cache file %s (%.2f MB)", filepath.Base(f.path), float64(f.size)/(1024*1024)))
		}
	}
}
