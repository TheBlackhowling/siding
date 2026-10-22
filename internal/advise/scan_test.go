package advise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/testenv"
)

func TestScanHelloCompose(t *testing.T) {
	rep := ScanRepo(testenv.HelloRepo(t), []string{"docker-compose.yml"}, nil, "")
	if rep.Ports["web"] != 18080 {
		t.Fatalf("ports=%#v", rep.Ports)
	}
	if rep.PortEnv["web"] != "WEB_PORT" {
		t.Fatalf("port_env=%#v", rep.PortEnv)
	}
	if HasProblems(rep) {
		t.Fatalf("hello should be clean: %#v", rep.Findings)
	}
}

func TestScanHardcodedAndPublicURL(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  web:
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_ROOT_DOMAIN: ${NEXT_PUBLIC_ROOT_DOMAIN:-app.local:3000}
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	if !HasProblems(rep) {
		t.Fatal("expected problems")
	}
	kinds := map[string]int{}
	for _, f := range rep.Findings {
		kinds[f.Kind]++
	}
	if kinds["port_hardcoded"] == 0 || kinds["public_url"] == 0 {
		t.Fatalf("kinds=%v findings=%#v", kinds, rep.Findings)
	}
	if len(rep.Tips) < 2 {
		t.Fatalf("tips=%v", rep.Tips)
	}
}

func TestScanSkipsHealthcheckLocalhost(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  backend:
    ports:
      - "${API_PORT:-8000}:8000"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://127.0.0.1:8000/health"]
    environment:
      INTERNAL_API_URL: http://backend:8000
  web:
    ports:
      - "${WEB_PORT:-3000}:3000"
    environment:
      ACME_API_URL_DEV: http://backend:8000
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	for _, f := range rep.Findings {
		if f.Kind == "localhost_internal" || f.Kind == "network_isolated" || f.Kind == "host_network" {
			t.Fatalf("unexpected problem %#v", f)
		}
	}
	if rep.Network.NeedsEnsure {
		t.Fatalf("network should already be shared: %#v", rep.Network)
	}
	if rep.Network.Shared != "default" {
		t.Fatalf("shared=%q", rep.Network.Shared)
	}
}

