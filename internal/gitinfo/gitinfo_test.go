package gitinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSnapshotEmptyDir(t *testing.T) {
	branch, sha := Snapshot(context.Background(), t.TempDir())
	if branch != "" || sha != "" {
		t.Fatalf("branch=%q sha=%q", branch, sha)
	}
}

func TestSnapshotBlankDir(t *testing.T) {
	if branch, sha := Snapshot(context.Background(), "  "); branch != "" || sha != "" {
		t.Fatalf("branch=%q sha=%q", branch, sha)
	}
}

func TestSnapshotGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Isolate from the developer's git config (gpgsign, hooksPath, templates)
	// for both the setup commands and Snapshot itself.
	scratch := t.TempDir()
	t.Setenv("HOME", scratch)
	t.Setenv("USERPROFILE", scratch)
	t.Setenv("XDG_CONFIG_HOME", scratch)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(scratch, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "siding")
	t.Setenv("GIT_AUTHOR_EMAIL", "siding@example")
	t.Setenv("GIT_COMMITTER_NAME", "siding")
	t.Setenv("GIT_COMMITTER_EMAIL", "siding@example")

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	// `git init -b` needs git 2.28; symbolic-ref works everywhere.
	run("symbolic-ref", "HEAD", "refs/heads/board-test")

	// Unborn branch: rev-parse fails, Snapshot stays empty rather than erroring.
	if branch, sha := Snapshot(context.Background(), dir); branch != "" || sha != "" {
		t.Fatalf("unborn HEAD branch=%q sha=%q", branch, sha)
	}

	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "init")
	branch, sha := Snapshot(context.Background(), dir)
	if branch != "board-test" {
		t.Fatalf("branch=%q", branch)
	}
	if len(sha) < 7 {
		t.Fatalf("expected short sha, got %q", sha)
	}
}

func TestDefaultRemoteBranchOriginHEAD(t *testing.T) {
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

	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("symbolic-ref", "HEAD", "refs/heads/develop")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "init")
	run("remote", "add", "origin", scratch)
	run("update-ref", "refs/remotes/origin/develop", "HEAD")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")

	if got := DefaultRemoteBranch(context.Background(), dir); got != "develop" {
		t.Fatalf("origin/HEAD = %q", got)
	}
}

func TestDefaultRemoteBranchMissing(t *testing.T) {
	if got := DefaultRemoteBranch(context.Background(), t.TempDir()); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}
