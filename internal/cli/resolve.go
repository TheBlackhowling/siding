package cli

import (
	"context"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/dockerps"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
	"github.com/TheBlackhowling/siding/internal/state"
)

func addPathFlag(cmd *cobra.Command) {
	cmd.Flags().String("path", "", "Start path (default: current directory; walks up to .siding/)")
}

func addStackModeFlags(cmd *cobra.Command, stack, mode *string) {
	addPathFlag(cmd)
	cmd.Flags().StringVar(stack, "stack", "", "Stack name (default: folder basename, minus folder_prefix)")
	cmd.Flags().StringVar(mode, "mode", "test", "Mode name (default: test, or the first recipe mode)")
}

func resolveOpts(cmd *cobra.Command, stack, mode string) (resolver.Options, error) {
	start, err := startPathFlag(cmd)
	if err != nil {
		return resolver.Options{}, err
	}
	return resolver.Options{
		StartPath:     start,
		StateOverride: globalRoot,
		Stack:         stack,
		Mode:          mode,
	}, nil
}

func openResolved(cmd *cobra.Command, stack, mode string) (*resolver.Result, *state.Store, *state.File, error) {
	opts, err := resolveOpts(cmd, stack, mode)
	if err != nil {
		return nil, nil, nil, err
	}
	res, err := resolver.Resolve(opts)
	if err != nil {
		return nil, nil, nil, err
	}
	statePaths, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	store := state.NewStore(statePaths)
	st, err := store.Read()
	if err != nil {
		return nil, nil, nil, err
	}
	if state.MigrateOffDefaultPorts(st) {
		if err := store.Write(st); err != nil {
			return nil, nil, nil, err
		}
	}
	return res, store, st, nil
}

func liveDockerPorts(ctx context.Context) map[int]struct{} {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := dockerps.Inspect(ctx)
	if err != nil {
		return map[int]struct{}{}
	}
	return snap.PortSet()
}

func inspectDockerPorts(ctx context.Context) dockerps.Snapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := dockerps.Inspect(ctx)
	if err != nil {
		return dockerps.Snapshot{HostPorts: map[int]struct{}{}, Holders: map[int][]string{}, Unavailable: err.Error()}
	}
	return snap
}

func printPorts(ports map[string]int) {
	keys := make([]string, 0, len(ports))
	for k := range ports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		printf("  %s: %s\n", dim(k), bold(strconv.Itoa(ports[k])))
	}
}

func attachProxy(ctx context.Context, res *resolver.Result, st *state.File) {
	if !proxy.InspectRunning(ctx) {
		return
	}
	res.ProxyActive = true
	res.ProxyListen = state.ListenPort(st)
}

func publicURL(res *resolver.Result) string {
	return proxy.PublicURL(contract.PublicHost(res.Host, "web", res.HTTPHosts), res.Ports, res.ProxyActive, res.ProxyListen)
}
