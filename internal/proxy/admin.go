package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TheBlackhowling/siding/internal/state"
)

// adminBase is the Caddy admin origin. Tests override it; production uses AdminURL().
var adminBase string

func adminAPI(path string) string {
	base := adminBase
	if base == "" {
		base = AdminURL()
	}
	return strings.TrimRight(base, "/") + path
}

// AdminURL is the loopback Caddy admin API published by siding-proxy-caddy.
// The API is unauthenticated; loopback limits remote access, not other local processes.
func AdminURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", state.ProxyAdminListen)
}

func adminClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}

func AdminReachable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adminAPI("/config/"), nil)
	if err != nil {
		return false
	}
	resp, err := adminClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

// LoadCaddyfile posts a Caddyfile to the in-container admin API (adapt + load).
// Caddy reloads without a process restart.
func LoadCaddyfile(ctx context.Context, caddyfile string) error {
	adapted, err := adaptCaddyfile(ctx, caddyfile)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adminAPI("/load"), bytes.NewReader(adapted))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := adminClient().Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin load: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("caddy admin load HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	return nil
}

func adaptCaddyfile(ctx context.Context, caddyfile string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adminAPI("/adapt"), bytes.NewBufferString(caddyfile))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/caddyfile")
	resp, err := adminClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("caddy admin adapt: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("caddy admin adapt HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	return body, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
