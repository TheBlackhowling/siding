package cli

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/internal/testenv"
	"github.com/TheBlackhowling/siding/internal/version"
	"github.com/TheBlackhowling/siding/siding"
)

func helloRepo(t *testing.T) string {
	t.Helper()
	return testenv.HelloRepo(t)
}

// addedUserPaths records what `siding install` would have put on the Windows
// user PATH while isolateRoot is active.
var addedUserPaths []string

// isolateRoot gives the test its own state root and fences it off from the
// host: no docker/git/netstat on PATH (compose project and container names are
// fixed, so an unfenced `up`/`proxy up` would touch the real stacks) and no
// registry writes.
func isolateRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIDING_ROOT", dir)
	t.Setenv("SIDING_DATA", dir)
	testenv.NoHostTools(t)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("SIDING_NO_UPDATE_CHECK", "1")
	setTTY(t, false) // CLI tests are non-interactive unless they opt in
	old := globalRoot
	globalRoot = dir
	oldAdd := addUserPath
	addedUserPaths = nil
	addUserPath = func(d string) error {
		addedUserPaths = append(addedUserPaths, d)
		return nil
	}
	t.Cleanup(func() {
		globalRoot = old
		addUserPath = oldAdd
	})
	return dir
}

// setTTY pins the wizard's interactive check for the test. Under `go test`
// stdin is the null device, which Stat reports as a character device, so the
// real check would otherwise say "interactive" on every CI runner.
func setTTY(t *testing.T, tty bool) {
	t.Helper()
	old := isTTY
	isTTY = func() bool { return tty }
	t.Cleanup(func() { isTTY = old })
}

// stdinLines feeds canned wizard answers for the test.
func stdinLines(t *testing.T, lines string) {
	t.Helper()
	old := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader(lines))
	t.Cleanup(func() { stdinReader = old })
}

func execCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	cmd := rootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestInstallCommand(t *testing.T) {
	isolateRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SIDING_DATA", home)
	t.Setenv("LOCALAPPDATA", home)
	out, err := execCLI(t, "install")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	dir, err := paths.CommandBinDir()
	if err != nil {
		t.Fatal(err)
	}
	name := "siding"
	if runtime.GOOS == "windows" {
		name = "siding.exe"
	}
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("binary not installed at %s: %v", dest, err)
	}
	if runtime.GOOS == "windows" {
		if len(addedUserPaths) != 1 || addedUserPaths[0] != filepath.Dir(dest) {
			t.Fatalf("user PATH update %v want %s", addedUserPaths, filepath.Dir(dest))
		}
	} else if len(addedUserPaths) != 0 {
		t.Fatalf("no PATH edit off Windows: %v", addedUserPaths)
	}
	if !strings.Contains(out, "installed "+dest) {
		t.Fatalf("install output missing dest:\n%s", out)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(out, "user PATH: updated") || !strings.Contains(out, "$env:Path") {
			t.Fatalf("windows install help:\n%s", out)
		}
	} else if !strings.Contains(out, "ensure the directory is on PATH") {
		t.Fatalf("unix install help:\n%s", out)
	}
}

