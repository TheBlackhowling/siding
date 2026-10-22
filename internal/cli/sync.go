package cli

import (
	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/siding"
)

func syncCmd() *cobra.Command {
	var stack, mode string
	var printHosts, dryRun, jsonOut bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Map the org recipe onto this machine's siding state",
		Long: `Read committed .siding/config.yml and apply it locally:

  • allocate (or merge) this checkout's ports in state.json
  • refresh host / compose project from the current recipe patterns
  • write .siding/env (and .siding/env.<mode> for extra compose stacks)
  • reload the shared proxy if it is already running

With no --mode, every recipe mode is allocated so independent compose
files can run at the same time without colliding. Pass --mode NAME to
sync one stack.

Does not start containers and does not copy or adapt compose.
If the recipe already exists, siding init runs this instead of the wizard.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, stack, mode, syncOpts{
				printHosts: printHosts,
				dryRun:     dryRun,
				jsonOut:    jsonOut,
			})
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().BoolVar(&printHosts, "print-hosts", false, "Remind that *.localhost must not be added to the hosts file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show the mapping without writing state or env")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

type syncOpts struct {
	printHosts, dryRun, jsonOut bool
	fromInit                    bool
}

func runSync(cmd *cobra.Command, stack, mode string, opts syncOpts) error {
	target, err := stackTarget(cmd, stack, mode)
	if err != nil {
		return err
	}
	snap := inspectDockerPorts(cmd.Context())
	if snap.Unavailable != "" {
		warnf("warning: docker host ports not inspected (%s) — allocation uses state.json only\n", snap.Unavailable)
	}

	result, err := siding.New(globalRoot).Sync(cmd.Context(), siding.SyncOptions{
		Target:   target,
		AllModes: !cmd.Flags().Changed("mode"),
		DryRun:   opts.dryRun,
	})
	if err != nil {
		return err
	}

	if opts.jsonOut {
		payloads := make([]map[string]any, 0, len(result.Modes))
		for _, row := range result.Modes {
			payloads = append(payloads, syncJSONFromLibrary(row, snap, opts.dryRun))
		}
		if len(payloads) == 1 {
			return encodeJSON(payloads[0])
		}
		return encodeJSON(map[string]any{"modes": payloads})
	}

	if opts.fromInit && len(result.Modes) > 0 {
		printf("%s %s already present — mapping it onto local siding state\n", dim("org recipe"), contract.ConfigPath(result.Modes[0].RepoRoot))
	}
	for i, row := range result.Modes {
		if i > 0 {
			println()
		}
		printSyncRowFromLibrary(row, snap, opts)
	}
	if !opts.dryRun && len(result.Modes) > 0 {
		println()
		printf("%s  %s\n", bold("next:"), cyan("siding doctor && siding up"))
		if len(result.Modes) > 1 {
			printf("       %s   %s\n", cyan("siding up --mode NAME"), dim("# other compose stacks"))
		}
		last := result.Modes[len(result.Modes)-1]
		if !last.ProxyActive {
			printf("       %s   %s\n", cyan("siding proxy up"), dim("# once per machine, Caddy on :80"))
		}
	}
	return nil
}

func syncJSONFromLibrary(row siding.SyncMode, snap dockerps.Snapshot, dryRun bool) map[string]any {
	return map[string]any{
		"recipe":          contract.FileName,
		"checkout_key":    row.CheckoutKey,
		"stack":           row.Stack,
		"mode":            row.Mode,
		"host":            row.Host,
		"compose_project": row.ComposeProject,
		"compose_files":   row.ComposeFiles,
		"ports":           row.Ports,
		"added_ports":     row.AddedPorts,
		"created":         row.Created,
		"docker_ports":    len(snap.HostPorts),
		"docker_host_net": snap.HostNetwork,
		"slot_env_file":   row.SlotEnvPath,
		"slot_env":        row.SlotEnv,
		"proxy_active":    row.ProxyActive,
		"proxy_listen":    row.ProxyListen,
		"state_file":      row.StateFile,
		"dry_run":         dryRun,
		"tips":            row.Tips,
	}
}

func printSyncRowFromLibrary(row siding.SyncMode, snap dockerps.Snapshot, opts syncOpts) {
	printf("%s %s\n", dim("recipe:  "), contract.ConfigPath(row.RepoRoot))
	printf("%s %s\n", dim("checkout:"), row.CheckoutKey)
	printf("%s %s (%s)\n", dim("stack:   "), bold(row.Stack), row.Mode)
	printf("%s %s\n", dim("host:    "), cyan(row.Host))
	printf("%s %s\n", dim("project: "), row.ComposeProject)
	printf("%s %v\n", dim("compose: "), row.ComposeFiles)
	println(dim("ports:"))
	printPorts(row.Ports)
	if row.Created {
		printf("%s %s\n", dim("local:   "), green("allocated"))
	} else if len(row.AddedPorts) > 0 {
		printf("%s merged new ports %v\n", dim("local:   "), row.AddedPorts)
	} else {
		printf("%s %s\n", dim("local:   "), "already allocated (host/env refreshed)")
	}
	if snap.Unavailable == "" {
		printf("%s %d published host ports considered\n", dim("docker:  "), len(snap.HostPorts))
		if len(snap.HostNetwork) > 0 {
			printf("%s host-network containers (binds hidden): %v\n", dim("docker:  "), snap.HostNetwork)
		}
	}
	if len(row.ComposeEnvFiles) > 0 {
		printf("%s %v  %s\n", dim("compose env:"), row.ComposeEnvFiles, dim("(mode "+row.Mode+" only)"))
	}
	if opts.dryRun {
		printf("%s %s\n", dim("env file:"), yellow("(dry run — not written)"))
		printf("%s %s\n", dim("state:   "), yellow("(dry run — not written)"))
	} else {
		printf("%s %s\n", dim("env file:"), row.SlotEnvPath)
		printf("%s %s\n", dim("state:   "), row.StateFile)
	}
	printf("%s %s\n", dim("url:     "), cyan(row.PublicURL))
	if row.ProxyActive {
		printf("%s %s\n", dim("board:   "), cyan(proxy.SiteAddress(proxy.BoardHost, row.ProxyListen)))
	} else {
		printf("%s %s\n", dim("proxy:   "), yellow("off — siding proxy up (Caddy on :80)"))
	}
	if opts.printHosts {
		println()
		printLocalhostHostsNote()
	}
	if len(row.Tips) > 0 {
		println(yellow("tips:"))
		for _, tip := range row.Tips {
			printf("  - %s\n", tip)
		}
	}
}
