//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddUnixUserPathIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := addUnixUserPath(dir); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, unixShellConfigFiles()[0])
	first, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), unixPathMarker) {
		t.Fatalf("missing marker in %s", first)
	}

	if err := addUnixUserPath(dir); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(second), unixPathMarker) != 1 {
		t.Fatalf("expected one marker, got:\n%s", second)
	}
}
