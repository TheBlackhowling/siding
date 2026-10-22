package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/gitworktree"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

func worktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Add or remove git worktrees as siding slots",
		Long: `Manage git worktrees as sibling folders and map them into siding.

New worktrees are created next to the current checkout's parent
(acme-betty + changelog → ../acme-changelog). Each add runs
siding sync only — start stacks with siding up yourself.

Remove runs siding down -v, siding remove, then git worktree remove.
If a step fails, siding prints what finished and what to run manually.

Sync lists git worktrees and runs siding sync on any checkout missing
from state.json (for example after a plain git worktree add).`,
	}
	cmd.AddCommand(worktreeAddCmd())
	cmd.AddCommand(worktreeRemoveCmd())
	cmd.AddCommand(worktreeListCmd())
	cmd.AddCommand(worktreeSyncCmd())
	return cmd
}

func worktreeAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add NAME BRANCH [from]",
		Short: "Create a sibling worktree and siding sync it",
		Long: `Create ../{folder_prefix}NAME as a git worktree, then siding sync.

NAME is the siding stack / folder suffix (acme-betty + genie →
../acme-genie). BRANCH is the new git branch (-b). Optional from defaults to the
recipe primary_branch (init records this, default main):

  siding worktree add genie feat/my-work
  siding worktree add genie feat/my-work v0.1.4

runs: git worktree add -b feat/my-work ../acme-genie main`,
		Args:         cobra.RangeArgs(2, 3),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			from := ""
			if len(args) == 3 {
				from = args[2]
			}
			return runWorktreeAdd(cmd, args[0], args[1], from)
		},
	}
	addPathFlag(cmd)
	return cmd
}

func worktreeRemoveCmd() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "remove [NAME]",
		Short: "Stop, forget, and remove a linked worktree",
		Long: `Remove a linked worktree by NAME from any checkout with the recipe,
run from the linked folder, or pass --path to it:

  siding worktree remove genie
  siding worktree remove --path C:\source\acme-genie

Steps:
  1. siding down -v for every mode
  2. siding remove
  3. git worktree remove

NAME is the same stack suffix as worktree add (acme-betty + genie →
../acme-genie). The primary worktree cannot be removed. Pass --force to
continue after compose down/remove failures. --dry-run prints the plan only.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runWorktreeRemove(cmd, name, worktreeRemoveOpts{force: force, dryRun: dryRun})
		},
	}
	addPathFlag(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "Continue after siding down/remove failures")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show steps without running them")
	return cmd
}

func worktreeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List git worktrees and siding slot status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeList(cmd)
		},
	}
	addPathFlag(cmd)
	return cmd
}

func worktreeSyncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync git worktrees missing from siding state",
		Long: `List git worktrees and run siding sync on any checkout not yet in
state.json — for example after a plain git worktree add.

Does not create worktrees and does not run siding up. Skips bare worktrees
and paths without a committed recipe.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeSync(cmd, dryRun)
		},
	}
	addPathFlag(cmd)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show which checkouts would be synced")
	return cmd
}

func runWorktreeAdd(cmd *cobra.Command, name, gitBranch, startPoint string) error {
	start, err := startPathFlag(cmd)
	if err != nil {
		return err
	}
	mainRoot, err := contract.FindRepoRoot(start)
	if err != nil {
		return err
	}
	c, err := contract.Load(mainRoot)
	if err != nil {
		return err
	}
	if !contract.HasContract(mainRoot) {
		return fmt.Errorf("no %s at %s — init or sync the main checkout first", contract.FileName, mainRoot)
	}

	dest, err := gitworktree.SiblingPath(mainRoot, c.FolderPrefix, name)
	if err != nil {
		return err
	}
	stack, err := contract.ResolveStackName(c, dest, "")
	if err != nil {
		return err
	}

	gitBranch = strings.TrimSpace(gitBranch)
	startPoint = strings.TrimSpace(startPoint)
	if startPoint == "" {
		startPoint = c.PrimaryBranchName()
	}
	printf("%s %s\n", dim("worktree:"), dest)
	printf("%s %s\n", dim("stack:    "), stack)
	printf("%s %s %s\n", dim("branch:   "), gitBranch, dim("(from "+startPoint+")"))
	printf("%s\n", dim("$ git worktree add -b "+gitBranch+" ... "+startPoint))

	if err := gitworktree.Add(cmd.Context(), mainRoot, dest, gitBranch, startPoint); err != nil {
		return err
	}

	if err := syncWorktreeCheckout(cmd, dest, false); err != nil {
		return fmt.Errorf(`worktree created at %s but siding sync failed: %w

The folder exists on disk. Fix the error and run:
  siding sync --path %s`, dest, err, dest)
	}

	println()
	printf("%s %s\n", green("worktree ready"), dest)
	printf("%s %s\n", dim("next:"), cyan("siding up --path "+dest))
	println(dim("Open this folder as its own workspace before editing it."))
	return nil
}

