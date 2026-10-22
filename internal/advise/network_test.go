package advise

import (
	"strings"
	"testing"
)

func TestAnalyzeImplicitDefaultWithAliases(t *testing.T) {
	in := `
services:
  backend:
    image: x
  web:
    networks:
      default:
        aliases:
          - app.local
`
	plan := AnalyzeNetworks([]byte(in))
	if plan.NeedsEnsure {
		t.Fatalf("implicit default should already be shared: %#v", plan)
	}
	if plan.Shared != "default" {
		t.Fatalf("shared=%q", plan.Shared)
	}
}

func TestMergeNetworkPlansKeepsBaseServices(t *testing.T) {
	base := AnalyzeNetworks([]byte(`
services:
  web:
    image: x
  api:
    image: x
`))
	over := AnalyzeNetworks([]byte(`
services:
  api:
    image: x
    networks:
      - extra
`))
	got := MergeNetworkPlans(base, over)
	names := map[string]bool{}
	for _, s := range got.Services {
		names[s.Name] = true
	}
	if !names["web"] || !names["api"] {
		t.Fatalf("override must not drop base services: %#v", got.Services)
	}
}

func TestEnsureSharedNetworkIsolated(t *testing.T) {
	in := `
services:
  frontend:
    image: x
    networks:
      - frontend
  backend:
    image: x
    networks:
      - backend
`
	out, changes := EnsureSharedNetwork(in)
	if len(changes) == 0 {
		t.Fatal("expected network changes")
	}
	if !strings.Contains(out, "siding:") {
		t.Fatalf("missing siding network: %s", out)
	}
	if !strings.Contains(out, "- frontend") || !strings.Contains(out, "- backend") {
		t.Fatalf("should keep existing networks: %s", out)
	}
	if strings.Count(out, "- siding") < 2 {
		t.Fatalf("both services should join siding: %s", out)
	}
}

func TestEnsureHostNetworkCommented(t *testing.T) {
	in := `
services:
  web:
    network_mode: host
    image: x
  backend:
    image: x
`
	out, changes := EnsureSharedNetwork(in)
	if len(changes) == 0 {
		t.Fatal("expected host-mode change")
	}
	if strings.Contains(out, "\n    network_mode: host") {
		t.Fatalf("host mode should be commented: %s", out)
	}
	if !strings.Contains(out, "# network_mode: host") {
		t.Fatalf("expected comment: %s", out)
	}
}

func TestEnsureSkipsHealthyDefault(t *testing.T) {
	in := `
services:
  web:
    image: x
  backend:
    image: x
`
	out, changes := EnsureSharedNetwork(in)
	if len(changes) != 0 {
		t.Fatalf("implicit default needs no rewrite: %v\n%s", changes, out)
	}
}
