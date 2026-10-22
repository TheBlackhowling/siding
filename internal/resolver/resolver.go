package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/state"
)

type Options struct {
	StartPath     string // cwd or --path; walks up to .siding/
	Stack         string
	Mode          string
	StateOverride string
	Loaded        *state.File // if set, skip reading state.json (multi-mode sync)
}

type Result struct {
	RepoRoot       string
	CheckoutKey    string
	StateFile      string
	StackKey       string
	Mode           string
	Host           string
	Ports          map[string]int
	ComposeProject string
	ComposeFiles   []string
	SlotEnvRel     string
	SlotEnvPath    string
	Contract       *contract.Contract
	ModeConfig     contract.Mode
	ProxyActive    bool
	ProxyListen    int
	HTTPHosts      map[string][]string
	EntryPoints    map[string][]string
}

func Resolve(opts Options) (*Result, error) {
	repoRoot, err := contract.FindRepoRoot(opts.StartPath)
	if err != nil {
		return nil, err
	}

	c, err := contract.Load(repoRoot)
	if err != nil {
		return nil, err
	}

	statePaths, err := paths.ResolvePaths(opts.StateOverride)
	if err != nil {
		return nil, err
	}
	store := state.NewStore(statePaths)
	st := opts.Loaded
	if st == nil {
		var err error
		st, err = store.Read()
		if err != nil {
			return nil, err
		}
	}

	checkoutKey := paths.NormalizeLockPath(repoRoot)
	stackKey, err := contract.ResolveStackName(c, repoRoot, opts.Stack)
	if err != nil {
		return nil, err
	}

	mode, err := contract.PickMode(c, opts.Mode)
	if err != nil {
		return nil, fmt.Errorf("%w (repo %q)", err, repoRoot)
	}

	contractMode, err := c.Mode(mode)
	if err != nil {
		return nil, fmt.Errorf("%w (repo %q)", err, repoRoot)
	}

	composeFiles := append([]string(nil), contractMode.ComposeFiles...)
	scan := advise.ScanRepo(repoRoot, composeFiles, nil, "")
	if strings.TrimSpace(c.HostPrefix) == "" && scan.HostPrefix != "" {
		if c.Host == "" || c.Host == contract.DefaultHost {
			contract.ApplyHostPrefix(c, scan.HostPrefix)
		}
	}
	expandedHost, err := c.ExpandHost(stackKey, mode, contractMode)
	if err != nil {
		return nil, err
	}
	host := contract.RootHost(expandedHost)
	composeProject, err := c.ExpandComposeProject(stackKey, mode, contractMode)
	if err != nil {
		return nil, err
	}
	ports, portEnv := advise.OwnPublished(contractMode.Ports, contractMode.PortEnv, scan)
	contractMode.Ports = ports
	contractMode.PortEnv = portEnv

	httpHosts := contract.ResolveHTTPHosts(c, contractMode)
	entryPoints := contract.ResolveEntryPoints(c, contractMode, ports)
	if alloc, ok := store.ModeAllocation(st, checkoutKey, mode); ok {
		if len(alloc.Ports) > 0 {
			ports = copyPorts(alloc.Ports)
		}
	}

	return &Result{
		RepoRoot:       repoRoot,
		CheckoutKey:    checkoutKey,
		StateFile:      statePaths.StateFile,
		StackKey:       stackKey,
		Mode:           mode,
		Host:           host,
		Ports:          ports,
		ComposeProject: composeProject,
		ComposeFiles:   composeFiles,
		SlotEnvRel:     contract.SlotEnvRel(c, mode),
		SlotEnvPath:    filepath.Join(repoRoot, contract.SlotEnvRel(c, mode)),
		Contract:       c,
		ModeConfig:     contractMode,
		HTTPHosts:      httpHosts,
		EntryPoints:    entryPoints,
	}, nil
}

func copyPorts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ApplyAllocation overlays a state.json mode record onto a resolved plan.
func (r *Result) ApplyAllocation(alloc state.ModeAllocation) {
	if alloc.Host != "" {
		r.Host = contract.RootHost(alloc.Host)
	}
	if alloc.ComposeProject != "" {
		r.ComposeProject = alloc.ComposeProject
	}
	if len(alloc.Ports) > 0 {
		r.Ports = copyPorts(alloc.Ports)
	}
	if len(alloc.HTTPHosts) > 0 {
		r.HTTPHosts = alloc.HTTPHosts
	}
	if len(alloc.EntryPoints) > 0 {
		r.EntryPoints = alloc.EntryPoints
	}
}

func (r *Result) SeedAllocation() state.ModeAllocation {
	return r.RecipeSeed()
}

