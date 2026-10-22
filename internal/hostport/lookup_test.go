package hostport

import (
	"strings"
	"testing"
)

func TestParseNetstatPIDs(t *testing.T) {
	body := `
  TCP    0.0.0.0:80             0.0.0.0:0              LISTENING       4
  TCP    [::]:80                [::]:0                 LISTENING       4
  TCP    0.0.0.0:8080           0.0.0.0:0              LISTENING       99
  TCP    127.0.0.1:1355         0.0.0.0:0              LISTENING       12
`
	pids := parseNetstatPIDs(body, 80)
	if len(pids) != 2 || pids[0] != 4 || pids[1] != 4 {
		t.Fatalf("pids=%v", pids)
	}
	if got := parseNetstatPIDs(body, 8080); len(got) != 1 || got[0] != 99 {
		t.Fatalf("8080=%v", got)
	}
}

func TestParseSSAndLsof(t *testing.T) {
	compact := `LISTEN 0 128 0.0.0.0:80 0.0.0.0:* users:(("caddy",pid=42,fd=8))
LISTEN 0 128 0.0.0.0:8080 0.0.0.0:* users:(("other",pid=9,fd=8))`
	occ := parseSS(compact, 80)
	if len(occ) != 1 || occ[0].Name != "caddy" || occ[0].PID != 42 {
		t.Fatalf("compact ss %#v", occ)
	}
	plain := parseSS("LISTEN 0 128 127.0.0.1:80 0.0.0.0:*\n", 80)
	if len(plain) != 1 || plain[0].Detail == "" {
		t.Fatalf("ss without users= %#v", plain)
	}

	aligned := `State  Recv-Q Send-Q Local Address:Port Peer Address:Port
LISTEN 0      4096         0.0.0.0:80        0.0.0.0:*    users:(("caddy",pid=321,fd=7))
LISTEN 0      4096         0.0.0.0:8080      0.0.0.0:*    users:(("node",pid=9,fd=3))
`
	occ = parseSS(aligned, 80)
	if len(occ) != 1 || occ[0].Name != "caddy" || occ[0].PID != 321 {
		t.Fatalf("aligned ss %#v", occ)
	}

	lsof := `COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
nginx     88 root    6u  IPv4  12345      0t0  TCP *:80 (LISTEN)
nginx     88 root    7u  IPv4  12346      0t0  TCP *:8080 (LISTEN)
caddy     7 nobody   3u  IPv4  1          0t0  TCP *:443 (LISTEN)
`
	occ = parseLsof(lsof, 80)
	if len(occ) != 1 || occ[0].Name != "nginx" || occ[0].PID != 88 {
		t.Fatalf("lsof %#v", occ)
	}
	if got := parseLsof(lsof, 443); len(got) != 1 || got[0].Name != "caddy" || got[0].PID != 7 {
		t.Fatalf("lsof 443 %#v", got)
	}
}

func TestBlockingSkipsOwnProxy(t *testing.T) {
	ours := Report{Port: 80, Occupants: []Occupant{
		{Source: "docker", Name: "siding-proxy-caddy"},
		{Source: "host", Name: "com.docker.backend", PID: 10},
		{Source: "windows", Name: "excluded port range", Detail: "TCP 80-80 reserved"},
	}}
	if got := ours.Blocking("siding-proxy-caddy"); len(got) != 0 {
		t.Fatalf("own caddy on :80 must not block: %#v", got)
	}

	other := Report{Port: 80, Occupants: []Occupant{
		{Source: "docker", Name: "nginx"},
		{Source: "host", Name: "httpd.exe", PID: 10},
	}}
	if got := other.Blocking("siding-proxy-caddy"); len(got) != 2 {
		t.Fatalf("other occupants should still block: %#v", got)
	}
}

func TestFormatAndBusyError(t *testing.T) {
	occ := []Occupant{
		{Source: "docker", Name: "nginx"},
		{Source: "host", Name: "httpd.exe", PID: 44},
		{Source: "windows", Name: "excluded port range", Detail: "TCP 80-80 reserved"},
	}
	lines := FormatOccupants(occ)
	if len(lines) != 3 || !strings.Contains(lines[0], "docker: nginx") || !strings.Contains(lines[1], "pid 44") {
		t.Fatalf("lines=%v", lines)
	}
	err := BusyError(80, occ)
	if err == nil || !strings.Contains(err.Error(), "host :80") || !strings.Contains(err.Error(), "nginx") {
		t.Fatalf("err=%v", err)
	}
	if BusyError(80, nil) != nil {
		t.Fatal("empty should be nil")
	}
}

func TestParseExcludedRange(t *testing.T) {
	// Real netsh output: administered exclusions carry a trailing "*" column
	// and the legend line at the bottom must not parse as a range.
	body := `
Protocol tcp Port Exclusion Ranges

Start Port    End Port
----------    --------
        80          80
      1024        1123
     50000       50059     *
      5357        5357

* - Administered port exclusions.
`
	got := map[string]string{}
	for _, m := range exclRange.FindAllStringSubmatch(body, -1) {
		got[m[1]] = m[2]
	}
	want := map[string]string{"80": "80", "1024": "1123", "50000": "50059", "5357": "5357"}
	if len(got) != len(want) {
		t.Fatalf("ranges %#v want %#v", got, want)
	}
	for start, end := range want {
		if got[start] != end {
			t.Fatalf("%s-%s missing from %#v", start, end, got)
		}
	}
	if exclRange.MatchString("Start Port    End Port") || exclRange.MatchString("* - Administered port exclusions.") {
		t.Fatal("header/legend must not parse as a range")
	}
}
