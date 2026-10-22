package proxy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/state"
)

func InspectRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", ContainerName)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) == "running"
}

func composePublishChanged(prev, next []byte) bool {
	return !bytes.Equal(bytes.TrimSpace(prev), bytes.TrimSpace(next))
}

func composeArgs(action string, extra ...string) []string {
	args := []string{"compose", "-f", ComposeFileName, action}
	return append(args, extra...)
}

// composeUpArgs recreates the container so a new Caddyfile is loaded and
// :latest of the board image is pulled. `up -d` alone is a no-op when
// siding-proxy-caddy is already running, which leaves the previous
// catch-all redirect in memory.
func ComposeUpArgs() []string {
	return composeUpArgs()
}

func composeUpArgs() []string {
	pull := "always"
	if strings.TrimSpace(os.Getenv("SIDING_PROXY_IMAGE")) != "" {
		pull = "never"
	}
	return composeArgs("up", "-d", "--force-recreate", "--remove-orphans", "--pull", pull)
}

func Up(ctx context.Context, proxyDir string, st *state.File) (Config, error) {
	cfg, err := writeProxyFiles(ctx, proxyDir, st)
	if err != nil {
		return Config{}, err
	}
	if err := compose.Run(ctx, proxyDir, composeUpArgs()); err != nil {
		return cfg, err
	}
	if err := waitReady(ctx, cfg); err != nil {
		ClearStatus(proxyDir)
		return cfg, err
	}
	_ = applyCaddyfile(ctx, cfg)
	if err := WriteStatus(proxyDir, cfg.Listen); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Down(ctx context.Context, proxyDir string) error {
	if _, err := os.Stat(filepath.Join(proxyDir, ComposeFileName)); os.IsNotExist(err) {
		ClearStatus(proxyDir)
		return nil
	}
	err := compose.Run(ctx, proxyDir, composeArgs("down"))
	ClearStatus(proxyDir)
	return err
}

func Reload(ctx context.Context, proxyDir string, st *state.File) (Config, error) {
	prev, _ := os.ReadFile(filepath.Join(proxyDir, ComposeFileName))
	cfg, err := writeProxyFiles(ctx, proxyDir, st)
	if err != nil {
		return Config{}, err
	}
	if !InspectRunning(ctx) {
		return cfg, fmt.Errorf("proxy is not running — start it with siding proxy up")
	}
	next, _ := os.ReadFile(cfg.ComposePath)
	if composePublishChanged(prev, next) {
		if err := compose.Run(ctx, proxyDir, composeUpArgs()); err != nil {
			return cfg, err
		}
		if err := waitReady(ctx, cfg); err != nil {
			return cfg, err
		}
	}
	if err := applyCaddyfile(ctx, cfg); err != nil {
		reload := exec.CommandContext(ctx, "docker", "exec", ContainerName, "caddy", "reload", "--config", "/etc/caddy/Caddyfile")
		if execErr := reload.Run(); execErr != nil {
			return cfg, fmt.Errorf("caddy admin reload: %v; exec reload: %w", err, execErr)
		}
	}
	if err := WriteStatus(proxyDir, cfg.Listen); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Ps(ctx context.Context, proxyDir string) error {
	return compose.Run(ctx, proxyDir, composeArgs("ps"))
}

// RefreshIfRunning rewrites routes when the proxy container is up.
// No-op if Caddy is not running. Used after siding up/down.
func RefreshIfRunning(ctx context.Context, proxyDir string, st *state.File) error {
	if !InspectRunning(ctx) {
		ClearStatus(proxyDir)
		return nil
	}
	_, err := Reload(ctx, proxyDir, st)
	return err
}

func waitReady(ctx context.Context, cfg Config) error {
	err := waitListen(ctx, cfg.Listen, 8*time.Second)
	if err == nil {
		return nil
	}
	if InspectRunning(ctx) {
		return fmt.Errorf("caddy is running but not answering on 127.0.0.1:%d: %w", cfg.Listen, err)
	}
	if logs := containerLogs(ctx); logs != "" {
		return fmt.Errorf("caddy did not start on 127.0.0.1:%d: %w\n%s", cfg.Listen, err, logs)
	}
	return fmt.Errorf("caddy did not start on 127.0.0.1:%d: %w", cfg.Listen, err)
}

func containerLogs(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "20", ContainerName)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	return strings.TrimSpace(out.String())
}

func applyCaddyfile(ctx context.Context, cfg Config) error {
	caddyfile, err := os.ReadFile(cfg.CaddyfilePath)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	var last error
	for {
		if AdminReachable(ctx) {
			if err := LoadCaddyfile(ctx, string(caddyfile)); err == nil {
				return nil
			} else {
				last = err
			}
		}
		if time.Now().After(deadline) {
			if last != nil {
				return last
			}
			return fmt.Errorf("caddy admin not reachable on %s", AdminURL())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func writeProxyFiles(ctx context.Context, proxyDir string, st *state.File) (Config, error) {
	return WriteFilesSkip(proxyDir, st, busyExtraPublishes(ctx, state.ListenPort(st)))
}

// busyExtraPublishes omits the admin publish when another container already
// holds that host port. The HTTP listen itself is never skipped.
func busyExtraPublishes(ctx context.Context, listen int) map[int]struct{} {
	snap, err := dockerps.Inspect(ctx)
	if err != nil {
		return nil
	}
	taken := snap.TakenExcept(ContainerName)
	skip := map[int]struct{}{}
	for _, p := range HTTPListenPorts(listen) {
		if p == listen {
			continue
		}
		if _, ok := taken[p]; ok {
			skip[p] = struct{}{}
		}
	}
	if _, ok := taken[state.ProxyAdminListen]; ok {
		skip[state.ProxyAdminListen] = struct{}{}
	}
	if len(skip) == 0 {
		return nil
	}
	return skip
}
