package siding

import (
	"context"

	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

// Client embeds siding against a machine state root (SIDING_ROOT / --root).
// Zero value uses the default user data directory.
type Client struct {
	StateRoot string
}

// New returns a client. StateRoot overrides the default state.json location.
func New(stateRoot string) *Client {
	return &Client{StateRoot: stateRoot}
}

type session struct {
	ctx   context.Context
	paths paths.Paths
	store *state.Store
	st    *state.File
}

func (c *Client) session(ctx context.Context) (*session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	p, err := paths.ResolvePaths(c.StateRoot)
	if err != nil {
		return nil, err
	}
	store := state.NewStore(p)
	st, err := store.Read()
	if err != nil {
		return nil, err
	}
	if state.MigrateOffDefaultPorts(st) {
		if err := store.Write(st); err != nil {
			return nil, err
		}
	}
	return &session{ctx: ctx, paths: p, store: store, st: st}, nil
}

func (c *Client) resolve(target Target) (*resolver.Result, *session, error) {
	s, err := c.session(context.Background())
	if err != nil {
		return nil, nil, err
	}
	res, err := resolver.Resolve(resolver.Options{
		StartPath:     target.StartPath,
		StateOverride: c.StateRoot,
		Stack:         target.Stack,
		Mode:          target.Mode,
	})
	if err != nil {
		return nil, nil, err
	}
	return res, s, nil
}

func (c *Client) resolveWithSession(ctx context.Context, s *session, target Target, loaded *state.File) (*resolver.Result, error) {
	res, err := resolver.Resolve(resolver.Options{
		StartPath:     target.StartPath,
		StateOverride: c.StateRoot,
		Stack:         target.Stack,
		Mode:          target.Mode,
		Loaded:        loaded,
	})
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx = ctx
	return res, nil
}

func (s *session) write() error {
	return s.store.Write(s.st)
}
