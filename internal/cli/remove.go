package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

func removeCmd() *cobra.Command {
	var stack, mode string
	var dryRun, force, jsonOut, volumes bool
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Stop a checkout and drop its allocations",
		Long: `Detect the checkout from this folder (walks up to .siding/, or matches
state.json by path), run siding down for each mode, then delete that
checkout's ports and locks from machine state.json.

Does not delete repo files: .siding/config.yml, compose copies, and
generated .siding/env stay on disk. The next sync/up rewrites env
from a new allocation.

With no --mode, every recipe mode on this checkout is downed and forgotten
(plus any extra modes still recorded in state). Run from the app repo —
you do not need --path if cwd is inside it.

Pass --force to drop state even if compose down fails.
Pass --volumes / -v to also delete named compose volumes.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd, stack, mode, removeOpts{
				dryRun:  dryRun,
				force:   force,
				jsonOut: jsonOut,
				volumes: volumes,
			})
		},
	}
	addPathFlag(cmd)
	cmd.Flags().StringVar(&stack, "stack", "", "Stack name (default: folder basename, minus folder_prefix)")
	cmd.Flags().StringVar(&mode, "mode", "", "One recipe mode (default: every mode on this checkout)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be stopped and forgotten; write nothing")
	cmd.Flags().BoolVar(&force, "force", false, "Drop state even if docker compose down fails")
	cmd.Flags().BoolVarP(&volumes, "volumes", "v", false, "Also remove named volumes (docker compose down --volumes)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

type removeOpts struct {
	dryRun, force, jsonOut, volumes bool
}

func runRemove(cmd *cobra.Command, stack, mode string, opts removeOpts) error {
	start, err := startPathFlag(cmd)
	if err != nil {
		return err
	}
	statePaths, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return err
	}
	store := state.NewStore(statePaths)
	st, err := store.Read()
	if err != nil {
		return err
	}

	if _, err := contract.FindRepoRoot(start); err == nil {
		return removeWithRecipe(cmd, store, st, stack, mode, opts)
	}
	if key, rec, ok := state.FindCheckout(st, start); ok {
		return removeFromState(cmd, store, st, key, rec, mode, opts)
	}
	return fmt.Errorf("no .siding/config.yml and no state.json checkout at or above %s — run from the app folder (or pass --path)", start)
}

func removeWithRecipe(cmd *cobra.Command, store *state.Store, st *state.File, stack, mode string, opts removeOpts) error {
	res, err := resolver.Resolve(mustResolveOpts(cmd, stack, mode))
	if err != nil {
		return err
	}
	rec := st.Checkouts[res.CheckoutKey]
	names := state.CheckoutModeNames(contract.ModeNames(res.Contract), rec, mode)

	var rows []removeRow
	for _, name := range names {
		cur, err := resolver.Resolve(resolver.Options{
			StartPath:     res.RepoRoot,
			StateOverride: globalRoot,
			Stack:         stack,
			Mode:          name,
			Loaded:        st,
		})
		if err != nil {
			if opts.force {
				warnf("warning: resolve %s: %v — continuing\n", name, err)
				rows = append(rows, removeRow{Mode: name, Forgot: state.RemoveMode(st, res.CheckoutKey, name)})
				continue
			}
			return err
		}
		row := removeRow{
			Mode:           cur.Mode,
			Host:           cur.Host,
			ComposeProject: cur.ComposeProject,
			Ports:          cur.Ports,
			EnvFile:        cur.SlotEnvPath,
		}
		if !opts.dryRun {
			if err := downResolved(cmd, cur, st, store, opts.volumes); err != nil {
				if !opts.force {
					return fmt.Errorf("down %s: %w (pass --force to drop state anyway)", name, err)
				}
				warnf("warning: down %s: %v — dropping state anyway\n", name, err)
			}
			row.Forgot = state.RemoveMode(st, cur.CheckoutKey, name)
		}
		rows = append(rows, row)
	}

	if !opts.dryRun {
		if mode == "" {
			state.RemoveCheckout(st, res.CheckoutKey)
		}
		if err := store.Write(st); err != nil {
			return err
		}
		if err := refreshProxy(cmd, st); err != nil {
			warnf("warning: proxy reload: %v\n", err)
		}
	}
	return printRemove(res.CheckoutKey, res.RepoRoot, rows, opts)
}

func removeFromState(cmd *cobra.Command, store *state.Store, st *state.File, key string, rec state.Checkout, mode string, opts removeOpts) error {
	names := state.CheckoutModeNames(nil, rec, mode)
	var rows []removeRow
	for _, name := range names {
		alloc := rec.Modes[name]
		row := removeRow{
			Mode:           name,
			Host:           alloc.Host,
			ComposeProject: alloc.ComposeProject,
			Ports:          alloc.Ports,
		}
		if !opts.dryRun && alloc.ComposeProject != "" {
			args := []string{"compose", "-p", alloc.ComposeProject, "--project-directory", ".", "--profile", "*", "down"}
			if opts.volumes {
				args = append(args, "--volumes")
			}
			printf("%s\n", dim("$ docker "+strings.Join(args, " ")))
			if err := compose.Run(cmd.Context(), key, args); err != nil {
				if !opts.force {
					return fmt.Errorf("down %s: %w (pass --force to drop state anyway)", name, err)
				}
				warnf("warning: down %s: %v — dropping state anyway\n", name, err)
			}
		}
		if !opts.dryRun {
			row.Forgot = state.RemoveMode(st, key, name)
		}
		rows = append(rows, row)
	}
	if !opts.dryRun {
		if mode == "" {
			state.RemoveCheckout(st, key)
		}
		if err := store.Write(st); err != nil {
			return err
		}
		if err := refreshProxy(cmd, st); err != nil {
			warnf("warning: proxy reload: %v\n", err)
		}
	}
	return printRemove(key, key, rows, opts)
}

func downResolved(cmd *cobra.Command, res *resolver.Result, st *state.File, store *state.Store, volumes bool) error {
	attachProxy(cmd.Context(), res, st)
	if err := resolver.WriteSlotEnv(res); err != nil {
		return err
	}
	args, err := compose.DockerArgs(res, "down", nil, compose.Options{Volumes: volumes})
	if err != nil {
		return err
	}
	printf("%s\n", dim("$ docker "+strings.Join(args, " ")))
	if err := compose.Run(cmd.Context(), res.RepoRoot, args); err != nil {
		return err
	}
	state.ReleaseLock(st, res.Contract.Project, res.StackKey, res.Mode, res.RepoRoot)
	return store.Write(st)
}

func mustResolveOpts(cmd *cobra.Command, stack, mode string) resolver.Options {
	start, _ := startPathFlag(cmd)
	if mode == "" {
		mode = "test"
	}
	return resolver.Options{
		StartPath:     start,
		StateOverride: globalRoot,
		Stack:         stack,
		Mode:          mode,
	}
}

type removeRow struct {
	Mode           string
	Host           string
	ComposeProject string
	Ports          map[string]int
	EnvFile        string
	Forgot         bool
}

func printRemove(checkout, repo string, rows []removeRow, opts removeOpts) error {
	if opts.jsonOut {
		return encodeJSON(map[string]any{
			"checkout": checkout,
			"repo":     repo,
			"modes":    rows,
			"dry_run":  opts.dryRun,
		})
	}
	printf("%s %s\n", dim("checkout:"), checkout)
	if repo != "" && repo != checkout {
		printf("%s %s\n", dim("repo:    "), repo)
	}
	for _, row := range rows {
		printf("%s %s\n", dim("mode:    "), bold(row.Mode))
		if row.Host != "" {
			printf("  %s %s\n", dim("host:   "), cyan(row.Host))
		}
		if row.ComposeProject != "" {
			printf("  %s %s\n", dim("project:"), row.ComposeProject)
		}
		if len(row.Ports) > 0 {
			println(dim("  ports:"))
			printPorts(row.Ports)
		}
	}
	switch {
	case opts.dryRun:
		println(yellow("dry run — state not written"))
	default:
		println(green("removed from state.json (recipe on disk is unchanged)"))
	}
	return nil
}
