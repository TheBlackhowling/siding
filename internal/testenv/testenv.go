// Package testenv fences unit tests off from tools on the host
// and locates the shared examples/hello fixture.
package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// NoHostTools points PATH at an empty directory for the rest of the test, so
// docker, git, netstat, ss, lsof, tasklist and netsh all fail with "not found".
//
// Call it in every test that could otherwise reach the developer's Docker
// daemon: siding's compose project and container names are fixed, so an
// unfenced test can recreate or stop the real siding-proxy-caddy and app stacks.
func NoHostTools(t testing.TB) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// HelloRepo is examples/hello, resolved from this file so cwd does not matter.
// The example is the golden recipe/compose fixture shared by advise, contract,
// resolver, doctor, and cli — edit it only when the example itself should change.
func HelloRepo(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "examples", "hello")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		t.Fatalf("examples/hello missing at %s: %v", abs, err)
	}
	return abs
}