func TestExecuteVersionAndHelp(t *testing.T) {
	isolateRoot(t)
	out, err := execCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"siding " + version.Version, "commit: " + version.Commit, "built: ", "go: "} {
		if !strings.Contains(out, want) {
			t.Fatalf("version output missing %q:\n%s", want, out)
		}
	}
	out, err = execCLI(t, "help")
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"init", "sync", "up", "down", "worktree", "proxy", "remove", "doctor", "update", "install"} {
		if !strings.Contains(out, "\n  "+sub) {
			t.Fatalf("help missing subcommand %q:\n%s", sub, out)
		}
	}
	if _, err := execCLI(t, "help", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := execCLI(t, "help", "proxy"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanDoctorSyncAddHello(t *testing.T) {
	root := isolateRoot(t)
	hello := helloRepo(t)

	out, err := execCLI(t, "--root", root, "plan", "--path", hello)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, want := range []string{"stack:     hello (test)", "host:      hello.test.localhost", "ports:", "  web:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan missing %q:\n%s", want, out)
		}
	}
	out, err = execCLI(t, "--root", root, "plan", "--path", hello, "--json")
	if err != nil {
		t.Fatalf("plan json: %v", err)
	}
	for _, want := range []string{`"stack": "hello"`, `"mode": "test"`, `"host":`} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan json missing %q:\n%s", want, out)
		}
	}
	out, _ = execCLI(t, "--root", root, "doctor", "--path", hello)
	if !strings.Contains(out, "[FAIL] docker_compose:") {
		t.Fatalf("doctor without docker must report docker_compose fail:\n%s", out)
	}
	out, _ = execCLI(t, "--root", root, "doctor", "--path", hello, "--json")
	if !strings.Contains(out, `"Name": "docker_compose"`) {
		t.Fatalf("doctor json:\n%s", out)
	}
	out, err = execCLI(t, "--root", root, "sync", "--path", hello, "--dry-run")
	if err != nil {
		t.Fatalf("sync dry: %v", err)
	}
	if !strings.Contains(out, "env file: (dry run — not written)") || !strings.Contains(out, "stack:    hello (test)") {
		t.Fatalf("sync dry:\n%s", out)
	}
	out, err = execCLI(t, "--root", root, "sync", "--path", hello, "--json", "--print-hosts")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(out, `"created": true`) || !strings.Contains(out, `"stack": "hello"`) {
		t.Fatalf("sync json:\n%s", out)
	}
	out, err = execCLI(t, "--root", root, "add", "--path", hello, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("add dry: %v", err)
	}
	if !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("add dry json:\n%s", out)
	}
	out, err = execCLI(t, "--root", root, "add", "--path", hello)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "already allocated") && !strings.Contains(out, "allocated and written") {
		t.Fatalf("add:\n%s", out)
	}
}

func TestInitExistingRecipeSyncs(t *testing.T) {
	root := isolateRoot(t)
	hello := helloRepo(t)
	out, err := execCLI(t, "--root", root, "init", "--path", hello, "--rewrite-compose")
	if err != nil {
		t.Fatalf("init existing: %v", err)
	}
	if !strings.Contains(out, "already present — mapping it onto local siding state") {
		t.Fatalf("init existing:\n%s", out)
	}
}

func TestInitNewRepo(t *testing.T) {
	root := isolateRoot(t)
	repo := t.TempDir()
	compose := []byte(`
services:
  web:
    image: nginx:alpine
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_ROOT_DOMAIN: app.example.test
  backend:
    image: alpine
    ports:
      - "8000:8000"
`)
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), compose, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execCLI(t,
		"--root", root,
		"init",
		"--path", repo,
		"--project", "demo",
		"--folder-prefix=",
		"--compose", "docker-compose.yml",
		"--mode-name", "docker-compose.yml=test",
		"--host-prefix", "example",
		"--entry", "web=app",
		"--entry", "api=api",
		"--prefer-proxy=false",
		"--rewrite-compose",
		"--print-hosts",
		"--port", "web=3000",
	)
	if err != nil {
		t.Fatalf("init new: %v", err)
	}
	cfg := filepath.Join(repo, ".siding", "config.yml")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "wrote "+cfg) || !strings.Contains(out, "next:  siding doctor && siding up") {
		t.Fatalf("init new:\n%s", out)
	}
}

func TestInitDryRunRewrite(t *testing.T) {
	root := isolateRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"3000:3000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execCLI(t,
		"--root", root,
		"init",
		"--path", repo,
		"--project", "dry",
		"--folder-prefix=",
		"--compose", "docker-compose.yml",
		"--mode-name", "docker-compose.yml=test",
		"--host-prefix", "none",
		"--prefer-proxy=false",
		"--rewrite-compose",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("dry init: %v", err)
	}
	if !strings.Contains(out, "dry run — config and compose not written") {
		t.Fatalf("dry init:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".siding", "config.yml")); err == nil {
		t.Fatal("dry run should not write recipe")
	}
}

