package hostport

import (
	"context"
	"fmt"
	"testing"

	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestLookupWithoutHostTools(t *testing.T) {
	testenv.NoHostTools(t)
	if len(Lookup(context.Background(), 0).Occupants) != 0 {
		t.Fatal("invalid port")
	}
	rep := Lookup(context.Background(), 80)
	if rep.Port != 80 || len(rep.Occupants) != 0 {
		t.Fatalf("no docker/netstat/ss means no occupants: %#v", rep)
	}
	if got := rep.Blocking(); got != nil {
		t.Fatalf("%#v", got)
	}
	if _, err := run(context.Background(), "definitely-not-a-siding-binary"); err == nil {
		t.Fatal("expected missing binary")
	}
}

func TestFormatAndDedupe(t *testing.T) {
	lines := FormatOccupants([]Occupant{
		{Source: "docker", Name: "caddy"},
		{Source: "host", Name: "nginx", PID: 9},
		{Source: "host", PID: 8},
		{Source: "windows", Name: "excluded port range", Detail: "TCP 80-80"},
		{Source: "windows", Name: "reserved"},
		{Source: "host"},
	})
	if len(lines) != 6 {
		t.Fatalf("%#v", lines)
	}

	dup := dedupe([]Occupant{
		{Source: "host", Name: "a", PID: 1},
		{Source: "host", Name: "a", PID: 1},
		{Source: "docker", Name: "b"},
	})
	if len(dup) != 2 {
		t.Fatalf("%#v", dup)
	}
}

// Every tool failing (missing binary, access denied, timeout) must read as
// "nothing found", never as an error or a phantom occupant.
func TestOSOccupantsWhenToolsFail(t *testing.T) {
	t.Cleanup(func() { cmdRun = execRun })
	cmdRun = func(_ context.Context, name string, _ ...string) (string, error) {
		return "", fmt.Errorf("%s: not available", name)
	}
	ctx := context.Background()
	if got := osOccupants(ctx, 80); got != nil {
		t.Fatalf("%#v", got)
	}
	if got := windowsOccupants(ctx, 80); got != nil {
		t.Fatalf("%#v", got)
	}
	if got := unixOccupants(ctx, 80); got != nil {
		t.Fatalf("%#v", got)
	}
	if got := windowsExcluded(ctx, 80); got != nil {
		t.Fatalf("%#v", got)
	}
	if name := windowsProcessName(ctx, 1); name != "" {
		t.Fatalf("%q", name)
	}
}
