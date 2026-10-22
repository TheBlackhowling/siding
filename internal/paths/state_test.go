package paths

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePathsDefault(t *testing.T) {
	data := t.TempDir()
	t.Setenv(EnvRoot, "") // a developer shell may export SIDING_ROOT
	t.Setenv(EnvData, data)

	p, err := ResolvePaths("")
	if err != nil {
		t.Fatal(err)
	}
	if p.UserDataRoot != data {
		t.Fatalf("user data root = %q want %q", p.UserDataRoot, data)
	}
	if p.StateFile != filepath.Join(data, "state.json") {
		t.Fatalf("state file = %q", p.StateFile)
	}
	if p.ProxyDir != filepath.Join(data, "siding-proxy") {
		t.Fatalf("proxy dir = %q", p.ProxyDir)
	}
}

// Explicit argument beats SIDING_ROOT, which beats SIDING_DATA.
func TestResolvePathsPrecedence(t *testing.T) {
	arg, root, data := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv(EnvRoot, root)
	t.Setenv(EnvData, data)

	p, err := ResolvePaths(arg)
	if err != nil || p.UserDataRoot != arg {
		t.Fatalf("arg wins: %q %v", p.UserDataRoot, err)
	}
	p, err = ResolvePaths("")
	if err != nil || p.UserDataRoot != root {
		t.Fatalf("SIDING_ROOT beats SIDING_DATA: %q %v", p.UserDataRoot, err)
	}
	t.Setenv(EnvRoot, "  ")
	p, err = ResolvePaths("")
	if err != nil || p.UserDataRoot != data {
		t.Fatalf("blank SIDING_ROOT is unset: %q %v", p.UserDataRoot, err)
	}
}

func TestSIDING_ROOTFileOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "my-state.json")
	t.Setenv("SIDING_ROOT", custom)

	p, err := ResolvePaths("")
	if err != nil {
		t.Fatal(err)
	}
	if p.StateFile != custom {
		t.Fatalf("state file = %q, want %q", p.StateFile, custom)
	}
}

func TestSIDING_ROOTDirOverride(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("SIDING_ROOT", custom)

	p, err := ResolvePaths("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(custom, "state.json")
	if p.StateFile != want {
		t.Fatalf("state file = %q, want %q", p.StateFile, want)
	}
}

func TestCommandBinDir(t *testing.T) {
	home := fakeHome(t)
	t.Setenv(EnvData, home)
	dir, err := CommandBinDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin")
	if runtime.GOOS == "windows" {
		want = filepath.Join(home, "bin")
	}
	if dir != want {
		t.Fatalf("bin dir %q want %q", dir, want)
	}
}
