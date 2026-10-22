package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestCanonicalHost(t *testing.T) {
	if got := CanonicalHost("hello.test"); got != "hello.test.localhost" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalHost("hello.test.localhost"); got != "hello.test.localhost" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandCollapsesDuplicateSegments(t *testing.T) {
	if got := Expand(DefaultHost, "hello", "hello", "test"); got != "hello.test" {
		t.Fatalf("host = %q", got)
	}
	if got := Expand(DefaultCompose, "hello", "hello", "test"); got != "hello-test" {
		t.Fatalf("compose = %q", got)
	}
	if got := Expand(DefaultHost, "betty", "acme", "test"); got != "betty.acme.test" {
		t.Fatalf("acme host = %q", got)
	}
	if got := Expand(DefaultCompose, "betty", "acme", "test"); got != "acme-betty-test" {
		t.Fatalf("acme compose = %q", got)
	}
	if got := Expand(PrefixedHost, "genie", "acme-genie", "test", "acme"); got != "acme.genie.test" {
		t.Fatalf("root test = %q", got)
	}
	if got := Expand(PrefixedHost, "genie", "acme-genie", "prod", "acme"); got != "acme.genie.prod" {
		t.Fatalf("root prod = %q", got)
	}
}

func TestExpandHostUsesHostPrefix(t *testing.T) {
	c := &Contract{
		Project:    "acme-genie",
		HostPrefix: "app.acme",
		Host:       DefaultHost,
		Modes:      map[string]Mode{"test": {ComposeFiles: []string{"docker-compose.yml"}}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Host != PrefixedHost {
		t.Fatalf("validate should switch host pattern, got %q", c.Host)
	}
	got, err := c.ExpandHost("genie", "test", c.Modes["test"])
	if err != nil || got != "acme.genie.test.localhost" {
		t.Fatalf("root host = %q err=%v (app. is the public subdomain, not ROOT_DOMAIN)", got, err)
	}
}

func TestPrimaryBranchName(t *testing.T) {
	if got := (&Contract{}).PrimaryBranchName(); got != DefaultPrimaryBranch {
		t.Fatalf("empty = %q", got)
	}
	if got := (&Contract{PrimaryBranch: "develop"}).PrimaryBranchName(); got != "develop" {
		t.Fatalf("set = %q", got)
	}
}

func TestLoadExampleContract(t *testing.T) {
	c, err := Load(testenv.HelloRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != "hello" {
		t.Fatalf("project = %q", c.Project)
	}
	if _, err := c.Mode("test"); err != nil {
		t.Fatal(err)
	}
}

func TestPickModeAndSlotEnvRel(t *testing.T) {
	c := &Contract{
		Project:     "x",
		SlotEnvFile: ".siding/env",
		Modes: map[string]Mode{
			"prod-db": {ComposeFiles: []string{"docker-compose.prod-db.yml"}},
			"husky":   {ComposeFiles: []string{"docker-compose.husky.yml"}},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	got, err := PickMode(c, "")
	if err != nil || got != "husky" {
		t.Fatalf("default=%q err=%v", got, err)
	}
	got, err = PickMode(c, "prod-db")
	if err != nil || got != "prod-db" {
		t.Fatalf("explicit=%q err=%v", got, err)
	}
	if SlotEnvRel(c, "test") != ".siding/env" {
		t.Fatalf("test env %q", SlotEnvRel(c, "test"))
	}
	if SlotEnvRel(c, "prod-db") != ".siding/env.prod-db" {
		t.Fatalf("prod-db env %q", SlotEnvRel(c, "prod-db"))
	}
}

func TestNormalizeComposeEnvFiles(t *testing.T) {
	got := NormalizeComposeEnvFiles([]string{" docker\\.env.prod-db ", "docker/.env.prod-db", "none", ""})
	if len(got) != 1 || got[0] != "docker/.env.prod-db" {
		t.Fatalf("%v", got)
	}
}

func TestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := &Contract{
		Project: "demo",
		Modes: map[string]Mode{
			"test": {
				ComposeFiles: []string{"docker-compose.yml"},
				Ports:        map[string]int{"web": 18080},
				PortEnv:      map[string]string{"web": "WEB_PORT"},
			},
		},
	}
	path, err := Write(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(path)) != DirName {
		t.Fatalf("path = %q", path)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project != "demo" || loaded.Modes["test"].Ports["web"] != 18080 {
		t.Fatalf("loaded %#v", loaded)
	}
	if _, err := os.Stat(filepath.Join(dir, DirName, ".gitignore")); err != nil {
		t.Fatal(err)
	}
}

func TestSidingComposeRel(t *testing.T) {
	if got := SidingComposeRel("docker-compose.yml"); got != ".siding/compose/docker-compose.yml" {
		t.Fatalf("got %q", got)
	}
	if got := SidingComposeRel(`C:\src\app\docker-compose.prod-db.yml`); got != ".siding/compose/docker-compose.prod-db.yml" {
		t.Fatalf("got %q", got)
	}
}
