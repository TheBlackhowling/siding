package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBlackhowling/siding/internal/state"
)

func sampleState() *state.File {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/acme-betty", "acme", "betty", "test", state.ModeAllocation{
		Host:           "betty.acme.test",
		ComposeProject: "acme-betty-test",
		Ports:          map[string]int{"web": 3010, "api": 8010, "redis": 6379},
	})
	state.PutCheckoutMode(f, "c:/source/siding/examples/hello", "hello", "hello", "test", state.ModeAllocation{
		Host:           "hello.test.localhost",
		ComposeProject: "hello-test",
		Ports:          map[string]int{"web": 18080},
	})
	return f
}

func TestCollectRoutesAliases(t *testing.T) {
	routes := CollectRoutes(sampleState())
	got := map[string]int{}
	for _, r := range routes {
		got[r.Host] = r.Upstream
	}
	want := map[string]int{
		"hello.test.localhost":          18080,
		"betty.acme.test.localhost":     3010,
		"api.betty.acme.test.localhost": 8010,
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d got %#v", len(got), got)
	}
	for host, port := range want {
		if got[host] != port {
			t.Fatalf("%s = %d want %d", host, got[host], port)
		}
	}
	if _, ok := got["redis.betty.acme.test.localhost"]; ok {
		t.Fatal("redis is TCP — must not get an HTTP Caddy site")
	}
	if _, ok := got["app.hello.test.localhost"]; ok {
		t.Fatal("app. is not implied — only recipe http_hosts")
	}
}

func TestCollectRoutesPrefixedHost(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/acme-genie", "acme-genie", "genie", "test", state.ModeAllocation{
		Host:      "acme.genie.test",
		Ports:     map[string]int{"web": 3000, "api": 8000},
		HTTPHosts: map[string][]string{"web": {"app"}},
	})
	got := map[string]int{}
	for _, r := range CollectRoutes(f) {
		got[r.Host] = r.Upstream
	}
	if got["acme.genie.test.localhost"] != 3000 {
		t.Fatalf("root %#v", got)
	}
	if got["app.acme.genie.test.localhost"] != 3000 {
		t.Fatalf("app. web %#v", got)
	}
	if got["api.acme.genie.test.localhost"] != 8000 {
		t.Fatalf("api sibling %#v", got)
	}
	if _, ok := got["app.app.acme.genie.test.localhost"]; ok {
		t.Fatal("must not double app.")
	}
}

func TestCollectRoutesEntryPoints(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/app", "app", "shop", "test", state.ModeAllocation{
		Host:        "example.shop.test",
		Ports:       map[string]int{"web": 3000, "api": 8000},
		EntryPoints: map[string][]string{"web": {"app"}, "api": {"api"}},
	})
	got := map[string]int{}
	for _, r := range CollectRoutes(f) {
		got[r.Host] = r.Upstream
	}
	if got["app.example.shop.test.localhost"] != 3000 {
		t.Fatalf("app entry %#v", got)
	}
	if got["api.example.shop.test.localhost"] != 8000 {
		t.Fatalf("api entry %#v", got)
	}
}

func TestCollectRoutesHTTPHosts(t *testing.T) {
	f := sampleState()
	rec := f.Checkouts["c:/source/siding/examples/hello"]
	alloc := rec.Modes["test"]
	alloc.HTTPHosts = map[string][]string{"web": {"www"}}
	rec.Modes["test"] = alloc
	f.Checkouts["c:/source/siding/examples/hello"] = rec
	routes := CollectRoutes(f)
	found := false
	for _, r := range routes {
		if r.Host == "www.hello.test.localhost" && r.Upstream == 18080 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected www. from http_hosts")
	}
}

