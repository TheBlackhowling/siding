package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestGenerateComposeAndStatus(t *testing.T) {
	body := GenerateCompose(0)
	if !strings.Contains(body, "127.0.0.1:80:80") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, ProxyImage) || !strings.Contains(body, "pull_policy: always") {
		t.Fatalf("%s", body)
	}
	t.Setenv("SIDING_PROXY_IMAGE", "siding-proxy:local")
	local := GenerateCompose(0)
	if !strings.Contains(local, "image: siding-proxy:local") || !strings.Contains(local, "pull_policy: never") {
		t.Fatalf("%s", local)
	}
	dir := t.TempDir()
	if err := WriteStatus(dir, 80); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, StatusFileName)); err != nil {
		t.Fatal(err)
	}
	ClearStatus(dir)
	if _, err := os.Stat(filepath.Join(dir, StatusFileName)); !os.IsNotExist(err) {
		t.Fatal("status should be gone")
	}
	if got := skipKeys(nil); got != nil {
		t.Fatal(got)
	}
	if got := skipKeys(map[int]struct{}{2019: {}, 80: {}}); len(got) != 2 || got[0] != 80 {
		t.Fatalf("%v", got)
	}
}

func TestAdminURLAndTruncate(t *testing.T) {
	if AdminURL() != "http://127.0.0.1:2019" {
		t.Fatalf("%s", AdminURL())
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short")
	}
	if got := truncate(strings.Repeat("x", 50), 10); !strings.HasSuffix(got, "...") || len(got) != 13 {
		t.Fatalf("%q", got)
	}
}

func TestDownRefreshWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	ctx := context.Background()
	dir := t.TempDir()
	statusPath := filepath.Join(dir, StatusFileName)

	// Down with no compose file is a no-op that still clears stale status.
	if err := WriteStatus(dir, 80); err != nil {
		t.Fatal(err)
	}
	if err := Down(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatal("Down must clear status")
	}

	// Not running: RefreshIfRunning clears status and does not try to reload.
	if err := WriteStatus(dir, 80); err != nil {
		t.Fatal(err)
	}
	if InspectRunning(ctx) {
		t.Fatal("no docker on PATH cannot report a running proxy")
	}
	if err := RefreshIfRunning(ctx, dir, state.NewFile()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatal("RefreshIfRunning must clear status when the proxy is down")
	}
	if logs := containerLogs(ctx); logs != "" {
		t.Fatalf("no docker means no logs: %q", logs)
	}

	short, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	cfg := Config{Listen: 1, CaddyfilePath: filepath.Join(dir, "missing")}
	err := waitReady(short, cfg)
	if err == nil || !strings.Contains(err.Error(), "did not start on 127.0.0.1:1") {
		t.Fatalf("port 1 is never ready: %v", err)
	}
}

func TestWriteFilesSkipKeys(t *testing.T) {
	dir := t.TempDir()
	st := state.NewFile()
	cfg, err := WriteFilesSkip(dir, st, map[int]struct{}{2019: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SkippedPublishes) != 1 || cfg.SkippedPublishes[0] != 2019 {
		t.Fatalf("%v", cfg.SkippedPublishes)
	}
	body, err := os.ReadFile(cfg.ComposePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "2019:2019") {
		t.Fatalf("%s", body)
	}
}

func TestBusyExtraPublishesAndWriteProxyFilesWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	ctx := context.Background()
	if skip := busyExtraPublishes(ctx, 80); skip != nil {
		t.Fatalf("docker unavailable must skip nothing: %v", skip)
	}
	dir := t.TempDir()
	cfg, err := writeProxyFiles(ctx, dir, state.NewFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SkippedPublishes) != 0 {
		t.Fatalf("%v", cfg.SkippedPublishes)
	}
	for _, p := range []string{cfg.ComposePath, cfg.CaddyfilePath, filepath.Join(dir, "board", StatusFileName)} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
}

// TestUpPsDownWithoutDocker: with docker off PATH, Up fails before the
// container step, leaves no status.json, and Ps/Down surface the same error.
func TestUpPsDownWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	ctx := context.Background()
	dir := t.TempDir()
	noDocker := "docker not found on PATH"

	cfg, err := Up(ctx, dir, state.NewFile())
	if err == nil || !strings.Contains(err.Error(), noDocker) {
		t.Fatalf("Up: %v", err)
	}
	if cfg.ComposePath == "" {
		t.Fatal("Up returns the written config even when compose fails")
	}
	if _, err := os.Stat(filepath.Join(dir, StatusFileName)); !os.IsNotExist(err) {
		t.Fatal("failed Up must not write status")
	}
	if err := Ps(ctx, dir); err == nil || !strings.Contains(err.Error(), noDocker) {
		t.Fatalf("Ps: %v", err)
	}
	if err := WriteStatus(dir, 80); err != nil {
		t.Fatal(err)
	}
	if err := Down(ctx, dir); err == nil || !strings.Contains(err.Error(), noDocker) {
		t.Fatalf("Down with a compose file present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, StatusFileName)); !os.IsNotExist(err) {
		t.Fatal("Down clears status even when compose fails")
	}
}

func TestApplyCaddyfileViaAdmin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config/" || strings.HasPrefix(r.URL.Path, "/config") {
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/adapt" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apps":{}}`))
			return
		}
		if r.URL.Path == "/load" {
			w.WriteHeader(200)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { adminBase = "" })
	adminBase = srv.URL

	dir := t.TempDir()
	cfg, err := WriteFiles(dir, state.NewFile())
	if err != nil {
		t.Fatal(err)
	}
	if err := applyCaddyfile(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestApplyCaddyfileUnreachable(t *testing.T) {
	t.Cleanup(func() { adminBase = "" })
	adminBase = "http://127.0.0.1:1"
	dir := t.TempDir()
	cfg, err := WriteFiles(dir, state.NewFile())
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := applyCaddyfile(short, cfg); err == nil {
		t.Fatal("expected unreachable admin")
	}
	_ = GenerateBoardJSON(nil)
	_ = GenerateBoardJSON(state.NewFile())
}

func TestReloadNotRunning(t *testing.T) {
	testenv.NoHostTools(t)
	dir := t.TempDir()
	cfg, err := Reload(context.Background(), dir, state.NewFile())
	if err == nil || !strings.Contains(err.Error(), "proxy is not running") {
		t.Fatalf("%v", err)
	}
	// Files are still (re)written so the next `proxy up` starts from them.
	if _, statErr := os.Stat(cfg.CaddyfilePath); statErr != nil {
		t.Fatal(statErr)
	}
}
