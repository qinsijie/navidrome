package musicengine

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

var (
	cacheDirOnce sync.Once
	cacheDirPath string
)

// GetEngineCacheDir dynamically resolves and creates the secure music engine cache directory.
// It strictly respects fnOS persistent volume boundaries, anchoring relative paths to --configfile,
// avoiding writing to the small root filesystem.
func GetEngineCacheDir() string {
	cacheDirOnce.Do(func() {
		rawCache := conf.Server.CacheFolder.String()

		var baseDir string
		if filepath.IsAbs(rawCache) {
			baseDir = rawCache
		} else {
			// Anchor relative paths to configfile's parent directory
			configFile := conf.Server.ConfigFile
			if configFile != "" && filepath.IsAbs(configFile) {
				// E.g. configFile: /vol1/@appcenter/navidrome/config/navidrome.toml
				// appRoot: /vol1/@appcenter/navidrome
				appRoot := filepath.Dir(filepath.Dir(configFile))
				baseDir = filepath.Join(appRoot, rawCache)
			} else if _, err := os.Stat("/vol1/@appcenter/navidrome"); err == nil {
				baseDir = filepath.Join("/vol1/@appcenter/navidrome/var/data/cache")
			} else {
				baseDir = filepath.Clean(rawCache)
			}
		}

		finalPath := filepath.Join(baseDir, "musicengine")

		// Root partition safety check
		if isRootFilesystem(finalPath) {
			// If on root filesystem and /vol1 exists, redirect to /vol1
			if _, err := os.Stat("/vol1"); err == nil {
				log.Warn("MusicEngine: Cache path is on root filesystem. Redirecting to /vol1 storage pool")
				finalPath = "/vol1/@appcenter/navidrome/var/data/cache/musicengine"
			}
		}

		if err := os.MkdirAll(finalPath, 0755); err != nil {
			log.Error("MusicEngine: Failed to create cache directory", "path", finalPath, err)
		}

		scriptsDir := filepath.Join(finalPath, "scripts")
		_ = os.MkdirAll(scriptsDir, 0755)

		cacheDirPath = finalPath
		log.Info("MusicEngine: Cache directory initialized", "path", cacheDirPath)
	})

	return cacheDirPath
}

func isRootFilesystem(path string) bool {
	var statPath, statRoot syscall.Statfs_t
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}
	if err := syscall.Statfs(dir, &statPath); err != nil {
		return false
	}
	if err := syscall.Statfs("/", &statRoot); err != nil {
		return false
	}
	return statPath.Fsid == statRoot.Fsid
}
