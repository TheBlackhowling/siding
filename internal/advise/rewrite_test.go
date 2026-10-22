package advise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteComposePortsAndPublicAndInternal(t *testing.T) {
	in := `
services:
  backend:
    ports:
      - "8000:8000"
    environment:
      INTERNAL_URL: http://localhost:3000
  web:
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_ROOT_DOMAIN: ${NEXT_PUBLIC_ROOT_DOMAIN:-app.local:3000}
      API_INNER: http://127.0.0.1:8000
`
	idx := ContainerPortIndex(in)
	out, changes, pub := RewriteCompose(in, idx)
	if len(changes) < 4 {
		t.Fatalf("changes=%d %#v", len(changes), changes)
	}
	if !strings.Contains(out, `${WEB_PORT:-3000}:3000`) {
		t.Fatalf("web port: %s", out)
	}
	// backend folds to the api key, so the env name is API_PORT, never BACKEND_PORT.
	if !strings.Contains(out, `${API_PORT:-8000}:8000`) || strings.Contains(out, "BACKEND_PORT") {
		t.Fatalf("api port: %s", out)
	}
	if !strings.Contains(out, "NEXT_PUBLIC_ROOT_DOMAIN: ${NEXT_PUBLIC_ROOT_DOMAIN:-app.local:3000}") {
		t.Fatalf("public: %s", out)
	}
	if pub["NEXT_PUBLIC_ROOT_DOMAIN"] != "{host}{proxy_suffix}" {
		t.Fatalf("template=%q", pub["NEXT_PUBLIC_ROOT_DOMAIN"])
	}
	if !strings.Contains(out, "http://backend:8000") {
		t.Fatalf("internal api: %s", out)
	}
	if !strings.Contains(out, "http://web:3000") {
		t.Fatalf("internal web: %s", out)
	}
}

