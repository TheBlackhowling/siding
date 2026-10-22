package version

import "testing"

func TestInfo(t *testing.T) {
	old := [4]string{Version, Commit, Date, GoVersion}
	t.Cleanup(func() { Version, Commit, Date, GoVersion = old[0], old[1], old[2], old[3] })

	// Info reflects the ldflags-set variables at call time, under stable keys
	// (the JSON/doctor output depends on these names).
	Version, Commit, Date, GoVersion = "1.2.3", "abc1234", "2026-09-04", "go1.22"
	got := Info()
	want := map[string]string{
		"version":    "1.2.3",
		"commit":     "abc1234",
		"date":       "2026-09-04",
		"go_version": "go1.22",
	}
	if len(got) != len(want) {
		t.Fatalf("keys %#v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q want %q", k, got[k], v)
		}
	}

	// Each call is a fresh map; callers may mutate their copy.
	got["version"] = "mutated"
	if Info()["version"] != "1.2.3" {
		t.Fatal("Info must not share state between calls")
	}
}
