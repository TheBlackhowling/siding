package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/internal/testenv"
)

// emptyStateRoot keeps Resolve away from the developer's real state.json,
// whose allocation for examples/hello would otherwise override recipe ports.
func emptyStateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(paths.EnvRoot, root)
	t.Setenv(paths.EnvData, root)
	return root
}

func TestResolveHelloFixture(t *testing.T) {
	emptyStateRoot(t)
	res, err := Resolve(Options{
		StartPath: testenv.HelloRepo(t),
		Mode:      "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StackKey != "hello" {
		t.Fatalf("stack = %q", res.StackKey)
	}
	if res.Ports["web"] != 18080 {
		t.Fatalf("web port = %d", res.Ports["web"])
	}
	if res.CheckoutKey == "" {
		t.Fatal("expected checkout key")
	}
	env := BuildSlotEnv(res)
	if env["WEB_PORT"] != "18080" {
		t.Fatalf("WEB_PORT = %q", env["WEB_PORT"])
	}
	if env["SIDING_URL"] != "http://hello.test.localhost:18080" {
		t.Fatalf("SIDING_URL = %q", env["SIDING_URL"])
	}
	res.ProxyActive = true
	res.ProxyListen = 1355
	env = BuildSlotEnv(res)
	if env["SIDING_URL"] != "http://hello.test.localhost:1355" || env["SIDING_PROXY"] != "1" {
		t.Fatalf("proxy env SIDING_URL=%q SIDING_PROXY=%q", env["SIDING_URL"], env["SIDING_PROXY"])
	}
	res.ModeConfig.Env = map[string]string{
		"NEXT_PUBLIC_ROOT_DOMAIN": "{host}{proxy_suffix}",
	}
	if got := BuildSlotEnv(res)["NEXT_PUBLIC_ROOT_DOMAIN"]; got != "hello.test.localhost:1355" {
		t.Fatalf("expanded = %q", got)
	}
	res.ProxyListen = 80
	if got := BuildSlotEnv(res)["NEXT_PUBLIC_ROOT_DOMAIN"]; got != "hello.test.localhost" {
		t.Fatalf("listen80 expanded = %q", got)
	}
	res.HTTPHosts = map[string][]string{"web": {"www"}}
	res.ModeConfig.Env["DEV_FRONTEND_URL"] = "http://{web_host}{proxy_suffix}"
	res.ProxyActive = true
	if got := BuildSlotEnv(res)["DEV_FRONTEND_URL"]; got != "http://www.hello.test.localhost" {
		t.Fatalf("web_host expanded = %q", got)
	}
	res.ProxyActive = false
	if got := BuildSlotEnv(res)["NEXT_PUBLIC_ROOT_DOMAIN"]; got != "hello.test.localhost:18080" {
		t.Fatalf("no-proxy expanded = %q", got)
	}
}

// State.json allocations override recipe ports for the same checkout+mode,
// and only for that checkout+mode.
func TestResolveUsesStateAllocation(t *testing.T) {
	root := emptyStateRoot(t)
	hello := testenv.HelloRepo(t)
	key := paths.NormalizeLockPath(hello)

	f := state.NewFile()
	f.Checkouts[key] = state.Checkout{
		Project: "hello",
		Stack:   "hello",
		Modes: map[string]state.ModeAllocation{
			"test": {Host: "hello.test.localhost", ComposeProject: "hello-hello-test", Ports: map[string]int{"web": 23456}},
		},
	}
	p, err := paths.ResolvePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.NewStore(p).Write(f); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(Options{StartPath: hello, Mode: "test", StateOverride: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ports["web"] != 23456 {
		t.Fatalf("state allocation must win: %v", res.Ports)
	}
	if res.StateFile != p.StateFile {
		t.Fatalf("state file %q want %q", res.StateFile, p.StateFile)
	}

	// Loaded short-circuits the read; an in-memory file with no allocation
	// falls back to the recipe port.
	res, err = Resolve(Options{StartPath: hello, Mode: "test", StateOverride: root, Loaded: state.NewFile()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ports["web"] != 18080 {
		t.Fatalf("recipe port without allocation: %v", res.Ports)
	}
}

func TestPortSuffixUsesApiPortWhenProxyOff(t *testing.T) {
	res := &Result{
		Host:     "acme.genie.test.localhost",
		StackKey: "genie",
		Mode:     "test",
		Ports:    map[string]int{"web": 18010, "api": 23010},
		ModeConfig: contract.Mode{Env: map[string]string{
			"DEV_FRONTEND_URL": "http://{web_host}{web_suffix}",
			"DEV_API_URL":      "http://{api_host}{api_suffix}",
		}},
	}
	env := BuildSlotEnv(res)
	if env["DEV_FRONTEND_URL"] != "http://acme.genie.test.localhost:18010" {
		t.Fatalf("web=%q", env["DEV_FRONTEND_URL"])
	}
	if env["DEV_API_URL"] != "http://api.acme.genie.test.localhost:23010" {
		t.Fatalf("api must use api port, not web: %q", env["DEV_API_URL"])
	}
	if ProxySuffix(res) != ":18010" {
		t.Fatalf("proxy_suffix is web: %q", ProxySuffix(res))
	}
}

func TestBuildSlotEnvStripsAppFromRootDomain(t *testing.T) {
	res := &Result{
		Host:       "app.acme.genie.test.localhost",
		StackKey:   "genie",
		Mode:       "test",
		Ports:      map[string]int{"web": 3000},
		HTTPHosts:  map[string][]string{"web": {"app"}},
		ModeConfig: contract.Mode{Env: map[string]string{"NEXT_PUBLIC_ROOT_DOMAIN": "{host}{proxy_suffix}"}},
	}
	res.ProxyActive = true
	res.ProxyListen = 80
	env := BuildSlotEnv(res)
	if env["NEXT_PUBLIC_ROOT_DOMAIN"] != "acme.genie.test.localhost" {
		t.Fatalf("ROOT_DOMAIN must not include app. (else Next redirects to app.app.): %q", env["NEXT_PUBLIC_ROOT_DOMAIN"])
	}
	if env["SIDING_HOST"] != "acme.genie.test.localhost" {
		t.Fatalf("SIDING_HOST=%q", env["SIDING_HOST"])
	}
	if env["SIDING_URL"] != "http://app.acme.genie.test.localhost" {
		t.Fatalf("SIDING_URL=%q", env["SIDING_URL"])
	}
}

func TestRecipeSeedUsesContractDefaults(t *testing.T) {
	c := &contract.Contract{
		Project: "hello",
		Host:    "{stack}.{project}.{mode}",
		Modes: map[string]contract.Mode{
			"test": {ComposeFiles: []string{"docker-compose.yml"}, Ports: map[string]int{"web": 3000, "api": 8000}},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	mode, _ := c.Mode("test")
	res := &Result{
		Contract:       c,
		StackKey:       "hello",
		Mode:           "test",
		ModeConfig:     mode,
		Host:           "stale.localhost",
		ComposeProject: "stale",
		Ports:          map[string]int{"web": 3010},
	}
	seed := res.RecipeSeed()
	if seed.Host != "hello.test.localhost" {
		t.Fatalf("host=%q", seed.Host)
	}
	if seed.Ports["web"] != 3000 || seed.Ports["api"] != 8000 {
		t.Fatalf("ports=%#v", seed.Ports)
	}
}

func TestRecipeSeedHostPrefix(t *testing.T) {
	c := &contract.Contract{
		Project:    "acme-genie",
		HostPrefix: "app.acme",
		Host:       contract.DefaultHost,
		Modes: map[string]contract.Mode{
			"test": {ComposeFiles: []string{"docker-compose.yml"}, Ports: map[string]int{"web": 3000, "api": 8000}},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	mode, _ := c.Mode("test")
	res := &Result{Contract: c, StackKey: "genie", Mode: "test", ModeConfig: mode, Host: "genie.acme-genie.test.localhost"}
	seed := res.RecipeSeed()
	if seed.Host != "acme.genie.test.localhost" {
		t.Fatalf("root host=%q", seed.Host)
	}
	if contract.PublicHost(seed.Host, "web", seed.HTTPHosts) != "app.acme.genie.test.localhost" {
		t.Fatalf("web=%q hosts=%#v", contract.PublicHost(seed.Host, "web", seed.HTTPHosts), seed.HTTPHosts)
	}
	if contract.PublicHost(seed.Host, "api", seed.HTTPHosts) != "api.acme.genie.test.localhost" {
		t.Fatalf("api=%q", contract.PublicHost(seed.Host, "api", seed.HTTPHosts))
	}
	if seed.Ports["web"] != 3000 || seed.Ports["api"] != 8000 {
		t.Fatalf("ports=%#v", seed.Ports)
	}
	if len(seed.EntryPoints["web"]) != 1 || seed.EntryPoints["web"][0] != "app" {
		t.Fatalf("entry web %#v", seed.EntryPoints)
	}
	if len(seed.EntryPoints["api"]) != 1 || seed.EntryPoints["api"][0] != "api" {
		t.Fatalf("entry api %#v", seed.EntryPoints)
	}
}

func TestRecipeSeedCollapsesHyphenUnderscore(t *testing.T) {
	c := &contract.Contract{
		Project: "acme",
		Host:    "{stack}.{project}.{mode}",
		Modes: map[string]contract.Mode{
			"test": {
				ComposeFiles: []string{"docker-compose.yml"},
				Ports:        map[string]int{"gcs-emulator": 4443, "gcs_emulator": 4443, "web": 3000},
			},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	mode, _ := c.Mode("test")
	res := &Result{Contract: c, StackKey: "genie", Mode: "test", ModeConfig: mode}
	seed := res.RecipeSeed()
	if _, ok := seed.Ports["gcs-emulator"]; ok {
		t.Fatalf("hyphen leaked: %#v", seed.Ports)
	}
	if seed.Ports["gcs_emulator"] != 4443 || seed.Ports["web"] != 3000 || len(seed.Ports) != 2 {
		t.Fatalf("ports=%#v", seed.Ports)
	}
}

func TestRecipeSeedOwnsEveryScannedPublish(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte(`
services:
  web:
    ports:
      - "${WEB_PORT:-3000}:3000"
  redis:
    ports:
      - "${REDIS_PORT:-6379}:6379"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &contract.Contract{
		Project: "demo",
		Host:    "{stack}.{project}.{mode}",
		Modes: map[string]contract.Mode{
			"test": {
				ComposeFiles: []string{"docker-compose.yml"},
				Ports:        map[string]int{"web": 3000},
				PortEnv:      map[string]string{"web": "WEB_PORT"},
			},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	mode, _ := c.Mode("test")
	res := &Result{
		RepoRoot:     dir,
		Contract:     c,
		StackKey:     "demo",
		Mode:         "test",
		ModeConfig:   mode,
		ComposeFiles: []string{"docker-compose.yml"},
	}
	seed := res.RecipeSeed()
	if seed.Ports["web"] != 3000 {
		t.Fatalf("web=%#v", seed.Ports)
	}
	if seed.Ports["redis"] != 6379 {
		t.Fatalf("redis not owned from scan: %#v", seed.Ports)
	}
}

func TestBuildSlotEnvDefaultsPortName(t *testing.T) {
	res := &Result{
		Host:       "hello.test.localhost",
		StackKey:   "hello",
		Mode:       "test",
		Ports:      map[string]int{"web": 18080, "redis": 16379},
		ModeConfig: contract.Mode{},
	}
	env := BuildSlotEnv(res)
	if env["WEB_PORT"] != "18080" || env["REDIS_PORT"] != "16379" {
		t.Fatalf("env=%#v", env)
	}
}

func TestFormatEnvFileSorted(t *testing.T) {
	got := FormatEnvFile(map[string]string{"B": "2", "A": "1"})
	if !strings.HasPrefix(got, "# Generated by siding. Do not edit.\nA=1\nB=2\n") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatEnvFileQuotesValues(t *testing.T) {
	got := FormatEnvFile(map[string]string{"SIDING_CHECKOUT": `C:\my project\repo`})
	if !strings.Contains(got, `SIDING_CHECKOUT="C:\\my project\\repo"`) {
		t.Fatalf("got %q", got)
	}
}

func TestWriteSlotEnv(t *testing.T) {
	dir := t.TempDir()
	res := &Result{
		ComposeProject: "hello-test",
		StackKey:       "hello",
		Mode:           "test",
		Host:           "hello.test.localhost",
		CheckoutKey:    "c:/tmp/hello",
		Ports:          map[string]int{"web": 18080},
		SlotEnvPath:    filepath.Join(dir, ".siding.env"),
		ModeConfig: contract.Mode{
			PortEnv: map[string]string{"web": "WEB_PORT"},
		},
	}
	if err := WriteSlotEnv(res); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.SlotEnvPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "WEB_PORT=18080") {
		t.Fatalf("missing WEB_PORT: %s", body)
	}
	if !strings.Contains(body, "COMPOSE_PROJECT_NAME=hello-test") {
		t.Fatalf("missing project: %s", body)
	}
}