func TestRewriteMultiPublishGetsUniqueEnv(t *testing.T) {
	in := `
services:
  firebase:
    ports:
      - "4000:4000"
      - "0.0.0.0:8080:8080"
      - "9099"
  redis:
    ports:
      - "6379"
`
	out, changes, _ := RewriteCompose(in, nil)
	if len(changes) < 4 {
		t.Fatalf("changes=%d %#v", len(changes), changes)
	}
	for _, want := range []string{
		`${FIREBASE_4000_PORT:-4000}:4000`,
		`${FIREBASE_8080_PORT:-8080}:8080`,
		`${FIREBASE_9099_PORT:-9099}:9099`,
		`${REDIS_PORT:-6379}:6379`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}

func TestRewriteComposeCRLFAttachesSharedNetwork(t *testing.T) {
	in := "services:\r\n  frontend:\r\n    ports:\r\n      - \"3000:3000\"\r\n    networks:\r\n      - frontend\r\n  backend:\r\n    ports:\r\n      - \"8000:8000\"\r\n    networks:\r\n      - backend\r\n"
	out, changes, _ := RewriteCompose(in, ContainerPortIndex(in))
	if strings.Count(out, "- siding") < 2 {
		t.Fatalf("crlf attach failed (%d changes): %s", len(changes), out)
	}
}

func TestRewriteLeavesHealthcheckAndHostFacing(t *testing.T) {
	in := `
services:
  backend:
    ports:
      - "8000:8000"
    environment:
      DEV_API_URL: ${DEV_API_URL:-http://localhost:8000}
      INTERNAL_URL: http://localhost:3000
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://127.0.0.1:8000/health"]
  web:
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_ROOT_DOMAIN: ${NEXT_PUBLIC_ROOT_DOMAIN:-app.local:3000}
      DEV_FRONTEND_URL: ${DEV_FRONTEND_URL:-http://localhost:3000}
      GCS_PUBLIC_HOST: http://localhost:4443
`
	idx := ContainerPortIndex(in)
	out, _, pub := RewriteCompose(in, idx)
	if !strings.Contains(out, "http://127.0.0.1:8000/health") {
		t.Fatalf("healthcheck rewritten: %s", out)
	}
	if !strings.Contains(out, "GCS_PUBLIC_HOST: http://localhost:4443") {
		t.Fatalf("gcs public rewritten: %s", out)
	}
	if !strings.Contains(out, "DEV_API_URL: ${DEV_API_URL:-http://localhost:8000}") {
		t.Fatalf("dev api should come from slot env: %s", out)
	}
	if !strings.Contains(out, "http://web:3000") {
		t.Fatalf("internal still localhost: %s", out)
	}
	if strings.Contains(out, "DEV_API_URL: http://backend") {
		t.Fatal("host-facing DEV_API_URL must not become service DNS")
	}
	if pub["DEV_FRONTEND_URL"] != "http://{web_host}{web_suffix}" {
		t.Fatalf("frontend template=%q", pub["DEV_FRONTEND_URL"])
	}
	if strings.Contains(out, "networks:") && strings.Contains(out, "siding:") {
		t.Fatalf("should not invent a network when default already works: %s", out)
	}
}

func TestRewriteDoesNotTreatNetworkAliasesAsService(t *testing.T) {
	in := `
services:
  web:
    networks:
      default:
        aliases:
          - app.acme.test
    ports:
      - "3000:3000"
  backend:
    ports:
      - "8000:8000"
`
	out, _, _ := RewriteCompose(in, ContainerPortIndex(in))
	if !strings.Contains(out, `${WEB_PORT:-3000}:3000`) {
		t.Fatalf("web port stolen by aliases: %s", out)
	}
	if strings.Contains(out, "ALIASES_PORT") {
		t.Fatalf("networks.aliases must not become a port key: %s", out)
	}
	if idx := ContainerPortIndex(in); idx[3000] != "web" {
		t.Fatalf("index 3000=%q want web", idx[3000])
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	if rep.Ports["web"] != 3000 {
		t.Fatalf("scan after rewrite ports=%#v", rep.Ports)
	}
	if _, ok := rep.Ports["aliases"]; ok {
		t.Fatalf("aliases leaked into scan: %#v", rep.Ports)
	}
}

func TestRewriteCommentedPorts(t *testing.T) {
	in := `
services:
  firebase:
    ports:
      - "4000:4000"   # Emulator UI
      - "4400:4400"   # Emulator hub
      - "8080:8080"   # Firestore
      - "9099:9099"   # Auth
`
	out, changes, _ := RewriteCompose(in, ContainerPortIndex(in))
	if len(changes) < 4 {
		t.Fatalf("commented ports skipped: %d %#v", len(changes), changes)
	}
	for _, want := range []string{
		`${FIREBASE_4000_PORT:-4000}:4000"  # Emulator UI`,
		`${FIREBASE_4400_PORT:-4400}:4400"  # Emulator hub`,
		`${FIREBASE_8080_PORT:-8080}:8080"  # Firestore`,
		`${FIREBASE_9099_PORT:-9099}:9099"  # Auth`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
	idx := ContainerPortIndex(in)
	if idx[4000] != "firebase" || idx[9099] != "firebase" {
		t.Fatalf("commented ports must feed ContainerPortIndex: %#v", idx)
	}
}

func TestRewriteLongFormPublished(t *testing.T) {
	in := `
services:
  backend:
    ports:
      - target: 8000
        published: 8000
`
	out, changes, _ := RewriteCompose(in, nil)
	if len(changes) == 0 || !strings.Contains(out, `${API_PORT:-8000}`) {
		t.Fatalf("long-form published not rewritten (%d): %s", len(changes), out)
	}
}

func TestRewriteMixedEnvAndHardcodedOnOneService(t *testing.T) {
	in := `
services:
  api:
    ports:
      - "${API_PORT:-8000}:8000"
      - "5678:5678"
`
	out, _, _ := RewriteCompose(in, nil)
	if !strings.Contains(out, `${API_PORT:-8000}:8000`) {
		t.Fatalf("keep existing env: %s", out)
	}
	if !strings.Contains(out, `${API_5678_PORT:-5678}:5678`) {
		t.Fatalf("second publish must not reuse API_PORT: %s", out)
	}
	if strings.Contains(out, `${API_PORT:-5678}`) {
		t.Fatal("must not collide env vars")
	}
}

func TestRewriteServiceAnchorAndNamedTest(t *testing.T) {
	in := `
services:
  backend: &backend
    ports:
      - "8000:8000"
  test:
    ports:
      - "9999:9999"
`
	out, _, _ := RewriteCompose(in, nil)
	if !strings.Contains(out, `${API_PORT:-8000}:8000`) {
		t.Fatalf("anchor service: %s", out)
	}
	if !strings.Contains(out, `${TEST_PORT:-9999}:9999`) {
		t.Fatalf("service named test: %s", out)
	}
}

func TestRewriteFourSpaceIndent(t *testing.T) {
	in := `
services:
    web:
        ports:
            - "3000:3000"
`
	out, changes, _ := RewriteCompose(in, nil)
	if len(changes) == 0 || !strings.Contains(out, `${WEB_PORT:-3000}:3000`) {
		t.Fatalf("4-space indent produced no rewrite (%d): %s", len(changes), out)
	}
}

func TestScanRewritePortEnvParity(t *testing.T) {
	dir := t.TempDir()
	body := `
services:
  frontend:
    ports:
      - "3000:3000"
  api:
    ports:
      - "${API_PORT:-8000}:8000"
      - "5678:5678"
  firebase:
    ports:
      - "4000:4000" # UI
`
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ScanRepo(dir, []string{"docker-compose.yml"}, nil, "")
	out, _, _ := RewriteCompose(body, ContainerPortIndex(body))
	for key, env := range rep.PortEnv {
		if env == "API_PORT" {
			continue
		}
		needle := "${" + env + ":-"
		if !strings.Contains(out, needle) {
			t.Fatalf("scan port_env.%s=%s missing from rewrite (key %s): %s", key, env, key, out)
		}
	}
}

func TestWriteSidingComposeLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(src, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destRel, err := WriteSidingCompose(dir, "docker-compose.yml", "new\n")
	if err != nil {
		t.Fatal(err)
	}
	if destRel != ".siding/compose/docker-compose.yml" {
		t.Fatalf("destRel=%q", destRel)
	}
	got, _ := os.ReadFile(src)
	if string(got) != "old\n" {
		t.Fatalf("original changed: %q", got)
	}
	copyBody, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(destRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(copyBody), "Generated by siding from docker-compose.yml") {
		t.Fatalf("missing header: %s", copyBody)
	}
	if !strings.Contains(string(copyBody), "new\n") {
		t.Fatalf("copy=%q", copyBody)
	}
}
