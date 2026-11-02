package musicengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	cacheKey := fmt.Sprintf("%s:%s:%s", source, songID, quality)
	if state != nil {
		cacheKey = fmt.Sprintf("%s:%s:%s:%s", state.version, source, songID, quality)
	}

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

		// Attempt resolution with automatic quality fallback and native fallback
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
	if state != nil && state.pool != nil {
		for _, q := range qualities {
			stream, err := e.callScriptGetMusicUrl(ctx, state, source, songID, q)
			if err == nil && stream != nil && stream.URL != "" {
				return stream, nil
			}
			if err != nil {
				lastErr = err
			}
		}
	}

	// Direct native stream fallback for kuwo & netease
	for _, q := range qualities {
		stream, err := resolveNativeStream(ctx, source, songID, q)
		if err == nil && stream != nil && stream.URL != "" {
			return stream, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("resolving stream URL for %s:%s: %w", source, songID, lastErr)
	}
	return nil, fmt.Errorf("no playable stream link found for %s:%s", source, songID)
}

func resolveNativeStream(ctx context.Context, source, songID, quality string) (*ResolvedStream, error) {
	switch strings.ToLower(source) {
	case "kw":
		rid := strings.TrimPrefix(songID, "MUSIC_")
		client := &http.Client{Timeout: 6 * time.Second}
		reqURL := fmt.Sprintf("http://antiserver.kuwo.cn/anti.s?type=convert_url&rid=%s&format=mp3&response=url", rid)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		targetURL := strings.TrimSpace(string(body))
		if strings.HasPrefix(targetURL, "http://") || strings.HasPrefix(targetURL, "https://") {
			return &ResolvedStream{
				URL: targetURL,
				Headers: map[string]string{
					"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				},
			}, nil
		}
		return nil, fmt.Errorf("invalid kuwo stream response: %s", targetURL)

	case "wy":
		client := &http.Client{
			Timeout: 6 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		outerURL := fmt.Sprintf("http://music.163.com/song/media/outer/url?id=%s.mp3", songID)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, outerURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		loc := resp.Header.Get("Location")
		if loc != "" && !strings.Contains(loc, "404") {
			return &ResolvedStream{
				URL: loc,
				Headers: map[string]string{
					"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				},
			}, nil
		}
		return nil, fmt.Errorf("netease song %s is restricted or unavailable", songID)
	}

	return nil, fmt.Errorf("no native resolver for source %s", source)
}

func (e *MusicEngine) callScriptGetMusicUrl(ctx context.Context, state *EngineState, source, songID, quality string) (*ResolvedStream, error) {
	val, err := state.pool.ExecuteWithTimeout(ctx, 10*time.Second, func(vm *goja.Runtime) (any, error) {
		// Look up apis[source].getMusicUrl or lx.getMusicUrl or _lx_events.request
		script := fmt.Sprintf(`
			(function() {
				var finalResult = null;
				function unwrap(p) {
					if (!p) return null;
					if (typeof p.then === 'function') {
						p.then(function(v) { finalResult = v; }).catch(function() { finalResult = null; });
						return finalResult || p;
					}
					return p;
				}

				if (typeof globalThis._lx_events !== 'undefined' && typeof globalThis._lx_events.request === 'function') {
					var p = globalThis._lx_events.request({
						action: 'musicUrl',
						source: '%s',
						info: {
							type: '%s',
							musicInfo: {
								id: '%s',
								songmid: '%s',
								hash: '%s'
							}
						}
					});
					return unwrap(p);
				}
				if (typeof global.lxMusicSdk !== 'undefined' && global.lxMusicSdk['%s']) {
					return unwrap(global.lxMusicSdk['%s'].getMusicUrl({ id: '%s' }, '%s'));
				}
				if (typeof global.userApi !== 'undefined' && global.userApi.getMusicUrl) {
					return unwrap(global.userApi.getMusicUrl('%s', { id: '%s' }, '%s'));
				}
				return null;
			})()
		`, source, quality, songID, songID, songID, source, source, songID, quality, source, songID, quality)

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
			case goja.PromiseStatePending:
				return nil, errors.New("script promise returned pending")
			}
		}

		resolved := extractResolvedStream(res)
		if resolved == nil || resolved.URL == "" {
			return nil, errors.New("empty stream URL returned from script")
		}
		return resolved, nil
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
		res.URL = strings.TrimSpace(str)
		return res
	}

	if obj, ok := val.Export().(map[string]any); ok {
		if u, ok := obj["url"].(string); ok {
			res.URL = strings.TrimSpace(u)
		}
		if h, ok := obj["headers"].(map[string]any); ok {
			for k, v := range h {
				res.Headers[k] = fmt.Sprint(v)
			}
		} else if h, ok := obj["header"].(map[string]any); ok {
			for k, v := range h {
				res.Headers[k] = fmt.Sprint(v)
			}
		}
	}

	if res.URL == "" {
		return nil
	}
	return res
}

// Search queries online songs across sources
func (e *MusicEngine) Search(ctx context.Context, source, query string, page, limit int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 30
	}
	if page <= 0 {
		page = 1
	}

	// 1. Try native high-performance search first
	res, err := SearchNative(ctx, source, query, page, limit)
	if err == nil && res != nil && len(res.List) > 0 {
		return res, nil
	}

	// 2. Fall back to script pool if available
	state := e.state.Load()
	if state != nil && state.pool != nil {
		val, poolErr := state.pool.ExecuteWithTimeout(ctx, 12*time.Second, func(vm *goja.Runtime) (any, error) {
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

			sRes, sErr := vm.RunString(script)
			if sErr != nil {
				return nil, sErr
			}
			return parseSearchResult(source, page, limit, sRes), nil
		})
		if poolErr == nil && val != nil {
			if sRes, ok := val.(*SearchResult); ok && len(sRes.List) > 0 {
				return sRes, nil
			}
		}
	}

	if res != nil {
		return res, nil
	}
	return &SearchResult{Source: source, Page: page, Limit: limit, List: make([]*OnlineTrack, 0)}, err
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

// ExtractScriptMetadata parses JSDoc headers from standard LX Music scripts
func ExtractScriptMetadata(content string) SourceScriptMeta {
	reTag := func(tag string) string {
		re := regexp.MustCompile(`(?i)@` + tag + `\s+([^\r\n*]+)`)
		m := re.FindStringSubmatch(content)
		if len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	name := reTag("name")
	if name == "" {
		name = "自定义音源"
	}
	description := reTag("description")
	version := reTag("version")
	author := reTag("author")
	homepage := reTag("homepage")

	h := sha256.Sum256([]byte(content))
	scriptID := hex.EncodeToString(h[:6])

	if version == "" {
		version = hex.EncodeToString(h[:4])
	}

	return SourceScriptMeta{
		ID:          scriptID,
		Name:        name,
		Description: description,
		Version:     version,
		Author:      author,
		Homepage:    homepage,
		Sources:     []string{"wy", "tx", "kg", "kw", "mg"},
		UpdatedAt:   time.Now(),
	}
}

// GetAllScripts returns metadata for all installed user scripts
func (e *MusicEngine) GetAllScripts() []SourceScriptMeta {
	installedDir := filepath.Join(GetEngineCacheDir(), "scripts", "installed")
	_ = os.MkdirAll(installedDir, 0755)

	activeID := ""
	if activeBytes, err := os.ReadFile(filepath.Join(GetEngineCacheDir(), "scripts", "active_id.txt")); err == nil {
		activeID = strings.TrimSpace(string(activeBytes))
	}

	entries, err := os.ReadDir(installedDir)
	if err != nil {
		return nil
	}

	var results []SourceScriptMeta
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			metaBytes, err := os.ReadFile(filepath.Join(installedDir, entry.Name()))
			if err != nil {
				continue
			}
			var meta SourceScriptMeta
			if err := json.Unmarshal(metaBytes, &meta); err == nil {
				meta.IsActive = (meta.ID == activeID)
				results = append(results, meta)
			}
		}
	}

	// If no scripts in installed/ but active_meta.json exists, load it
	if len(results) == 0 {
		activeMetaFile := filepath.Join(GetEngineCacheDir(), "scripts", "active_meta.json")
		if metaBytes, err := os.ReadFile(activeMetaFile); err == nil {
			var meta SourceScriptMeta
			if err := json.Unmarshal(metaBytes, &meta); err == nil {
				meta.IsActive = true
				results = append(results, meta)
			}
		}
	}

	return results
}

// ImportScript compiles, saves, and activates a script
func (e *MusicEngine) ImportScript(content string, subscriptionURL string) (*SourceScriptMeta, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("script content is empty")
	}

	// 1. Verify compilation
	program, err := goja.Compile("user_script.js", content, false)
	if err != nil {
		return nil, fmt.Errorf("script syntax compilation failed: %w", err)
	}

	// 2. Pre-verify in isolated worker
	testWorker, err := NewWorker(program)
	if err != nil {
		return nil, fmt.Errorf("testing script runtime failed: %w", err)
	}

	// 3. Extract metadata
	meta := ExtractScriptMetadata(content)
	if subscriptionURL != "" {
		meta.SubscriptionURL = subscriptionURL
	}

	// Extract sources & qualities from _lx_initedData if available
	if initedVal := testWorker.vm.Get("_lx_initedData"); initedVal != nil && !goja.IsUndefined(initedVal) && !goja.IsNull(initedVal) {
		if initedMap, ok := initedVal.Export().(map[string]any); ok {
			if srcMap, ok := initedMap["sources"].(map[string]any); ok && len(srcMap) > 0 {
				var sources []string
				qualitiesMap := make(map[string][]string)
				for k, v := range srcMap {
					sources = append(sources, k)
					if itemMap, ok := v.(map[string]any); ok {
						if qList, ok := itemMap["qualitys"].([]any); ok {
							for _, q := range qList {
								qualitiesMap[k] = append(qualitiesMap[k], fmt.Sprint(q))
							}
						}
					}
				}
				if len(sources) > 0 {
					meta.Sources = sources
					meta.Qualities = qualitiesMap
				}
			}
		}
	}
	_ = testWorker // GC
	meta.IsActive = true
	meta.UpdatedAt = time.Now()

	// 4. Save to installed dir
	installedDir := filepath.Join(GetEngineCacheDir(), "scripts", "installed")
	_ = os.MkdirAll(installedDir, 0755)

	scriptPath := filepath.Join(installedDir, meta.ID+".js")
	metaPath := filepath.Join(installedDir, meta.ID+".json")

	if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to save script file: %w", err)
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(metaPath, metaBytes, 0644)

	// Also mirror to active files
	scriptsDir := filepath.Join(GetEngineCacheDir(), "scripts")
	_ = os.WriteFile(filepath.Join(scriptsDir, "active.js"), []byte(content), 0644)
	_ = os.WriteFile(filepath.Join(scriptsDir, "active_meta.json"), metaBytes, 0644)
	_ = os.WriteFile(filepath.Join(scriptsDir, "active_id.txt"), []byte(meta.ID), 0644)

	// 5. Activate runtime
	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	newPool := NewWorkerPool(program)
	newState := &EngineState{
		version:   meta.Version,
		scriptID:  meta.ID,
		meta:      meta,
		program:   program,
		pool:      newPool,
		sources:   meta.Sources,
	}
	e.state.Store(newState)

	// Clear stream cache on switch
	e.cacheMu.Lock()
	e.streamCache = make(map[string]*cachedStreamItem)
	e.cacheMu.Unlock()

	log.Info("MusicEngine: Script imported and activated successfully", "id", meta.ID, "name", meta.Name, "version", meta.Version)
	return &meta, nil
}

