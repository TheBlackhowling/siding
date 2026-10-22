package doctor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/internal/version"
)

type Report struct {
	Checks []Check
	Tips   []string
	OK     bool
}

type Check struct {
	Name    string
	Status  string // pass, warn, fail
	Message string
}

func Run(ctx context.Context, startPath, stateOverride string) Report {
	var checks []Check
	pass := func(name, msg string) {
		checks = append(checks, Check{Name: name, Status: "pass", Message: msg})
	}
	warn := func(name, msg string) {
		checks = append(checks, Check{Name: name, Status: "warn", Message: msg})
	}
	fail := func(name, msg string) {
		checks = append(checks, Check{Name: name, Status: "fail", Message: msg})
	}

	info := version.Info()
	pass("version", fmt.Sprintf("%s (commit %s, built %s)", info["version"], info["commit"], info["date"]))

	start, err := paths.Abs(startPath)
	if err != nil {
		fail("start_path", err.Error())
		return finalize(checks, nil)
	}
	pass("start_path", start)

	repoRoot, err := contract.FindRepoRoot(start)
	if err != nil {
		fail("repo_root", err.Error())
		warn("repo_hint", "run from a repo checkout or pass --path; siding init writes .siding/config.yml")
	} else {
		pass("repo_root", repoRoot)
		checkoutKey := paths.NormalizeLockPath(repoRoot)
		pass("checkout_key", checkoutKey)

		if _, err := contract.Load(repoRoot); err != nil {
			fail("contract", err.Error())
		} else {
			pass("contract", contract.FileName)
		}
	}

	statePaths, err := paths.ResolvePaths(stateOverride)
	if err != nil {
		fail("state_paths", err.Error())
	} else {
		pass("user_data", statePaths.UserDataRoot)
		pass("state_file", statePaths.StateFile)
		if err := os.MkdirAll(statePaths.UserDataRoot, 0o755); err != nil {
			fail("state_writable", err.Error())
		} else {
			pass("state_writable", statePaths.UserDataRoot)
		}
		store := state.NewStore(statePaths)
		if _, err := os.Stat(statePaths.StateFile); os.IsNotExist(err) {
			warn("state", fmt.Sprintf("no %s yet (created on first up or add)", statePaths.StateFile))
		} else if err != nil {
			fail("state", err.Error())
		} else if _, err := store.Read(); err != nil {
			fail("state", err.Error())
		} else {
			pass("state", statePaths.StateFile)
		}
	}

	var tips []string
	var resolved *resolver.Result
	if res, err := resolver.Resolve(resolver.Options{
		StartPath:     start,
		StateOverride: stateOverride,
	}); err != nil {
		warn("resolve", err.Error())
	} else {
		resolved = res
		pass("resolve", "stack/mode resolves from checkout path + contract + state")
		if statePaths, err := paths.ResolvePaths(stateOverride); err == nil {
			store := state.NewStore(statePaths)
			if st, err := store.Read(); err == nil {
				if _, ok := store.ModeAllocation(st, res.CheckoutKey, res.Mode); ok {
					pass("local", "org recipe is mapped onto this checkout in state.json")
				} else {
					warn("local", "org recipe is not mapped onto this machine yet — run siding sync")
				}
			}
		}
		advice := advise.ScanRepo(res.RepoRoot, res.ComposeFiles, res.Contract, res.Mode)
		tips = advice.Tips
		applyScanFindings(advice.Findings, pass, warn, fail)
	}

	if msg, ok := dockerCompose(ctx); !ok {
		fail("docker_compose", msg)
	} else {
		pass("docker_compose", msg)
	}

	snap, snapErr := dockerps.Inspect(ctx)
	applyDockerSnapshot(snap, snapErr, resolved, pass, warn)
	applyProxyCheck(ctx, stateOverride, pass, warn)

	return finalize(checks, tips)
}

func applyScanFindings(findings []advise.Finding, pass, warn, fail func(string, string)) {
	for _, f := range findings {
		switch f.Kind {
		case "port_ok":
			pass("compose_port", f.Detail)
		case "port_hardcoded", "public_url", "missing_port_env", "unowned_port", "host_network", "localhost_internal":
			warn(f.Kind, f.Detail)
		case "compose_missing", "compose_invalid":
			fail("compose_file", f.Detail)
		}
	}
}

func dockerCompose(ctx context.Context) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return composeVersionResult(out.String(), cmd.Run())
}

func composeVersionResult(out string, err error) (string, bool) {
	if err != nil {
		return "docker compose not available (install Docker Desktop and ensure docker is on PATH)", false
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "docker compose available", true
	}
	return line, true
}

func applyDockerSnapshot(snap dockerps.Snapshot, snapErr error, resolved *resolver.Result, pass, warn func(string, string)) {
	if snapErr != nil {
		warn("docker_ports", "could not list running container binds: "+snapErr.Error())
		return
	}
	pass("docker_ports", fmt.Sprintf("%d published host binds on running containers", len(snap.HostPorts)))
	if len(snap.HostNetwork) > 0 {
		warn("docker_host_network", "network_mode=host (published ports hidden): "+strings.Join(snap.HostNetwork, ", "))
	}
	if resolved == nil {
		return
	}
	var hit []string
	keys := make([]string, 0, len(resolved.Ports))
	for k := range resolved.Ports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := resolved.Ports[k]
		if _, ok := snap.HostPorts[p]; ok {
			hit = append(hit, fmt.Sprintf("%s:%d", k, p))
		}
	}
	if len(hit) > 0 {
		warn("port_live", "running containers already publish "+strings.Join(hit, ", ")+" — ok if this stack is up; otherwise stop them before siding up")
	}
}

func applyProxyCheck(ctx context.Context, stateOverride string, pass, warn func(string, string)) {
	if !proxy.InspectRunning(ctx) {
		warn("proxy", "not running — siding proxy up (Caddy on host :80)")
		return
	}
	listen := 80
	if statePaths, err := paths.ResolvePaths(stateOverride); err == nil {
		if st, err := state.NewStore(statePaths).Read(); err == nil {
			listen = state.ListenPort(st)
		}
	}
	pass("proxy", fmt.Sprintf("%s listening on 127.0.0.1:%d", proxy.ContainerName, listen))
}

func finalize(checks []Check, tips []string) Report {
	ok := true
	for _, c := range checks {
		if c.Status == "fail" {
			ok = false
		}
	}
	return Report{Checks: checks, Tips: tips, OK: ok}
}

func FormatText(r Report) string {
	var b strings.Builder
	for _, c := range r.Checks {
		icon := "ok"
		switch c.Status {
		case "fail":
			icon = "FAIL"
		case "warn":
			icon = "warn"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", icon, c.Name, c.Message)
	}
	if len(r.Tips) > 0 {
		b.WriteString("\nTips\n")
		for _, tip := range r.Tips {
			fmt.Fprintf(&b, "  • %s\n", tip)
		}
	}
	if r.OK {
		b.WriteString("\nAll required checks passed.\n")
	} else {
		b.WriteString("\nFix failures above before siding up.\n")
	}
	return b.String()
}
