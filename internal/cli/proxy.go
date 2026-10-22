package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/state"
	"github.com/TheBlackhowling/siding/siding"
)

func proxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Manage the shared loopback reverse proxy",
		Long: `Machine-wide Caddy on 127.0.0.1 routing *.localhost hosts from
state.json to each checkout's allocated ports.

Caddy listens on host :80 by design (override with --listen). Init and
proxy up refuse to use :80 if something else already holds it, and try
to name the occupant. Reload posts the generated Caddyfile to Caddy's
admin API on 127.0.0.1:2019 (unauthenticated; any local process can
use it — same trust as Docker on your account). docker exec reload is
the fallback.

Run the host binary so compose bind-mounts the generated Caddyfile from your user data directory.`,
	}
	cmd.AddCommand(proxyUpCmd())
	cmd.AddCommand(proxyDownCmd())
	cmd.AddCommand(proxyReloadCmd())
	cmd.AddCommand(proxyPsCmd())
	return cmd
}

func proxyUpCmd() *cobra.Command {
	var listen int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:          "up",
		Short:        "Start the shared Caddy proxy on loopback",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			printf("%s\n", dim("$ docker "+strings.Join(proxy.ComposeUpArgs(), " ")))
			cfg, err := siding.New(globalRoot).ProxyUp(cmd.Context(), siding.ProxyUpOptions{Listen: listen})
			if err != nil {
				return err
			}
			return printProxyResultFromLibrary(cfg, jsonOut, "up")
		},
	}
	cmd.Flags().IntVar(&listen, "listen", 0, "Host HTTP port (default: 80). Must be free unless it is already siding-proxy-caddy.")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func proxyDownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "down",
		Short:        "Stop the shared Caddy proxy",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			printf("%s\n", dim("$ docker compose -f "+proxy.ComposeFileName+" down"))
			return siding.New(globalRoot).ProxyDown(cmd.Context())
		},
	}
	return cmd
}

func proxyReloadCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:          "reload",
		Short:        "Rewrite routes from state.json and reload Caddy via its admin API",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := siding.New(globalRoot).ProxyReload(cmd.Context())
			if err != nil {
				return err
			}
			return printProxyResultFromLibrary(cfg, jsonOut, "reload")
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func proxyPsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "ps",
		Short:        "Show the shared proxy container",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, _, err := openProxyStore()
			if err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(p.ProxyDir, proxy.ComposeFileName)); err != nil {
				if os.IsNotExist(err) {
					println(yellow("proxy is not initialized — siding proxy up"))
					return nil
				}
				return err
			}
			return proxy.Ps(cmd.Context(), p.ProxyDir)
		},
	}
	return cmd
}

func openProxyStore() (paths.Paths, *state.Store, *state.File, error) {
	p, err := paths.ResolvePaths(globalRoot)
	if err != nil {
		return paths.Paths{}, nil, nil, err
	}
	store := state.NewStore(p)
	st, err := store.Read()
	if err != nil {
		return paths.Paths{}, nil, nil, err
	}
	return p, store, st, nil
}

func printProxyResultFromLibrary(cfg siding.ProxyConfig, jsonOut bool, action string) error {
	hosts := make([]string, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		hosts = append(hosts, r.Host)
	}
	if jsonOut {
		return encodeJSON(map[string]any{
			"action":    action,
			"listen":    cfg.Listen,
			"board":     cfg.BoardURL,
			"admin":     cfg.AdminURL,
			"proxy_dir": cfg.ProxyDir,
			"caddyfile": cfg.CaddyfilePath,
			"routes":    cfg.Routes,
		})
	}
	println()
	printf("%s %s\n", dim("proxy:    "), cyan(fmt.Sprintf("http://127.0.0.1:%d", cfg.Listen)))
	printf("%s %s\n", dim("board:    "), cyan(cfg.BoardURL))
	if len(cfg.SkippedPublishes) > 0 {
		printf("%s skipped publishing %v (already in use — Caddy still on :%d)\n", dim("note:     "), cfg.SkippedPublishes, cfg.Listen)
	}
	printf("%s %s %s\n", dim("api:      "), cyan(cfg.AdminURL), dim("(Caddy admin — siding proxy reload)"))
	if cfg.Listen == 80 {
		printf("%s %s  %s\n", dim("urls:     "), cyan("http://<host>"), dim("(Caddy on host :80)"))
	} else {
		printf("%s %s  %s\n", dim("urls:     "), cyan(fmt.Sprintf("http://<host>:%d", cfg.Listen)), dim("(Caddy http_port rewrite)"))
	}
	printf("%s %s\n", dim("caddyfile:"), cfg.CaddyfilePath)
	printf("%s %s\n", dim("routes:   "), bold(fmt.Sprintf("%d", len(cfg.Routes))))
	if len(hosts) > 0 {
		printf("%s %s\n", dim("hosts:    "), strings.Join(hosts, ", "))
	}
	return nil
}