// TestStackAndProxyWithoutDocker drives every docker-backed command with docker
// off PATH. Each must fail with a clear message instead of hanging or
// pretending to succeed, and none may leave proxy status behind.
func TestStackAndProxyWithoutDocker(t *testing.T) {
	root := isolateRoot(t)
	hello := helloRepo(t)
	if _, err := execCLI(t, "--root", root, "sync", "--path", hello); err != nil {
		t.Fatalf("sync needs no docker: %v", err)
	}

	// Before any `proxy up` there is no compose file: ps is a friendly no-op.
	out, err := execCLI(t, "--root", root, "proxy", "ps")
	if err != nil {
		t.Fatalf("proxy ps before init: %v", err)
	}
	if !strings.Contains(out, "proxy is not initialized — siding proxy up") {
		t.Fatalf("proxy ps before init:\n%s", out)
	}

	noDocker := "docker not found on PATH"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"up", []string{"up", "--path", hello}, noDocker},
		{"down", []string{"down", "--path", hello}, noDocker},
		{"ps", []string{"ps", "--path", hello}, noDocker},
		{"recycle", []string{"recycle", "--path", hello}, noDocker},
		{"reset", []string{"reset", "--path", hello}, noDocker},
		{"tail", []string{"tail", "--path", hello, "web"}, noDocker},
		{"proxy reload", []string{"proxy", "reload"}, "proxy is not running"},
		{"proxy up", []string{"proxy", "up"}, noDocker},
		{"proxy ps", []string{"proxy", "ps"}, noDocker},
		{"proxy down", []string{"proxy", "down"}, noDocker},
	}
	for _, tc := range cases {
		_, err := execCLI(t, append([]string{"--root", root}, tc.args...)...)
		if err == nil {
			t.Fatalf("%s: expected failure without docker", tc.name)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v (want %q)", tc.name, err, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "siding-proxy", proxy.StatusFileName)); !os.IsNotExist(err) {
		t.Fatalf("failed proxy up must not leave status.json: %v", err)
	}

	out, err = execCLI(t, "--root", root, "remove", "--path", hello, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("remove dry: %v", err)
	}
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, `"Mode": "test"`) {
		t.Fatalf("remove dry json:\n%s", out)
	}
	if _, err := execCLI(t, "--root", root, "remove", "--path", hello); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("remove without --force must stop when compose down fails: %v", err)
	}
	if _, err := execCLI(t, "--root", root, "remove", "--path", hello, "--force"); err != nil {
		t.Fatalf("remove --force: %v", err)
	}
	st, err := mustOpenStore(t, root).Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Checkouts) != 0 {
		t.Fatalf("remove --force must drop the checkout: %#v", st.Checkouts)
	}
}

