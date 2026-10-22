package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/siding"
)

func upCmd() *cobra.Command {
	var stack, mode, profile, pull string
	var build, forceRecreate, removeOrphans bool
	cmd := &cobra.Command{
		Use:   "up [--] [compose args...]",
		Short: "Start the stack for a repo path",
		Long: `Write .siding/env, claim the slot lock, and run docker compose on the host.

Allocates ports into state.json if this checkout has none yet.
Caddy defaults to host :80. If you overrode --listen away from 80, Caddy must
already be answering on that port — run siding proxy up first.
Run the host binary (not docker/dev.ps1 run) so compose bind mounts resolve.

Always detached. Extra compose arguments after -- :
  siding up --path examples/hello -- --no-deps web`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalized, err := compose.NormalizePull(pull)
			if err != nil {
				return err
			}
			return runStack(cmd, "up", stack, mode, args, compose.Options{
				Profile:       profile,
				Build:         build,
				ForceRecreate: forceRecreate,
				Pull:          normalized,
				RemoveOrphans: removeOrphans,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	cmd.Flags().BoolVar(&build, "build", false, "Pass --build to docker compose up")
	cmd.Flags().BoolVar(&forceRecreate, "force-recreate", false, "Recreate containers even if config is unchanged")
	cmd.Flags().StringVar(&pull, "pull", "", "Pull images before up: always, missing, or never")
	cmd.Flags().BoolVar(&removeOrphans, "remove-orphans", false, "Remove containers for services not in the current compose files")
	return cmd
}

func downCmd() *cobra.Command {
	var stack, mode, profile string
	var volumes, removeOrphans bool
	var timeout int
	cmd := &cobra.Command{
		Use:   "down [--] [compose args...]",
		Short: "Stop the stack for a repo path",
		Long: `docker compose down on the host, release the lock, reload the proxy if running.

When you omit --profile, siding passes --profile "*" so profiled services
from an earlier siding up --profile … are stopped too (avoids "network has
active endpoints" on down). Pass --profile NAME for a partial teardown only.

Keeps port allocations. Named volumes stay unless you pass --volumes / -v.
Extra compose arguments after -- (e.g. -- --rmi local).`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStack(cmd, "down", stack, mode, args, compose.Options{
				Profile:       profile,
				Volumes:       volumes,
				RemoveOrphans: removeOrphans,
				Timeout:       timeout,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	cmd.Flags().BoolVarP(&volumes, "volumes", "v", false, "Remove named volumes declared in compose and anonymous volumes")
	cmd.Flags().BoolVar(&removeOrphans, "remove-orphans", false, "Remove containers for services not in the current compose files")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "Shutdown timeout in seconds (compose --timeout)")
	return cmd
}

func tailCmd() *cobra.Command {
	var stack, mode, profile, tailLines, since string
	var timestamps bool
	cmd := &cobra.Command{
		Use:   "tail [service...] [--] [compose args...]",
		Short: "Follow compose logs for this checkout",
		Long: `docker compose logs --follow on this checkout's project.

Optional service names:  siding tail web api
Does not claim or release the slot lock. Writes .siding/env first so the
compose project matches siding up.`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStack(cmd, "logs", stack, mode, args, compose.Options{
				Profile:    profile,
				Follow:     true,
				Tail:       tailLines,
				Since:      since,
				Timestamps: timestamps,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	cmd.Flags().StringVar(&tailLines, "tail", "100", "Number of lines per container to show from the end (or all)")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since a timestamp or relative time (e.g. 10m)")
	cmd.Flags().BoolVar(&timestamps, "timestamps", false, "Show timestamps")
	return cmd
}

func recycleCmd() *cobra.Command {
	var stack, mode, profile, pull string
	var volumes, build, forceRecreate, removeOrphans bool
	var timeout int
	cmd := &cobra.Command{
		Use:   "recycle [--] [compose args...]",
		Short: "Stop then start the stack for a mode",
		Long: `docker compose down then up on the host for this mode.

Keeps port allocations. Named volumes stay unless you pass --volumes / -v.
Extra compose arguments after -- apply to the up half only.

For a clean slate with rebuilt images and wiped volumes, use siding reset.`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalized, err := compose.NormalizePull(pull)
			if err != nil {
				return err
			}
			if err := runStack(cmd, "down", stack, mode, nil, compose.Options{
				Profile:       profile,
				Volumes:       volumes,
				RemoveOrphans: removeOrphans,
				Timeout:       timeout,
			}); err != nil {
				return err
			}
			return runStack(cmd, "up", stack, mode, args, compose.Options{
				Profile:       profile,
				Build:         build,
				ForceRecreate: forceRecreate,
				Pull:          normalized,
				RemoveOrphans: removeOrphans,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	cmd.Flags().BoolVarP(&volumes, "volumes", "v", false, "Remove named volumes on down")
	cmd.Flags().BoolVar(&removeOrphans, "remove-orphans", false, "Remove containers for services not in the current compose files")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "Shutdown timeout in seconds (compose --timeout)")
	cmd.Flags().BoolVar(&build, "build", false, "Pass --build to docker compose up")
	cmd.Flags().BoolVar(&forceRecreate, "force-recreate", false, "Recreate containers even if config is unchanged")
	cmd.Flags().StringVar(&pull, "pull", "", "Pull images before up: always, missing, or never")
	return cmd
}

func resetCmd() *cobra.Command {
	var stack, mode, profile string
	cmd := &cobra.Command{
		Use:   "reset [--] [compose args...]",
		Short: "Wipe volumes and bring the stack back up rebuilt",
		Long: `Opinionated clean slate:

  docker compose down --volumes --remove-orphans
  docker compose up -d --build --force-recreate --remove-orphans

Extra arguments after -- apply to the up half only.
If you do not want a rebuild, run siding down -v then siding up yourself.`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runStack(cmd, "down", stack, mode, nil, compose.Options{
				Profile:       profile,
				Volumes:       true,
				RemoveOrphans: true,
			}); err != nil {
				return err
			}
			return runStack(cmd, "up", stack, mode, args, compose.Options{
				Profile:       profile,
				Build:         true,
				ForceRecreate: true,
				RemoveOrphans: true,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	return cmd
}

func psCmd() *cobra.Command {
	var stack, mode, profile string
	cmd := &cobra.Command{
		Use:          "ps [--] [compose args...]",
		Short:        "Show stack status for a repo path",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStack(cmd, "ps", stack, mode, args, compose.Options{Profile: profile})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (passed before the action)")
	return cmd
}

func runStack(cmd *cobra.Command, action, stack, mode string, extra []string, opts compose.Options) error {
	target, err := stackTarget(cmd, stack, mode)
	if err != nil {
		return err
	}
	client := siding.New(globalRoot)
	composeOpts := siding.ComposeOptions{
		Profile:       opts.Profile,
		Build:         opts.Build,
		ForceRecreate: opts.ForceRecreate,
		Pull:          opts.Pull,
		RemoveOrphans: opts.RemoveOrphans,
		Volumes:       opts.Volumes,
		Timeout:       opts.Timeout,
	}

	switch action {
	case "up":
		res, err := openResolvedForPrint(cmd, stack, mode)
		if err != nil {
			return err
		}
		if err := printDockerComposeLine(res, action, extra, opts); err != nil {
			return err
		}
		result, err := client.Up(cmd.Context(), siding.UpOptions{
			Target:  target,
			Compose: composeOpts,
			Extra:   extra,
		})
		if err != nil {
			return err
		}
		printStackUp(result)
		return nil
	case "down":
		res, err := openResolvedForPrint(cmd, stack, mode)
		if err != nil {
			return err
		}
		if err := printDockerComposeLine(res, action, extra, opts); err != nil {
			return err
		}
		_, err = client.Down(cmd.Context(), siding.DownOptions{
			Target:  target,
			Compose: composeOpts,
			Extra:   extra,
		})
		return err
	}

	res, _, _, err := openResolved(cmd, stack, mode)
	if err != nil {
		return err
	}
	if err := resolver.WriteSlotEnv(res); err != nil {
		return err
	}
	dockerArgs, err := compose.DockerArgs(res, action, extra, opts)
	if err != nil {
		return err
	}
	printf("%s\n", dim("$ docker "+strings.Join(dockerArgs, " ")))
	runErr := compose.Run(cmd.Context(), res.RepoRoot, dockerArgs)
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("docker compose %s exited %d", action, exitErr.ExitCode())
		}
		return runErr
	}
	return nil
}

func printDockerComposeLine(res *resolver.Result, action string, extra []string, opts compose.Options) error {
	dockerArgs, err := compose.DockerArgs(res, action, extra, opts)
	if err != nil {
		return err
	}
	printf("%s\n", dim("$ docker "+strings.Join(dockerArgs, " ")))
	if action == "up" {
		_, skipped, _ := contract.SelectComposeEnvPaths(res.RepoRoot, res.ModeConfig.ComposeEnvFiles)
		if len(skipped) > 0 {
			printf("%s %v\n", dim("skipped optional compose env (missing):"), skipped)
		}
	}
	return nil
}

func stackTarget(cmd *cobra.Command, stack, mode string) (siding.Target, error) {
	start, err := startPathFlag(cmd)
	if err != nil {
		return siding.Target{}, err
	}
	return siding.Target{StartPath: start, Stack: stack, Mode: mode}, nil
}

func openResolvedForPrint(cmd *cobra.Command, stack, mode string) (*resolver.Result, error) {
	res, _, _, err := openResolved(cmd, stack, mode)
	return res, err
}

func printStackUp(result siding.StackResult) {
	println()
	printf("%s %s (%s)\n", dim("stack:   "), bold(result.Stack), result.Mode)
	printf("%s %s\n", dim("project: "), result.ComposeProject)
	printf("%s %s\n", dim("url:     "), cyan(result.PublicURL))
	if result.ProxyActive {
		printf("%s %s\n", dim("board:   "), cyan(proxy.SiteAddress(proxy.BoardHost, result.ProxyListen)))
	} else {
		printf("%s %s\n", dim("proxy:   "), yellow("off (siding proxy up — Caddy on :80)"))
	}
	if len(result.ComposeEnvFiles) > 0 {
		printf("%s %v  %s\n", dim("compose env:"), result.ComposeEnvFiles, dim("(mode "+result.Mode+" only)"))
	}
	if len(result.SkippedOptionalComposeEnvFiles) > 0 {
		printf("%s %v\n", dim("skipped optional compose env (missing):"), result.SkippedOptionalComposeEnvFiles)
	}
	printf("%s %s\n", dim("env file:"), result.SlotEnvPath)
	println(dim("ports:"))
	printPorts(result.Ports)
}

func refreshProxy(cmd *cobra.Command, st *state.File) error {
	p, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return err
	}
	return proxy.RefreshIfRunning(cmd.Context(), p.ProxyDir, st)
}
