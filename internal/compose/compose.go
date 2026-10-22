package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/resolver"
)

type Options struct {
	Profile       string
	Build         bool
	ForceRecreate bool
	Pull          string // always | missing | never; empty = omit
	RemoveOrphans bool
	Volumes       bool
	Timeout       int // seconds; 0 = omit
	Follow        bool
	Tail          string // "100", "all", …
	Since         string
	Timestamps    bool
}

// DockerArgs builds host-side `docker compose …` arguments. Compose files and
// the slot env file are repo-relative so the daemon resolves them from cwd.
func DockerArgs(res *resolver.Result, action string, extra []string, opts Options) ([]string, error) {
	args := []string{"compose"}
	if res.ComposeProject != "" {
		args = append(args, "-p", res.ComposeProject)
	}
	// Files under .siding/compose/ still resolve build/volume paths from the
	// repo root (cwd). Originals at the root are left for non-siding users.
	args = append(args, "--project-directory", ".")
	for _, file := range res.ComposeFiles {
		args = append(args, "-f", file)
	}
	envFile := res.SlotEnvRel
	if envFile == "" {
		envFile = contract.SlotEnvRel(res.Contract, res.Mode)
	}
	if envFile == "" {
		envFile = ".siding/env"
	}
	// Repo env files first so the generated slot file wins on port/host keys.
	repoEnv, _, err := contract.SelectComposeEnvPaths(res.RepoRoot, res.ModeConfig.ComposeEnvFiles)
	if err != nil {
		return nil, err
	}
	for _, extra := range repoEnv {
		if extra == "" || extra == envFile {
			continue
		}
		args = append(args, "--env-file", extra)
	}
	args = append(args, "--env-file", envFile)
	profile := opts.Profile
	// Tear down every service in the project. Plain `down` only stops the
	// default compose model, so profiled containers (or a prior `up --profile`)
	// can keep running and block network removal.
	if profile == "" && action == "down" {
		profile = "*"
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	args = append(args, action)
	args = append(args, actionFlags(action, extra, opts)...)
	args = append(args, extra...)
	return args, nil
}

func actionFlags(action string, extra []string, opts Options) []string {
	var flags []string
	switch action {
	case "up":
		if !hasFlag(extra, "-d", "--detach") {
			flags = append(flags, "-d")
		}
		if opts.Build && !hasFlag(extra, "--build", "--no-build") {
			flags = append(flags, "--build")
		}
		if opts.ForceRecreate && !hasFlag(extra, "--force-recreate", "--no-recreate") {
			flags = append(flags, "--force-recreate")
		}
		if opts.Pull != "" && !hasFlag(extra, "--pull") {
			flags = append(flags, "--pull", opts.Pull)
		}
		if opts.RemoveOrphans && !hasFlag(extra, "--remove-orphans") {
			flags = append(flags, "--remove-orphans")
		}
	case "down":
		if opts.Volumes && !hasFlag(extra, "-v", "--volumes") {
			flags = append(flags, "--volumes")
		}
		if opts.RemoveOrphans && !hasFlag(extra, "--remove-orphans") {
			flags = append(flags, "--remove-orphans")
		}
		if opts.Timeout > 0 && !hasFlag(extra, "-t", "--timeout") {
			flags = append(flags, "--timeout", strconv.Itoa(opts.Timeout))
		}
	case "logs":
		if opts.Follow && !hasFlag(extra, "-f", "--follow") {
			flags = append(flags, "--follow")
		}
		if opts.Tail != "" && !hasFlag(extra, "--tail") {
			flags = append(flags, "--tail", opts.Tail)
		}
		if opts.Since != "" && !hasFlag(extra, "--since") {
			flags = append(flags, "--since", opts.Since)
		}
		if opts.Timestamps && !hasFlag(extra, "-t", "--timestamps") {
			flags = append(flags, "--timestamps")
		}
	}
	return flags
}

func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		for _, name := range names {
			if a == name || strings.HasPrefix(a, name+"=") {
				return true
			}
		}
	}
	return false
}

// NormalizePull accepts Compose --pull values. Empty is omit.
func NormalizePull(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", nil
	}
	switch s {
	case "always", "missing", "never":
		return s, nil
	default:
		return "", fmt.Errorf("pull must be always, missing, or never (got %q)", raw)
	}
}

// Run executes `docker <args>` in repoRoot with inherited stdio.
func Run(ctx context.Context, repoRoot string, args []string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.Error); ok {
			return fmt.Errorf("docker not found on PATH — install Docker Desktop and run siding on the host")
		}
		return err
	}
	return nil
}
