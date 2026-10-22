package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheBlackhowling/siding/internal/paths"
)

func TestClaimAssertReleaseLock(t *testing.T) {
	f := NewFile()
	newly, err := ClaimLock(f, "hello", "hello", "test", "c:/source/hello", "hello-hello-test")
	if err != nil || !newly {
		t.Fatalf("claim %v %v", newly, err)
	}
	if err := AssertLock(f, "hello", "hello", "test", "c:/source/hello"); err != nil {
		t.Fatal(err)
	}
	if err := AssertLock(f, "hello", "hello", "test", "c:/source/other"); err == nil {
		t.Fatal("other checkout should conflict")
	}
	ReleaseLock(f, "hello", "hello", "test", "c:/source/other")
	if _, ok := f.Locks[LockKey("hello", "hello", "test")]; !ok {
		t.Fatal("other checkout must not release")
	}
	ReleaseLock(f, "hello", "hello", "test", "c:/source/hello")
	if _, ok := f.Locks[LockKey("hello", "hello", "test")]; ok {
		t.Fatal("lock should be gone")
	}
	ReleaseLock(f, "hello", "hello", "test", "c:/source/hello")
}

func TestModeAllocationLookup(t *testing.T) {
	store := NewStore(paths.Paths{StateFile: filepath.Join(t.TempDir(), "state.json")})
	f := NewFile()
	if _, ok := store.ModeAllocation(f, "missing", "test"); ok {
		t.Fatal("missing checkout")
	}
	f.Checkouts["c:/a"] = Checkout{Modes: map[string]ModeAllocation{"test": {Host: "a.test.localhost"}}}
	got, ok := store.ModeAllocation(f, "c:/a", "test")
	if !ok || got.Host != "a.test.localhost" {
		t.Fatalf("%v %#v", ok, got)
	}
	if _, ok := store.ModeAllocation(f, "c:/a", "prod"); ok {
		t.Fatal("missing mode")
	}
}

func TestReadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(paths.Paths{StateFile: filepath.Join(dir, "state.json")})
	if err := os.WriteFile(store.StateFile, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestStampLastUpEmpty(t *testing.T) {
	f := NewFile()
	StampLastUp(f, "missing", "test", "main", "abc", "now")
	f.Checkouts["c:/a"] = Checkout{Modes: map[string]ModeAllocation{"test": {}}}
	StampLastUp(f, "c:/a", "test", "main", "abc", "now")
	if f.Checkouts["c:/a"].Modes["test"].GitBranch != "main" {
		t.Fatal("stamp")
	}
}
