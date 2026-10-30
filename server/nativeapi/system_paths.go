package nativeapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
)

type SystemPathItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// addSystemPathsRoute registers the GET /api/system/paths endpoint
func (api *Router) addSystemPathsRoute(r chi.Router) {
	r.Get("/system/paths", func(w http.ResponseWriter, r *http.Request) {
		paths := discoverSystemPaths()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(paths); err != nil {
			log.Error(r.Context(), "Failed to encode system paths", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// discoverSystemPaths scans fnOS volume mount points (/vol*) and local music pool
func discoverSystemPaths() []SystemPathItem {
	var result []SystemPathItem
	seen := make(map[string]bool)

	addPath := func(name, path, itemType string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		// Check read access
		if f, err := os.Open(clean); err == nil {
			_ = f.Close()
			seen[clean] = true
			result = append(result, SystemPathItem{
				Name: name,
				Path: clean,
				Type: itemType,
			})
		}
	}

	// 1. Check fnOS music pool
	pkgVar := os.Getenv("TRIM_PKGVAR")
	if pkgVar == "" {
		pkgVar = "/vol1/@appdata/navidrome"
	}
	poolDir := filepath.Join(pkgVar, "music_pool")
	addPath("飞牛聚合曲库池 (自动挂载所有卷)", "./var/music_pool", "pool")
	addPath("飞牛聚合曲库池 (绝对路径)", poolDir, "pool")

	// 2. Discover /vol* partitions
	volMatches, _ := filepath.Glob("/vol*")
	subDirs := []string{"Music", "music", "Media/Music", "media/music", "我的音乐", "音乐"}

	for _, vol := range volMatches {
		fi, err := os.Stat(vol)
		if err != nil || !fi.IsDir() {
			continue
		}
		volBase := filepath.Base(vol)
		for _, sub := range subDirs {
			target := filepath.Join(vol, sub)
			if info, err := os.Stat(target); err == nil && info.IsDir() {
				displayName := strings.Replace(volBase, "vol", "存储空间 ", 1) + " / " + sub
				addPath(displayName, target, "volume")
			}
		}
	}

	// If no volume found, add default fallback
	if len(result) == 0 {
		addPath("默认曲库", "./var/music_pool", "pool")
	}

	return result
}
