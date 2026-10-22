package hostport

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/TheBlackhowling/siding/internal/dockerps"
)

type Occupant struct {
	Source string // docker, host, windows
	Name   string
	PID    int
	Detail string
}

type Report struct {
	Port      int
	Occupants []Occupant
}

func Lookup(ctx context.Context, port int) Report {
	rep := Report{Port: port}
	if port < 1 || port > 65535 {
		return rep
	}
	if snap, err := dockerps.Inspect(ctx); err == nil {
		for _, name := range snap.Holders[port] {
			rep.Occupants = append(rep.Occupants, Occupant{
				Source: "docker",
				Name:   name,
				Detail: "published host port",
			})
		}
	}
	rep.Occupants = append(rep.Occupants, osOccupants(ctx, port)...)
	if runtime.GOOS == "windows" {
		rep.Occupants = append(rep.Occupants, windowsExcluded(ctx, port)...)
	}
	rep.Occupants = dedupe(rep.Occupants)
	return rep
}

// Blocking is occupants that are not our own proxy. If siding-proxy-caddy
// already publishes this port, host-side docker-proxy / WinNAT noise is ignored.
func (r Report) Blocking(exceptDocker ...string) []Occupant {
	skip := map[string]struct{}{}
	for _, n := range exceptDocker {
		n = strings.TrimPrefix(strings.TrimSpace(n), "/")
		if n != "" {
			skip[n] = struct{}{}
		}
	}
	if r.heldByDocker(skip) {
		return nil
	}
	var out []Occupant
	for _, o := range r.Occupants {
		if o.Source == "docker" {
			if _, ok := skip[strings.TrimPrefix(o.Name, "/")]; ok {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

func (r Report) heldByDocker(skip map[string]struct{}) bool {
	for _, o := range r.Occupants {
		if o.Source != "docker" {
			continue
		}
		if _, ok := skip[strings.TrimPrefix(o.Name, "/")]; ok {
			return true
		}
	}
	return false
}

func FormatOccupants(occ []Occupant) []string {
	var lines []string
	for _, o := range occ {
		switch {
		case o.Source == "docker":
			lines = append(lines, fmt.Sprintf("docker: %s", o.Name))
		case o.PID > 0 && o.Name != "":
			lines = append(lines, fmt.Sprintf("host: %s (pid %d)", o.Name, o.PID))
		case o.PID > 0:
			lines = append(lines, fmt.Sprintf("host: pid %d", o.PID))
		case o.Name != "" && o.Detail != "":
			lines = append(lines, fmt.Sprintf("%s: %s — %s", o.Source, o.Name, o.Detail))
		case o.Name != "":
			lines = append(lines, fmt.Sprintf("%s: %s", o.Source, o.Name))
		default:
			lines = append(lines, fmt.Sprintf("port in use (%s)", o.Source))
		}
	}
	return lines
}

func BusyError(port int, occ []Occupant) error {
	if len(occ) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "host :%d is in use — the siding proxy needs that port\n", port)
	for _, line := range FormatOccupants(occ) {
		fmt.Fprintf(&b, "  • %s\n", line)
	}
	fmt.Fprintf(&b, "stop that process (or container), then retry")
	return fmt.Errorf("%s", strings.TrimSuffix(b.String(), "\n"))
}

func dedupe(in []Occupant) []Occupant {
	seen := map[string]struct{}{}
	var out []Occupant
	for _, o := range in {
		key := o.Source + "\t" + o.Name + "\t" + fmt.Sprintf("%d", o.PID) + "\t" + o.Detail
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].PID < out[j].PID
	})
	return out
}
