package musicengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"golang.org/x/sync/singleflight"
)

// EngineState holds immutable runtime state for active script
type EngineState struct {
	version    string
	scriptID   string
	meta       SourceScriptMeta
	program    *goja.Program
	pool       *WorkerPool
	sources    []string
	qualities  map[string][]string
}

// MusicEngine manages user script engines, caching, and stream resolution
type MusicEngine struct {
	state       atomic.Pointer[EngineState]
	reloadMu    sync.Mutex
	ds          model.DataStore
	cacheMu     sync.RWMutex
	streamCache map[string]*cachedStreamItem
	flight      singleflight.Group
}

type cachedStreamItem struct {
	stream    *ResolvedStream
	expiresAt time.Time
}

var (
	globalEngineInstance *MusicEngine
	globalEngineOnce     sync.Once
)

// GetEngine returns the singleton MusicEngine instance
func GetEngine(ds model.DataStore) *MusicEngine {
	globalEngineOnce.Do(func() {
		globalEngineInstance = &MusicEngine{
			ds:          ds,
			streamCache: make(map[string]*cachedStreamItem),
		}
		// Try loading active script from disk
		globalEngineInstance.loadActiveScriptFromDisk()
	})
	return globalEngineInstance
}

// ResolveStreamURL resolves the live CDN stream URL using singleflight and 20-min LRU cache
func (e *MusicEngine) ResolveStreamURL(ctx context.Context, source, songID, quality string) (*ResolvedStream, error) {
	state := e.state.Load()
	if state == nil || state.pool == nil {
		return nil, errors.New("music engine has no active source script configured")
	}

	cacheKey := fmt.Sprintf("%s:%s:%s:%s", state.version, source, songID, quality)

	// 1. Fast cache check
	e.cacheMu.RLock()
	if item, ok := e.streamCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		e.cacheMu.RUnlock()
		return item.stream, nil
	}
	e.cacheMu.RUnlock()

	// 2. Singleflight coalescing: only 1 goroutine executes script resolution
	val, err, _ := e.flight.Do(cacheKey, func() (any, error) {
		// DCL inside singleflight
		e.cacheMu.RLock()
		if item, ok := e.streamCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
			e.cacheMu.RUnlock()
			return item.stream, nil
		}
		e.cacheMu.RUnlock()

		// Attempt resolution with automatic quality fallback
		stream, resolveErr := e.doResolveWithFallback(ctx, state, source, songID, quality)
		if resolveErr != nil {
			return nil, resolveErr
		}

		// Cache for 20 minutes
		e.cacheMu.Lock()
		e.streamCache[cacheKey] = &cachedStreamItem{
			stream:    stream,
			expiresAt: time.Now().Add(20 * time.Minute),
		}
		e.cacheMu.Unlock()

		return stream, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*ResolvedStream), nil
}