func TestPrintHelpers(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	old := stdOut
	stdOut = &buf
	t.Cleanup(func() { stdOut = old })

	printPorts(map[string]int{"web": 1, "api": 2})
	if !strings.Contains(buf.String(), "  api: 2\n  web: 1\n") {
		t.Fatalf("ports sorted:\n%s", buf.String())
	}
	printLocalhostHostsNote()
	if !strings.Contains(buf.String(), "*.localhost is resolved by the OS") {
		t.Fatalf("hosts note:\n%s", buf.String())
	}
	if got := publicURL(&resolver.Result{Host: "hello.test.localhost", Ports: map[string]int{"web": 18080}}); got != "http://hello.test.localhost:18080" {
		t.Fatalf("publicURL %q", got)
	}

	buf.Reset()
	if err := printProxyResultFromLibrary(siding.ProxyConfig{
		Listen:   80,
		BoardURL: proxy.SiteAddress(proxy.BoardHost, 80),
		AdminURL: proxy.AdminURL(),
		Routes:   []siding.ProxyRoute{{Host: "a.localhost"}},
	}, false, "up"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"proxy:     http://127.0.0.1:80", "board:     http://siding.localhost", "hosts:     a.localhost"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("proxy :80 missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	if err := printProxyResultFromLibrary(siding.ProxyConfig{
		Listen:           1355,
		SkippedPublishes: []int{2019},
		CaddyfilePath:    "x",
		AdminURL:         proxy.AdminURL(),
	}, true, "reload"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"action": "reload"`) || !strings.Contains(buf.String(), `"listen": 1355`) {
		t.Fatalf("proxy json:\n%s", buf.String())
	}

	buf.Reset()
	if err := printRemove("c:/a", "c:/a/repo", []removeRow{{Mode: "test", Host: "h", ComposeProject: "p", Ports: map[string]int{"web": 1}}}, removeOpts{dryRun: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"checkout: c:/a", "repo:     c:/a/repo", "  web: 1", "dry run — state not written"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("remove missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	if err := printRemove("c:/a", "c:/a", []removeRow{{Mode: "test"}}, removeOpts{jsonOut: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"checkout": "c:/a"`) || !strings.Contains(buf.String(), `"Mode": "test"`) {
		t.Fatalf("remove json:\n%s", buf.String())
	}

	buf.Reset()
	printSyncRowFromLibrary(siding.SyncMode{
		Resolved: siding.Resolved{
			RepoRoot:       "r",
			CheckoutKey:    "c",
			Stack:          "s",
			Mode:           "test",
			Host:           "h.localhost",
			ComposeProject: "p",
			Ports:          map[string]int{"web": 1},
			SlotEnvPath:    "e",
			StateFile:      "st",
		},
		Created: true,
		Tips:    []string{"tip"},
	}, inspectDockerPorts(context.Background()), syncOpts{printHosts: true, dryRun: true})
	for _, want := range []string{"stack:    s (test)", "local:    allocated", "env file: (dry run — not written)", "  - tip"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("sync created missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	printSyncRowFromLibrary(siding.SyncMode{
		Resolved: siding.Resolved{
			Host:        "h.localhost",
			Ports:       map[string]int{"web": 1},
			SlotEnvPath: "e",
			StateFile:   "st",
		},
		AddedPorts:  []string{"api"},
		ProxyActive: true,
		ProxyListen: 80,
	}, dockerps.Snapshot{HostPorts: map[int]struct{}{1: {}}, HostNetwork: []string{"n"}}, syncOpts{})
	for _, want := range []string{"merged new ports [api]", "docker:   1 published host ports", "host-network containers", "board:    http://siding.localhost"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("sync merge missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	printSyncRowFromLibrary(siding.SyncMode{
		Resolved: siding.Resolved{Host: "h.localhost", Ports: map[string]int{"web": 1}},
	}, dockerps.Snapshot{Unavailable: "no docker"}, syncOpts{dryRun: true})
	if !strings.Contains(buf.String(), "already allocated (host/env refreshed)") || strings.Contains(buf.String(), "docker:") {
		t.Fatalf("unavailable docker should skip docker lines:\n%s", buf.String())
	}
	got := syncJSONFromLibrary(siding.SyncMode{Resolved: siding.Resolved{Mode: "test", Ports: map[string]int{}}}, inspectDockerPorts(context.Background()), true)
	if got["mode"] != "test" || got["dry_run"] != true {
		t.Fatalf("syncJSONFromLibrary %#v", got)
	}
}

func TestLiveDockerAndAttachWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	ctx := context.Background()
	if live := liveDockerPorts(ctx); len(live) != 0 {
		t.Fatalf("no docker means no live ports: %v", live)
	}
	snap := inspectDockerPorts(ctx)
	if snap.Unavailable == "" || snap.HostPorts == nil || snap.Holders == nil {
		t.Fatalf("unavailable snapshot must say so and keep maps usable: %#v", snap)
	}
	res := &resolver.Result{}
	attachProxy(ctx, res, state.NewFile())
	if res.ProxyActive || res.ProxyListen != 0 {
		t.Fatalf("proxy cannot be active without docker: %#v", res)
	}
}
