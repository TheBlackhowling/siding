package proxy

import (
	"sort"
	"strconv"
	"strings"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/state"
)

// UpstreamHost is how Caddy (in Docker) reaches ports published on the host.
const UpstreamHost = "host.docker.internal"

type Route struct {
	Host     string
	Upstream int
	PortKey  string
	Checkout string
	Stack    string
	Mode     string
}

// CollectRoutes builds HTTP reverse-proxy sites from every checkout in state.json.
//
//	web  → {host}, plus recipe http_hosts.web labels (ServiceHost)
//	api  → ServiceHost (api.acme… when host is app.acme…; else api.{host})
//
// TCP keys (redis, postgres, …) are allocated as Docker host ports only — stock
// Caddy cannot route them by hostname. First host wins when two allocations collide.
func CollectRoutes(f *state.File) []Route {
	if f == nil {
		return nil
	}
	byHost := map[string]Route{}
	add := func(r Route) {
		r.Host = strings.TrimSpace(strings.ToLower(r.Host))
		if r.Host == "" || r.Upstream < 1 || r.Upstream > 65535 {
			return
		}
		if _, exists := byHost[r.Host]; exists {
			return
		}
		byHost[r.Host] = r
	}

	checkouts := make([]string, 0, len(f.Checkouts))
	for k := range f.Checkouts {
		checkouts = append(checkouts, k)
	}
	sort.Strings(checkouts)

	for _, checkout := range checkouts {
		rec := f.Checkouts[checkout]
		modes := make([]string, 0, len(rec.Modes))
		for m := range rec.Modes {
			modes = append(modes, m)
		}
		sort.Strings(modes)
		for _, mode := range modes {
			alloc := rec.Modes[mode]
			if strings.TrimSpace(alloc.Host) == "" || len(alloc.Ports) == 0 {
				continue
			}
			host := contract.CanonicalHost(alloc.Host)
			if host == "" {
				continue
			}
			keys := make([]string, 0, len(alloc.Ports))
			for k := range alloc.Ports {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			if web, ok := alloc.Ports["web"]; ok {
				add(Route{Host: host, Upstream: web, PortKey: "web", Checkout: checkout, Stack: rec.Stack, Mode: mode})
			}
			for _, key := range keys {
				if !HTTPPortKey(key) {
					continue
				}
				add(Route{
					Host:     contract.ServiceHost(host, key),
					Upstream: alloc.Ports[key],
					PortKey:  key,
					Checkout: checkout,
					Stack:    rec.Stack,
					Mode:     mode,
				})
				seen := map[string]struct{}{}
				for _, label := range alloc.HTTPHosts[key] {
					seen[label] = struct{}{}
					add(Route{
						Host:     contract.ServiceHost(host, label),
						Upstream: alloc.Ports[key],
						PortKey:  key,
						Checkout: checkout,
						Stack:    rec.Stack,
						Mode:     mode,
					})
				}
				for _, label := range alloc.EntryPoints[key] {
					if _, ok := seen[label]; ok {
						continue
					}
					add(Route{
						Host:     contract.EntryHost(host, label),
						Upstream: alloc.Ports[key],
						PortKey:  key,
						Checkout: checkout,
						Stack:    rec.Stack,
						Mode:     mode,
					})
				}
			}
		}
	}

	hosts := make([]string, 0, len(byHost))
	for h := range byHost {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	out := make([]Route, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, byHost[h])
	}
	return out
}

func HTTPPortKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "web", "api":
		return true
	default:
		return false
	}
}

func HTTPListenPorts(primary int) []int {
	if primary <= 0 {
		primary = state.ProxyHTTPListen
	}
	return []int{primary}
}

func SiteAddress(host string, listen int) string {
	if listen <= 0 || listen == 80 {
		return "http://" + host
	}
	return "http://" + host + ":" + strconv.Itoa(listen)
}

func PublicURL(host string, ports map[string]int, proxyActive bool, listen int) string {
	if host == "" {
		return ""
	}
	if proxyActive {
		return SiteAddress(host, listen)
	}
	if web, ok := ports["web"]; ok {
		return "http://" + host + ":" + strconv.Itoa(web)
	}
	return "http://" + host
}
