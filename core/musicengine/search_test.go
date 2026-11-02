package musicengine

import (
	"context"
	"testing"
	"time"
)

func TestGetSearchTips(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	tips, err := GetSearchTips(ctx, "陈奕迅")
	if err != nil {
		t.Fatalf("GetSearchTips failed: %v", err)
	}
	if len(tips) == 0 {
		t.Logf("Warning: search tips returned 0 results (network dependent)")
	} else {
		t.Logf("Success: search tips returned %d suggestions: %v", len(tips), tips)
	}
}

func TestSearchNativeAggregated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := SearchNative(ctx, "all", "陈奕迅", 1, 10)
	if err != nil {
		t.Fatalf("SearchNative aggregated failed: %v", err)
	}
	if res == nil || len(res.List) == 0 {
		t.Fatalf("SearchNative returned 0 songs for query '陈奕迅'")
	}
	t.Logf("Success: Aggregated search returned %d songs", len(res.List))
	for i, track := range res.List {
		if i >= 3 {
			break
		}
		t.Logf("  [%d] Source: %s, Title: %s, Artist: %s, Qualities: %v",
			i+1, track.Source, track.Title, track.Artist, track.Qualities)
	}
}

func TestGetSonglistDetailNative(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	detail, err := GetSonglistDetailNative(ctx, "wy", "3778678")
	if err != nil {
		t.Fatalf("GetSonglistDetailNative failed: %v", err)
	}
	if detail == nil || len(detail.List) == 0 {
		t.Fatalf("GetSonglistDetailNative returned 0 tracks for playlist 3778678")
	}
	t.Logf("Success: NetEase playlist detail fetched %d tracks (Title: %s)", len(detail.List), detail.Title)
}
