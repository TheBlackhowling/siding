package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/resolver"
)

func mustDockerArgs(t *testing.T, res *resolver.Result, action string, extra []string, opts Options) []string {
	t.Helper()
	got, err := DockerArgs(res, action, extra, opts)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestDockerArgsUpDefaults(t *testing.T) {
	res := &resolver.Result{
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
	}
	got := mustDockerArgs(t, res, "up", nil, Options{})
	want := []string{"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "--env-file", ".siding/env", "up", "-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsUpBuildAndPassthrough(t *testing.T) {
	res := &resolver.Result{
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml", "docker-compose.override.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
	}
	got := mustDockerArgs(t, res, "up", []string{"web"}, Options{Build: true, Profile: "e2e"})
	want := []string{
		"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "-f", "docker-compose.override.yml",
		"--env-file", ".siding/env", "--profile", "e2e", "up", "-d", "--build", "web",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsRespectsDetachAndNoBuild(t *testing.T) {
	res := &resolver.Result{
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
	}
	got := mustDockerArgs(t, res, "up", []string{"--detach", "--no-build"}, Options{Build: true})
	want := []string{"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "--env-file", ".siding/env", "up", "--detach", "--no-build"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsUpFirstClassFlags(t *testing.T) {
	res := sampleComposeResult()
	got := mustDockerArgs(t, res, "up", nil, Options{Build: true, ForceRecreate: true, Pull: "missing", RemoveOrphans: true})
	want := []string{
		"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml",
		"--env-file", ".siding/env", "up", "-d", "--build", "--force-recreate", "--pull", "missing", "--remove-orphans",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsDownFirstClassFlags(t *testing.T) {
	res := sampleComposeResult()
	got := mustDockerArgs(t, res, "down", nil, Options{Volumes: true, RemoveOrphans: true, Timeout: 20})
	want := []string{
		"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml",
		"--env-file", ".siding/env", "--profile", "*", "down", "--volumes", "--remove-orphans", "--timeout", "20",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsDownSkipsDuplicateVolumes(t *testing.T) {
	res := sampleComposeResult()
	got := mustDockerArgs(t, res, "down", []string{"-v"}, Options{Volumes: true})
	want := []string{"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "--env-file", ".siding/env", "--profile", "*", "down", "-v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsLogsFollow(t *testing.T) {
	res := sampleComposeResult()
	got := mustDockerArgs(t, res, "logs", []string{"web", "api"}, Options{Follow: true, Tail: "100", Since: "10m", Timestamps: true})
	want := []string{
		"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml",
		"--env-file", ".siding/env", "logs", "--follow", "--tail", "100", "--since", "10m", "--timestamps", "web", "api",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNormalizePull(t *testing.T) {
	got, err := NormalizePull("Always")
	if err != nil || got != "always" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizePull("sometimes"); err == nil {
		t.Fatal("expected error")
	}
}

func sampleComposeResult() *resolver.Result {
	return &resolver.Result{
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
	}
}

func TestDockerArgsComposeEnvFilesBeforeSlot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", ".env.prod-db"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := &resolver.Result{
		RepoRoot:       root,
		ComposeProject: "hello-prod",
		ComposeFiles:   []string{".siding/compose/docker-compose.prod-db.yml"},
		SlotEnvRel:     ".siding/env.prod",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
		ModeConfig: contract.Mode{ComposeEnvFiles: contract.ComposeEnvFileList{
			{Path: "docker/.env.prod-db"},
		}},
	}
	got := mustDockerArgs(t, res, "up", nil, Options{})
	want := []string{
		"compose", "-p", "hello-prod", "--project-directory", ".",
		"-f", ".siding/compose/docker-compose.prod-db.yml",
		"--env-file", "docker/.env.prod-db",
		"--env-file", ".siding/env.prod",
		"up", "-d",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsSkipsOptionalMissingComposeEnv(t *testing.T) {
	root := t.TempDir()
	res := &resolver.Result{
		RepoRoot:       root,
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
		ModeConfig: contract.Mode{ComposeEnvFiles: contract.ComposeEnvFileList{
			{Path: "docker/.env.docker", Optional: true},
		}},
	}
	got := mustDockerArgs(t, res, "up", nil, Options{})
	want := []string{"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "--env-file", ".siding/env", "up", "-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsRequiredMissingComposeEnv(t *testing.T) {
	root := t.TempDir()
	res := &resolver.Result{
		RepoRoot:       root,
		ComposeProject: "hello-prod",
		ComposeFiles:   []string{"docker-compose.yml"},
		SlotEnvRel:     ".siding/env",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
		ModeConfig: contract.Mode{ComposeEnvFiles: contract.ComposeEnvFileList{
			{Path: "docker/.env.prod-db"},
		}},
	}
	if _, err := DockerArgs(res, "up", nil, Options{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDockerArgsDownKeepsExplicitProfile(t *testing.T) {
	res := sampleComposeResult()
	got := mustDockerArgs(t, res, "down", nil, Options{Profile: "e2e"})
	want := []string{
		"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml",
		"--env-file", ".siding/env", "--profile", "e2e", "down",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDockerArgsDown(t *testing.T) {
	res := &resolver.Result{
		ComposeProject: "hello-test",
		ComposeFiles:   []string{"docker-compose.yml"},
		Mode:           "prod-db",
		SlotEnvRel:     ".siding/env.prod-db",
		Contract:       &contract.Contract{SlotEnvFile: ".siding/env"},
	}
	got := mustDockerArgs(t, res, "down", []string{"-v"}, Options{})
	want := []string{"compose", "-p", "hello-test", "--project-directory", ".", "-f", "docker-compose.yml", "--env-file", ".siding/env.prod-db", "--profile", "*", "down", "-v"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
