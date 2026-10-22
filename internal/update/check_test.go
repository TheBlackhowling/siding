package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAndNewer(t *testing.T) {
	if IsReleaseVersion("dev") || IsReleaseVersion("") || IsReleaseVersion("none") {
		t.Fatal("dev")
	}
	if IsReleaseVersion("0.1.4-rc.1") || IsReleaseVersion("1.2") {
		t.Fatal("prerelease / short")
	}
	if !IsReleaseVersion("0.1.3") || !IsReleaseVersion("v0.1.3") {
		t.Fatal("stable")
	}
	if !Newer("0.1.3", "v0.1.4") || Newer("v0.1.4", "0.1.3") || Newer("0.1.3", "0.1.3") {
		t.Fatal("compare")
	}
	if Display("0.1.3") != "v0.1.3" || Display("v0.1.3") != "v0.1.3" {
		t.Fatal("display")
	}
}

func TestCheckNewerAndCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent")
		}
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.1.4"})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	opt := Options{
		Current:   "0.1.3",
		CacheDir:  dir,
		Now:       now,
		Client:    srv.Client(),
		LatestURL: srv.URL,
	}
	got, err := Check(context.Background(), opt)
	if err != nil || !got.Newer || got.Latest != "v0.1.4" {
		t.Fatalf("%+v %v", got, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}

	opt.Now = now.Add(DefaultTTL - time.Second)
	got, err = Check(context.Background(), opt)
	if err != nil || !got.Newer || hits.Load() != 1 {
		t.Fatalf("cache hit: %+v %v hits=%d", got, err, hits.Load())
	}

	opt.Now = now.Add(DefaultTTL + time.Second)
	got, err = Check(context.Background(), opt)
	if err != nil || hits.Load() != 2 {
		t.Fatalf("ttl: %+v %v hits=%d", got, err, hits.Load())
	}
}

func TestCheckSameAndOlderSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.1.3"})
	}))
	t.Cleanup(srv.Close)
	got, err := Check(context.Background(), Options{
		Current:   "v0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err != nil || got.Newer {
		t.Fatalf("same: %+v %v", got, err)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.1.2"})
	}))
	t.Cleanup(srv2.Close)
	got, err = Check(context.Background(), Options{
		Current:   "0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv2.Client(),
		LatestURL: srv2.URL,
	})
	if err != nil || got.Newer {
		t.Fatalf("older latest: %+v %v", got, err)
	}
}

func TestCheckDevSkipsHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not fetch")
	}))
	t.Cleanup(srv.Close)
	got, err := Check(context.Background(), Options{
		Current:   "dev",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err != nil || got.Newer || got.Latest != "" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestCheckHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	_, err := Check(context.Background(), Options{
		Current:   "0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err == nil {
		t.Fatal("404")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	cancel()
	_, err = Check(ctx, Options{
		Current:   "0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err == nil {
		t.Fatal("canceled")
	}

	_, err = Check(context.Background(), Options{
		Current:   "0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: "http://127.0.0.1:1",
	})
	if err == nil {
		t.Fatal("refused")
	}
}

func TestCheckRejectsPrerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.1.4-rc.1", Prerelease: true})
	}))
	t.Cleanup(srv.Close)
	_, err := Check(context.Background(), Options{
		Current:   "0.1.3",
		CacheDir:  t.TempDir(),
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err == nil {
		t.Fatal("prerelease")
	}
}

func TestDisabled(t *testing.T) {
	t.Setenv(EnvDisable, "")
	if Disabled() {
		t.Fatal("empty")
	}
	t.Setenv(EnvDisable, "1")
	if !Disabled() {
		t.Fatal("set")
	}
}

func TestCorruptCacheIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, CacheFileName), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.2.0"})
	}))
	t.Cleanup(srv.Close)
	got, err := Check(context.Background(), Options{
		Current:   "0.1.0",
		CacheDir:  dir,
		Client:    srv.Client(),
		LatestURL: srv.URL,
	})
	if err != nil || !got.Newer || got.Latest != "v0.2.0" {
		t.Fatalf("%+v %v", got, err)
	}
}
