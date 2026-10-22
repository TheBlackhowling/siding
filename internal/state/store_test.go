package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheBlackhowling/siding/internal/paths"
)

func TestReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(paths.Paths{
		StateFile: filepath.Join(dir, "state.json"),
		ProxyDir:  filepath.Join(dir, "siding-proxy"),
	})

	f := NewFile()
	f.Checkouts["c:/source/hello"] = Checkout{
		Project: "hello",
		Stack:   "hello",
		Modes: map[string]ModeAllocation{
			"test": {Ports: map[string]int{"web": 18080}},
		},
	}
	if err := store.Write(f); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Checkouts["c:/source/hello"].Modes["test"].Ports["web"] != 18080 {
		t.Fatal("allocation not persisted")
	}
}

func TestReadMigratesLeftoverListen(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(paths.Paths{
		StateFile: filepath.Join(dir, "state.json"),
		ProxyDir:  filepath.Join(dir, "siding-proxy"),
	})
	if err := os.WriteFile(store.StateFile, []byte(`{"version":1,"proxy":{"listen":1355},"checkouts":{},"locks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if ListenPort(loaded) != 80 {
		t.Fatalf("read should migrate 1355 → 80, got %d", ListenPort(loaded))
	}
}

func TestMigrateOffDefaultPorts(t *testing.T) {
	if MigrateOffDefaultPorts(nil) {
		t.Fatal("nil file has nothing to migrate")
	}
	SetListenPort(nil, 80) // must not panic
	if ListenPort(nil) != 80 {
		t.Fatalf("nil listen = %d", ListenPort(nil))
	}
	f := NewFile()
	if !MigrateOffDefaultPorts(f) || ListenPort(f) != 80 || f.Proxy == nil || f.Proxy.Listen != 80 {
		t.Fatalf("unset listen → %d (%#v)", ListenPort(f), f.Proxy)
	}
	SetListenPort(f, 1355)
	if !MigrateOffDefaultPorts(f) || ListenPort(f) != 80 {
		t.Fatalf("1355 → %d", ListenPort(f))
	}
	SetListenPort(f, 80)
	if MigrateOffDefaultPorts(f) {
		t.Fatal("80 should stay")
	}
	SetListenPort(f, 1365)
	if MigrateOffDefaultPorts(f) {
		t.Fatal("explicit --listen should stay")
	}
	SetListenPort(f, 8080)
	if MigrateOffDefaultPorts(f) || ListenPort(f) != 8080 {
		t.Fatalf("explicit 8080 should stay, got %d", ListenPort(f))
	}
}

func TestWriteKeepsExplicitListen(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(paths.Paths{
		StateFile: filepath.Join(dir, "state.json"),
		ProxyDir:  filepath.Join(dir, "siding-proxy"),
	})
	f := NewFile()
	SetListenPort(f, 8080)
	if err := store.Write(f); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if ListenPort(loaded) != 8080 {
		t.Fatalf("write must not migrate explicit --listen 8080, got %d", ListenPort(loaded))
	}
}

func TestCollectUsedPorts(t *testing.T) {
	f := NewFile()
	f.Checkouts["a"] = Checkout{Modes: map[string]ModeAllocation{
		"test": {Ports: map[string]int{"web": 3000, "api": 8000}},
	}}
	used := CollectUsedPorts(f)
	for _, p := range []int{3000, 8000, ProxyHTTPListen, ProxyAdminListen} {
		if _, ok := used[p]; !ok {
			t.Fatalf("missing %d in %#v", p, used)
		}
	}
}
