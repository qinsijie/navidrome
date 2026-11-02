package musicengine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// Router handles MusicEngine REST API endpoints
type Router struct {
	http.Handler
	ds     model.DataStore
	engine *MusicEngine
}

// NewRouter constructs and routes MusicEngine endpoints
func NewRouter(ctx context.Context, ds model.DataStore) *Router {
	engine := GetEngine(ds)
	r := &Router{
		ds:     ds,
		engine: engine,
	}
	r.Handler = r.routes()
	return r
}

func (api *Router) routes() http.Handler {
	r := chi.NewRouter()

	// Sources management
	r.Get("/sources", api.handleGetSources)
	r.Post("/sources/upload", api.handleUploadSource)
	r.Post("/sources/import-url", api.handleImportURL)
	r.Post("/sources/activate", api.handleActivateSource)
	r.Post("/sources/{id}/activate", api.handleActivateSource)
	r.Delete("/sources/{id}", api.handleDeleteSource)
	r.Post("/sources/{id}/check-update", api.handleCheckUpdate)
	r.Post("/sources/ping", api.handlePing)

	// Online music exploration
	r.Get("/search", api.handleSearch)
	r.Get("/songlists", api.handleGetSonglists)
	r.Get("/songlist", api.handleGetSonglistDetail)
	r.Get("/leaderboards", api.handleGetLeaderboards)
	r.Get("/leaderboard", api.handleGetLeaderboardDetail)

	// Shadow track creation for playlist integration
	r.Post("/shadow", api.handleEnsureShadowTrack)

	// Stream Relay endpoints
	r.Get("/stream", api.handleStream)
	r.Get("/stream/{id}", api.handleStream)

	return r
}

func (api *Router) handleGetSources(w http.ResponseWriter, r *http.Request) {
	meta := api.engine.GetActiveScriptMeta()
	sources := api.engine.GetAllScripts()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"active":  meta,
		"sources": sources,
	})
}

func (api *Router) handleImportURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	meta, err := api.engine.ImportScriptFromURL(r.Context(), req.URL)
	if err != nil {
		log.Error(r.Context(), "MusicEngine: Import from URL failed", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"meta":    meta,
		"sources": api.engine.GetAllScripts(),
	})
}

func (api *Router) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		Script          string `json:"script"`
		SubscriptionURL string `json:"subscriptionUrl"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		req.Script = string(body)
	}

	if strings.TrimSpace(req.Script) == "" {
		http.Error(w, "Script content cannot be empty", http.StatusBadRequest)
		return
	}

	meta, err := api.engine.ImportScript(req.Script, req.SubscriptionURL)
	if err != nil {
		log.Error(r.Context(), "MusicEngine: Script import failed", err)
		http.Error(w, "Script import failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"meta":    meta,
		"sources": api.engine.GetAllScripts(),
	})
}

func (api *Router) handleActivateSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		var req struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		id = req.ID
	}

	if id != "" {
		if err := api.engine.ActivateScriptByID(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	meta := api.engine.GetActiveScriptMeta()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"meta":    meta,
		"sources": api.engine.GetAllScripts(),
	})
}

func (api *Router) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Missing script ID", http.StatusBadRequest)
		return
	}

	if err := api.engine.DeleteScript(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"sources": api.engine.GetAllScripts(),
	})
}

func (api *Router) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Missing script ID", http.StatusBadRequest)
		return
	}

	hasUpdate, newVer, err := api.engine.CheckUpdate(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"hasUpdate":  hasUpdate,
		"newVersion": newVer,
	})
}

func (api *Router) handlePing(w http.ResponseWriter, r *http.Request) {
	results, err := api.engine.PingPlatforms(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": results,
	})
}

func (api *Router) handleSearch(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "wy"
	}
	query := r.URL.Query().Get("query")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if strings.TrimSpace(query) == "" {
		_ = json.NewEncoder(w).Encode(&SearchResult{Source: source, List: []*OnlineTrack{}})
		return
	}

	res, err := api.engine.Search(r.Context(), source, query, page, limit)
	if err != nil {
		http.Error(w, "Search failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (api *Router) handleGetSonglists(w http.ResponseWriter, r *http.Request) {
	// Return curated default songlists or call script if supported
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "wy"
	}

	lists := []*SongList{
		{ID: "3778678", Source: source, Title: "云音乐热歌榜", Author: "网易云音乐"},
		{ID: "19723756", Source: source, Title: "云音乐飙升榜", Author: "网易云音乐"},
		{ID: "3779629", Source: source, Title: "云音乐新歌榜", Author: "网易云音乐"},
		{ID: "2884035", Source: source, Title: "网易原创歌曲榜", Author: "网易云音乐"},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"source": source,
		"list":   lists,
	})
}

func (api *Router) handleGetSonglistDetail(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "wy"
	}
	id := r.URL.Query().Get("id")

	detail := &SongListDetail{
		ID:     id,
		Source: source,
		Title:  "精选歌单",
		List:   make([]*OnlineTrack, 0),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (api *Router) handleGetLeaderboards(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "wy"
	}

	boards := []*Leaderboard{
		{ID: "hot", Source: source, Name: "热歌榜"},
		{ID: "soar", Source: source, Name: "飙升榜"},
		{ID: "new", Source: source, Name: "新歌榜"},
		{ID: "origin", Source: source, Name: "原创榜"},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"source": source,
		"list":   boards,
	})
}

func (api *Router) handleGetLeaderboardDetail(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "wy"
	}
	id := r.URL.Query().Get("id")

	detail := &LeaderboardDetail{
		ID:     id,
		Source: source,
		Name:   "排行榜",
		List:   make([]*OnlineTrack, 0),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (api *Router) handleEnsureShadowTrack(w http.ResponseWriter, r *http.Request) {
	var track OnlineTrack
	if err := json.NewDecoder(r.Body).Decode(&track); err != nil {
		http.Error(w, "Invalid track payload", http.StatusBadRequest)
		return
	}

	mf, err := EnsureShadowMediaFile(r.Context(), api.ds, &track)
	if err != nil {
		http.Error(w, "Failed to ensure shadow track: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"mediaFile": mf,
	})
}

func (api *Router) handleStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}

	source, songID, ok := ParseUniversalID(id)
	quality := QualityFlac
	if !ok {
		// Try parsing from query
		source = r.URL.Query().Get("source")
		songID = r.URL.Query().Get("songId")
		if q := r.URL.Query().Get("quality"); q != "" {
			quality = q
		}
	}

	if source == "" || songID == "" {
		http.Error(w, "Missing source or song ID", http.StatusBadRequest)
		return
	}

	// 1. Resolve live stream URL
	resolved, err := api.engine.ResolveStreamURL(r.Context(), source, songID, quality)
	if err != nil {
		log.Error(r.Context(), "MusicEngine: Failed to resolve stream URL", "id", id, err)
		http.Error(w, "Failed to resolve online track: "+err.Error(), http.StatusNotFound)
		return
	}

	// 2. Initialize StreamRelay and serve
	relay := NewStreamRelay(resolved, 0, 30*1024*1024)
	defer relay.Close()

	if err := relay.StartPull(r.Context(), 0); err != nil {
		http.Error(w, "Failed to connect to audio CDN: "+err.Error(), http.StatusBadGateway)
		return
	}

	relay.ServeHTTP(w, r)
}
