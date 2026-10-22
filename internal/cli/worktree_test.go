package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/gitworktree"
)

func TestRunSyncWithDetachedCommandContext(t *testing.T) {
	isolateRoot(t)
	hello := helloRepo(t)
	sc := syncCmd()
	sc.SetContext(context.Background())
	if err := sc.Flags().Set("path", hello); err != nil {
		t.Fatal(err)
	}
	if err := runSync(sc, "", "test", syncOpts{dryRun: true}); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeAddSyncUsesParentContext(t *testing.T) {
	parent := &cobra.Command{}
	parent.SetContext(context.Background())
	sc := syncCmd()
	sc.SetContext(parent.Context())
	if sc.Context() == nil {
		t.Fatal("expected context on detached sync command")
	}
}

func TestRunWorktreeSyncDryRunFindsMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	scratch := t.TempDir()
	t.Setenv("HOME", scratch)
	t.Setenv("USERPROFILE", scratch)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(scratch, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "siding")
	t.Setenv("GIT_AUTHOR_EMAIL", "siding@example")
	t.Setenv("GIT_COMMITTER_NAME", "siding")
	t.Setenv("GIT_COMMITTER_EMAIL", "siding@example")

	main := filepath.Join(scratch, "main")
	sibling := filepath.Join(scratch, "main-feature")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitWorktreeTest(t, main, "init", "-q")
	runGitWorktreeTest(t, main, "symbolic-ref", "HEAD", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(main, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &contract.Contract{
		Project:      "main",
		FolderPrefix: "main-",
		Modes:        map[string]contract.Mode{"test": {ComposeFiles: []string{"docker-compose.yml"}}},
	}
	if _, err := contract.Write(main, c); err != nil {
		t.Fatal(err)
	}
	runGitWorktreeTest(t, main, "add", ".siding", "README")
	runGitWorktreeTest(t, main, "commit", "-q", "-m", "recipe")

	ctx := context.Background()
	if err := gitworktree.Add(ctx, main, sibling, "feature", ""); err != nil {
		t.Fatal(err)
	}

	stateRoot := t.TempDir()
	t.Setenv("SIDING_ROOT", stateRoot)
	t.Setenv("SIDING_DATA", stateRoot)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("SIDING_NO_UPDATE_CHECK", "1")
	oldRoot := globalRoot
	globalRoot = stateRoot
	t.Cleanup(func() { globalRoot = oldRoot })

	out, err := execCLI(t, "worktree", "sync", "--path", main, "--dry-run")
	if err != nil {
		t.Fatalf("worktree sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, filepath.Base(sibling)) {
		t.Fatalf("expected sibling in output:\n%s", out)
	}
	if !strings.Contains(out, "would sync") {
		t.Fatalf("expected dry-run plan:\n%s", out)
	}
}

func TestResolveWorktreeRemoveTargetByName(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	scratch := t.TempDir()
	t.Setenv("HOME", scratch)
	t.Setenv("USERPROFILE", scratch)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(scratch, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "siding")
	t.Setenv("GIT_AUTHOR_EMAIL", "siding@example")
	t.Setenv("GIT_COMMITTER_NAME", "siding")
	t.Setenv("GIT_COMMITTER_EMAIL", "siding@example")

	main := filepath.Join(scratch, "main")
	sibling := filepath.Join(scratch, "main-feature")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitWorktreeTest(t, main, "init", "-q")
	runGitWorktreeTest(t, main, "symbolic-ref", "HEAD", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(main, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitWorktreeTest(t, main, "add", "README")
	runGitWorktreeTest(t, main, "commit", "-q", "-m", "init")

	c := &contract.Contract{
		Project:      "main",
		FolderPrefix: "main-",
		Modes:        map[string]contract.Mode{"test": {ComposeFiles: []string{"docker-compose.yml"}}},
	}
	if _, err := contract.Write(main, c); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := gitworktree.Add(ctx, main, sibling, "feature", ""); err != nil {
		t.Fatal(err)
	}

	cmd := worktreeRemoveCmd()
	cmd.SetContext(ctx)
	if err := cmd.Flags().Set("path", main); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWorktreeRemoveTarget(cmd, "feature")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(sibling)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	if _, err := resolveWorktreeRemoveTarget(cmd, ""); err == nil || !strings.Contains(err.Error(), "primary worktree") {
		t.Fatalf("primary without NAME: got %v", err)
	}
}

func runGitWorktreeTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