func TestScanOwnsEveryPublishedPort(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  web:
    ports:
      - "${WEB_PORT:-3000}:3000"
  redis:
    ports:
      - "${REDIS_PORT}:6379"
  postgres:
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	if rep.Ports["web"] != 3000 || rep.Ports["redis"] != 6379 || rep.Ports["postgres"] != 5432 {
		t.Fatalf("ports=%#v", rep.Ports)
	}
	if rep.PortEnv["redis"] != "REDIS_PORT" {
		t.Fatalf("port_env=%#v", rep.PortEnv)
	}

	c := &contract.Contract{Project: "x", Modes: map[string]contract.Mode{
		"test": {
			ComposeFiles: []string{"docker-compose.yml"},
			Ports:        map[string]int{"web": 3000},
			PortEnv:      map[string]string{"web": "WEB_PORT"},
		},
	}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	owned := ScanRepo(dir, []string{"docker-compose.yml"}, c, "test")
	kinds := map[string]int{}
	for _, f := range owned.Findings {
		kinds[f.Kind]++
	}
	if kinds["unowned_port"] < 2 {
		t.Fatalf("expected unowned redis+postgres: %#v", owned.Findings)
	}
	if !HasProblems(owned) {
		t.Fatal("unowned ports are problems")
	}
}

func TestScanOwnsHardcodedAndLongSyntax(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  redis:
    ports:
      - 6379
  postgres:
    ports:
      - "0.0.0.0:5432:5432"
  firebase:
    ports:
      - "4000:4000"
      - "8080:8080"
      - "9099:9099"
  tika:
    ports:
      - target: 9998
        published: 9998
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	if rep.Ports["redis"] != 6379 {
		t.Fatalf("redis=%#v", rep.Ports)
	}
	if rep.Ports["postgres"] != 5432 {
		t.Fatalf("postgres=%#v", rep.Ports)
	}
	if rep.Ports["firebase_4000"] != 4000 || rep.Ports["firebase_8080"] != 8080 || rep.Ports["firebase_9099"] != 9099 {
		t.Fatalf("firebase keys=%#v", rep.Ports)
	}
	if rep.Ports["tika"] != 9998 {
		t.Fatalf("tika=%#v", rep.Ports)
	}
	if rep.PortEnv["firebase_8080"] != "FIREBASE_8080_PORT" {
		t.Fatalf("port_env=%#v", rep.PortEnv)
	}
}

func TestOwnPublishedFillsRecipeHoles(t *testing.T) {
	scan := Report{
		Ports:   map[string]int{"web": 3000, "redis": 6379},
		PortEnv: map[string]string{"web": "WEB_PORT", "redis": "REDIS_PORT"},
	}
	ports, env := OwnPublished(map[string]int{"web": 3010}, map[string]string{"web": "WEB_PORT"}, scan)
	if ports["web"] != 3010 {
		t.Fatalf("flag/recipe must win: %#v", ports)
	}
	if ports["redis"] != 6379 || env["redis"] != "REDIS_PORT" {
		t.Fatalf("scan hole not owned: ports=%#v env=%#v", ports, env)
	}
}

func TestScanFoldsHyphenatedServiceToEnvKey(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  gcs-emulator:
    ports:
      - "${GCS_EMULATOR_PORT:-4443}:4443"
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	if _, ok := rep.Ports["gcs-emulator"]; ok {
		t.Fatalf("hyphen key should fold: %#v", rep.Ports)
	}
	if rep.Ports["gcs_emulator"] != 4443 || rep.PortEnv["gcs_emulator"] != "GCS_EMULATOR_PORT" {
		t.Fatalf("ports=%#v env=%#v", rep.Ports, rep.PortEnv)
	}
}

func TestOwnPublishedCollapsesHyphenUnderscore(t *testing.T) {
	ports, env := OwnPublished(
		map[string]int{"gcs-emulator": 4443, "gcs_emulator": 4443, "web": 3000},
		map[string]string{"gcs-emulator": "GCS_EMULATOR_PORT", "gcs_emulator": "GCS_EMULATOR_PORT"},
		Report{},
	)
	if len(ports) != 2 {
		t.Fatalf("ports=%#v", ports)
	}
	if _, ok := ports["gcs-emulator"]; ok {
		t.Fatalf("hyphen leaked: %#v", ports)
	}
	if ports["gcs_emulator"] != 4443 || ports["web"] != 3000 {
		t.Fatalf("ports=%#v", ports)
	}
	if env["gcs_emulator"] != "GCS_EMULATOR_PORT" {
		t.Fatalf("env=%#v", env)
	}
}

func TestScanContractPortEnvMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &contract.Contract{Project: "x", Modes: map[string]contract.Mode{
		"test": {ComposeFiles: []string{"docker-compose.yml"}, PortEnv: map[string]string{"web": "WEB_PORT"}},
	}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, c, "test")
	found := false
	for _, f := range rep.Findings {
		if f.Kind == "missing_port_env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing_port_env: %#v", rep.Findings)
	}
}

func TestScanHostFacingEnvPlaceholder(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  web:
    ports:
      - "${WEB_PORT:-3000}:3000"
    environment:
      NEXT_PUBLIC_ROOT_DOMAIN: ${NEXT_PUBLIC_ROOT_DOMAIN}
      DEV_API_URL: ${DEV_API_URL}
`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	kinds := map[string]int{}
	for _, f := range rep.Findings {
		kinds[f.Kind]++
	}
	if kinds["public_url"] < 2 {
		t.Fatalf("expected public_url for placeholder env: %#v", rep.Findings)
	}

	c := &contract.Contract{Project: "x", Modes: map[string]contract.Mode{
		"test": {
			ComposeFiles: []string{"docker-compose.yml"},
			Ports:        map[string]int{"web": 3000},
			PortEnv:      map[string]string{"web": "WEB_PORT"},
			Env: map[string]string{
				"NEXT_PUBLIC_ROOT_DOMAIN": "{host}{proxy_suffix}",
				"DEV_API_URL":             "http://api.{host}{proxy_suffix}",
			},
		},
	}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	covered := ScanRepo(dir, []string{"docker-compose.yml"}, c, "test")
	for _, f := range covered.Findings {
		if f.Kind == "public_url" {
			t.Fatalf("recipe env should cover placeholders: %#v", covered.Findings)
		}
	}
}

func TestScanInvalidYAMLIsFinding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	found := false
	for _, f := range rep.Findings {
		if f.Kind == "compose_invalid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected compose_invalid, got %#v", rep.Findings)
	}
}
