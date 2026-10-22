package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheBlackhowling/siding/internal/paths"
)

func TestFindCheckoutWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	key := paths.NormalizeLockPath(root)
	f := NewFile()
	f.Checkouts[key] = Checkout{
		Project: "hello",
		Stack:   "hello",
		Modes:   map[string]ModeAllocation{"test": {ComposeProject: "hello-test"}},
	}

	gotKey, rec, ok := FindCheckout(f, nested)
	if !ok {
		t.Fatalf("expected checkout from %s (want key %q)", nested, key)
	}
	if gotKey != key || rec.Stack != "hello" {
		t.Fatalf("got key %q stack %q", gotKey, rec.Stack)
	}
}

func TestRemoveModeAndCheckout(t *testing.T) {
	f := NewFile()
	key := "c:/source/hello"
	f.Checkouts[key] = Checkout{
		Project: "hello",
		Stack:   "hello",
		Modes: map[string]ModeAllocation{
			"test": {Ports: map[string]int{"web": 18000}},
			"prod": {Ports: map[string]int{"web": 18010}},
		},
	}
	for _, mode := range []string{"test", "prod"} {
		newly, err := ClaimLock(f, "hello", "hello", mode, key, "hello-"+mode)
		if err != nil || !newly {
			t.Fatalf("claim %s: newly=%v err=%v", mode, newly, err)
		}
	}

	if !RemoveMode(f, key, "test") {
		t.Fatal("remove test")
	}
	if _, ok := f.Checkouts[key].Modes["test"]; ok {
		t.Fatal("test mode still present")
	}
	if _, ok := f.Locks[LockKey("hello", "hello", "test")]; ok {
		t.Fatal("removing a mode must release its lock")
	}
	if _, ok := f.Locks[LockKey("hello", "hello", "prod")]; !ok {
		t.Fatal("other mode's lock must survive")
	}
	if _, ok := f.Checkouts[key].Modes["prod"]; !ok {
		t.Fatal("prod should remain")
	}

	if !RemoveCheckout(f, key) {
		t.Fatal("remove checkout")
	}
	if _, ok := f.Checkouts[key]; ok {
		t.Fatal("checkout still present")
	}
	if len(f.Locks) != 0 {
		t.Fatalf("removing the checkout must release its locks: %#v", f.Locks)
	}
	if RemoveCheckout(f, key) {
		t.Fatal("already gone")
	}
}

func TestCheckoutModeNames(t *testing.T) {
	rec := Checkout{Modes: map[string]ModeAllocation{"test": {}, "legacy": {}}}
	got := CheckoutModeNames([]string{"prod", "test"}, rec, "")
	if len(got) != 3 || got[0] != "prod" || got[1] != "test" || got[2] != "legacy" {
		t.Fatalf("got %#v", got)
	}
	if only := CheckoutModeNames(nil, rec, "prod"); len(only) != 1 || only[0] != "prod" {
		t.Fatalf("only %#v", only)
	}
}
