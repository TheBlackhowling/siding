package dockerps

import "testing"

func TestParsePSPorts(t *testing.T) {
	spec := "0.0.0.0:3000->3000/tcp, [::]:3000->3000/tcp, 127.0.0.1:5432->5432/tcp, 0.0.0.0:8000-8002->8000-8002/tcp, 80/tcp"
	got := ParsePSPorts(spec)
	for _, p := range []int{3000, 5432, 8000, 8001, 8002} {
		if _, ok := got[p]; !ok {
			t.Fatalf("missing %d in %#v", p, got)
		}
	}
	if _, ok := got[80]; ok {
		t.Fatalf("unpublished 80/tcp should not count: %#v", got)
	}
}

func TestTakenExcept(t *testing.T) {
	s := Snapshot{Holders: map[int][]string{
		80:   {"siding-proxy-caddy"},
		8000: {"prod-db-dev-backend-1"},
	}}
	got := s.TakenExcept("siding-proxy-caddy")
	if _, ok := got[8000]; !ok {
		t.Fatalf("8000 should be busy: %#v", got)
	}
	if _, ok := got[80]; ok {
		t.Fatalf("own 80 should be ignored: %#v", got)
	}
}
