package siding

import (
	"context"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

// SyncOptions map the org recipe onto local siding state (siding sync).
type SyncOptions struct {
	Target
	AllModes bool // when true, sync every recipe mode (default when Mode unchanged)
	DryRun   bool
}

// Sync allocates or merges ports, writes slot env, and reloads the proxy if running.
func (c *Client) Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	res, s, err := c.resolve(opts.Target)
	if err != nil {
		return SyncResult{}, err
	}
	s.ctx = ctx

	names := []string{res.Mode}
	if opts.AllModes {
		if all := contract.ModeNames(res.Contract); len(all) > 0 {
			names = all
		}
	}

	snap := inspectDockerPorts(ctx)
	var modes []SyncMode
	for _, m := range names {
		cur := res
		if m != res.Mode {
			cur, err = c.resolveWithSession(ctx, s, Target{
				StartPath: opts.StartPath,
				Stack:     opts.Stack,
				Mode:      m,
			}, s.st)
			if err != nil {
				return SyncResult{}, err
			}
		}
		rec, recErr := state.ReconcileModeAllocation(
			s.st,
			cur.CheckoutKey,
			cur.Contract.Project,
			cur.StackKey,
			cur.Mode,
			cur.RecipeSeed(),
			snap.PortSet(),
		)
		if recErr != nil {
			return SyncResult{}, recErr
		}
		cur.ApplyAllocation(rec.Alloc)
		attachProxy(ctx, cur, s.st)
		if !opts.DryRun {
			if err := resolver.WriteSlotEnv(cur); err != nil {
				return SyncResult{}, err
			}
		}
		advice := advise.ScanRepo(cur.RepoRoot, cur.ComposeFiles, cur.Contract, cur.Mode)
		modes = append(modes, SyncMode{
			Resolved:    fromResult(cur),
			SlotEnv:     resolver.BuildSlotEnv(cur),
			AddedPorts:  rec.AddedPorts,
			Created:     rec.Created,
			ProxyActive: cur.ProxyActive,
			ProxyListen: cur.ProxyListen,
			Tips:        advice.Tips,
		})
	}

	if !opts.DryRun {
		if err := s.write(); err != nil {
			return SyncResult{}, err
		}
		_ = proxyRefresh(ctx, s)
	}

	return SyncResult{Modes: modes, DryRun: opts.DryRun}, nil
}

func ensureAllocation(ctx context.Context, res *resolver.Result, st *state.File) (state.ModeAllocation, bool, error) {
	live := liveDockerPorts(ctx)
	alloc, created, err := state.EnsureModeAllocation(
		st,
		res.CheckoutKey,
		res.Contract.Project,
		res.StackKey,
		res.Mode,
		res.SeedAllocation(),
		live,
	)
	if err != nil {
		return state.ModeAllocation{}, false, err
	}
	res.ApplyAllocation(alloc)
	return alloc, created, nil
}

func liveDockerPorts(ctx context.Context) map[int]struct{} {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := dockerps.Inspect(ctx)
	if err != nil {
		return map[int]struct{}{}
	}
	return snap.PortSet()
}

func inspectDockerPorts(ctx context.Context) dockerps.Snapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := dockerps.Inspect(ctx)
	if err != nil {
		return dockerps.Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}, Unavailable: err.Error()}
	}
	return snap
}

func proxyRefresh(ctx context.Context, s *session) error {
	return proxyRefreshDir(ctx, s.paths.ProxyDir, s.st)
}
