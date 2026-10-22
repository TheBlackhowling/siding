package cli

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/hostport"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/internal/testenv"
)

func mustOpenStore(t *testing.T, root string) *state.Store {
	t.Helper()
	p, err := paths.ResolvePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	return state.NewStore(p)
}

func TestParseFlags(t *testing.T) {
	ports, env, err := parsePortFlags([]string{"web=3000", "api=8000"})
	if err != nil || ports["web"] != 3000 || env["api"] != "API_PORT" {
		t.Fatalf("%v %v %v", ports, env, err)
	}
	if _, _, err := parsePortFlags([]string{"bad"}); err == nil {
		t.Fatal("port flag")
	}
	if _, _, err := parsePortFlags([]string{"web=x"}); err == nil {
		t.Fatal("port number")
	}
	if _, _, err := parsePortFlags(nil); err != nil {
		t.Fatal(err)
	}

	modes, err := parseModeNameFlags([]string{"docker-compose.yml=test", "docker-compose.prod.yml=prod"})
	if err != nil || modes["docker-compose.yml"] != "test" {
		t.Fatalf("%v %v", modes, err)
	}
	if _, err := parseModeNameFlags([]string{"noscale"}); err == nil {
		t.Fatal("mode-name")
	}
	if _, err := parseModeNameFlags(nil); err != nil {
		t.Fatal(err)
	}

	envFiles, err := parseComposeEnvFileFlags([]string{"prod=docker/.env.prod-db"})
	if err != nil || envFiles["prod"][0] != "docker/.env.prod-db" {
		t.Fatalf("%v %v", envFiles, err)
	}
	if _, err := parseComposeEnvFileFlags([]string{"nopath"}); err == nil {
		t.Fatal("compose-env-file")
	}

	entries, err := parseEntryFlags([]string{"web=app,www", "frontend=app"})
	if err != nil || entries["web"][0] != "app" {
		t.Fatalf("%v %v", entries, err)
	}
	if _, err := parseEntryFlags([]string{"redis=x"}); err == nil {
		t.Fatal("entry")
	}

	if !strings.Contains(composeGuess("docker-compose.override.yml"), "override") {
		t.Fatalf("%q", composeGuess("docker-compose.override.yml"))
	}
	if composeGuess("docker-compose.yml") == "" {
		t.Fatal("guess")
	}
}

func TestDefaultEntryLabels(t *testing.T) {
	c := &contract.Contract{HostPrefix: "example", HTTPHosts: map[string][]string{"web": {"app"}}}
	if got := defaultEntryLabels("web", c); len(got) != 1 || got[0] != "app" {
		t.Fatalf("%v", got)
	}
	if got := defaultEntryLabels("api", c); len(got) != 1 || got[0] != "api" {
		t.Fatalf("%v", got)
	}
	if got := defaultEntryLabels("web", nil); got[0] != "web" {
		t.Fatalf("%v", got)
	}
	c2 := &contract.Contract{EntryPoints: map[string][]string{"web": {"www"}}}
	if defaultEntryLabels("web", c2)[0] != "www" {
		t.Fatal("entries")
	}
	c3 := &contract.Contract{
		Modes: map[string]contract.Mode{"test": {HTTPHosts: map[string][]string{"web": {"app"}}}},
	}
	if defaultEntryLabels("web", c3)[0] != "app" {
		t.Fatal("mode hosts")
	}
	c4 := &contract.Contract{HostPrefix: "acme"}
	if defaultEntryLabels("web", c4)[0] != "app" {
		t.Fatal("prefix default")
	}
	if httpEntryKeysFromContract(nil) != nil {
		t.Fatal("nil contract")
	}
}

func TestPromptAndTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	oldOut := stdOut
	old := stdinReader
	t.Cleanup(func() {
		stdinReader = old
		stdOut = oldOut
	})
	stdOut = &buf
	stdinReader = bufio.NewReader(strings.NewReader("\n"))
	if prompt("x", "def") != "def" {
		t.Fatal("fallback")
	}
	if !strings.Contains(buf.String(), "[default: def]") {
		t.Fatalf("prompt must label the bracket as default: %q", buf.String())
	}
	buf.Reset()
	stdinReader = bufio.NewReader(strings.NewReader("\n"))
	if !promptYesNo("q", true) {
		t.Fatal("default yes")
	}
	if !strings.Contains(buf.String(), "[default: Y]") {
		t.Fatalf("yes/no must label the bracket as default: %q", buf.String())
	}
	stdinReader = bufio.NewReader(strings.NewReader("typed\n"))
	if prompt("x", "") != "typed" {
		t.Fatal("no fallback")
	}
	stdinReader = bufio.NewReader(strings.NewReader("hi\n"))
	if prompt("x", "def") != "hi" {
		t.Fatal("typed")
	}
	stdinReader = bufio.NewReader(strings.NewReader("n\n"))
	if promptYesNo("q", true) {
		t.Fatal("n")
	}
	stdinReader = bufio.NewReader(strings.NewReader("yes\n"))
	if !promptYesNo("q", false) {
		t.Fatal("yes")
	}
	// Under go test stdin is the null device; whatever Stat says, it must not panic.
	_ = stdinIsTTY()
}

func TestCopySameFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sameFile(src, dst) {
		t.Fatal("missing dest")
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if sameFile(src, src) != true {
		t.Fatal("same")
	}
	if err := copyFile("missing", dst); err == nil {
		t.Fatal("missing src")
	}
}

func TestPickHostPrefixAndEntries(t *testing.T) {
	got, err := pickHostPrefix(initOpts{askHTTPPrefix: false, httpPrefixes: []string{"example"}}, false, "")
	if err != nil || got != "example" {
		t.Fatalf("%q %v", got, err)
	}
	got, err = pickHostPrefix(initOpts{askHTTPPrefix: false}, false, "")
	if err != nil || got != "" {
		t.Fatalf("%q %v", got, err)
	}
	got, err = pickHostPrefix(initOpts{askHTTPPrefix: true}, false, "acme")
	if err != nil || got != "acme" {
		t.Fatalf("guess %q %v", got, err)
	}

	c := &contract.Contract{
		HostPrefix: "example",
		HTTPHosts:  map[string][]string{"web": {"app"}},
		Modes:      map[string]contract.Mode{"test": {Ports: map[string]int{"web": 1, "api": 2}}},
	}
	entries, err := pickEntryPoints(initOpts{askEntries: false, entries: []string{"web=www"}}, false, c)
	if err != nil || entries["web"][0] != "www" || entries["api"][0] != "api" {
		t.Fatalf("%v %v", entries, err)
	}
	if got, err := pickEntryPoints(initOpts{}, false, nil); err != nil || got != nil {
		t.Fatalf("empty keys %v %v", got, err)
	}
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	stdinReader = bufio.NewReader(strings.NewReader("none\napi\n"))
	entries, err = pickEntryPoints(initOpts{askEntries: true}, true, c)
	if err != nil || len(entries["web"]) == 0 || entries["api"][0] != "api" {
		t.Fatalf("tty none %v %v", entries, err)
	}
}

func TestExecuteAndInstallSelf(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"siding", "version"}
	t.Cleanup(func() { os.Args = oldArgs })
	if err := Execute(); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SIDING_DATA", home)
	dest, err := installSelf()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	again, err := installSelf()
	if err != nil || again != dest {
		t.Fatalf("same %q %v", again, err)
	}
}

func TestPickComposeAndModesTTY(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	stdinReader = bufio.NewReader(strings.NewReader("all\n"))
	files, err := pickComposeFiles(repo, initOpts{askCompose: true}, true)
	if err != nil || len(files) != 1 {
		t.Fatalf("%v %v", files, err)
	}
	stdinReader = bufio.NewReader(strings.NewReader("prod\n"))
	groups, err := pickModeSuffixes(compose.GroupFiles(files), initOpts{askModes: true}, true)
	if err != nil || len(groups) != 1 || groups[0].Mode != "prod" {
		t.Fatalf("%v %v", groups, err)
	}
	stdinReader = bufio.NewReader(strings.NewReader("example\n"))
	prefix, err := pickHostPrefix(initOpts{askHTTPPrefix: true}, true, "")
	if err != nil || prefix != "example" {
		t.Fatalf("%q %v", prefix, err)
	}
	empty := t.TempDir()
	stdinReader = bufio.NewReader(strings.NewReader("\n"))
	files, err = pickComposeFiles(empty, initOpts{askCompose: true}, true)
	if err != nil || files[0] != "docker-compose.yml" {
		t.Fatalf("default compose %v %v", files, err)
	}
}

