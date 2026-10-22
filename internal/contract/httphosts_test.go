package contract

import "testing"

func TestPublicHostAndNormalize(t *testing.T) {
	if NormalizeHTTPLabel("App.acme.test") != "app" {
		t.Fatal("label from host")
	}
	if NormalizeHTTPLabel("web") != "web" {
		t.Fatal("web")
	}
	hosts := map[string][]string{"web": {"app", "web", "app"}}
	got := NormalizeHTTPLabels("web", hosts["web"])
	if len(got) != 1 || got[0] != "app" {
		t.Fatalf("normalize extra only: %#v", got)
	}
	if PublicHost("hello.test", "web", nil) != "hello.test.localhost" {
		t.Fatalf("apex web %q", PublicHost("hello.test", "web", nil))
	}
	if PublicHost("hello.test", "web", map[string][]string{"web": {"www"}}) != "www.hello.test.localhost" {
		t.Fatal("www web")
	}
	if PublicHost("hello.test", "api", nil) != "api.hello.test.localhost" {
		t.Fatal("api default")
	}
	if PublicHost("acme.genie.test", "web", map[string][]string{"web": {"app"}}) != "app.acme.genie.test.localhost" {
		t.Fatalf("app on root %q", PublicHost("acme.genie.test", "web", map[string][]string{"web": {"app"}}))
	}
	if PublicHost("app.acme.genie.test", "web", map[string][]string{"web": {"app"}}) != "app.acme.genie.test.localhost" {
		t.Fatalf("must not double app. %q", PublicHost("app.acme.genie.test", "web", map[string][]string{"web": {"app"}}))
	}
	if PublicHost("acme.genie.test", "api", nil) != "api.acme.genie.test.localhost" {
		t.Fatalf("prefixed api %q", PublicHost("acme.genie.test", "api", nil))
	}
	if RootHost("app.acme.genie.test.localhost") != "acme.genie.test.localhost" {
		t.Fatalf("root %q", RootHost("app.acme.genie.test.localhost"))
	}
	role, rest := SplitHostPrefix("app.acme")
	if role != "app" || rest != "acme" {
		t.Fatalf("split %q %q", role, rest)
	}
}

func TestHostPrefix(t *testing.T) {
	if got := NormalizeHostPrefix("App.acme"); got != "app.acme" {
		t.Fatalf("normalize %q", got)
	}
	if got := ParseHostPrefixInput("app.example.test.localhost"); got != "example" {
		t.Fatalf("pasted host %q", got)
	}
	if got := ParseHostPrefixInput("example"); got != "example" {
		t.Fatalf("product only %q", got)
	}
	if got := ParseHostPrefixInput("none"); got != "" {
		t.Fatalf("none %q", got)
	}
	if got := InferHostPrefixFromHostname("app.example.test"); got != "example" {
		t.Fatalf("infer %q", got)
	}
	if InferHostPrefixFromHostname("hello.test") != "" {
		t.Fatal("single leftover label is not a product prefix")
	}
	c := &Contract{
		Host:      DefaultHost,
		HTTPHosts: map[string][]string{"web": {"app"}},
		Modes: map[string]Mode{
			"test": {HTTPHosts: map[string][]string{"web": {"app"}, "api": {"graphql"}}},
		},
	}
	ApplyHostPrefix(c, "app.example")
	if c.HostPrefix != "example" || c.Host != PrefixedHost {
		t.Fatalf("apply %#v %#v", c.HostPrefix, c.Host)
	}
	if len(c.HTTPHosts["web"]) != 1 || c.HTTPHosts["web"][0] != "app" {
		t.Fatalf("role should be http_hosts.web: %#v", c.HTTPHosts)
	}
	if _, ok := c.Modes["test"].HTTPHosts["web"]; ok {
		t.Fatal("mode web extras should inherit contract role")
	}
	if c.Modes["test"].HTTPHosts["api"][0] != "graphql" {
		t.Fatal("api extras stay")
	}
}

func TestParseHTTPPrefixInput(t *testing.T) {
	if got := ParseHTTPPrefixInput("app.{host}, www"); len(got) != 2 || got[0] != "app" || got[1] != "www" {
		t.Fatalf("got %#v", got)
	}
	if got := ParseHTTPPrefixInput("none"); got != nil {
		t.Fatalf("none %#v", got)
	}
	if got := ParseHTTPPrefixInput("web"); len(got) != 0 {
		t.Fatalf("web is already a port-key site: %#v", got)
	}
}

func TestApplyHTTPPrefixes(t *testing.T) {
	c := &Contract{
		HTTPHosts: map[string][]string{"web": {"old"}},
		Modes: map[string]Mode{
			"test": {HTTPHosts: map[string][]string{"web": {"app"}, "api": {"graphql"}}},
		},
	}
	ApplyHTTPPrefixes(c, []string{"www"})
	if len(c.HTTPHosts["web"]) != 1 || c.HTTPHosts["web"][0] != "www" {
		t.Fatalf("contract %#v", c.HTTPHosts)
	}
	if _, ok := c.Modes["test"].HTTPHosts["web"]; ok {
		t.Fatal("mode web should inherit contract")
	}
	if c.Modes["test"].HTTPHosts["api"][0] != "graphql" {
		t.Fatal("api inference should stay")
	}
}

func TestResolveHTTPHostsModeWins(t *testing.T) {
	c := &Contract{HTTPHosts: map[string][]string{"web": {"app"}}}
	mode := Mode{HTTPHosts: map[string][]string{"web": {"www"}}}
	got := ResolveHTTPHosts(c, mode)
	if len(got["web"]) != 1 || got["web"][0] != "www" {
		t.Fatalf("got %#v", got)
	}
}
