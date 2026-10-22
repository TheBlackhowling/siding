package hostport

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	netstatListen = regexp.MustCompile(`(?i)\sTCP\s+(\S+):(\d+)\s+\S+\s+LISTENING\s+(\d+)`)
	ssListen      = regexp.MustCompile(`(?i):(\d+)\s+\S+.*?users:\(\("([^"]+)",pid=(\d+)`)
	lsofListen    = regexp.MustCompile(`(?m)^(\S+)\s+(\d+)\s+`)
	tasklistCSV   = regexp.MustCompile(`(?m)^"([^"]+)","(\d+)"`)
	// netsh marks administered exclusions with a trailing "*" column.
	exclRange = regexp.MustCompile(`(?m)^\s*(\d+)\s+(\d+)\s*\*?\s*$`)
)

func osOccupants(ctx context.Context, port int) []Occupant {
	switch runtime.GOOS {
	case "windows":
		return windowsOccupants(ctx, port)
	default:
		return unixOccupants(ctx, port)
	}
}

func windowsOccupants(ctx context.Context, port int) []Occupant {
	out, err := run(ctx, "netstat", "-ano", "-p", "tcp")
	if err != nil {
		return nil
	}
	var occ []Occupant
	seen := map[int]struct{}{}
	for _, pid := range parseNetstatPIDs(out, port) {
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		name := windowsProcessName(ctx, pid)
		occ = append(occ, Occupant{Source: "host", Name: name, PID: pid})
	}
	return occ
}

func parseNetstatPIDs(body string, port int) []int {
	var pids []int
	for _, m := range netstatListen.FindAllStringSubmatch(body, -1) {
		p, err := strconv.Atoi(m[2])
		if err != nil || p != port {
			continue
		}
		pid, err := strconv.Atoi(m[3])
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

// windowsProcessName resolves a PID via tasklist. When the filter matches
// nothing tasklist prints an INFO line (not CSV) and exits 0, so a non-match
// is "" rather than a retry with a different format.
func windowsProcessName(ctx context.Context, pid int) string {
	out, err := run(ctx, "tasklist", "/FO", "CSV", "/NH", "/FI", fmt.Sprintf("PID eq %d", pid))
	if err != nil {
		return ""
	}
	for _, m := range tasklistCSV.FindAllStringSubmatch(out, -1) {
		if n, _ := strconv.Atoi(m[2]); n == pid {
			return m[1]
		}
	}
	return ""
}

func windowsExcluded(ctx context.Context, port int) []Occupant {
	out, err := run(ctx, "netsh", "interface", "ipv4", "show", "excludedportrange", "protocol=tcp")
	if err != nil {
		return nil
	}
	for _, m := range exclRange.FindAllStringSubmatch(out, -1) {
		start, err1 := strconv.Atoi(m[1])
		end, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil || port < start || port > end {
			continue
		}
		detail := fmt.Sprintf("TCP %d-%d reserved (Hyper-V / WinNAT often holds :80)", start, end)
		return []Occupant{{Source: "windows", Name: "excluded port range", Detail: detail}}
	}
	return nil
}

func unixOccupants(ctx context.Context, port int) []Occupant {
	if out, err := run(ctx, "ss", "-ltnp"); err == nil {
		if occ := parseSS(out, port); len(occ) > 0 {
			return occ
		}
	}
	if out, err := run(ctx, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN"); err == nil {
		return parseLsof(out, port)
	}
	return nil
}

func parseSS(body string, port int) []Occupant {
	var occ []Occupant
	want := fmt.Sprintf(":%d", port)
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, want) || !strings.Contains(strings.ToUpper(line), "LISTEN") {
			continue
		}
		// Avoid :8080 matching :80 — require :port as a token end.
		if !portToken(line, port) {
			continue
		}
		if m := ssListen.FindStringSubmatch(line); m != nil {
			p, _ := strconv.Atoi(m[1])
			if p != port {
				continue
			}
			pid, _ := strconv.Atoi(m[3])
			occ = append(occ, Occupant{Source: "host", Name: m[2], PID: pid})
			continue
		}
		occ = append(occ, Occupant{Source: "host", Detail: strings.TrimSpace(line)})
	}
	return occ
}

func parseLsof(body string, port int) []Occupant {
	var occ []Occupant
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "COMMAND") || !portToken(line, port) {
			continue
		}
		m := lsofListen.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pid, _ := strconv.Atoi(m[2])
		occ = append(occ, Occupant{Source: "host", Name: m[1], PID: pid})
	}
	return occ
}

var portTokenCache sync.Map

func portToken(line string, port int) bool {
	v, ok := portTokenCache.Load(port)
	if !ok {
		re := regexp.MustCompile(fmt.Sprintf(`:%d(?:\s|$|])`, port))
		v, _ = portTokenCache.LoadOrStore(port, re)
	}
	return v.(*regexp.Regexp).MatchString(line)
}

var cmdRun = execRun

func run(ctx context.Context, name string, args ...string) (string, error) {
	return cmdRun(ctx, name, args...)
}

func execRun(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