func TestGenerateCaddyfileHTTPPort(t *testing.T) {
	caddy := GenerateCaddyfile([]Route{{Host: "hello.test.localhost", Upstream: 18080}}, 1355)
	if !strings.Contains(caddy, "http_port 1355") {
		t.Fatalf("missing http_port rewrite: %s", caddy)
	}
	if !strings.Contains(caddy, "admin 0.0.0.0:2019") {
		t.Fatalf("missing admin api: %s", caddy)
	}
	if !strings.Contains(caddy, "host hello.test.localhost") {
		t.Fatalf("stack host matcher missing: %s", caddy)
	}
	if strings.Contains(caddy, "hello.test.localhost:1355") {
		t.Fatalf("site address should not include :1355 when http_port is set: %s", caddy)
	}
	if n := strings.Count(caddy, ":1355 {"); n != 1 {
		t.Fatalf("exactly one listen site, got %d: %s", n, caddy)
	}
	if strings.Contains(caddy, "\nhttp://") || strings.Contains(caddy, "host_regexp") {
		t.Fatalf("no per-host site blocks and no host_regexp (stock Caddy lacks it): %s", caddy)
	}
	if !strings.Contains(caddy, "reverse_proxy host.docker.internal:18080") {
		t.Fatalf("missing upstream: %s", caddy)
	}
	if strings.Contains(caddy, "redir http://siding.localhost") {
		t.Fatalf("must not redirect unknown/stack Hosts to the board: %s", caddy)
	}
}

func TestGenerateCaddyfilePrefixedGenie(t *testing.T) {
	caddy := GenerateCaddyfile([]Route{
		{Host: "acme.genie.test.localhost", Upstream: 3000, PortKey: "web"},
		{Host: "app.acme.genie.test.localhost", Upstream: 3000, PortKey: "web"},
		{Host: "api.acme.genie.test.localhost", Upstream: 8000, PortKey: "api"},
	}, 80)
	apiAt := strings.Index(caddy, "host api.acme.genie.test.localhost")
	webAt := strings.Index(caddy, "*.acme.genie.test.localhost")
	if apiAt < 0 || webAt < 0 || apiAt > webAt {
		t.Fatalf("api matcher must come before web wildcard: %s", caddy)
	}
	if !strings.Contains(caddy, "app.acme.genie.test.localhost") {
		t.Fatalf("missing app host: %s", caddy)
	}
	if strings.Contains(caddy, "app.app.acme") {
		t.Fatal("doubled app.")
	}
	if !strings.Contains(caddy, "@doubled header_regexp doubled Host") || !strings.Contains(caddy, "redir http://{http.regexp.doubled.1}{uri} 308") {
		t.Fatalf("must strip app.app. back to app.: %s", caddy)
	}
}

func TestGenerateCaddyfileEmpty(t *testing.T) {
	caddy := GenerateCaddyfile(nil, 1355)
	if strings.Contains(caddy, "redir http://siding.localhost") {
		t.Fatalf("unknown Hosts must not redirect to the board: %s", caddy)
	}
	if !strings.Contains(caddy, "No siding stack for this Host") {
		t.Fatalf("unknown Host should 404: %s", caddy)
	}
	if !strings.Contains(caddy, `<a href="http://siding.localhost:1355">Open the siding board</a>`) {
		t.Fatalf("board link should be clickable: %s", caddy)
	}
	if !strings.Contains(caddy, "http_port 1355") {
		t.Fatalf("got %s", caddy)
	}
}

func TestPublicURL(t *testing.T) {
	if p := HTTPListenPorts(0); len(p) != 1 || p[0] != 80 {
		t.Fatalf("default listen %v", p)
	}
	if p := HTTPListenPorts(8080); len(p) != 1 || p[0] != 8080 {
		t.Fatalf("explicit listen %v", p)
	}
	ports := map[string]int{"web": 18080}
	if got := PublicURL("", nil, false, 80); got != "" {
		t.Fatalf("empty host %q", got)
	}
	if got := PublicURL("h.localhost", nil, false, 80); got != "http://h.localhost" {
		t.Fatalf("no web port %q", got)
	}
	if got := PublicURL("hello.test.localhost", ports, false, 80); got != "http://hello.test.localhost:18080" {
		t.Fatalf("direct = %q", got)
	}
	if got := PublicURL("hello.test.localhost", ports, true, 80); got != "http://hello.test.localhost" {
		t.Fatalf("proxy80 = %q", got)
	}
	if got := PublicURL("hello.test.localhost", ports, true, 1355); got != "http://hello.test.localhost:1355" {
		t.Fatalf("proxy1355 = %q", got)
	}
}

