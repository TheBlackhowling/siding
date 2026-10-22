package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGuessComposeEnvFilesFromDeclaredAndSuffix(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := []byte(`
services:
  backend:
    env_file:
      - path: apps/backend/.env
        required: false
      - path: docker/.env.prod-db
        required: true
`)
	if err := os.WriteFile(filepath.Join(root, "docker-compose.prod-db.yml"), yml, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", ".env.prod-db"), []byte("GCP_ADC_FILE=/tmp/adc.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := GuessComposeEnvFiles(root, "prod", []string{"docker-compose.prod-db.yml"})
	want := []string{"docker/.env.prod-db"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestGuessComposeEnvFilesMissingIsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GuessComposeEnvFiles(root, "test", []string{"docker-compose.yml"}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestKeepGuessedEnvFile(t *testing.T) {
	if !keepGuessedEnvFile("docker/.env.prod-db") || keepGuessedEnvFile("apps/backend/.env") {
		t.Fatal("filter")
	}
	if keepGuessedEnvFile(".siding/env.prod") {
		t.Fatal("slot file")
	}
}

func TestListComposeEnvFilesGuessedFirst(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.prod-db.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", ".env.prod"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", ".env.prod-db"), []byte("B=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, guessed := ListComposeEnvFiles(root, "prod", []string{"docker-compose.prod-db.yml"})
	if !reflect.DeepEqual(guessed, []string{"docker/.env.prod-db", "docker/.env.prod"}) {
		t.Fatalf("guessed %v", guessed)
	}
	if !reflect.DeepEqual(all, guessed) {
		t.Fatalf("all %v", all)
	}
	if DefaultEnvFileSelection(all, guessed) != "1,2" {
		t.Fatalf("default %q", DefaultEnvFileSelection(all, guessed))
	}
}

func TestListComposeEnvFilesStaysOnThisMode(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker-compose.prod-db.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", ".env.prod-db"), []byte("B=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, _ := ListComposeEnvFiles(root, "test", []string{"docker-compose.yml"})
	if len(all) != 0 {
		t.Fatalf("test must not list prod env files: %v", all)
	}
	all, _ = ListComposeEnvFiles(root, "prod", []string{"docker-compose.prod-db.yml"})
	if !reflect.DeepEqual(all, []string{"docker/.env.prod-db"}) {
		t.Fatalf("prod %v", all)
	}
}
