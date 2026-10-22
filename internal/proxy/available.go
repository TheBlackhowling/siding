package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TheBlackhowling/siding/internal/state"
)

func effectiveListen(listen int) int {
	if listen <= 0 {
		return state.ProxyHTTPListen
	}
	return listen
}

// HTTPRewriteOn is true when Caddy http_port is not 80. Site blocks stay
// http://*.localhost while Docker publishes 127.0.0.1:{listen}:{listen}.
func HTTPRewriteOn(listen int) bool {
	return effectiveListen(listen) != 80
}

// RequireForUp fails siding up when http_port rewrite is on and Caddy is not
// answering on the listen port. Listen 80 skips the check (no rewrite).
func RequireForUp(ctx context.Context, listen int) error {
	listen = effectiveListen(listen)
	if !HTTPRewriteOn(listen) {
		return nil
	}
	if !InspectRunning(ctx) {
		return fmt.Errorf("caddy proxy is not running (http_port rewrite on 127.0.0.1:%d; host :80 is free) — start it with: siding proxy up", listen)
	}
	if err := waitListen(ctx, listen, 3*time.Second); err != nil {
		return fmt.Errorf("caddy is up but not answering on 127.0.0.1:%d: %w — try: siding proxy up", listen, err)
	}
	return nil
}

func waitListen(ctx context.Context, listen int, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	var last error
	for {
		last = probeListen(ctx, listen)
		if last == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func probeListen(ctx context.Context, listen int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", listen), nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 800 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return nil
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}
