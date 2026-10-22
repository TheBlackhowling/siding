package siding

import (
	"context"
	"fmt"

	"github.com/TheBlackhowling/siding/internal/hostport"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/state"
)

// ProxyUpOptions start the shared Caddy proxy (siding proxy up).
type ProxyUpOptions struct {
	Listen int // 0 = keep or migrate default (80)
}

// ProxyUp starts Caddy on loopback and writes routes from state.json.
func (c *Client) ProxyUp(ctx context.Context, opts ProxyUpOptions) (ProxyConfig, error) {
	s, err := c.session(ctx)
	if err != nil {
		return ProxyConfig{}, err
	}
	listenPort := state.ListenPort(s.st)
	if opts.Listen > 0 {
		if opts.Listen < 1 || opts.Listen > 65535 {
			return ProxyConfig{}, fmt.Errorf("listen port must be 1-65535")
		}
		listenPort = opts.Listen
	} else if listenPort == 0 || listenPort == 1355 {
		listenPort = state.ProxyHTTPListen
	}
	if occ := hostport.Lookup(ctx, listenPort).Blocking(proxy.ContainerName); len(occ) > 0 {
		return ProxyConfig{}, hostport.BusyError(listenPort, occ)
	}
	if opts.Listen > 0 {
		state.SetListenPort(s.st, opts.Listen)
	} else {
		state.MigrateOffDefaultPorts(s.st)
	}
	if err := s.write(); err != nil {
		return ProxyConfig{}, err
	}
	cfg, err := proxy.Up(ctx, s.paths.ProxyDir, s.st)
	if err != nil {
		return ProxyConfig{}, err
	}
	return proxyConfigFrom(cfg), nil
}

// ProxyDown stops the shared Caddy proxy (siding proxy down).
func (c *Client) ProxyDown(ctx context.Context) error {
	s, err := c.session(ctx)
	if err != nil {
		return err
	}
	return proxy.Down(ctx, s.paths.ProxyDir)
}

// ProxyReload rewrites routes from state.json and reloads Caddy.
func (c *Client) ProxyReload(ctx context.Context) (ProxyConfig, error) {
	s, err := c.session(ctx)
	if err != nil {
		return ProxyConfig{}, err
	}
	cfg, err := proxy.Reload(ctx, s.paths.ProxyDir, s.st)
	if err != nil {
		return ProxyConfig{}, err
	}
	return proxyConfigFrom(cfg), nil
}

func proxyRefreshDir(ctx context.Context, proxyDir string, st *state.File) error {
	return proxy.RefreshIfRunning(ctx, proxyDir, st)
}
