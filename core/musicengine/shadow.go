package musicengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"golang.org/x/sync/singleflight"
)

var shadowFlight singleflight.Group

// ResolveTargetLibraryID dynamically queries the most appropriate library ID
// for the current user or system, avoiding hardcoded IDs.
func ResolveTargetLibraryID(ctx context.Context, ds model.DataStore) (int, error) {
	user, ok := request.UserFrom(ctx)
	if ok && user.ID != "" && !user.IsAdmin {
		userLibs, err := ds.User().GetUserLibraries(ctx, user.ID)
		if err == nil && len(userLibs) > 0 {
			return userLibs[0].ID, nil
		}
	}

	allLibs, err := ds.Library().GetAll(ctx)
	if err != nil || len(allLibs) == 0 {
		return model.DefaultLibraryID, nil
	}

	for _, l := range allLibs {
		if l.DefaultNewUsers {
			return l.ID, nil
		}
	}
	return allLibs[0].ID, nil
}

// BuildDefensiveShadowTrack constructs a model.MediaFile with safe defensive defaults
// to prevent divide-by-zero or corrupted-file marking in mobile and desktop Subsonic clients.
func BuildDefensiveShadowTrack(onlineTrack *OnlineTrack, libID int) *model.MediaFile {
	duration := onlineTrack.Duration
	if duration <= 0 {
		duration = 240.0 // 4 minutes fallback
	}

	bitRate := 960 // CD FLAC standard
	suffix := "flac"
	codec := "FLAC"
	bitDepth := 16
	sampleRate := 44100

	switch onlineTrack.Quality {
	case QualityFlac24b, "hires":
		bitRate = 2400
		suffix = "flac"
		codec = "FLAC"
		bitDepth = 24
		sampleRate = 96000
	case QualityFlac:
		bitRate = 960
		suffix = "flac"
		codec = "FLAC"
	case Quality320k:
		bitRate = 320
		suffix = "mp3"
		codec = "MP3"
	default:
		bitRate = 128
		suffix = "mp3"
		codec = "MP3"
	}

	// Defensive size estimate: bytes = duration * (bitrate * 1000 / 8)
	estimatedSize := int64(duration * float64(bitRate*1000/8))

	universalID := onlineTrack.UniversalID
	if universalID == "" {
		universalID = BuildUniversalID(onlineTrack.Source, onlineTrack.SongID)
	}

	protocolPath := onlineTrack.ProtocolPath
	if protocolPath == "" {
		protocolPath = BuildProtocolPath(onlineTrack.Source, onlineTrack.SongID, onlineTrack.Quality)
	}

	return &model.MediaFile{
		ID:          universalID,
		Path:        protocolPath,
		LibraryID:   libID,
		FolderID:    "", // Isolated from physical folder scanner
		Title:       onlineTrack.Title,
		Artist:      onlineTrack.Artist,
		Album:       onlineTrack.Album,
		Duration:    float32(duration),
		Size:        estimatedSize,
		BitRate:     bitRate,
		Suffix:      suffix,
		Codec:       codec,
		BitDepth:    &bitDepth,
		SampleRate:  sampleRate,
		Channels:    2,
		HasCoverArt: onlineTrack.CoverURL != "",
		Missing:     false,
		UpdatedAt:   time.Now(),
		CreatedAt:   time.Now(),
	}
}

// EnsureShadowMediaFile idempotently creates or verifies the shadow media_file record
// for an online track, using singleflight to eliminate SQLite lock contention and duplicate key conflicts.
func EnsureShadowMediaFile(ctx context.Context, ds model.DataStore, track *OnlineTrack) (*model.MediaFile, error) {
	if track.UniversalID == "" {
		track.UniversalID = BuildUniversalID(track.Source, track.SongID)
	}

	// 1. Fast check if already exists in DB
	if existing, err := ds.MediaFile().Get(ctx, track.UniversalID); err == nil && existing != nil {
		return existing, nil
	}

	// 2. Coalesce concurrent requests for the exact same track ID via singleflight
	val, err, _ := shadowFlight.Do(track.UniversalID, func() (any, error) {
		// Double check locking
		if existing, err := ds.MediaFile().Get(ctx, track.UniversalID); err == nil && existing != nil {
			return existing, nil
		}

		libID, err := ResolveTargetLibraryID(ctx, ds)
		if err != nil {
			return nil, err
		}

		shadow := BuildDefensiveShadowTrack(track, libID)

		// 3. Write with retry transaction
		txErr := ds.WithTxRetry(ctx, func(ctx context.Context, tx model.DataStore) error {
			return tx.MediaFile().Put(ctx, shadow)
		}, "musicengine: ensure shadow track")

		if txErr != nil {
			if strings.Contains(txErr.Error(), "UNIQUE constraint failed") {
				return ds.MediaFile().Get(ctx, track.UniversalID)
			}
			return nil, fmt.Errorf("inserting shadow media file: %w", txErr)
		}

		return shadow, nil
	})

	if err != nil {
		return nil, err
	}
	return val.(*model.MediaFile), nil
}

// UpdateRealMetadata updates the shadow record with actual verified stream metrics
func UpdateRealMetadata(ctx context.Context, ds model.DataStore, id string, size int64, bitRate int, sampleRate int, bitDepth int) {
	go func() {
		mf, err := ds.MediaFile().Get(context.Background(), id)
		if err != nil || mf == nil {
			return
		}
		mf.Size = size
		if bitRate > 0 {
			mf.BitRate = bitRate
		}
		if sampleRate > 0 {
			mf.SampleRate = sampleRate
		}
		if bitDepth > 0 {
			mf.BitDepth = &bitDepth
		}
		mf.UpdatedAt = time.Now()
		_ = ds.WithTxRetry(context.Background(), func(ctx context.Context, tx model.DataStore) error {
			return tx.MediaFile().Put(ctx, mf)
		}, "musicengine: update real metadata")
		log.Debug(context.Background(), "MusicEngine: Self-healing metadata updated", "id", id, "size", size, "bitRate", bitRate)
	}()
}
