package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/testenv"
)

func helloRepo(t *testing.T) string {
	t.Helper()
	return testenv.HelloRepo(t)
}

func checkByName(rep Report, name string) (Check, bool) {
	for _, c := range rep.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func TestRunHelloAndMissingWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	rep := Run(ctx, helloRepo(t), root)
	for _, name := range []string{"version", "start_path", "repo_root", "contract", "resolve"} {
		c, ok := checkByName(rep, name)
		if !ok || c.Status != "pass" {
			t.Fatalf("%s: %#v\n%s", name, c, FormatText(rep))
		}
	}
	if c, _ := checkByName(rep, "docker_compose"); c.Status != "fail" {
		t.Fatalf("docker off PATH must fail docker_compose: %#v", c)
	}
	if c, _ := checkByName(rep, "docker_ports"); c.Status != "warn" {
		t.Fatalf("docker off PATH must warn docker_ports: %#v", c)
	}
	if c, _ := checkByName(rep, "proxy"); c.Status != "warn" {
		t.Fatalf("proxy cannot be running: %#v", c)
	}
	if rep.OK || !strings.Contains(FormatText(rep), "Fix failures") {
		t.Fatalf("report with a fail must not be OK:\n%s", FormatText(rep))
	}

	missing := Run(ctx, filepath.Join(t.TempDir(), "nope"), root)
	if c, _ := checkByName(missing, "repo_root"); c.Status != "fail" {
		t.Fatalf("%#v", c)
	}
	if _, ok := checkByName(missing, "repo_hint"); !ok {
		t.Fatal("missing repo should hint at siding init")
	}
	if missing.OK {
		t.Fatalf("missing repo should fail: %s", FormatText(missing))
	}
}

func TestFormatTextFailAndTips(t *testing.T) {
	pass := FormatText(Report{OK: true, Checks: []Check{{Name: "a", Status: "pass", Message: "ok"}}})
	if !strings.Contains(pass, "All required checks passed") {
		t.Fatal(pass)
	}
	fail := FormatText(Report{
		OK:     false,
		Tips:   []string{"wrap NEXT_PUBLIC"},
		Checks: []Check{{Name: "c", Status: "fail", Message: "bad"}, {Name: "w", Status: "warn", Message: "hmm"}},
	})
	if !strings.Contains(fail, "FAIL") || !strings.Contains(fail, "Tips") || !strings.Contains(fail, "Fix failures") {
		t.Fatal(fail)
	}
}

func TestFinalize(t *testing.T) {
	r := finalize([]Check{{Status: "pass"}, {Status: "warn"}}, []string{"tip"})
	if !r.OK {
		t.Fatal("warn is not fail")
	}
	r = finalize([]Check{{Status: "fail"}}, nil)
	if r.OK {
		t.Fatal("fail")
	}
}

func TestApplyScanFindings(t *testing.T) {
	var kinds []string
	note := func(name, _ string) { kinds = append(kinds, name) }
	applyScanFindings([]advise.Finding{
		{Kind: "port_ok", Detail: "ok"},
		{Kind: "port_hardcoded", Detail: "h"},
		{Kind: "public_url", Detail: "u"},
		{Kind: "missing_port_env", Detail: "e"},
		{Kind: "unowned_port", Detail: "o"},
		{Kind: "host_network", Detail: "n"},
		{Kind: "localhost_internal", Detail: "l"},
		{Kind: "compose_missing", Detail: "m"},
		{Kind: "compose_invalid", Detail: "i"},
		{Kind: "other", Detail: "x"},
	}, note, note, note)
	if len(kinds) != 9 {
		t.Fatalf("%v", kinds)
	}
}

func TestComposeVersionAndSnapshot(t *testing.T) {
	if msg, ok := composeVersionResult("", fmt.Errorf("no")); ok || msg == "" {
		t.Fatal("err")
	}
	if msg, ok := composeVersionResult("  ", nil); !ok || msg != "docker compose available" {
		t.Fatalf("%q %v", msg, ok)
	}
	if msg, ok := composeVersionResult("v2\n", nil); !ok || msg != "v2" {
		t.Fatalf("%q %v", msg, ok)
	}

	var names []string
	note := func(name, _ string) { names = append(names, name) }
	applyDockerSnapshot(dockerps.Snapshot{}, fmt.Errorf("nope"), nil, note, note)
	applyDockerSnapshot(dockerps.Snapshot{
		HostPorts:   map[int]struct{}{18080: {}},
		HostNetwork: []string{"hosty"},
	}, nil, &resolver.Result{Ports: map[string]int{"web": 18080}}, note, note)
	testenv.NoHostTools(t)
	applyProxyCheck(context.Background(), t.TempDir(), note, note)
	want := []string{"docker_ports", "docker_ports", "docker_host_network", "port_live", "proxy"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", names, want)
	}
}

func TestDockerComposeWithoutDocker(t *testing.T) {
	testenv.NoHostTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msg, ok := dockerCompose(ctx)
	if ok || !strings.Contains(msg, "docker compose not available") {
		t.Fatalf("%q %v", msg, ok)
	}
}

func TestRunWithStateAndBadContract(t *testing.T) {
	testenv.NoHostTools(t)
	root := t.TempDir()
	ctx := context.Background()

	// Existing state.json + hello recipe covers the mapped/unmapped local check.
	hello := helloRepo(t)
	key := paths.NormalizeLockPath(hello)
	mapped := `{"version":1,"checkouts":{"` + key + `":{"project":"hello","stack":"hello","modes":{"test":{"host":"hello.test.localhost"}}}},"locks":{}}`
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(mapped), 0o644); err != nil {
		t.Fatal(err)
	}
	mappedRep := Run(ctx, hello, root)
	if c, _ := checkByName(mappedRep, "local"); c.Status != "pass" {
		t.Fatalf("mapped checkout: %#v", c)
	}

	unmappedRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(unmappedRoot, "state.json"), []byte(`{"version":1,"checkouts":{},"locks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Run(ctx, hello, unmappedRoot)
	if c, _ := checkByName(rep, "local"); c.Status != "warn" || !strings.Contains(c.Message, "siding sync") {
		t.Fatalf("unmapped checkout should point at siding sync: %#v", c)
	}

	bad := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bad, ".siding"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, ".siding", "config.yml"), []byte("project: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "docker-compose.yml"), []byte("services: {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badRep := Run(ctx, bad, root)
	if c, _ := checkByName(badRep, "contract"); c.Status != "fail" {
		t.Fatalf("unparseable recipe must fail contract: %#v", c)
	}
	if c, _ := checkByName(badRep, "resolve"); c.Status != "warn" {
		t.Fatalf("unparseable recipe cannot resolve: %#v", c)
	}

	hard := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hard, ".siding"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hard, ".siding", "config.yml"), []byte("version: 1\nproject: x\nmodes:\n  test:\n    compose_files:\n      - docker-compose.yml\n    ports:\n      web: 3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hard, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    ports:\n      - \"3000:3000\"\n    environment:\n      NEXT_PUBLIC_URL: http://localhost:3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hardRep := Run(ctx, hard, root)
	for _, name := range []string{"port_hardcoded", "public_url"} {
		if c, ok := checkByName(hardRep, name); !ok || c.Status != "warn" {
			t.Fatalf("%s: %#v\n%s", name, c, FormatText(hardRep))
		}
	}
}
