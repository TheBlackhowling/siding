package state

const FileName = "state.json"

// File is the single machine-wide siding state store.
type File struct {
	Version   int                 `json:"version"`
	Proxy     *ProxyConfig        `json:"proxy,omitempty"`
	Checkouts map[string]Checkout `json:"checkouts"`
	Locks     map[string]Lock     `json:"locks"`
}

const (
	// Loopback ports the shared HTTP proxy binds. Allocation never claims these.
	// Default HTTP is host :80 (Caddy's normal port). --listen can move it.
	ProxyHTTPListen  = 80
	ProxyAdminListen = 2019
)

type ProxyConfig struct {
	Listen int `json:"listen"`
}

// Checkout holds per-repo-root allocations. Map key is NormalizeLockPath(repo_root).
type Checkout struct {
	Project string                    `json:"project"`
	Stack   string                    `json:"stack"`
	Modes   map[string]ModeAllocation `json:"modes"`
}

type ModeAllocation struct {
	Host           string              `json:"host,omitempty"`
	ComposeProject string              `json:"compose_project,omitempty"`
	Ports          map[string]int      `json:"ports,omitempty"`
	GitBranch      string              `json:"git_branch,omitempty"`
	GitSHA         string              `json:"git_sha,omitempty"`
	LastUpAt       string              `json:"last_up_at,omitempty"`
	HTTPHosts      map[string][]string `json:"http_hosts,omitempty"`
	EntryPoints    map[string][]string `json:"entry_points,omitempty"`
}

type Lock struct {
	Checkout       string `json:"checkout"`
	ComposeProject string `json:"compose_project"`
}

func NewFile() *File {
	return &File{
		Version:   1,
		Checkouts: map[string]Checkout{},
		Locks:     map[string]Lock{},
	}
}

func LockKey(project, stack, mode string) string {
	return project + ":" + stack + ":" + mode
}

func ListenPort(f *File) int {
	if f != nil && f.Proxy != nil && f.Proxy.Listen > 0 {
		return f.Proxy.Listen
	}
	return ProxyHTTPListen
}

func SetListenPort(f *File, listen int) {
	if f == nil {
		return
	}
	if listen <= 0 {
		listen = ProxyHTTPListen
	}
	if f.Proxy == nil {
		f.Proxy = &ProxyConfig{}
	}
	f.Proxy.Listen = listen
}

// MigrateOffDefaultPorts moves a leftover rewrite listen (0 or 1355) onto
// host :80. Explicit --listen values (443, 8000, 8080, 1365, …) stay.
func MigrateOffDefaultPorts(f *File) bool {
	if f == nil {
		return false
	}
	p := 0
	if f.Proxy != nil {
		p = f.Proxy.Listen
	}
	if p == 0 || p == 1355 {
		SetListenPort(f, ProxyHTTPListen)
		return true
	}
	return false
}
