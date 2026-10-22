package contract

import "testing"

func TestCanonicalHTTPEntryKey(t *testing.T) {
	key, ok := CanonicalHTTPEntryKey("frontend")
	if !ok || key != "web" {
		t.Fatalf("frontend → %q %v", key, ok)
	}
	key, ok = CanonicalHTTPEntryKey("backend")
	if !ok || key != "api" {
		t.Fatalf("backend → %q %v", key, ok)
	}
	if _, ok := CanonicalHTTPEntryKey("aliases"); ok {
		t.Fatal("aliases is not an HTTP entry")
	}
	for _, k := range []string{"WEB", "frontend", "backend", " api "} {
		if !IsHTTPEntryKey(k) {
			t.Fatalf("%q should be an HTTP entry key", k)
		}
	}
	for _, k := range []string{"redis", "aliases", ""} {
		if IsHTTPEntryKey(k) {
			t.Fatalf("%q should not be an HTTP entry key", k)
		}
	}
}

func TestParseEntryFlag(t *testing.T) {
	key, labels, err := ParseEntryFlag("web=app,www")
	if err != nil || key != "web" || len(labels) != 2 || labels[0] != "app" || labels[1] != "www" {
		t.Fatalf("got %q %#v %v", key, labels, err)
	}
	key, labels, err = ParseEntryFlag("frontend=app")
	if err != nil || key != "web" || len(labels) != 1 || labels[0] != "app" {
		t.Fatalf("frontend=app → %q %#v %v", key, labels, err)
	}
	if _, _, err := ParseEntryFlag("redis=cache"); err == nil {
		t.Fatal("redis is not an HTTP entry key")
	}
	if _, _, err := ParseEntryFlag("web"); err == nil {
		t.Fatal("missing =")
	}
}

func TestResolveEntryPointsDefaults(t *testing.T) {
	c := &Contract{HostPrefix: "example", HTTPHosts: map[string][]string{"web": {"app"}}}
	mode := Mode{}
	ports := map[string]int{"web": 3000, "api": 8000}
	got := ResolveEntryPoints(c, mode, ports)
	if len(got["web"]) != 1 || got["web"][0] != "app" {
		t.Fatalf("web %#v", got)
	}
	if len(got["api"]) != 1 || got["api"][0] != "api" {
		t.Fatalf("api %#v", got)
	}

	plain := ResolveEntryPoints(&Contract{}, Mode{}, ports)
	if plain["web"][0] != "web" || plain["api"][0] != "api" {
		t.Fatalf("plain %#v", plain)
	}
}

func TestApplyEntryPointsMirrorsWebHosts(t *testing.T) {
	c := &Contract{HTTPHosts: map[string][]string{"web": {"old"}}}
	ApplyEntryPoints(c, map[string][]string{"web": {"app", "www"}, "api": {"api"}})
	if len(c.EntryPoints["web"]) != 2 || c.EntryPoints["web"][0] != "app" {
		t.Fatalf("entries %#v", c.EntryPoints)
	}
	if len(c.HTTPHosts["web"]) != 2 || c.HTTPHosts["web"][0] != "app" || c.HTTPHosts["web"][1] != "www" {
		t.Fatalf("http_hosts %#v", c.HTTPHosts)
	}
	if _, ok := c.HTTPHosts["api"]; ok {
		t.Fatal("api is a port-key site, not an http_hosts extra")
	}
}

func TestEntryHost(t *testing.T) {
	root := "example.my-stack.test.localhost"
	if got := EntryHost(root, "app"); got != "app.example.my-stack.test.localhost" {
		t.Fatalf("app %q", got)
	}
	if got := EntryHost(root, "api"); got != "api.example.my-stack.test.localhost" {
		t.Fatalf("api %q", got)
	}
	if got := EntryHost(root, "web"); got != root {
		t.Fatalf("apex %q", got)
	}
	if got := EntryHost("app.example.my-stack.test.localhost", "app"); got != "app.example.my-stack.test.localhost" {
		t.Fatalf("must not double app. %q", got)
	}
}
