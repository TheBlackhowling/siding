package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/doctor"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/update"
	"github.com/TheBlackhowling/siding/internal/version"
	"github.com/TheBlackhowling/siding/siding"
)

var globalRoot string

func Execute() error {
	return rootCmd().Execute()
}

func rootCmd() *cobra.Command {
	// Nested commands (proxy up) must still run root Persistent* hooks.
	cobra.EnableTraverseRunHooks = true
	root := &cobra.Command{
		Use:   "siding",
		Short: "Park a stack on a siding — named local Docker stacks across repos",
		Long: `Siding allocates named slots (ports, compose project, *.localhost hosts)
from the repo you run in (or --path). Compose always runs on the host.

Run from a repo checkout, or pass --path. Siding walks up to .siding/config.yml.
Machine state is a single path-keyed state.json under your user data directory.

See https://github.com/TheBlackhowling/siding-releases`,
	}
	root.PersistentFlags().StringVar(&globalRoot, "root", "", "Override state file or directory (also SIDING_ROOT)")
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		bindStdOut(cmd)
	}
	root.PersistentPostRun = func(cmd *cobra.Command, _ []string) {
		maybeNotifyUpdate(cmd)
		resetStdOut()
	}

	root.AddCommand(versionCmd())
	root.AddCommand(installCmd())
	root.AddCommand(updateCmd())
	root.AddCommand(initCmd())
	root.AddCommand(syncCmd())
	root.AddCommand(doctorCmd())
	root.AddCommand(planCmd())
	root.AddCommand(addCmd())
	root.AddCommand(upCmd())
	root.AddCommand(downCmd())
	root.AddCommand(recycleCmd())
	root.AddCommand(resetCmd())
	root.AddCommand(tailCmd())
	root.AddCommand(removeCmd())
	root.AddCommand(worktreeCmd())
	root.AddCommand(psCmd())
	root.AddCommand(proxyCmd())
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Info()
			printf("%s %s\n%s %s\n%s %s\n%s %s\n",
				dim("siding"), bold(info["version"]),
				dim("commit:"), info["commit"],
				dim("built:"), info["date"],
				dim("go:"), info["go_version"])
		},
	}
}

func doctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check install, contract, state, and Docker Compose",
		RunE: func(cmd *cobra.Command, args []string) error {
			start, err := startPathFlag(cmd)
			if err != nil {
				return err
			}
			report := doctor.Run(context.Background(), start, globalRoot)
			if jsonOut {
				return encodeJSON(report)
			}
			printDoctor(report)
			if !report.OK {
				return fmt.Errorf("doctor found failures")
			}
			return nil
		},
	}
	cmd.Flags().String("path", "", "Start path (default: current directory; walks up to .siding/)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func planCmd() *cobra.Command {
	var stack, mode string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Resolve slot env and compose plan without starting containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := stackTarget(cmd, stack, mode)
			if err != nil {
				return err
			}
			plan, err := siding.New(globalRoot).Plan(cmd.Context(), target)
			if err != nil {
				return err
			}
			if jsonOut {
				return encodeJSON(map[string]any{
					"start_path":        plan.StartPath,
					"repo_root":         plan.RepoRoot,
					"checkout_key":      plan.CheckoutKey,
					"user_data_root":    plan.UserDataRoot,
					"state_file":        plan.StateFile,
					"stack":             plan.Stack,
					"mode":              plan.Mode,
					"host":              plan.Host,
					"compose_project":   plan.ComposeProject,
					"compose_files":     plan.ComposeFiles,
					"compose_env_files":              plan.ComposeEnvFiles,
					"skipped_optional_compose_env_files": plan.SkippedOptionalComposeEnvFiles,
					"ports":                          plan.Ports,
					"slot_env_file":     plan.SlotEnvPath,
					"slot_env":          plan.SlotEnv,
					"tips":              plan.Tips,
				})
			}
			printPlanFromLibrary(plan)
			return nil
		},
	}
	cmd.Flags().String("path", "", "Start path (default: current directory; walks up to .siding/)")
	cmd.Flags().StringVar(&stack, "stack", "", "Stack name (default: folder basename, minus folder_prefix)")
	cmd.Flags().StringVar(&mode, "mode", "test", "Mode name (default: test)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func printPlanFromLibrary(plan siding.Plan) {
	printf("%s %s\n", dim("repo:     "), plan.RepoRoot)
	printf("%s %s\n", dim("checkout: "), plan.CheckoutKey)
	printf("%s %s\n", dim("state:    "), plan.StateFile)
	printf("%s %s (%s)\n", dim("stack:    "), bold(plan.Stack), plan.Mode)
	printf("%s %s\n", dim("host:     "), cyan(plan.Host))
	printf("%s %s\n", dim("project:  "), plan.ComposeProject)
	printf("%s %v\n", dim("compose:  "), plan.ComposeFiles)
	if len(plan.ComposeEnvFiles) > 0 {
		printf("%s %v  %s\n", dim("compose env:"), plan.ComposeEnvFiles, dim("(mode "+plan.Mode+" only)"))
	}
	if len(plan.SkippedOptionalComposeEnvFiles) > 0 {
		printf("%s %v\n", dim("skipped optional compose env (missing):"), plan.SkippedOptionalComposeEnvFiles)
	}
	printf("%s %s\n", dim("env file: "), plan.SlotEnvPath)
	println(dim("ports:"))
	printPorts(plan.Ports)
	if len(plan.Tips) > 0 {
		println(yellow("tips:"))
		for _, tip := range plan.Tips {
			printf("  - %s\n", tip)
		}
	}
}

func printDoctor(r doctor.Report) {
	for _, c := range r.Checks {
		icon := green("ok")
		switch c.Status {
		case "fail":
			icon = red("FAIL")
		case "warn":
			icon = yellow("warn")
		}
		printf("[%s] %s: %s\n", icon, c.Name, c.Message)
	}
	if len(r.Tips) > 0 {
		println()
		heading("Tips")
		for _, tip := range r.Tips {
			printf("  • %s\n", tip)
		}
	}
	println()
	if r.OK {
		println(green("All required checks passed."))
	} else {
		println(red("Fix failures above before siding up."))
	}
}

func startPathFlag(cmd *cobra.Command) (string, error) {
	path, _ := cmd.Flags().GetString("path")
	return paths.Abs(path)
}

func maybeNotifyUpdate(cmd *cobra.Command) {
	if update.Disabled() || !update.IsReleaseVersion(version.Version) {
		return
	}
	if cmd == nil || jsonRequested(cmd) {
		return
	}
	switch cmd.Name() {
	case "help", "completion", "update":
		return
	}
	p, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), update.HTTPTimeout)
	defer cancel()
	got, err := update.Check(ctx, update.Options{
		Current:  version.Version,
		CacheDir: p.UserDataRoot,
	})
	if err != nil || !got.Newer {
		return
	}
	warnf("update available: %s (this binary is %s). Run: siding update\n",
		update.Display(got.Latest), update.Display(got.Current))
}

func jsonRequested(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		f := c.Flags().Lookup("json")
		if f == nil || !f.Changed {
			continue
		}
		v, err := c.Flags().GetBool("json")
		return err == nil && v
	}
	return false
}