func TestWriteFiles(t *testing.T) {
	dir := t.TempDir()
	cfg, err := WriteFiles(dir, sampleState())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg.CaddyfilePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "host hello.test.localhost") {
		t.Fatalf("caddyfile: %s", body)
	}
	// redis (6379) is a TCP port key in sampleState: no site, no upstream.
	if strings.Contains(body, "6379") || strings.Contains(body, "redis.") {
		t.Fatalf("TCP keys must not be proxied: %s", body)
	}
	if !strings.Contains(body, "host api.betty.acme.test.localhost") {
		t.Fatalf("missing api route: %s", body)
	}
	composeBody, err := os.ReadFile(filepath.Join(dir, ComposeFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(composeBody), "127.0.0.1:2019:2019") {
		t.Fatalf("missing admin publish: %s", composeBody)
	}
	if !strings.Contains(string(composeBody), "127.0.0.1:80:80") {
		t.Fatalf("missing default :80 publish: %s", composeBody)
	}
	// Only the listen and admin ports are published; stack ports stay on the host.
	if n := strings.Count(string(composeBody), "127.0.0.1:"); n != 2 {
		t.Fatalf("expected exactly 2 publishes, got %d: %s", n, composeBody)
	}
	if !strings.Contains(string(composeBody), ProxyImage) {
		t.Fatalf("missing published image: %s", composeBody)
	}
	if !strings.Contains(string(composeBody), "pull_policy: always") {
		t.Fatalf("missing pull always: %s", composeBody)
	}
	if strings.Contains(string(composeBody), "./board:/srv/board") {
		t.Fatalf("must not overlay the baked SPA: %s", composeBody)
	}
	if !strings.Contains(string(composeBody), "./board/status.json:/srv/board/status.json") {
		t.Fatalf("missing board status mount: %s", composeBody)
	}
	board, err := os.ReadFile(filepath.Join(dir, "board", StatusFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(board), "hello.test.localhost") || !strings.Contains(string(board), "betty") {
		t.Fatalf("board: %s", board)
	}
}

func TestGenerateCaddyfileBoard(t *testing.T) {
	caddy := GenerateCaddyfile(nil, 80)
	if !strings.Contains(caddy, "@board host siding.localhost") {
		t.Fatalf("missing board host matcher: %s", caddy)
	}
	if !strings.Contains(caddy, "root * /srv/board") {
		t.Fatalf("missing board root: %s", caddy)
	}
	if !strings.Contains(caddy, "try_files {path} /index.html") {
		t.Fatalf("missing spa try_files: %s", caddy)
	}
	if strings.Contains(caddy, "redir http://siding.localhost") {
		t.Fatalf("board must not steal other Hosts via redirect: %s", caddy)
	}
	if !strings.Contains(caddy, `<a href="http://siding.localhost">Open the siding board</a>`) {
		t.Fatalf("board link should be clickable: %s", caddy)
	}
}

func TestGenerateComposeSkipBusyAdmin(t *testing.T) {
	body := GenerateComposeSkip(1355, map[int]struct{}{2019: {}})
	if strings.Contains(body, "127.0.0.1:2019:2019") {
		t.Fatalf("should omit busy admin: %s", body)
	}
	if !strings.Contains(body, "127.0.0.1:1355:1355") {
		t.Fatalf("must still publish http_port: %s", body)
	}
}

func TestGenerateCaddyfileIgnoresInvalidStateHost(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/hello", "hello", "hello", "test", state.ModeAllocation{
		Host:  "hello.test\n  handle {\n    respond \"pwned\" 200\n  }\n  @z host z",
		Ports: map[string]int{"web": 18080},
	})
	caddy := GenerateCaddyfile(CollectRoutes(f), 80)
	if strings.Contains(caddy, `respond "pwned" 200`) {
		t.Fatalf("injection landed in Caddyfile:\n%s", caddy)
	}
}
