package resolver

import (
	"testing"

	"github.com/TheBlackhowling/siding/internal/state"
)

func TestApplyAllocationAndSeed(t *testing.T) {
	r := &Result{
		Host:           "old.localhost",
		ComposeProject: "old",
		Ports:          map[string]int{"web": 1},
	}
	r.ApplyAllocation(state.ModeAllocation{
		Host:           "app.acme.genie.test.localhost",
		ComposeProject: "proj",
		Ports:          map[string]int{"web": 18010, "api": 23010},
		HTTPHosts:      map[string][]string{"web": {"app"}},
		EntryPoints:    map[string][]string{"web": {"app"}},
	})
	if r.Host != "acme.genie.test.localhost" {
		t.Fatalf("root host %q", r.Host)
	}
	if r.ComposeProject != "proj" || r.Ports["api"] != 23010 {
		t.Fatalf("%q %#v", r.ComposeProject, r.Ports)
	}
	if r.HTTPHosts["web"][0] != "app" || r.EntryPoints["web"][0] != "app" {
		t.Fatalf("hosts %#v entries %#v", r.HTTPHosts, r.EntryPoints)
	}
	// Without a contract the seed is the plan itself: the root host, the
	// applied ports (ModeConfig has none), and the applied hosts/entries.
	seed := r.SeedAllocation()
	if seed.Host != "acme.genie.test.localhost" || seed.ComposeProject != "proj" {
		t.Fatalf("seed host/project %q %q", seed.Host, seed.ComposeProject)
	}
	if len(seed.Ports) != 2 || seed.Ports["web"] != 18010 || seed.Ports["api"] != 23010 {
		t.Fatalf("seed ports %#v", seed.Ports)
	}
	if seed.HTTPHosts["web"][0] != "app" || seed.EntryPoints["web"][0] != "app" {
		t.Fatalf("seed hosts %#v entries %#v", seed.HTTPHosts, seed.EntryPoints)
	}

	// Empty fields in the allocation leave the plan alone.
	before := *r
	r.ApplyAllocation(state.ModeAllocation{})
	if r.Host != before.Host || r.ComposeProject != before.ComposeProject || len(r.Ports) != 2 {
		t.Fatalf("empty allocation changed the plan: %#v", r)
	}
}