func TestRemoveFromStateOnly(t *testing.T) {
	root := isolateRoot(t)
	dir := t.TempDir()
	store := mustOpenStore(t, root)
	st := state.NewFile()
	key := paths.NormalizeLockPath(dir)
	st.Checkouts[key] = state.Checkout{
		Project: "p",
		Stack:   "s",
		Modes:   map[string]state.ModeAllocation{"test": {Host: "h", ComposeProject: "p-s-test", Ports: map[string]int{"web": 1}}},
	}
	cmd := removeCmd()
	cmd.SetContext(context.Background())
	if err := cmd.ParseFlags([]string{"--path", dir}); err != nil {
		t.Fatal(err)
	}
	if err := removeFromState(cmd, store, st, key, st.Checkouts[key], "", removeOpts{dryRun: true, jsonOut: true}); err != nil {
		t.Fatal(err)
	}
	if err := removeFromState(cmd, store, st, key, st.Checkouts[key], "", removeOpts{force: true}); err != nil {
		t.Fatal(err)
	}

	empty := removeCmd()
	empty.SetContext(context.Background())
	if err := empty.ParseFlags([]string{"--path", t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := runRemove(empty, "", "", removeOpts{}); err == nil {
		t.Fatal("expected no recipe/state")
	}
}

func TestDecideProxyPort(t *testing.T) {
	ok, err := decideProxyPort(false, 80, nil)
	if !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	block := []hostport.Occupant{{Source: "host", Name: "nginx", PID: 1}}
	_, err = decideProxyPort(false, 80, block)
	if err == nil {
		t.Fatal("busy")
	}
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })
	stdinReader = bufio.NewReader(strings.NewReader("y\n"))
	if _, err := decideProxyPort(true, 80, block); err == nil {
		t.Fatal("stop")
	}
	stdinReader = bufio.NewReader(strings.NewReader("n\n"))
	ok, err = decideProxyPort(true, 80, block)
	if ok || err != nil {
		t.Fatalf("continue %v %v", ok, err)
	}
}

