package siding

import (
	"context"
)

// AddOptions pre-allocates globally unique ports for a checkout.
type AddOptions struct {
	Target
	DryRun bool
}

// Add records ports in state.json without starting containers.
func (c *Client) Add(ctx context.Context, opts AddOptions) (AddResult, error) {
	res, s, err := c.resolve(opts.Target)
	if err != nil {
		return AddResult{}, err
	}
	s.ctx = ctx
	_, created, err := ensureAllocation(ctx, res, s.st)
	if err != nil {
		return AddResult{}, err
	}
	if !opts.DryRun {
		if err := s.write(); err != nil {
			return AddResult{}, err
		}
	}
	return AddResult{
		Resolved: fromResult(res),
		Created:  created,
		DryRun:   opts.DryRun,
	}, nil
}