type worktreeRemoveOpts struct {
	force, dryRun bool
}

func resolveWorktreeRemoveTarget(cmd *cobra.Command, name string) (string, error) {
	start, err := startPathFlag(cmd)
	if err != nil {
		return "", err
	}
	repoRoot, err := contract.FindRepoRoot(start)
	if err != nil {
		return "", err
	}
	if !contract.HasContract(repoRoot) {
		return "", fmt.Errorf("no %s at %s — init or sync a checkout with a recipe first", contract.FileName, repoRoot)
	}
	c, err := contract.Load(repoRoot)
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	if name != "" {
		target, err := gitworktree.SiblingPath(repoRoot, c.FolderPrefix, name)
		if err != nil {
			return "", err
		}
		captured, err := filepath.Abs(target)
		if err != nil {
			return "", err
		}
		linked, err := gitworktree.IsLinked(cmd.Context(), captured)
		if err != nil {
			return "", fmt.Errorf("worktree %s: %w", captured, err)
		}
		if !linked {
			return "", fmt.Errorf("%s is not a linked worktree (check NAME or run siding worktree list)", captured)
		}
		return captured, nil
	}

	captured, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	linked, err := gitworktree.IsLinked(cmd.Context(), captured)
	if err != nil {
		return "", err
	}
	if !linked {
		return "", fmt.Errorf("%s is the primary worktree — pass NAME or run from a linked checkout", captured)
	}
	return captured, nil
}

func runWorktreeRemove(cmd *cobra.Command, name string, opts worktreeRemoveOpts) error {
	captured, err := resolveWorktreeRemoveTarget(cmd, name)
	if err != nil {
		return err
	}

	if opts.dryRun {
		printf("%s %s\n", dim("path:"), captured)
		println(dim("plan:"))
		println("  1. siding down -v (every mode)")
		println("  2. siding remove")
		println("  3. git worktree remove")
		return nil
	}

	var downModes []string
	res, store, st, err := openResolvedAt(cmd, captured, "", "test")
	if err != nil {
		return fmt.Errorf("resolve checkout %s: %w", captured, err)
	}
	modeNames := state.CheckoutModeNames(contract.ModeNames(res.Contract), st.Checkouts[res.CheckoutKey], "")

	for _, modeName := range modeNames {
		cur, err := resolver.Resolve(resolver.Options{
			StartPath:     captured,
			StateOverride: globalRoot,
			Mode:          modeName,
			Loaded:        st,
		})
		if err != nil {
			if opts.force {
				warnf("warning: resolve %s: %v — continuing\n", modeName, err)
				continue
			}
			return worktreeRemoveFailed(captured, "resolve "+modeName, downModes, nil, err)
		}
		if err := downResolved(cmd, cur, st, store, true); err != nil {
			if !opts.force {
				return worktreeRemoveFailed(captured, "down "+modeName, downModes, nil, err)
			}
			warnf("warning: down %s: %v — continuing\n", modeName, err)
		} else {
			downModes = append(downModes, modeName)
		}
	}

	rc := removeCmd()
	rc.SetContext(cmd.Context())
	if err := rc.Flags().Set("path", captured); err != nil {
		return err
	}
	if err := runRemove(rc, "", "", removeOpts{force: opts.force}); err != nil {
		if !opts.force {
			return worktreeRemoveFailed(captured, "siding remove", downModes, nil, err)
		}
		warnf("warning: siding remove: %v — continuing\n", err)
	}

	if err := gitworktree.Remove(cmd.Context(), captured); err != nil {
		return worktreeRemoveFailed(captured, "git worktree remove", downModes, []string{"siding remove"}, err)
	}

	printf("%s %s\n", green("removed worktree"), captured)
	return nil
}

func worktreeRemoveFailed(path, step string, downDone, skipped []string, err error) error {
	var b strings.Builder
	fmt.Fprintf(&b, "worktree remove stopped at %s\n\n", step)
	fmt.Fprintf(&b, "  path:    %s\n", path)
	if len(downDone) > 0 {
		fmt.Fprintf(&b, "  down:    %s\n", strings.Join(downDone, ", "))
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&b, "  done:    %s\n", strings.Join(skipped, ", "))
	}
	fmt.Fprintf(&b, "  failed:  %v\n", err)
	fmt.Fprintf(&b, "\nFix the error, then run:\n")
	fmt.Fprintf(&b, "  siding remove --path %s --force\n", path)
	fmt.Fprintf(&b, "  git worktree remove %s\n", path)
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

