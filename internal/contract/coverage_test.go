package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandComposeProject(t *testing.T) {
	c := &Contract{
		Project:        "hello",
		ComposeProject: DefaultCompose,
		Modes:          map[string]Mode{"test": {ComposeFiles: []string{"docker-compose.yml"}}},
	}
	got, err := c.ExpandComposeProject("hello", "test", c.Modes["test"])
	if err != nil || got != "hello-test" {
		t.Fatalf("got %q err=%v", got, err)
	}
	mode := Mode{ComposeProject: "custom-{mode}"}
	got, err = c.ExpandComposeProject("hello", "prod", mode)
	if err != nil || got != "custom-prod" {
		t.Fatalf("mode override %q err=%v", got, err)
	}
}

func TestApplyHostPrefixNoneClearsWeb(t *testing.T) {
	c := &Contract{
		HostPrefix: "example",
		HTTPHosts:  map[string][]string{"web": {"app"}},
		Modes: map[string]Mode{
			"test": {HTTPHosts: map[string][]string{"web": {"app"}, "api": {"graphql"}}},
		},
	}
	ApplyHostPrefix(c, "none")
	if c.HostPrefix != "" {
		t.Fatalf("prefix %q", c.HostPrefix)
	}
	if _, ok := c.HTTPHosts["web"]; ok {
		t.Fatalf("web hosts %#v", c.HTTPHosts)
	}
	if _, ok := c.Modes["test"].HTTPHosts["web"]; ok {
		t.Fatal("mode web should clear")
	}
}

func TestWriteRendersEntryPoints(t *testing.T) {
	dir := t.TempDir()
	c := &Contract{
		Project:     "demo",
		HostPrefix:  "example",
		HTTPHosts:   map[string][]string{"web": {"app", "www"}},
		EntryPoints: map[string][]string{"web": {"app"}, "api": {"api"}},
		Modes: map[string]Mode{
			"test": {
				ComposeFiles: []string{"docker-compose.yml"},
				Ports:        map[string]int{"web": 3000, "api": 8000},
				PortEnv:      map[string]string{"web": "WEB_PORT"},
				Env:             map[string]string{"SIDING_DEMO": "1"},
				ComposeEnvFiles: ComposeEnvFileList{{Path: "docker/.env.prod-db"}},
				EntryPoints:     map[string][]string{"web": {"app"}},
			},
		},
	}
	path, err := Write(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"host_prefix: example", "entry_points:", "http_hosts:", "SIDING_DEMO", "compose_env_files:", "docker/.env.prod-db"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, DirName, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingAndConfigPath(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected missing contract")
	}
	repo := t.TempDir()
	if got, want := ConfigPath(repo), filepath.Join(repo, DirName, ConfigFileName); got != want {
		t.Fatalf("config path %q want %q", got, want)
	}
	if got := SidingComposeRel("docker-compose.yml"); got != ".siding/compose/docker-compose.yml" {
		t.Fatalf("siding compose rel %q", got)
	}
	if got := SidingComposeRel(".siding/compose/docker-compose.yml"); got != ".siding/compose/docker-compose.yml" {
		t.Fatalf("already a siding copy %q", got)
	}
}

func TestApplyEntryPointsClearsModeWeb(t *testing.T) {
	c := &Contract{
		HTTPHosts: map[string][]string{"web": {"old"}},
		Modes: map[string]Mode{
			"test": {HTTPHosts: map[string][]string{"web": {"old"}}},
		},
	}
	ApplyEntryPoints(c, map[string][]string{"web": {"web"}, "api": {"api"}})
	if _, ok := c.HTTPHosts["web"]; ok {
		t.Fatalf("apex web should drop extras %#v", c.HTTPHosts)
	}
	if c.Modes["test"].HTTPHosts != nil {
		t.Fatalf("mode hosts %#v", c.Modes["test"].HTTPHosts)
	}
}

func TestMergeEntryPointsEmpty(t *testing.T) {
	if MergeEntryPoints(nil, nil) != nil {
		t.Fatal("nil")
	}
	got := MergeEntryPoints(map[string][]string{"web": {"  "}}, nil)
	if got != nil {
		t.Fatalf("%#v", got)
	}
}

func TestSanitizeStackName(t *testing.T) {
	cases := map[string]string{
		"  ACME!  ":        "acme",
		"  my_app.v2  ":   "my-app-v2",
		"--dashed--":      "dashed",
		"Already-Fine-01": "already-fine-01",
	}
	for in, want := range cases {
		got, err := sanitizeStackName(in)
		if err != nil || got != want {
			t.Fatalf("%q → %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "!!!", "___", " . "} {
		if got, err := sanitizeStackName(in); err == nil || !strings.Contains(err.Error(), "empty after sanitizing") {
			t.Fatalf("%q → %q %v, want error", in, got, err)
		}
	}
}

func TestDefaultProject(t *testing.T) {
	if got := DefaultProject("acme-genie2", "acme-"); got != "acme" {
		t.Fatalf("prefix stem %q", got)
	}
	if got := DefaultProject("acme-genie3", "acme"); got != "acme" {
		t.Fatalf("prefix without dash %q", got)
	}
	if got := DefaultProject("hello", ""); got != "hello" {
		t.Fatalf("no prefix %q", got)
	}
	if got := DefaultProject("/tmp/acme-genie2", ""); got != "acme-genie2" {
		t.Fatalf("basename only when no prefix %q", got)
	}
}
