package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestLoadCaddyfileAdaptThenLoad(t *testing.T) {
	var (
		mu     sync.Mutex
		loaded string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/config"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/adapt":
			if r.Header.Get("Content-Type") != "text/caddyfile" {
				t.Errorf("adapt content-type %q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "reverse_proxy") {
				t.Errorf("adapt body: %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apps":{}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/load":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("load content-type %q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			loaded = string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { adminBase = "" })
	adminBase = srv.URL

	if !AdminReachable(context.Background()) {
		t.Fatal("admin should be reachable")
	}
	if err := LoadCaddyfile(context.Background(), "http://x {\n  reverse_proxy host.docker.internal:1\n}\n"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if loaded != `{"apps":{}}` {
		t.Fatalf("loaded %q", loaded)
	}
}
