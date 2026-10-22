package contract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStackNamePrefix(t *testing.T) {
	c := &Contract{FolderPrefix: "acme-"}
	got, err := ResolveStackName(c, `C:\source\acme-betty`, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "betty" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveStackNameBasename(t *testing.T) {
	c := &Contract{}
	got, err := ResolveStackName(c, `C:\source\hello`, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveStackNameOverride(t *testing.T) {
	c := &Contract{FolderPrefix: "acme-"}
	got, err := ResolveStackName(c, `C:\source\acme-betty`, "ruth")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ruth" {
		t.Fatalf("got %q", got)
	}
}

func TestFindRepoRootPrefersSidingDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "src", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DirName, ConfigFileName), []byte("version: 1\nproject: x\nmodes:\n  test:\n    compose_files: [docker-compose.yml]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("got %q want %q", got, root)
	}
}
