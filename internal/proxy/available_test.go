package proxy

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestHTTPRewriteOn(t *testing.T) {
	if HTTPRewriteOn(80) {
		t.Fatal("listen 80 is not a rewrite")
	}
	if !HTTPRewriteOn(1355) {
		t.Fatal("1355 should rewrite off :80")
	}
	if HTTPRewriteOn(0) {
		t.Fatal("unset listen defaults to 80")
	}
}

func TestRequireForUpSkipsListen80(t *testing.T) {
	if err := RequireForUp(context.Background(), 80); err != nil {
		t.Fatalf("listen 80 should skip Caddy check: %v", err)
	}
}

func TestRequireForUpWhenRewriteOn(t *testing.T) {
	testenv.NoHostTools(t)
	err := RequireForUp(context.Background(), 1355)
	if err == nil {
		t.Fatal("rewrite listen with no running proxy must fail")
	}
	if !strings.Contains(err.Error(), "is not running") || !strings.Contains(err.Error(), "siding proxy up") {
		t.Fatalf("error should say the proxy is down and how to start it: %v", err)
	}
}

func TestProbeListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://siding.localhost:1355", http.StatusFound)
	})}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	go func() { _ = srv.Serve(ln) }()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitListen(context.Background(), port, 2*time.Second); err != nil {
		t.Fatalf("expected reachable listener: %v", err)
	}

	if err := probeListen(context.Background(), 1); err == nil {
		t.Fatal("expected closed port to fail")
	}
}