func syncWorktreeCheckout(cmd *cobra.Command, path string, dryRun bool) error {
	sc := syncCmd()
	sc.SetContext(cmd.Context())
	if err := sc.Flags().Set("path", path); err != nil {
		return err
	}
	return runSync(sc, "", "test", syncOpts{dryRun: dryRun})
}

func runWorktreeSync(cmd *cobra.Command, dryRun bool) error {
	start, err := startPathFlag(cmd)
	if err != nil {
		return err
	}
	repoRoot, err := contract.FindRepoRoot(start)
	if err != nil {
		return err
	}
	if !contract.HasContract(repoRoot) {
		return fmt.Errorf("no %s at %s — init or sync the main checkout first", contract.FileName, repoRoot)
	}

	entries, err := gitworktree.List(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}

	statePaths, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return err
	}
	st, err := state.NewStore(statePaths).Read()
	if err != nil {
		return err
	}

	var synced, skipped, noRecipe []string
	for _, e := range entries {
		if e.Bare {
			continue
		}
		path, err := filepath.Abs(e.Path)
		if err != nil {
			return err
		}
		key := paths.NormalizeLockPath(path)
		if _, inState := st.Checkouts[key]; inState {
			skipped = append(skipped, path)
			continue
		}
		if !contract.HasContract(path) {
			noRecipe = append(noRecipe, path)
			continue
		}
		if dryRun {
			printf("%s %s\n", yellow("would sync"), path)
			synced = append(synced, path)
			continue
		}
		printf("%s %s\n", dim("syncing:"), path)
		if err := syncWorktreeCheckout(cmd, path, false); err != nil {
			return fmt.Errorf("sync %s: %w", path, err)
		}
		synced = append(synced, path)
	}

	for _, path := range noRecipe {
		warnf("warning: skipping %s — no %s (commit the recipe or run siding init there)\n", path, contract.FileName)
	}

	if len(synced) == 0 && len(noRecipe) == 0 {
		println(green("all worktrees already in siding state"))
		return nil
	}
	if dryRun {
		printf("%s %d checkout(s)\n", dim("would sync:"), len(synced))
		if len(skipped) > 0 {
			printf("%s %d checkout(s)\n", dim("already in state:"), len(skipped))
		}
		return nil
	}
	if len(synced) > 0 {
		println()
		printf("%s %d checkout(s)\n", green("synced"), len(synced))
	}
	if len(skipped) > 0 {
		printf("%s %d checkout(s)\n", dim("already in state:"), len(skipped))
	}
	return nil
}

func runWorktreeList(cmd *cobra.Command) error {
	start, err := startPathFlag(cmd)
	if err != nil {
		return err
	}
	mainRoot, err := contract.FindRepoRoot(start)
	if err != nil {
		return err
	}
	c, err := contract.Load(mainRoot)
	if err != nil {
		return err
	}

	entries, err := gitworktree.List(cmd.Context(), mainRoot)
	if err != nil {
		return err
	}

	statePaths, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return err
	}
	st, err := state.NewStore(statePaths).Read()
	if err != nil {
		return err
	}

	println(dim("worktrees:"))
	for _, e := range entries {
		key := paths.NormalizeLockPath(e.Path)
		rec, inState := st.Checkouts[key]
		stack := "?"
		if inState {
			stack = rec.Stack
		} else if contract.HasContract(e.Path) {
			if s, err := contract.ResolveStackName(c, e.Path, ""); err == nil {
				stack = s
			}
		}
		running := false
		if inState {
			for mode := range rec.Modes {
				if stateLockHeld(st, rec.Project, rec.Stack, mode, key) {
					running = true
					break
				}
			}
		}
		status := dim("not in state.json")
		if inState {
			if running {
				status = green("up")
			} else {
				status = yellow("allocated")
			}
		}
		branch := e.Branch
		if branch == "" {
			branch = dim("(detached)")
		}
		printf("  %s\n", bold(e.Path))
		printf("    %s %s   %s %s   %s\n", dim("stack:"), stack, dim("branch:"), branch, status)
	}
	return nil
}

func stateLockHeld(f *state.File, project, stack, mode, checkout string) bool {
	if f == nil || f.Locks == nil {
		return false
	}
	entry, ok := f.Locks[state.LockKey(project, stack, mode)]
	if !ok {
		return false
	}
	return paths.NormalizeLockPath(entry.Checkout) == paths.NormalizeLockPath(checkout)
}

func openResolvedAt(cmd *cobra.Command, repoPath, stack, mode string) (*resolver.Result, *state.Store, *state.File, error) {
	if err := cmd.Flags().Set("path", repoPath); err != nil {
		return nil, nil, nil, err
	}
	return openResolved(cmd, stack, mode)
}
