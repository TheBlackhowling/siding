package dockerps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Snapshot is host binds currently held by running containers.
type Snapshot struct {
	HostPorts   map[int]struct{}
	Holders     map[int][]string // host port → container names
	HostNetwork []string         // container names using network_mode: host
	Unavailable string           // why inspect failed (empty if ok / no docker)
}

type inspectContainer struct {
	Name       string `json:"Name"`
	HostConfig struct {
		NetworkMode  string                          `json:"NetworkMode"`
		PortBindings map[string][]inspectPortBinding `json:"PortBindings"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Ports map[string][]inspectPortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type inspectPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

var hostPublish = regexp.MustCompile(`(\d+)(?:-(\d+))?->`)

// Inspect lists published host ports on running containers. Fail-open callers
// should treat an error as "no extra live ports" so allocation still works
// without Docker.
func Inspect(ctx context.Context) (Snapshot, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
	}
	ids, err := runningIDs(ctx)
	if err != nil {
		return Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}, Unavailable: err.Error()}, err
	}
	snap := Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}}
	if len(ids) == 0 {
		return snap, nil
	}
	args := append([]string{"inspect"}, ids...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}, Unavailable: msg}, fmt.Errorf("docker inspect: %s", msg)
	}
	return snapshotFromInspectJSON(out.Bytes())
}

func snapshotFromInspectJSON(data []byte) (Snapshot, error) {
	var boxes []inspectContainer
	if err := json.Unmarshal(data, &boxes); err != nil {
		msg := "docker inspect json: " + err.Error()
		return Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}, Unavailable: msg}, errors.New(msg)
	}
	snap := Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}}
	for _, c := range boxes {
		name := strings.TrimPrefix(c.Name, "/")
		mode := strings.ToLower(strings.TrimSpace(c.HostConfig.NetworkMode))
		if mode == "host" {
			snap.HostNetwork = append(snap.HostNetwork, name)
		}
		collectBindings(snap.HostPorts, snap.Holders, name, c.HostConfig.PortBindings)
		collectBindings(snap.HostPorts, snap.Holders, name, c.NetworkSettings.Ports)
	}
	return snap, nil
}

func runningIDs(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-q", "--filter", "status=running")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "docker not available"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return splitRunningIDs(out.String()), nil
}

func splitRunningIDs(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func collectBindings(dst map[int]struct{}, holders map[int][]string, name string, bindings map[string][]inspectPortBinding) {
	for _, list := range bindings {
		for _, b := range list {
			spec := strings.TrimSpace(b.HostPort)
			if spec == "" {
				continue
			}
			start, end, ok := parsePortOrRange(spec)
			if !ok {
				continue
			}
			for p := start; p <= end; p++ {
				dst[p] = struct{}{}
				if holders != nil && name != "" {
					holders[p] = appendUnique(holders[p], name)
				}
			}
		}
	}
}

func appendUnique(list []string, name string) []string {
	for _, n := range list {
		if n == name {
			return list
		}
	}
	return append(list, name)
}

func parsePortOrRange(spec string) (start, end int, ok bool) {
	a, b, ranged := strings.Cut(spec, "-")
	start, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil || start < 1 || start > 65535 {
		return 0, 0, false
	}
	end = start
	if ranged {
		end, err = strconv.Atoi(strings.TrimSpace(b))
		if err != nil || end < start || end > 65535 {
			return 0, 0, false
		}
	}
	return start, end, true
}

// ParsePSPorts extracts host binds from a docker ps Ports column (tests + fallback).
func ParsePSPorts(spec string) map[int]struct{} {
	used := map[int]struct{}{}
	for _, m := range hostPublish.FindAllStringSubmatch(spec, -1) {
		start, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		end := start
		if m[2] != "" {
			if n, err := strconv.Atoi(m[2]); err == nil {
				end = n
			}
		}
		if start > end {
			start, end = end, start
		}
		for p := start; p <= end && p <= 65535; p++ {
			if p > 0 {
				used[p] = struct{}{}
			}
		}
	}
	return used
}

// TakenExcept is host ports published by containers other than exceptName.
func (s Snapshot) TakenExcept(exceptName string) map[int]struct{} {
	out := map[int]struct{}{}
	if len(s.Holders) == 0 {
		return s.PortSet()
	}
	exceptName = strings.TrimPrefix(exceptName, "/")
	for p, names := range s.Holders {
		for _, n := range names {
			if strings.TrimPrefix(n, "/") != exceptName {
				out[p] = struct{}{}
				break
			}
		}
	}
	return out
}

// PortSet is a copy of snap.HostPorts (never nil).
func (s Snapshot) PortSet() map[int]struct{} {
	out := map[int]struct{}{}
	for p := range s.HostPorts {
		out[p] = struct{}{}
	}
	return out
}