// ImportScriptFromURL fetches remote script with timeout and imports it
func (e *MusicEngine) ImportScriptFromURL(ctx context.Context, scriptURL string) (*SourceScriptMeta, error) {
	scriptURL = strings.TrimSpace(scriptURL)
	if !strings.HasPrefix(scriptURL, "http://") && !strings.HasPrefix(scriptURL, "https://") {
		return nil, errors.New("url must start with http:// or https://")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download script from url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote server returned status: %s", resp.Status)
	}

	// Limit script size to 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read remote script content: %w", err)
	}

	return e.ImportScript(string(body), scriptURL)
}

// ActivateScriptByID switches active script to a previously installed one
func (e *MusicEngine) ActivateScriptByID(scriptID string) error {
	installedDir := filepath.Join(GetEngineCacheDir(), "scripts", "installed")
	scriptPath := filepath.Join(installedDir, scriptID+".js")
	metaPath := filepath.Join(installedDir, scriptID+".json")

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("script file not found: %w", err)
	}

	var meta SourceScriptMeta
	if mBytes, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(mBytes, &meta)
	} else {
		meta = ExtractScriptMetadata(string(content))
		meta.ID = scriptID
	}
	meta.IsActive = true

	program, err := goja.Compile("user_script.js", string(content), false)
	if err != nil {
		return fmt.Errorf("script compilation failed: %w", err)
	}

	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	newPool := NewWorkerPool(program)
	newState := &EngineState{
		version:   meta.Version,
		scriptID:  meta.ID,
		meta:      meta,
		program:   program,
		pool:      newPool,
		sources:   meta.Sources,
	}
	e.state.Store(newState)

	// Record active id
	scriptsDir := filepath.Join(GetEngineCacheDir(), "scripts")
	_ = os.WriteFile(filepath.Join(scriptsDir, "active.js"), content, 0644)
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(scriptsDir, "active_meta.json"), metaBytes, 0644)
	_ = os.WriteFile(filepath.Join(scriptsDir, "active_id.txt"), []byte(scriptID), 0644)

	// Clear stream cache on switch
	e.cacheMu.Lock()
	e.streamCache = make(map[string]*cachedStreamItem)
	e.cacheMu.Unlock()

	log.Info("MusicEngine: Script activated by ID", "id", scriptID, "name", meta.Name)
	return nil
}