func (e *MusicEngine) doResolveWithFallback(ctx context.Context, state *EngineState, source, songID, targetQuality string) (*ResolvedStream, error) {
	qualities := []string{targetQuality}
	if targetQuality == QualityFlac24b || targetQuality == "hires" {
		qualities = append(qualities, QualityFlac, Quality320k, Quality128k)
	} else if targetQuality == QualityFlac {
		qualities = append(qualities, Quality320k, Quality128k)
	} else if targetQuality == Quality320k {
		qualities = append(qualities, Quality128k)
	}

	var lastErr error
	for _, q := range qualities {
		stream, err := e.callScriptGetMusicUrl(ctx, state, source, songID, q)
		if err == nil && stream != nil && stream.URL != "" {
			return stream, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("resolving stream URL for %s:%s: %w", source, songID, lastErr)
	}
	return nil, fmt.Errorf("no playable stream link found for %s:%s", source, songID)
}

func (e *MusicEngine) callScriptGetMusicUrl(ctx context.Context, state *EngineState, source, songID, quality string) (*ResolvedStream, error) {
	val, err := state.pool.ExecuteWithTimeout(ctx, 10*time.Second, func(vm *goja.Runtime) (any, error) {
		// Look up apis[source].getMusicUrl or lx.getMusicUrl
		script := fmt.Sprintf(`
			(function() {
				if (typeof global.lxMusicSdk !== 'undefined' && global.lxMusicSdk['%s']) {
					return global.lxMusicSdk['%s'].getMusicUrl({ id: '%s' }, '%s');
				}
				if (typeof global.userApi !== 'undefined' && global.userApi.getMusicUrl) {
					return global.userApi.getMusicUrl('%s', { id: '%s' }, '%s');
				}
				return null;
			})()
		`, source, source, songID, quality, source, songID, quality)

		res, err := vm.RunString(script)
		if err != nil {
			return nil, err
		}
		if res == nil || goja.IsUndefined(res) || goja.IsNull(res) {
			return nil, errors.New("script returned null")
		}

		// Handle Promise if returned
		if p, ok := res.Export().(*goja.Promise); ok {
			switch p.State() {
			case goja.PromiseStateFulfilled:
				res = p.Result()
			case goja.PromiseStateRejected:
				return nil, fmt.Errorf("promise rejected: %v", p.Result())
			}
		}

		return extractResolvedStream(res), nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*ResolvedStream), nil
}

func extractResolvedStream(val goja.Value) *ResolvedStream {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	res := &ResolvedStream{
		Headers: make(map[string]string),
	}

	if str, ok := val.Export().(string); ok {
		res.URL = str
		return res
	}

	if obj, ok := val.Export().(map[string]any); ok {
		if u, ok := obj["url"].(string); ok {
			res.URL = u
		}
		if h, ok := obj["headers"].(map[string]any); ok {
			for k, v := range h {
				res.Headers[k] = fmt.Sprint(v)
			}
		}
	}

	return res
}

// Search queries online songs across sources
func (e *MusicEngine) Search(ctx context.Context, source, query string, page, limit int) (*SearchResult, error) {
	state := e.state.Load()
	if state == nil || state.pool == nil {
		return nil, errors.New("music engine has no active source script configured")
	}

	if limit <= 0 {
		limit = 30
	}
	if page <= 0 {
		page = 1
	}

	val, err := state.pool.ExecuteWithTimeout(ctx, 12*time.Second, func(vm *goja.Runtime) (any, error) {
		script := fmt.Sprintf(`
			(function() {
				if (typeof global.lxMusicSdk !== 'undefined' && global.lxMusicSdk['%s'] && global.lxMusicSdk['%s'].musicSearch) {
					return global.lxMusicSdk['%s'].musicSearch.search('%s', %d, %d);
				}
				if (typeof global.userApi !== 'undefined' && global.userApi.search) {
					return global.userApi.search('%s', '%s', %d, %d);
				}
				return null;
			})()
		`, source, source, source, query, page, limit, source, query, page, limit)

		res, err := vm.RunString(script)
		if err != nil {
			return nil, err
		}

		return parseSearchResult(source, page, limit, res), nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*SearchResult), nil
}

func parseSearchResult(source string, page, limit int, val goja.Value) *SearchResult {
	result := &SearchResult{
		Source: source,
		Page:   page,
		Limit:  limit,
		List:   make([]*OnlineTrack, 0),
	}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return result
	}

	exported := val.Export()
	var listItems []any
	if m, ok := exported.(map[string]any); ok {
		if total, ok := m["total"].(int64); ok {
			result.Total = int(total)
		}
		if l, ok := m["list"].([]any); ok {
			listItems = l
		}
	} else if arr, ok := exported.([]any); ok {
		listItems = arr
		result.Total = len(arr)
	}

	for _, item := range listItems {
		if itemMap, ok := item.(map[string]any); ok {
			track := mapToOnlineTrack(source, itemMap)
			if track != nil {
				result.List = append(result.List, track)
			}
		}
	}

	return result
}

func mapToOnlineTrack(source string, m map[string]any) *OnlineTrack {
	id := fmt.Sprint(m["id"])
	if id == "" || id == "<nil>" {
		id = fmt.Sprint(m["songmid"])
	}
	if id == "" || id == "<nil>" {
		return nil
	}

	title := fmt.Sprint(m["name"])
	if title == "" || title == "<nil>" {
		title = fmt.Sprint(m["title"])
	}

	artist := fmt.Sprint(m["singer"])
	if artist == "" || artist == "<nil>" {
		artist = fmt.Sprint(m["artist"])
	}

	album := fmt.Sprint(m["albumName"])
	if album == "" || album == "<nil>" {
		album = fmt.Sprint(m["album"])
	}

	cover := fmt.Sprint(m["img"])
	if cover == "" || cover == "<nil>" {
		cover = fmt.Sprint(m["pic"])
	}

	var duration float64
	if dur, ok := m["interval"].(float64); ok {
		duration = dur
	} else if dur, ok := m["duration"].(float64); ok {
		duration = dur
	}

	universalID := BuildUniversalID(source, id)
	protocolPath := BuildProtocolPath(source, id, QualityFlac)

	return &OnlineTrack{
		UniversalID:  universalID,
		ProtocolPath: protocolPath,
		Source:       source,
		SongID:       id,
		Title:        title,
		Artist:       artist,
		Album:        album,
		Duration:     duration,
		CoverURL:     cover,
		Quality:      QualityFlac,
	}
}

// ActivateScript compiles, verifies, and atomically switches the active user script
func (e *MusicEngine) ActivateScript(scriptContent string, scriptID, name, description string) error {
	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	// 1. Compile script
	program, err := goja.Compile("user_script.js", scriptContent, false)
	if err != nil {
		return fmt.Errorf("script compilation failed: %w", err)
	}

	// 2. Pre-verify in isolated worker
	testWorker, err := NewWorker(program)
	if err != nil {
		return fmt.Errorf("testing script runtime failed: %w", err)
	}

	// Calculate version hash
	h := sha256.Sum256([]byte(scriptContent))
	versionHash := hex.EncodeToString(h[:8])

	meta := SourceScriptMeta{
		ID:          scriptID,
		Name:        name,
		Description: description,
		Version:     versionHash,
		Sources:     []string{"wy", "tx", "kg", "kw", "mg"},
		IsActive:    true,
		UpdatedAt:   time.Now(),
	}

	// 3. Initialize new worker pool
	newPool := NewWorkerPool(program)

	// 4. Atomic pointer swap
	newState := &EngineState{
		version:   versionHash,
		scriptID:  scriptID,
		meta:      meta,
		program:   program,
		pool:      newPool,
		sources:   meta.Sources,
	}

	e.state.Store(newState)

	// 5. Persist script to cache dir
	scriptsDir := filepath.Join(GetEngineCacheDir(), "scripts")
	_ = os.WriteFile(filepath.Join(scriptsDir, "active.js"), []byte(scriptContent), 0644)
	metaBytes, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(scriptsDir, "active_meta.json"), metaBytes, 0644)

	log.Info("MusicEngine: Script activated successfully", "id", scriptID, "version", versionHash)
	_ = testWorker // GC
	return nil
}

// GetActiveScriptMeta returns current active script info
func (e *MusicEngine) GetActiveScriptMeta() *SourceScriptMeta {
	state := e.state.Load()
	if state == nil {
		return nil
	}
	meta := state.meta
	return &meta
}

func (e *MusicEngine) loadActiveScriptFromDisk() {
	scriptsDir := filepath.Join(GetEngineCacheDir(), "scripts")
	activeFile := filepath.Join(scriptsDir, "active.js")
	content, err := os.ReadFile(activeFile)
	if err != nil {
		return
	}

	var meta SourceScriptMeta
	metaFile := filepath.Join(scriptsDir, "active_meta.json")
	if mBytes, err := os.ReadFile(metaFile); err == nil {
		_ = json.Unmarshal(mBytes, &meta)
	}

	_ = e.ActivateScript(string(content), meta.ID, meta.Name, meta.Description)
}
