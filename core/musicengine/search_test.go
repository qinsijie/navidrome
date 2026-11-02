package musicengine

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
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

func TestGojaPromiseExport(t *testing.T) {
	vm := goja.New()
	val, err := vm.RunString(`
		new Promise(function(resolve) {
			resolve("https://example.com/song.mp3");
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	t.Logf("val type: %T, val.Export() type: %T", val, val.Export())
	if p, ok := val.Export().(*goja.Promise); ok {
		t.Logf("Successfully exported as *goja.Promise! State: %v, Result: %v", p.State(), p.Result())
	} else {
		t.Logf("Could not export as *goja.Promise directly. val: %#v", val)
	}

	valAsync, err := vm.RunString(`
		(async function() {
			return "https://example.com/async.mp3";
		})()
	`)
	if err != nil {
		t.Fatalf("RunString async failed: %v", err)
	}
	pAsync := valAsync.Export().(*goja.Promise)
	t.Logf("async function: State: %v, Result: %v", pAsync.State(), pAsync.Result())
}

func TestResolveStreamURLNative(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	engine := &MusicEngine{
		streamCache: make(map[string]*cachedStreamItem),
	}

	stream, err := engine.ResolveStreamURL(ctx, "kw", "284144", Quality128k)
	if err != nil {
		t.Fatalf("ResolveStreamURL failed: %v", err)
	}
	if stream == nil || stream.URL == "" {
		t.Fatalf("ResolveStreamURL returned empty stream")
	}
	t.Logf("Successfully resolved native stream URL: %s", stream.URL)
}