// DeleteScript removes an installed script
func (e *MusicEngine) DeleteScript(scriptID string) error {
	state := e.state.Load()
	if state != nil && state.scriptID == scriptID {
		return errors.New("无法删除当前正在激活的音源，请先切换至其他音源或停用")
	}

	installedDir := filepath.Join(GetEngineCacheDir(), "scripts", "installed")
	_ = os.Remove(filepath.Join(installedDir, scriptID+".js"))
	_ = os.Remove(filepath.Join(installedDir, scriptID+".json"))
	return nil
}

// CheckUpdate checks if a subscription script has remote updates
func (e *MusicEngine) CheckUpdate(ctx context.Context, scriptID string) (bool, string, error) {
	installedDir := filepath.Join(GetEngineCacheDir(), "scripts", "installed")
	metaPath := filepath.Join(installedDir, scriptID+".json")

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return false, "", errors.New("音源配置不存在")
	}
	var meta SourceScriptMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return false, "", err
	}

	if meta.SubscriptionURL == "" {
		return false, "", errors.New("该音源非网络订阅导入，无远端更新地址")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.SubscriptionURL, nil)
	if err != nil {
		return false, "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("检查更新失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return false, "", err
	}

	remoteMeta := ExtractScriptMetadata(string(body))
	if remoteMeta.Version != "" && remoteMeta.Version != meta.Version {
		return true, remoteMeta.Version, nil
	}

	// Compare content hash if version tag is identical
	currentContent, _ := os.ReadFile(filepath.Join(installedDir, scriptID+".js"))
	if len(currentContent) > 0 && sha256.Sum256(currentContent) != sha256.Sum256(body) {
		return true, "最新修订版", nil
	}

	return false, meta.Version, nil
}

