package state

import "testing"

func TestPreferHostPortLiftsCommonDefaults(t *testing.T) {
	cases := map[int]int{
		3000:  18000,
		8000:  23000,
		5432:  20432,
		6379:  21379,
		8080:  23080,
		27017: 42017,
		18080: 18080,
		18000: 18000,
	}
	for in, want := range cases {
		if got := PreferHostPort(in); got != want {
			t.Errorf("PreferHostPort(%d)=%d want %d", in, got, want)
		}
	}
}

func TestLiftCommonPorts(t *testing.T) {
	got := LiftCommonPorts(map[string]int{"web": 3000, "api": 8000, "redis": 6379})
	if got["web"] != 18000 || got["api"] != 23000 || got["redis"] != 21379 {
		t.Fatalf("%#v", got)
	}
}

func TestCollapsePortKeysHyphenUnderscore(t *testing.T) {
	got := CollapsePortKeys(map[string]int{"gcs-emulator": 4443, "gcs_emulator": 4443, "web": 3000})
	if len(got) != 2 {
		t.Fatalf("%#v", got)
	}
	if got["gcs_emulator"] != 4443 || got["web"] != 3000 {
		t.Fatalf("%#v", got)
	}
	if _, ok := got["gcs-emulator"]; ok {
		t.Fatal("hyphen key should fold into gcs_emulator")
	}
}

func TestReconcileCollapsesDuplicateGcsKeys(t *testing.T) {
	f := NewFile()
	alloc, created, err := EnsureModeAllocation(f, "c:/src/genie", "acme", "genie", "test", ModeAllocation{
		Ports: map[string]int{
			"web":          3000,
			"gcs-emulator": 4443,
			"gcs_emulator": 4443,
		},
	}, nil)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, ok := alloc.Ports["gcs-emulator"]; ok {
		t.Fatalf("hyphen key leaked: %#v", alloc.Ports)
	}
	if alloc.Ports["gcs_emulator"] != 19443 {
		t.Fatalf("gcs=%#v", alloc.Ports)
	}
	if alloc.Ports["web"] != 18000 {
		t.Fatalf("web=%#v", alloc.Ports)
	}
}
