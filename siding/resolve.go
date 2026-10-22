package siding

import (
	"context"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

// Resolve maps a checkout to its slot without writing env or state.
func (c *Client) Resolve(ctx context.Context, target Target) (Resolved, error) {
	res, s, err := c.resolve(target)
	if err != nil {
		return Resolved{}, err
	}
	_ = s
	attachProxy(ctx, res, s.st)
	return fromResult(res), nil
}

// Plan resolves slot env and compose metadata without starting containers.
func (c *Client) Plan(ctx context.Context, target Target) (Plan, error) {
	start := target.StartPath
	if start == "" {
		var err error
		start, err = paths.Abs("")
		if err != nil {
			return Plan{}, err
		}
	}
	res, err := resolver.Resolve(resolver.Options{
		StartPath:     start,
		StateOverride: c.StateRoot,
		Stack:         target.Stack,
		Mode:          target.Mode,
	})
	if err != nil {
		return Plan{}, err
	}
	if _, _, err := contract.SelectComposeEnvPaths(res.RepoRoot, res.ModeConfig.ComposeEnvFiles); err != nil {
		return Plan{}, err
	}
	statePaths, _ := paths.ResolvePaths(c.StateRoot)
	advice := advise.ScanRepo(res.RepoRoot, res.ComposeFiles, res.Contract, res.Mode)
	return Plan{
		Resolved:     fromResult(res),
		StartPath:    start,
		UserDataRoot: statePaths.UserDataRoot,
		SlotEnv:      resolver.BuildSlotEnv(res),
		Tips:         advice.Tips,
	}, nil
}

func attachProxy(ctx context.Context, res *resolver.Result, st *state.File) {
	if !proxy.InspectRunning(ctx) {
		return
	}
	res.ProxyActive = true
	res.ProxyListen = state.ListenPort(st)
}

func publicURL(res *resolver.Result) string {
	return proxy.PublicURL(contract.PublicHost(res.Host, "web", res.HTTPHosts), res.Ports, res.ProxyActive, res.ProxyListen)
}
