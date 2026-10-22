package contract

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeEnvFileListUnmarshalMixed(t *testing.T) {
	var c Contract
	if err := yaml.Unmarshal([]byte(`
project: demo
modes:
  test:
    compose_files: [docker-compose.yml]
    compose_env_files:
      - docker/.env.docker
      - path: docker/.env.extra
        optional: true
  prod:
    compose_files: [docker-compose.yml]
    compose_env_files:
      - docker/.env.prod-db
`), &c); err != nil {
		t.Fatal(err)
	}
	test := c.Modes["test"].ComposeEnvFiles
	if len(test) != 2 || test[0].Path != "docker/.env.docker" || test[0].Optional || !test[1].Optional {
		t.Fatalf("test %#v", test)
	}
	prod := c.Modes["prod"].ComposeEnvFiles
	if len(prod) != 1 || prod[0].Path != "docker/.env.prod-db" || prod[0].Optional {
		t.Fatalf("prod %#v", prod)
	}
}

func TestSelectComposeEnvPathsOptionalMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "present.env"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := ComposeEnvFileList{
		{Path: "present.env"},
		{Path: "missing.env", Optional: true},
		{Path: "gone.env", Optional: true},
	}
	included, skipped, err := SelectComposeEnvPaths(root, list)
	if err != nil {
		t.Fatal(err)
	}
	if len(included) != 1 || included[0] != "present.env" {
		t.Fatalf("included %v", included)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped %v", skipped)
	}
}

func TestSelectComposeEnvPathsRequiredMissing(t *testing.T) {
	root := t.TempDir()
	_, _, err := SelectComposeEnvPaths(root, ComposeEnvFileList{{Path: "need.env"}})
	if err == nil {
		t.Fatal("expected error")
	}
}