func TestRunInitTTYPrompts(t *testing.T) {
	testenv.NoHostTools(t)
	setTTY(t, true)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"3000:3000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// proxy? n · compose copies? n · host prefix: none · web entry: web
	stdinLines(t, "n\nn\nnone\nweb\n")
	c, err := runInit(repo, initOpts{
		project:        "demo",
		composeFiles:   []string{"docker-compose.yml"},
		askCompose:     false,
		askModes:       false,
		askPrefix:      true,
		askProxy:       true,
		askRewrite:     true,
		askHTTPPrefix:  true,
		askEntries:     true,
		preferProxy:    true,
		rewriteCompose: false,
		dryRun:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != "demo" {
		t.Fatalf("%q", c.Project)
	}
	if c.HostPrefix != "" || c.Modes["test"].Ports["web"] != 3000 {
		t.Fatalf("answers not applied: %#v", c)
	}

	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "docker-compose.yml"), []byte("services:\n  worker:\n    image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdinLines(t, "19000\n")
	c, err = runInit(empty, initOpts{
		project:      "empty",
		composeFiles: []string{"docker-compose.yml"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Modes["test"].Ports["web"] != 19000 {
		t.Fatalf("typed default web port must be used: %#v", c.Modes["test"].Ports)
	}

	// Non-interactive: same repo, no prompt, falls back to 18080.
	setTTY(t, false)
	c, err = runInit(empty, initOpts{project: "empty", composeFiles: []string{"docker-compose.yml"}, dryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.Modes["test"].Ports["web"] != 18080 {
		t.Fatalf("non-tty default web port: %#v", c.Modes["test"].Ports)
	}
}

func TestRunInitNonInteractiveSkipsGuessedEnvFiles(t *testing.T) {
	testenv.NoHostTools(t)
	setTTY(t, false)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := "services:\n  web:\n    image: nginx\n    env_file:\n      - docker/.env.prod-db\n"
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.prod-db.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docker", ".env.prod-db"), []byte("GCP_ADC_FILE=/tmp/adc.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	oldErr := stdErr
	stdErr = &warn
	t.Cleanup(func() { stdErr = oldErr })
	c, err := runInit(repo, initOpts{
		project:      "acme",
		composeFiles: []string{"docker-compose.prod-db.yml"},
		modeNames:    []string{"docker-compose.prod-db.yml=prod"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Modes["prod"].ComposeEnvFiles; len(got) != 0 {
		t.Fatalf("non-interactive must not guess: %#v", got)
	}
	if !strings.Contains(warn.String(), "docker/.env.prod-db") || !strings.Contains(warn.String(), "--compose-env-file") {
		t.Fatalf("missing warning:\n%s", warn.String())
	}

	both := t.TempDir()
	if err := os.Mkdir(filepath.Join(both, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(both, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(both, "docker-compose.prod-db.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(both, "docker", ".env.prod-db"), []byte("GCP_ADC_FILE=/tmp/adc.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warn.Reset()
	c, err = runInit(both, initOpts{
		project:      "acme",
		composeFiles: []string{"docker-compose.yml", "docker-compose.prod-db.yml"},
		modeNames:    []string{"docker-compose.yml=test", "docker-compose.prod-db.yml=prod"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Modes["test"].ComposeEnvFiles; len(got) != 0 {
		t.Fatalf("test must stay empty: %#v", got)
	}
	if strings.Contains(warn.String(), "mode test") {
		t.Fatalf("test must not warn about prod env files:\n%s", warn.String())
	}

	c, err = runInit(repo, initOpts{
		project:         "acme",
		composeFiles:    []string{"docker-compose.prod-db.yml"},
		modeNames:       []string{"docker-compose.prod-db.yml=prod"},
		composeEnvFiles: []string{"prod=docker/.env.prod-db"},
		preferProxy:     false,
		dryRun:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := c.Modes["prod"].ComposeEnvFiles
	if len(got) != 1 || got[0].Path != "docker/.env.prod-db" || got[0].Optional {
		t.Fatalf("flag %#v", got)
	}
}

func TestRunInitTTYPicksComposeEnvFilesByNumber(t *testing.T) {
	testenv.NoHostTools(t)
	setTTY(t, true)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := "services:\n  web:\n    image: nginx\n    env_file:\n      - docker/.env.prod-db\n"
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.prod-db.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docker", ".env.prod"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docker", ".env.prod-db"), []byte("B=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdinLines(t, "2,1\n\n\n")
	c, err := runInit(repo, initOpts{
		project:        "acme",
		composeFiles:   []string{"docker-compose.prod-db.yml"},
		modeNames:      []string{"docker-compose.prod-db.yml=prod"},
		preferProxy:    false,
		dryRun:         true,
		askComposeEnv:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := c.Modes["prod"].ComposeEnvFiles
	if len(got) != 2 || got[0].Path != "docker/.env.prod" || got[1].Path != "docker/.env.prod-db" {
		t.Fatalf("picked %#v", got)
	}
	if got[0].Optional != true || got[1].Optional != true {
		t.Fatalf("init defaults optional yes: %#v", got)
	}
}

func TestRunInitProjectFromFolderPrefix(t *testing.T) {
	testenv.NoHostTools(t)
	setTTY(t, false)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"3000:3000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := runInit(repo, initOpts{
		prefix:       "acme-",
		composeFiles: []string{"docker-compose.yml"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != "acme" || c.FolderPrefix != "acme-" {
		t.Fatalf("project must be the prefix stem, not the clone folder: %#v", c)
	}
	if guessFolderPrefix("acme-genie2") != "acme-" || guessFolderPrefix("hello") != "" {
		t.Fatalf("guess %q %q", guessFolderPrefix("acme-genie2"), guessFolderPrefix("hello"))
	}
	if !isNone("none") || isNone("acme-") {
		t.Fatal("isNone")
	}

	// Non-interactive, no --folder-prefix: still guess from the clone folder.
	parent := t.TempDir()
	clone := filepath.Join(parent, "acme-genie3")
	if err := os.Mkdir(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"3000:3000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = runInit(clone, initOpts{
		askPrefix:    true,
		composeFiles: []string{"docker-compose.yml"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != "acme" || c.FolderPrefix != "acme-" {
		t.Fatalf("scripted init must not write the clone folder as project: %#v", c)
	}

	// Explicit empty / none: do not guess.
	c, err = runInit(clone, initOpts{
		prefix:       "none",
		askPrefix:    false,
		composeFiles: []string{"docker-compose.yml"},
		preferProxy:  false,
		dryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != "acme-genie3" || c.FolderPrefix != "" {
		t.Fatalf("--folder-prefix none must keep the folder as project: %#v", c)
	}
}

func TestConfirmProxyPortFree(t *testing.T) {
	// No docker/netstat/ss on PATH: nothing can be reported as holding :80.
	testenv.NoHostTools(t)
	ok, err := confirmProxyPortFree(false)
	if !ok || err != nil {
		t.Fatalf("port must look free when no tool can report occupants: %v %v", ok, err)
	}
}

func TestStartPathFlag(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	p := planCmd()
	if err := p.ParseFlags([]string{"--path", "."}); err != nil {
		t.Fatal(err)
	}
	if got, err := startPathFlag(p); err != nil || got != wd {
		t.Fatalf("--path . → %q %v, want %q", got, err, wd)
	}

	// No flag: current directory.
	if got, err := startPathFlag(planCmd()); err != nil || got != wd {
		t.Fatalf("default → %q %v, want %q", got, err, wd)
	}

	// Relative paths resolve against the cwd and are cleaned.
	p = planCmd()
	if err := p.ParseFlags([]string{"--path", filepath.Join("sub", "..", "other")}); err != nil {
		t.Fatal(err)
	}
	if got, err := startPathFlag(p); err != nil || got != filepath.Join(wd, "other") {
		t.Fatalf("relative → %q %v", got, err)
	}
}
