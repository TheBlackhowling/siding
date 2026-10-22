//go:build windows

package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestReadUnexpandedAndBroadcast(t *testing.T) {
	name := "siding-cover-" + filepath.Base(filepath.Dir(t.TempDir()))
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\`+name, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		k.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\`+name)
	}()
	if err := k.SetExpandStringValue("Path", `%USERPROFILE%\bin`); err != nil {
		t.Fatal(err)
	}
	raw, typ, err := readUnexpanded(k, "Path")
	if err != nil {
		t.Fatal(err)
	}
	if typ != registry.EXPAND_SZ || !strings.Contains(raw, "%USERPROFILE%") {
		t.Fatalf("raw=%q typ=%d", raw, typ)
	}
	if _, _, err := readUnexpanded(k, "missing"); err == nil {
		t.Fatal("missing")
	}
	if err := broadcastEnv(); err != nil {
		t.Fatal(err)
	}
}

// scratchEnvKey points addWindowsUserPath at a throwaway HKCU key for the test
// so the real user PATH is never read or written.
func scratchEnvKey(t *testing.T) registry.Key {
	t.Helper()
	// t.TempDir() is <tmp>/<TestName><random>/001; the parent is unique per test.
	name := `Software\siding-test-` + filepath.Base(filepath.Dir(t.TempDir()))
	k, _, err := registry.CreateKey(registry.CURRENT_USER, name, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	old := userEnvKeyPath
	userEnvKeyPath = name
	t.Cleanup(func() {
		userEnvKeyPath = old
		k.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, name)
	})
	return k
}

func TestAddWindowsUserPathIdempotent(t *testing.T) {
	k := scratchEnvKey(t)
	if err := k.SetStringValue("Path", `C:\existing`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := addWindowsUserPath(dir); err != nil {
		t.Fatal(err)
	}
	if err := addWindowsUserPath(dir); err != nil {
		t.Fatal(err)
	}
	got, typ, err := readUnexpanded(k, "Path")
	if err != nil {
		t.Fatal(err)
	}
	if typ != registry.SZ {
		t.Fatalf("plain values stay REG_SZ, got %d", typ)
	}
	if got != dir+`;C:\existing` {
		t.Fatalf("path %q", got)
	}
	if strings.Count(got, dir) != 1 {
		t.Fatalf("second add must not duplicate: %q", got)
	}
}

func TestAddWindowsUserPathExpandAndEmpty(t *testing.T) {
	k := scratchEnvKey(t)
	dir := t.TempDir()
	if err := addWindowsUserPath(dir); err != nil {
		t.Fatal(err)
	}
	got, _, err := readUnexpanded(k, "Path")
	if err != nil || got != dir {
		t.Fatalf("empty Path becomes just the dir: %q %v", got, err)
	}

	if err := k.SetExpandStringValue("Path", `%USERPROFILE%\bin`); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := addWindowsUserPath(other); err != nil {
		t.Fatal(err)
	}
	got, typ, err := readUnexpanded(k, "Path")
	if err != nil {
		t.Fatal(err)
	}
	if typ != registry.EXPAND_SZ || got != other+`;%USERPROFILE%\bin` {
		t.Fatalf("expand values stay REG_EXPAND_SZ and unexpanded: %q %d", got, typ)
	}
}
