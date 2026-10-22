package gitinfo

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// DefaultRemoteBranch returns the branch origin/HEAD points at (e.g. main), or "".
func DefaultRemoteBranch(ctx context.Context, dir string) string {
	ref := run(ctx, dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	const prefix = "refs/remotes/origin/"
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	branch := strings.TrimPrefix(ref, prefix)
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

// Snapshot is HEAD at dir, or empty strings if git is missing / not a repo.
func Snapshot(ctx context.Context, dir string) (branch, sha string) {
	if strings.TrimSpace(dir) == "" {
		return "", ""
	}
	branch = run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	sha = run(ctx, dir, "rev-parse", "--short", "HEAD")
	return branch, sha
}

func run(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}
