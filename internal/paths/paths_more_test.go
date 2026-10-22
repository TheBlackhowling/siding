package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAbsEmptyUsesWD(t *testing.T) {
	got, err := Abs("")
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(wd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParent(t *testing.T) {
	if got := Parent(filepath.Join("x", "y", "z")); got != filepath.Join("x", "y") {
		t.Fatalf("parent %q", got)
	}
	if got := Parent(filepath.Join("x", "y")); got != "x" {
		t.Fatalf("parent %q", got)
	}
	if runtime.GOOS == "windows" {
		if got := Parent(`C:\a\b\c`); got != `C:\a\b` {
			t.Fatalf("parent %q", got)
		}
	} else if got := Parent("/a/b/c"); got != "/a/b" {
		t.Fatalf("parent %q", got)
	}
}

// fakeHome points every home-directory lookup at a temp dir: HOME for Unix,
// USERPROFILE for Windows, plus the platform data-dir variables.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv(EnvRoot, "")
	t.Setenv(EnvData, "")
	return home
}

func TestUserDataRootOverrideAndEnv(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	got, err := UserDataRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("override %q want %q", got, abs)
	}
	t.Setenv(EnvData, dir)
	got, err = UserDataRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("SIDING_DATA %q want %q", got, abs)
	}
}

func TestUserDataRootPlatform(t *testing.T) {
	home := fakeHome(t)
	got, err := UserDataRoot("")
	if err != nil {
		t.Fatal(err)
	}
	var want string
	switch runtime.GOOS {
	case "windows":
		want = filepath.Join(home, "AppData", "Local", "siding")
	case "darwin":
		want = filepath.Join(home, "Library", "Application Support", "siding")
	default:
		want = filepath.Join(home, ".local", "share", "siding")
	}
	if got != want {
		t.Fatalf("platform root %q want %q", got, want)
	}
}

func TestUserDataRootForEachGOOS(t *testing.T) {
	home := fakeHome(t)
	lad := filepath.Join(t.TempDir(), "lad")
	t.Setenv("LOCALAPPDATA", lad)
	win, err := userDataRootFor("windows")
	if err != nil || win != filepath.Join(lad, "siding") {
		t.Fatalf("windows %q %v", win, err)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, err := userDataRootFor("windows"); err == nil {
		t.Fatal("missing LOCALAPPDATA")
	}
	mac, err := userDataRootFor("darwin")
	if err != nil || mac != filepath.Join(home, "Library", "Application Support", "siding") {
		t.Fatalf("darwin %q %v", mac, err)
	}
	nix, err := userDataRootFor("linux")
	if err != nil || nix != filepath.Join(home, ".local", "share", "siding") {
		t.Fatalf("linux %q %v", nix, err)
	}
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	nix, err = userDataRootFor("linux")
	if err != nil || nix != filepath.Join(xdg, "siding") {
		t.Fatalf("linux XDG %q %v", nix, err)
	}
	t.Setenv("XDG_DATA_HOME", "  ")
	nix, err = userDataRootFor("linux")
	if err != nil || nix != filepath.Join(home, ".local", "share", "siding") {
		t.Fatalf("blank XDG is unset %q %v", nix, err)
	}
}

func TestCommandBinDirForEachGOOS(t *testing.T) {
	home := fakeHome(t)
	data := t.TempDir()
	t.Setenv(EnvData, data)
	win, err := commandBinDirFor("windows")
	if err != nil || win != filepath.Join(data, "bin") {
		t.Fatalf("windows %q %v", win, err)
	}
	unix, err := commandBinDirFor("linux")
	if err != nil || unix != filepath.Join(home, ".local", "bin") {
		t.Fatalf("linux %q %v", unix, err)
	}
}

func TestResolvePathsExplicitArg(t *testing.T) {
	dir := t.TempDir()
	p, err := ResolvePaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.StateFile != filepath.Join(dir, "state.json") {
		t.Fatalf("%q", p.StateFile)
	}
	file := filepath.Join(dir, "custom.json")
	p, err = ResolvePaths(file)
	if err != nil {
		t.Fatal(err)
	}
	if p.StateFile != file {
		t.Fatalf("file override %q", p.StateFile)
	}
}