// PingPlatforms tests latency and health of supported platforms
func (e *MusicEngine) PingPlatforms(ctx context.Context) ([]PingResult, error) {
	state := e.state.Load()
	if state == nil || state.pool == nil {
		return nil, errors.New("music engine has no active source script configured")
	}

	platformNames := map[string]string{
		"wy": "网易云音乐",
		"tx": "QQ音乐",
		"kg": "酷狗音乐",
		"kw": "酷我音乐",
		"mg": "咪咕音乐",
	}

	var results []PingResult
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, src := range state.sources {
		source := src
		name := platformNames[source]
		if name == "" {
			name = strings.ToUpper(source)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			// Probe platform with simple search
			_, err := e.Search(ctx, source, "海阔天空", 1, 1)
			delay := time.Since(start).Milliseconds()

			res := PingResult{
				Platform: source,
				Name:     name,
				DelayMS:  delay,
				Status:   "healthy",
			}
			if err != nil {
				res.Status = "error"
				res.Error = err.Error()
			} else if delay > 1200 {
				res.Status = "slow"
			}

			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results, nil
}

// ActivateScript compiles, verifies, and atomically switches the active user script
func (e *MusicEngine) ActivateScript(scriptContent string, scriptID, name, description string) error {
	_, err := e.ImportScript(scriptContent, "")
	return err
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
	activeIDFile := filepath.Join(GetEngineCacheDir(), "scripts", "active_id.txt")
	if idBytes, err := os.ReadFile(activeIDFile); err == nil {
		activeID := strings.TrimSpace(string(idBytes))
		if activeID != "" {
			if err := e.ActivateScriptByID(activeID); err == nil {
				return
			}
		}
	}

	// Fallback to active.js
	scriptsDir := filepath.Join(GetEngineCacheDir(), "scripts")
	activeFile := filepath.Join(scriptsDir, "active.js")
	content, err := os.ReadFile(activeFile)
	if err != nil {
		return
	}

	_, _ = e.ImportScript(string(content), "")
}
