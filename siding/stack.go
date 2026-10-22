package siding

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/gitinfo"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

// UpOptions start the stack for a checkout (siding up).
type UpOptions struct {
	Target
	Compose ComposeOptions
	Extra   []string // compose args after --
}

// DownOptions stop the stack for a checkout (siding down).
type DownOptions struct {
	Target
	Compose ComposeOptions
	Extra   []string
}

// Up writes slot env, claims the lock, and runs docker compose up -d.
func (c *Client) Up(ctx context.Context, opts UpOptions) (StackResult, error) {
	return c.runStack(ctx, "up", opts.Target, opts.Extra, opts.Compose)
}

// Down runs docker compose down and releases the slot lock.
func (c *Client) Down(ctx context.Context, opts DownOptions) (StackResult, error) {
	return c.runStack(ctx, "down", opts.Target, opts.Extra, opts.Compose)
}

func (c *Client) runStack(ctx context.Context, action string, target Target, extra []string, composeOpts ComposeOptions) (StackResult, error) {
	res, s, err := c.resolve(target)
	if err != nil {
		return StackResult{}, err
	}
	s.ctx = ctx

	if action == "up" {
		if _, _, err := ensureAllocation(ctx, res, s.st); err != nil {
			return StackResult{}, err
		}
		if err := proxy.RequireForUp(ctx, state.ListenPort(s.st)); err != nil {
			return StackResult{}, err
		}
	}

	attachProxy(ctx, res, s.st)

	if err := resolver.WriteSlotEnv(res); err != nil {
		return StackResult{}, err
	}

	newlyClaimed := false
	if action == "up" {
		newly, err := state.ClaimLock(s.st, res.Contract.Project, res.StackKey, res.Mode, res.RepoRoot, res.ComposeProject)
		if err != nil {
			return StackResult{}, err
		}
		newlyClaimed = newly
		if err := s.write(); err != nil {
			return StackResult{}, err
		}
	}

	dockerArgs, err := compose.DockerArgs(res, action, extra, composeOpts.internal())
	if err != nil {
		return StackResult{}, err
	}
	runErr := compose.Run(ctx, res.RepoRoot, dockerArgs)
	if runErr != nil {
		if action == "up" && newlyClaimed {
			state.ReleaseLock(s.st, res.Contract.Project, res.StackKey, res.Mode, res.RepoRoot)
			_ = s.write()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return StackResult{}, fmt.Errorf("docker compose %s exited %d", action, exitErr.ExitCode())
		}
		return StackResult{}, runErr
	}

	if action == "down" {
		state.ReleaseLock(s.st, res.Contract.Project, res.StackKey, res.Mode, res.RepoRoot)
		if err := s.write(); err != nil {
			return StackResult{}, err
		}
	}

	if action == "up" {
		branch, sha := gitinfo.Snapshot(ctx, res.RepoRoot)
		state.StampLastUp(s.st, res.CheckoutKey, res.Mode, branch, sha, time.Now().UTC().Format(time.RFC3339))
		if err := s.write(); err != nil {
			return StackResult{}, err
		}
	}

	if action == "up" || action == "down" {
		_ = proxy.RefreshIfRunning(ctx, s.paths.ProxyDir, s.st)
	}

	out := fromResult(res)
	return StackResult{
		Resolved:    out,
		PublicURL:   out.PublicURL,
		ProxyActive: out.ProxyActive,
	}, nil
}
