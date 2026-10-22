package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRootComposeFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"docker-compose.prod.yml",
		"README.md",
		"docker-compose.yml",
		"docker-compose.override.yml",
		"compose.yaml",
		"src",
		"composer.json",
	} {
		p := filepath.Join(dir, name)
		if name == "src" {
			if err := os.Mkdir(p, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "docker-compose.yml"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListRootComposeFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compose.yaml", "docker-compose.yml", "docker-compose.override.yml", "docker-compose.prod.yml"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestParseComposeSelection(t *testing.T) {
	files := []string{"docker-compose.yml", "docker-compose.override.yml", "docker-compose.prod.yml"}
	all, err := ParseComposeSelection("all", files)
	if err != nil || len(all) != 3 {
		t.Fatalf("all: %v %v", all, err)
	}
	empty, err := ParseComposeSelection("", files)
	if err != nil || len(empty) != 3 {
		t.Fatalf("empty: %v %v", empty, err)
	}
	got, err := ParseComposeSelection("1, 3", files)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != files[0] || got[1] != files[2] {
		t.Fatalf("got %v", got)
	}
	if _, err := ParseComposeSelection("9", files); err == nil {
		t.Fatal("expected out of range")
	}
	if _, err := ParseComposeSelection("web", files); err == nil {
		t.Fatal("expected bad token")
	}
}

func TestParseEnvFileSelectionOrderAndNone(t *testing.T) {
	files := []string{"docker/.env.a", "docker/.env.b", "docker/.env.c"}
	got, err := ParseEnvFileSelection("2,1", files)
	if err != nil || len(got) != 2 || got[0] != files[1] || got[1] != files[0] {
		t.Fatalf("order %v %v", got, err)
	}
	none, err := ParseEnvFileSelection("none", files)
	if err != nil || len(none) != 0 {
		t.Fatalf("none %v %v", none, err)
	}
	all, err := ParseEnvFileSelection("all", files)
	if err != nil || len(all) != 3 {
		t.Fatalf("all %v %v", all, err)
	}
}

func TestGroupFilesSplitsIndependentStacks(t *testing.T) {
	got := GroupFiles([]string{
		"docker-compose.yml",
		"docker-compose.override.yml",
		"docker-compose.husky.yml",
		"docker-compose.prod-db.yml",
	})
	if len(got) != 3 {
		t.Fatalf("%#v", got)
	}
	if got[0].Mode != "test" || len(got[0].Files) != 2 || got[0].Files[0] != "docker-compose.yml" || got[0].Files[1] != "docker-compose.override.yml" {
		t.Fatalf("primary=%#v", got[0])
	}
	if got[1].Mode != "husky" || got[1].Files[0] != "docker-compose.husky.yml" {
		t.Fatalf("husky=%#v", got[1])
	}
	if got[2].Mode != "prod-db" || got[2].Files[0] != "docker-compose.prod-db.yml" {
		t.Fatalf("prod-db=%#v", got[2])
	}
}

func TestGroupFilesSingleStandaloneIsTest(t *testing.T) {
	got := GroupFiles([]string{"docker-compose.prod-db.yml"})
	if len(got) != 1 || got[0].Mode != "test" || got[0].Files[0] != "docker-compose.prod-db.yml" {
		t.Fatalf("%#v", got)
	}
}

func TestGroupFilesProdOverrideAttachesToProd(t *testing.T) {
	got := GroupFiles([]string{
		"docker-compose.yml",
		"docker-compose.override.yml",
		"docker-compose.prod.yml",
		"docker-compose.prod.override.yml",
	})
	if len(got) != 2 {
		t.Fatalf("%#v", got)
	}
	if got[0].Mode != "test" || len(got[0].Files) != 2 || got[0].Files[1] != "docker-compose.override.yml" {
		t.Fatalf("test=%#v", got[0])
	}
	if got[1].Mode != "prod" || len(got[1].Files) != 2 || got[1].Files[1] != "docker-compose.prod.override.yml" {
		t.Fatalf("prod override should attach to prod, not test: %#v", got)
	}
}

func TestModeNameFromFile(t *testing.T) {
	if got := ModeNameFromFile("docker-compose.prod-db.yml"); got != "prod-db" {
		t.Fatalf("%q", got)
	}
	if got := ModeNameFromFile("compose.e2e.yaml"); got != "e2e" {
		t.Fatalf("%q", got)
	}
}

func TestApplyModeNamesOverridesSuffix(t *testing.T) {
	groups := GroupFiles([]string{
		"docker-compose.yml",
		"docker-compose.husky.yml",
		"docker-compose.prod-db.yml",
	})
	got := ApplyModeNames(groups, map[string]string{
		"docker-compose.yml":        "test",
		"docker-compose.prod-db.yml": "prod",
	})
	if len(got) != 3 {
		t.Fatalf("%#v", got)
	}
	if got[0].Mode != "test" || got[0].Files[0] != "docker-compose.yml" {
		t.Fatalf("primary=%#v", got[0])
	}
	if got[1].Mode != "husky" {
		t.Fatalf("husky=%#v", got[1])
	}
	if got[2].Mode != "prod" || got[2].Files[0] != "docker-compose.prod-db.yml" {
		t.Fatalf("prod=%#v", got[2])
	}
}

func TestRelComposePathKeepsSidingDir(t *testing.T) {
	if got := RelComposePath(".siding/compose/docker-compose.yml"); got != ".siding/compose/docker-compose.yml" {
		t.Fatalf("got %q", got)
	}
	if got := RelComposePath("docker-compose.yml"); got != "docker-compose.yml" {
		t.Fatalf("got %q", got)
	}
}
