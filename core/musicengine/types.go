package musicengine

import (
	"fmt"
	"strings"
	"time"
)

// Quality types recognized by online music engines
const (
	Quality128k    = "128k"
	Quality320k    = "320k"
	QualityFlac    = "flac"
	QualityFlac24b = "flac24bit"
)

// OnlineTrack represents a normalized online track entity
type OnlineTrack struct {
	UniversalID  string   `json:"id"`           // e.g. "online:wy:1859245776"
	ProtocolPath string   `json:"protocolPath"` // e.g. "online://wy/1859245776?quality=flac"
	Source       string   `json:"source"`       // e.g. "wy", "tx", "kg", "kw", "mg"
	SongID       string   `json:"songId"`       // e.g. "1859245776"
	Title        string   `json:"title"`
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	Duration     float64  `json:"duration"` // in seconds
	CoverURL     string   `json:"coverUrl"`
	Quality      string   `json:"quality"` // "128k", "320k", "flac", "flac24bit"
	Qualities    []string `json:"qualities,omitempty"`
}

// BuildUniversalID creates the unified online ID: "online:{source}:{id}"
func BuildUniversalID(source, songID string) string {
	return fmt.Sprintf("online:%s:%s", source, songID)
}

// BuildProtocolPath creates the virtual storage path: "online://{source}/{id}?quality={quality}"
func BuildProtocolPath(source, songID, quality string) string {
	if quality == "" {
		quality = QualityFlac
	}
	return fmt.Sprintf("online://%s/%s?quality=%s", source, songID, quality)
}

// ParseUniversalID parses "online:{source}:{id}" into source and songID
func ParseUniversalID(id string) (source, songID string, ok bool) {
	if !strings.HasPrefix(id, "online:") {
		return "", "", false
	}
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// ParseProtocolPath parses "online://{source}/{songId}?quality={quality}"
func ParseProtocolPath(path string) (source, songID, quality string, ok bool) {
	if !strings.HasPrefix(path, "online://") {
		return "", "", "", false
	}
	trimmed := strings.TrimPrefix(path, "online://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}
	source = parts[0]
	rest := parts[1]
	quality = QualityFlac
	if qIdx := strings.Index(rest, "?quality="); qIdx != -1 {
		songID = rest[:qIdx]
		quality = rest[qIdx+len("?quality="):]
	} else {
		songID = rest
	}
	return source, songID, quality, true
}

// ResolvedStream represents the actual stream link and anti-leech headers resolved from upstream
type ResolvedStream struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Format    string            `json:"format"`
	Bitrate   int               `json:"bitrate"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

// SearchResult represents online search query results
type SearchResult struct {
	Source string         `json:"source"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
	Total  int            `json:"total"`
	List   []*OnlineTrack `json:"list"`
}

// SongList represents an online curated playlist summary
type SongList struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Title    string `json:"title"`
	CoverURL string `json:"coverUrl"`
	PlayCount string `json:"playCount,omitempty"`
	Author   string `json:"author,omitempty"`
}

// SongListDetail represents detailed contents of an online playlist
type SongListDetail struct {
	ID       string         `json:"id"`
	Source   string         `json:"source"`
	Title    string         `json:"title"`
	CoverURL string         `json:"coverUrl"`
	Desc     string         `json:"desc"`
	Author   string         `json:"author"`
	List     []*OnlineTrack `json:"list"`
}

// Leaderboard represents an online music chart summary
type Leaderboard struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Name     string `json:"name"`
	CoverURL string `json:"coverUrl"`
	BangID   string `json:"bangId,omitempty"`
}

// LeaderboardDetail represents the track listing of an online music chart
type LeaderboardDetail struct {
	ID       string         `json:"id"`
	Source   string         `json:"source"`
	Name     string         `json:"name"`
	CoverURL string         `json:"coverUrl"`
	List     []*OnlineTrack `json:"list"`
}

// SourceScriptMeta holds metadata of an imported user script
type SourceScriptMeta struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Sources     []string `json:"sources"`
	IsActive    bool     `json:"isActive"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
