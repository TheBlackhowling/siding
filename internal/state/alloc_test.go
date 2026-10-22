package state

import "testing"

func TestAllocatePortsUsesDefaultsWhenFree(t *testing.T) {
	got, err := AllocatePorts(map[string]int{"web": 18080, "api": 18081}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["web"] != 18080 || got["api"] != 18081 {
		t.Fatalf("got %#v", got)
	}
}

func TestAllocatePortsShiftsTakenKeysOnly(t *testing.T) {
	used := map[int]struct{}{18080: {}, 18090: {}}
	got, err := AllocatePorts(map[string]int{"web": 18080, "api": 18081}, used)
	if err != nil {
		t.Fatal(err)
	}
	if got["web"] != 18100 {
		t.Fatalf("web=%d want 18100", got["web"])
	}
	if got["api"] != 18081 {
		t.Fatalf("api=%d want 18081 (free — do not move the whole block)", got["api"])
	}
}

func TestAllocatePortsSplitsDuplicateValues(t *testing.T) {
	got, err := AllocatePorts(map[string]int{"gcs_emulator": 19443, "gcs-emulator": 19443}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["gcs-emulator"] == got["gcs_emulator"] {
		t.Fatalf("duplicate values must not share a host port: %#v", got)
	}
}

func TestEnsureModeAllocationIdempotent(t *testing.T) {
	f := NewFile()
	seed := ModeAllocation{
		Host:           "hello.test.localhost",
		ComposeProject: "hello-test",
		Ports:          map[string]int{"web": 18080},
	}
	first, created, err := EnsureModeAllocation(f, "c:/source/hello", "hello", "hello", "test", seed, nil)
	if err != nil || !created {
		t.Fatalf("first: created=%v err=%v", created, err)
	}
	second, created, err := EnsureModeAllocation(f, "c:/source/hello", "hello", "hello", "test", seed, nil)
	if err != nil || created {
		t.Fatalf("second: created=%v err=%v", created, err)
	}
	if first.Ports["web"] != 18080 || second.Ports["web"] != 18080 {
		t.Fatalf("ports changed: %#v %#v", first.Ports, second.Ports)
	}
}

func TestEnsureModeAllocationMergesNewPortKeys(t *testing.T) {
	f := NewFile()
	PutCheckoutMode(f, "c:/source/hello", "hello", "hello", "test", ModeAllocation{
		Host:           "old.hello.test.localhost",
		ComposeProject: "old-hello-test",
		Ports:          map[string]int{"web": 18080},
	})
	PutCheckoutMode(f, "c:/source/other", "other", "other", "test", ModeAllocation{
		Ports: map[string]int{"web": 18081},
	})
	res, err := ReconcileModeAllocation(f, "c:/source/hello", "hello", "hello", "test", ModeAllocation{
		Host:           "hello.test.localhost",
		ComposeProject: "hello-test",
		Ports:          map[string]int{"web": 18080, "api": 18081},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Fatal("should merge, not create")
	}
	if res.Alloc.Ports["web"] != 18080 {
		t.Fatalf("kept web=%d", res.Alloc.Ports["web"])
	}
	if res.Alloc.Ports["api"] != 18091 {
		t.Fatalf("api=%d want 18091 (18081 taken)", res.Alloc.Ports["api"])
	}
	if len(res.AddedPorts) != 1 || res.AddedPorts[0] != "api" {
		t.Fatalf("added=%v", res.AddedPorts)
	}
	if res.Alloc.Host != "hello.test.localhost" || res.Alloc.ComposeProject != "hello-test" {
		t.Fatalf("host not refreshed: %#v", res.Alloc)
	}
}

func TestEnsureModeAllocationSkipsUsedPorts(t *testing.T) {
	f := NewFile()
	PutCheckoutMode(f, "c:/source/other", "other", "other", "test", ModeAllocation{
		Ports: map[string]int{"web": 18080},
	})
	alloc, created, err := EnsureModeAllocation(f, "c:/source/hello", "hello", "hello", "test", ModeAllocation{
		Host:           "hello.test.localhost",
		ComposeProject: "hello-test",
		Ports:          map[string]int{"web": 18080},
	}, nil)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if alloc.Ports["web"] != 18090 {
		t.Fatalf("web = %d", alloc.Ports["web"])
	}
}

func TestReconcileLiftsCommonDefaults(t *testing.T) {
	f := NewFile()
	alloc, created, err := EnsureModeAllocation(f, "c:/src/app", "app", "app", "test", ModeAllocation{
		Host:           "app.test.localhost",
		ComposeProject: "app-test",
		Ports:          map[string]int{"web": 3000, "api": 8000, "postgres": 5432},
	}, nil)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if alloc.Ports["web"] != 18000 || alloc.Ports["api"] != 23000 || alloc.Ports["postgres"] != 20432 {
		t.Fatalf("lifted=%#v", alloc.Ports)
	}
}

func TestReconcileSkipsLiveDockerPorts(t *testing.T) {
	f := NewFile()
	live := map[int]struct{}{18000: {}, 18010: {}}
	alloc, created, err := EnsureModeAllocation(f, "c:/src/app", "app", "app", "test", ModeAllocation{
		Ports: map[string]int{"web": 3000},
	}, live)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if alloc.Ports["web"] != 18020 {
		t.Fatalf("web=%d want 18020 (18000 and 18010 live)", alloc.Ports["web"])
	}
}

func TestReconcileKeepsExistingCommonPort(t *testing.T) {
	f := NewFile()
	PutCheckoutMode(f, "c:/src/app", "app", "app", "test", ModeAllocation{
		Ports: map[string]int{"web": 3000},
	})
	res, err := ReconcileModeAllocation(f, "c:/src/app", "app", "app", "test", ModeAllocation{
		Ports: map[string]int{"web": 3000, "redis": 6379},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Alloc.Ports["web"] != 3000 {
		t.Fatalf("must keep existing web: %#v", res.Alloc.Ports)
	}
	if res.Alloc.Ports["redis"] != 21379 {
		t.Fatalf("new redis should lift: %#v", res.Alloc.Ports)
	}
}

func TestReconcileSecondModeGetsDistinctPorts(t *testing.T) {
	f := NewFile()
	first, _, err := EnsureModeAllocation(f, "c:/src/app", "app", "app", "test", ModeAllocation{
		Ports: map[string]int{"web": 3000, "redis": 6379},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := EnsureModeAllocation(f, "c:/src/app", "app", "app", "prod-db", ModeAllocation{
		Ports: map[string]int{"web": 3000, "redis": 6379},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ports["web"] == second.Ports["web"] || first.Ports["redis"] == second.Ports["redis"] {
		t.Fatalf("modes must not share host ports: test=%#v prod-db=%#v", first.Ports, second.Ports)
	}
}

func TestStampLastUpSurvivesReconcile(t *testing.T) {
	f := NewFile()
	_, _, err := EnsureModeAllocation(f, "c:/source/hello", "hello", "hello", "test", ModeAllocation{
		Host:  "hello.test.localhost",
		Ports: map[string]int{"web": 18080},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	StampLastUp(f, "c:/source/hello", "test", "main", "abc1234", "2026-09-04T15:00:00Z")
	res, err := ReconcileModeAllocation(f, "c:/source/hello", "hello", "hello", "test", ModeAllocation{
		Host:  "hello.test.localhost",
		Ports: map[string]int{"web": 18080, "api": 18081},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Alloc.GitBranch != "main" || res.Alloc.GitSHA != "abc1234" || res.Alloc.LastUpAt != "2026-09-04T15:00:00Z" {
		t.Fatalf("lost git stamp: %#v", res.Alloc)
	}
}
