package dockerps

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestParsePortOrRangeAndBindings(t *testing.T) {
	start, end, ok := parsePortOrRange("8080")
	if !ok || start != 8080 || end != 8080 {
		t.Fatalf("%d %d %v", start, end, ok)
	}
	start, end, ok = parsePortOrRange("8000-8002")
	if !ok || start != 8000 || end != 8002 {
		t.Fatalf("%d %d %v", start, end, ok)
	}
	if _, _, ok := parsePortOrRange("nope"); ok {
		t.Fatal("nope")
	}
	if _, _, ok := parsePortOrRange("8000-7000"); ok {
		t.Fatal("reversed")
	}
	if _, _, ok := parsePortOrRange("0"); ok {
		t.Fatal("zero")
	}

	dst := map[int]struct{}{}
	holders := map[int][]string{}
	collectBindings(dst, holders, "web", map[string][]inspectPortBinding{
		"80/tcp": {{HostPort: "80"}, {HostPort: ""}, {HostPort: "bad"}},
		"rng":    {{HostPort: "9000-9001"}},
	})
	if _, ok := dst[80]; !ok {
		t.Fatal("80")
	}
	if _, ok := dst[9001]; !ok {
		t.Fatal("9001")
	}
	if holders[80][0] != "web" {
		t.Fatalf("%v", holders)
	}
	if got := appendUnique([]string{"web"}, "web"); len(got) != 1 {
		t.Fatal("dedupe")
	}
	if got := appendUnique([]string{"web"}, "api"); len(got) != 2 {
		t.Fatal("append")
	}
}

func TestPortSetAndInspect(t *testing.T) {
	empty := Snapshot{}
	if empty.PortSet() == nil || len(empty.PortSet()) != 0 {
		t.Fatal("empty")
	}
	s := Snapshot{HostPorts: map[int]struct{}{80: {}}}
	if _, ok := s.PortSet()[80]; !ok {
		t.Fatal("copy")
	}
	if got := s.TakenExcept("x"); len(got) != 1 {
		t.Fatalf("no holders falls back to PortSet: %#v", got)
	}

	testenv.NoHostTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snap, err := Inspect(ctx)
	if err == nil {
		t.Fatal("no docker on PATH must be an error")
	}
	if snap.Unavailable == "" {
		t.Fatal("error should set Unavailable")
	}
	if snap.HostPorts == nil || snap.Holders == nil || len(snap.PortSet()) != 0 {
		t.Fatalf("unavailable snapshot must stay usable and empty: %#v", snap)
	}
}

func TestSnapshotFromInspectJSON(t *testing.T) {
	data := []byte(`[
	  {
	    "Name": "/siding-proxy-caddy",
	    "HostConfig": {
	      "NetworkMode": "bridge",
	      "PortBindings": {"80/tcp": [{"HostIp": "127.0.0.1", "HostPort": "80"}]}
	    },
	    "NetworkSettings": {"Ports": {"80/tcp": [{"HostPort": "80"}]}}
	  },
	  {
	    "Name": "/hosty",
	    "HostConfig": {"NetworkMode": "host", "PortBindings": {}},
	    "NetworkSettings": {"Ports": {}}
	  }
	]`)
	snap, err := snapshotFromInspectJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.HostPorts[80]; !ok {
		t.Fatalf("%#v", snap)
	}
	if len(snap.HostNetwork) != 1 || snap.HostNetwork[0] != "hosty" {
		t.Fatalf("%v", snap.HostNetwork)
	}
	// Bad JSON is reported the same way as the other two failure paths:
	// error returned, Unavailable set, empty (non-nil) maps for fail-open callers.
	bad, err := snapshotFromInspectJSON([]byte("{"))
	if err == nil || !strings.Contains(err.Error(), "docker inspect json") {
		t.Fatalf("bad json err=%v", err)
	}
	if bad.Unavailable != err.Error() || bad.HostPorts == nil || bad.Holders == nil || len(bad.HostPorts) != 0 {
		t.Fatalf("bad json snapshot %#v", bad)
	}
	if got := splitRunningIDs("abc\n\n  def  \n"); len(got) != 2 || got[0] != "abc" || got[1] != "def" {
		t.Fatalf("%v", got)
	}
	if got := ParsePSPorts("0.0.0.0:8002-8000->8000-8002/tcp"); len(got) != 3 {
		t.Fatalf("reversed range %#v", got)
	}
}
