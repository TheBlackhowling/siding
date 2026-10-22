package siding

import (
	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/resolver"
)

// ProxyRoute is one Caddy site in the shared loopback proxy.
type ProxyRoute struct {
	Host     string
	Upstream int
	PortKey  string
	Checkout string
	Stack    string
	Mode     string
}

// Target selects a checkout and stack mode. StartPath walks up to .siding/;
// empty means the process working directory.
type Target struct {
	StartPath string
	Stack     string
	Mode      string
}

// Resolved is a checkout slot: ports, host, compose project, and env paths.
type Resolved struct {
	RepoRoot        string
	CheckoutKey     string
	StateFile       string
	Stack           string
	Mode            string
	Host            string
	Ports           map[string]int
	ComposeProject  string
	ComposeFiles    []string
	ComposeEnvFiles              []string // paths passed to compose (optional missing omitted)
	SkippedOptionalComposeEnvFiles []string
	SlotEnvPath                    string
	SlotEnvRel      string
	ProxyActive     bool
	ProxyListen     int
	PublicURL       string
}

func fromResult(res *resolver.Result) Resolved {
	if res == nil {
		return Resolved{}
	}
	included, skipped, _ := contract.SelectComposeEnvPaths(res.RepoRoot, res.ModeConfig.ComposeEnvFiles)
	out := Resolved{
		RepoRoot:                       res.RepoRoot,
		CheckoutKey:                    res.CheckoutKey,
		StateFile:                      res.StateFile,
		Stack:                          res.StackKey,
		Mode:                           res.Mode,
		Host:                           res.Host,
		Ports:                          copyPorts(res.Ports),
		ComposeProject:                 res.ComposeProject,
		ComposeFiles:                   append([]string(nil), res.ComposeFiles...),
		ComposeEnvFiles:                append([]string(nil), included...),
		SkippedOptionalComposeEnvFiles: append([]string(nil), skipped...),
		SlotEnvPath:                    res.SlotEnvPath,
		SlotEnvRel:      res.SlotEnvRel,
		ProxyActive:     res.ProxyActive,
		ProxyListen:     res.ProxyListen,
	}
	out.PublicURL = publicURL(res)
	return out
}

func copyPorts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Plan is resolve output plus slot env and repo tips (siding plan).
type Plan struct {
	Resolved
	StartPath    string
	UserDataRoot string
	SlotEnv      map[string]string
	Tips         []string
}

// AddResult is the outcome of pre-allocating ports (siding add).
type AddResult struct {
	Resolved
	Created bool
	DryRun  bool
}

// SyncMode is one mode row from siding sync.
type SyncMode struct {
	Resolved
	SlotEnv     map[string]string
	AddedPorts  []string
	Created     bool
	ProxyActive bool
	ProxyListen int
	Tips        []string
}

// SyncResult is the outcome of mapping recipe onto local state (siding sync).
type SyncResult struct {
	Modes  []SyncMode
	DryRun bool
}

// StackResult is returned after a successful up or down.
type StackResult struct {
	Resolved
	PublicURL   string
	ProxyActive bool
}

// ComposeOptions mirrors docker compose flags siding passes through.
type ComposeOptions struct {
	Profile       string
	Build         bool
	ForceRecreate bool
	Pull          string // always | missing | never; empty = omit
	RemoveOrphans bool
	Volumes       bool
	Timeout       int // seconds; 0 = omit
}

func (o ComposeOptions) internal() compose.Options {
	return compose.Options{
		Profile:       o.Profile,
		Build:         o.Build,
		ForceRecreate: o.ForceRecreate,
		Pull:          o.Pull,
		RemoveOrphans: o.RemoveOrphans,
		Volumes:       o.Volumes,
		Timeout:       o.Timeout,
	}
}

// ProxyConfig describes the shared Caddy proxy after up or reload.
type ProxyConfig struct {
	Listen          int
	BoardURL        string
	AdminURL        string
	ProxyDir        string
	CaddyfilePath   string
	Routes           []ProxyRoute
	SkippedPublishes []int
}

func proxyConfigFrom(cfg proxy.Config) ProxyConfig {
	routes := make([]ProxyRoute, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		routes = append(routes, ProxyRoute{
			Host:     r.Host,
			Upstream: r.Upstream,
			PortKey:  r.PortKey,
			Checkout: r.Checkout,
			Stack:    r.Stack,
			Mode:     r.Mode,
		})
	}
	return ProxyConfig{
		Listen:           cfg.Listen,
		BoardURL:         proxy.SiteAddress(proxy.BoardHost, cfg.Listen),
		AdminURL:         proxy.AdminURL(),
		ProxyDir:         cfg.ProxyDir,
		CaddyfilePath:    cfg.CaddyfilePath,
		Routes:           routes,
		SkippedPublishes: append([]int(nil), cfg.SkippedPublishes...),
	}
}