// RecipeSeed is the org recipe applied to this checkout: current host/compose
// patterns and default ports. Local allocated numbers live in state.json.
func (r *Result) RecipeSeed() state.ModeAllocation {
	host := r.Host
	composeProject := r.ComposeProject
	if r.Contract != nil {
		if expanded, err := r.Contract.ExpandHost(r.StackKey, r.Mode, r.ModeConfig); err == nil {
			host = contract.RootHost(expanded)
		}
		if project, err := r.Contract.ExpandComposeProject(r.StackKey, r.Mode, r.ModeConfig); err == nil {
			composeProject = project
		}
	}
	ports := copyPorts(r.ModeConfig.Ports)
	var scan advise.Report
	if r.RepoRoot != "" && len(r.ComposeFiles) > 0 {
		scan = advise.ScanRepo(r.RepoRoot, r.ComposeFiles, nil, "")
	}
	if len(ports) == 0 {
		ports = copyPorts(r.Ports)
	}
	ports, _ = advise.OwnPublished(ports, r.ModeConfig.PortEnv, scan)
	httpHosts := r.HTTPHosts
	if r.Contract != nil {
		httpHosts = contract.ResolveHTTPHosts(r.Contract, r.ModeConfig)
	}
	entryPoints := r.EntryPoints
	if r.Contract != nil {
		entryPoints = contract.ResolveEntryPoints(r.Contract, r.ModeConfig, ports)
	}
	return state.ModeAllocation{
		Host:           host,
		ComposeProject: composeProject,
		Ports:          ports,
		HTTPHosts:      httpHosts,
		EntryPoints:    entryPoints,
	}
}

// BuildSlotEnv maps contract port_env + static env into compose --env-file keys.
func BuildSlotEnv(res *Result) map[string]string {
	root := contract.RootHost(res.Host)
	webHost := contract.PublicHost(root, "web", res.HTTPHosts)
	url := "http://" + webHost
	if res.ProxyActive {
		if res.ProxyListen > 0 && res.ProxyListen != 80 {
			url = fmt.Sprintf("http://%s:%d", webHost, res.ProxyListen)
		}
	} else if web, ok := res.Ports["web"]; ok {
		url = fmt.Sprintf("http://%s:%d", webHost, web)
	}
	env := map[string]string{
		"COMPOSE_PROJECT_NAME": res.ComposeProject,
		"SIDING_STACK":         res.StackKey,
		"SIDING_MODE":          res.Mode,
		"SIDING_HOST":          root,
		"SIDING_CHECKOUT":      res.CheckoutKey,
		"SIDING_URL":           url,
	}
	if res.ProxyActive {
		listen := res.ProxyListen
		if listen <= 0 {
			listen = 80
		}
		env["SIDING_PROXY"] = "1"
		env["SIDING_PROXY_LISTEN"] = fmt.Sprintf("%d", listen)
	}
	for k, v := range res.ModeConfig.Env {
		env[k] = ExpandEnvValue(v, res, url)
	}
	for portKey, portVal := range res.Ports {
		envKey := advise.EnvNameForKey(portKey)
		if res.ModeConfig.PortEnv != nil {
			if mapped, ok := res.ModeConfig.PortEnv[portKey]; ok && strings.TrimSpace(mapped) != "" {
				envKey = mapped
			}
		}
		env[envKey] = fmt.Sprintf("%d", portVal)
	}
	return env
}

// ProxySuffix is the public listen suffix for web ({proxy_suffix} / {web_suffix}).
// Empty when the proxy listen is 80. A custom --listen adds :{port}.
// When the proxy is off, this is :{web}, not :{api}.
func ProxySuffix(res *Result) string {
	return PortSuffix(res, "web")
}

// PortSuffix is :{listen} when the proxy is up, else the allocated host port
// for that key (web → :18010, api → :23010).
func PortSuffix(res *Result, portKey string) string {
	if res == nil {
		return ""
	}
	if res.ProxyActive {
		if res.ProxyListen <= 0 || res.ProxyListen == 80 {
			return ""
		}
		return fmt.Sprintf(":%d", res.ProxyListen)
	}
	if p, ok := res.Ports[portKey]; ok && p > 0 {
		return fmt.Sprintf(":%d", p)
	}
	return ""
}

// ExpandEnvValue replaces {host}, {proxy_suffix}, {stack}, {project}, {mode},
// {url}, and {<port key>} in a contract env template.
func ExpandEnvValue(value string, res *Result, publicURL string) string {
	if value == "" || res == nil {
		return value
	}
	project := ""
	if res.Contract != nil {
		project = res.Contract.Project
	}
	root := contract.RootHost(res.Host)
	webHost := contract.PublicHost(root, "web", res.HTTPHosts)
	repl := []string{
		"{host}", root,
		"{web_host}", webHost,
		"{root_host}", root,
		"{api_host}", contract.PublicHost(root, "api", res.HTTPHosts),
		"{proxy_suffix}", ProxySuffix(res),
		"{web_suffix}", PortSuffix(res, "web"),
		"{api_suffix}", PortSuffix(res, "api"),
		"{stack}", res.StackKey,
		"{project}", project,
		"{mode}", res.Mode,
		"{url}", publicURL,
		"{checkout}", res.CheckoutKey,
	}
	for key, port := range res.Ports {
		repl = append(repl, "{"+key+"}", fmt.Sprintf("%d", port))
	}
	return strings.NewReplacer(repl...).Replace(value)
}

func FormatEnvFile(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Generated by siding. Do not edit.\n")
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(formatEnvValue(env[k]))
		b.WriteByte('\n')
	}
	return b.String()
}

func formatEnvValue(v string) string {
	if v == "" {
		return `""`
	}
	needsQuote := strings.ContainsAny(v, " \t#\"'$\\") || strings.Contains(v, "\n") || strings.Contains(v, "\r")
	if !needsQuote {
		return v
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func WriteSlotEnv(res *Result) error {
	if err := os.MkdirAll(filepath.Dir(res.SlotEnvPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(res.SlotEnvPath, []byte(FormatEnvFile(BuildSlotEnv(res))), 0o644)
}
