package gitworktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

)

func TestParsePorcelain(t *testing.T) {
	raw := `worktree /src/main
HEAD abcdef0123456789
branch refs/heads/main

worktree /src/feature
HEAD 1111111111111111
branch refs/heads/feat/x
`
	got, err := parsePorcelain(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "/src/main" || got[0].Branch != "main" || got[1].Branch != "feat/x" {
		t.Fatalf("%#v", got)
	}
}

func TestSiblingPathUsesFolderPrefix(t *testing.T) {
	main := filepath.Join("C:", "source", "acme-betty")
	got, err := SiblingPath(main, "acme-", "changelog")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("C:", "source", "acme-changelog")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSiblingPathWithoutPrefixUsesMainBasename(t *testing.T) {
	main := filepath.Join("C:", "source", "siding")
	got, err := SiblingPath(main, "siding-", "board")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("C:", "source", "siding-board")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestGitWorktreeAddRemove(t *testing.T) {
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
	runGit(t, main, "init", "-q")
	runGit(t, main, "symbolic-ref", "HEAD", "refs/heads/main")
	if err := os.WriteFile(filepath.Join(main, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "add", "README")
	runGit(t, main, "commit", "-q", "-m", "init")

	ctx := context.Background()
	if err := Add(ctx, main, sibling, "feature", ""); err != nil {
		t.Fatal(err)
	}
	sibling2 := filepath.Join(scratch, "main-from-main")
	if err := Add(ctx, main, sibling2, "from-main", "main"); err != nil {
		t.Fatal(err)
	}
	linked, err := IsLinked(ctx, sibling)
	if err != nil || !linked {
		t.Fatalf("linked=%v err=%v", linked, err)
	}
	entries, err := List(ctx, main)
	if err != nil || len(entries) != 3 {
		t.Fatalf("list %v err=%v", entries, err)
	}
	if err := Remove(ctx, sibling); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sibling); !os.IsNotExist(err) {
		t.Fatalf("sibling dir should be gone: %v", err)
	}
}

func TestRemoveRefusesPrimaryWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	if err := Remove(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "primary worktree") {
		t.Fatalf("got %v", err)
	}
}

func TestSiblingPathRejectsEmptyName(t *testing.T) {
	if _, err := SiblingPath("C:/source/app", "app-", ""); err == nil {
		t.Fatal("expected error")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
