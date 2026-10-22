package cli

import (
	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/siding"
)

func addCmd() *cobra.Command {
	var stack, mode string
	var jsonOut, dryRun bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Pre-allocate globally unique ports for a checkout",
		Long: `Record ports for this checkout in machine state.json.

Uses lifted host ports (compose 3000 → 18000, 5432 → 20432, …) when
claiming new keys, and treats running Docker publishes as already taken.

Does not start containers. Idempotent: existing allocations are kept.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := stackTarget(cmd, stack, mode)
			if err != nil {
				return err
			}
			result, err := siding.New(globalRoot).Add(cmd.Context(), siding.AddOptions{
				Target: target,
				DryRun: dryRun,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				return encodeJSON(map[string]any{
					"checkout_key":    result.CheckoutKey,
					"stack":           result.Stack,
					"mode":            result.Mode,
					"host":            result.Host,
					"compose_project": result.ComposeProject,
					"ports":           result.Ports,
					"state_file":      result.StateFile,
					"created":         result.Created,
					"dry_run":         dryRun,
				})
			}

			printf("%s %s\n", dim("checkout: "), result.CheckoutKey)
			printf("%s %s (%s)\n", dim("stack:    "), bold(result.Stack), result.Mode)
			printf("%s %s\n", dim("project:  "), result.ComposeProject)
			printf("%s %s\n", dim("host:     "), cyan(result.Host))
			println(dim("ports:"))
			printPorts(result.Ports)
			printf("%s %s\n", dim("state:    "), result.StateFile)
			switch {
			case dryRun:
				println(yellow("dry run — state not written"))
			case result.Created:
				println(green("allocated and written"))
			default:
				println("already allocated")
			}
			return nil
		},
	}
	addStackModeFlags(cmd, &stack, &mode)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print allocation without writing state")
	return cmd
}
