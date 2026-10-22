package gitworktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
)

// Entry is one row from git worktree list --porcelain.
type Entry struct {
	Path   string
	HEAD   string
	Branch string
	Bare   bool
}

// SiblingPath returns a sibling checkout folder under the parent of mainRoot.
func SiblingPath(mainRoot, folderPrefix, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("worktree name is required")
	}
	stack, err := contract.ResolveStackName(&contract.Contract{FolderPrefix: folderPrefix}, mainRoot, name)
	if err != nil {
		return "", err
	}
	parent := paths.Parent(mainRoot)
	prefix := strings.TrimSpace(folderPrefix)
	if prefix == "" {
		base := paths.Basename(mainRoot)
		if base != "" && base != stack {
			prefix = base + "-"
		}
	}
	return filepath.Join(parent, prefix+stack), nil
}

// List returns linked worktrees for the repo at repoRoot (includes main).
func List(ctx context.Context, repoRoot string) ([]Entry, error) {
	out, err := run(ctx, repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out)
}

// MainRoot returns the primary worktree path for repoRoot.
func MainRoot(ctx context.Context, repoRoot string) (string, error) {
	entries, err := List(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("git worktree list: no worktrees")
	}
	return entries[0].Path, nil
}

// IsLinked reports whether dir is a linked worktree (not the primary checkout).
func IsLinked(ctx context.Context, dir string) (bool, error) {
	gitDir, err := run(ctx, dir, "rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	gitDir = filepath.ToSlash(gitDir)
	return strings.Contains(gitDir, "/.git/worktrees/") || strings.Contains(gitDir, "\\.git\\worktrees\\"), nil
}

// Add creates a linked worktree at dest on a new branch (-b newBranch). When
// startPoint is set, the branch is created from that commit/branch/ref.
func Add(ctx context.Context, repoRoot, dest, newBranch, startPoint string) error {
	dest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("path already exists: %s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	newBranch = strings.TrimSpace(newBranch)
	if newBranch == "" {
		return fmt.Errorf("branch name is required")
	}
	args := []string{"worktree", "add", "-b", newBranch, "--", dest}
	if sp := strings.TrimSpace(startPoint); sp != "" {
		args = append(args, sp)
	}
	_, err = run(ctx, repoRoot, args...)
	if err != nil && strings.Contains(err.Error(), "already used by worktree") {
		return fmt.Errorf("%w\n\nThat branch is checked out in another worktree. Pass a start point instead:\n  siding worktree add NAME main   # new branch NAME from main", err)
	}
	return err
}

// Remove deletes a linked worktree directory via git.
func Remove(ctx context.Context, dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	linked, err := IsLinked(ctx, dir)
	if err != nil {
		return err
	}
	if !linked {
		return fmt.Errorf("%s is the primary worktree — use git worktree remove only on linked checkouts", dir)
	}
	mainRoot, err := MainRoot(ctx, dir)
	if err != nil {
		return err
	}
	_, err = run(ctx, mainRoot, "worktree", "remove", dir)
	return err
}

func parsePorcelain(raw string) ([]Entry, error) {
	var out []Entry
	var cur *Entry
	flush := func() {
		if cur == nil || cur.Path == "" {
			return
		}
		out = append(out, *cur)
		cur = nil
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "worktree":
			flush()
			cur = &Entry{Path: fields[1]}
		case "HEAD":
			if cur != nil {
				cur.HEAD = fields[1]
			}
		case "branch":
			if cur != nil && len(fields) >= 2 {
				cur.Branch = strings.TrimPrefix(fields[1], "refs/heads/")
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		}
	}
	flush()
	return out, nil
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}
