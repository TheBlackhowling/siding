package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DirName        = ".siding"
	ConfigFileName = "config.yml"
	LegacyFileName = ".siding.yml"
	ComposeDirName = "compose"
	DefaultSlotEnv = ".siding/env"
	DefaultHost    = "{stack}.{project}.{mode}"
	PrefixedHost   = "{prefix}.{stack}.{mode}"
	DefaultCompose       = "{project}-{stack}-{mode}"
	DefaultPrimaryBranch = "main"
)

// SidingComposeRel is the committed copy of a root compose file under .siding/compose/.
func SidingComposeRel(orig string) string {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(orig), "\\", "/"))
	if base == "" || base == "." || base == "/" {
		base = "docker-compose.yml"
	}
	return DirName + "/" + ComposeDirName + "/" + base
}

// FileName is the preferred contract path relative to the repo root.
const FileName = DirName + "/" + ConfigFileName

// Contract is the committed repo recipe: kinds of stack (modes), not a roster of slots.
type Contract struct {
	Version        int                 `yaml:"version"`
	Project        string              `yaml:"project"`
	SlotEnvFile    string              `yaml:"slot_env_file,omitempty"`
	FolderPrefix   string              `yaml:"folder_prefix,omitempty"`
	PrimaryBranch  string              `yaml:"primary_branch,omitempty"`
	HostPrefix     string              `yaml:"host_prefix,omitempty"`
	Host           string              `yaml:"host,omitempty"`
	ComposeProject string              `yaml:"compose_project,omitempty"`
	HTTPHosts      map[string][]string `yaml:"http_hosts,omitempty"`
	EntryPoints    map[string][]string `yaml:"entry_points,omitempty"`
	Modes          map[string]Mode     `yaml:"modes"`
}

type Mode struct {
	Host             string              `yaml:"host,omitempty"`
	ComposeProject   string              `yaml:"compose_project,omitempty"`
	ComposeFiles     []string            `yaml:"compose_files"`
	ComposeEnvFiles  ComposeEnvFileList  `yaml:"compose_env_files,omitempty"`
	Ports            map[string]int      `yaml:"ports,omitempty"`
	PortEnv          map[string]string   `yaml:"port_env,omitempty"`
	Env              map[string]string   `yaml:"env,omitempty"`
	HTTPHosts        map[string][]string `yaml:"http_hosts,omitempty"`
	EntryPoints      map[string][]string `yaml:"entry_points,omitempty"`
}

func ConfigPath(repoRoot string) string {
	preferred := filepath.Join(repoRoot, DirName, ConfigFileName)
	if st, err := os.Stat(preferred); err == nil && !st.IsDir() {
		return preferred
	}
	legacy := filepath.Join(repoRoot, LegacyFileName)
	if st, err := os.Stat(legacy); err == nil && !st.IsDir() {
		return legacy
	}
	return preferred
}

func HasContract(dir string) bool {
	for _, p := range []string{
		filepath.Join(dir, DirName, ConfigFileName),
		filepath.Join(dir, LegacyFileName),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func Load(repoRoot string) (*Contract, error) {
	path := ConfigPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing %s in %s (run siding init --path %s)", FileName, repoRoot, repoRoot)
		}
		return nil, err
	}
	var c Contract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Contract) Validate() error {
	if c.Version == 0 {
		c.Version = 1
	}
	if err := ValidateProjectID(c.Project); err != nil {
		return err
	}
	if c.SlotEnvFile == "" {
		c.SlotEnvFile = DefaultSlotEnv
	}
	c.HostPrefix = ParseHostPrefixInput(c.HostPrefix)
	if c.Host == "" {
		c.Host = DefaultHost
	}
	if c.HostPrefix != "" && c.Host == DefaultHost {
		c.Host = PrefixedHost
	}
	if c.HostPrefix != "" && len(c.HTTPHosts["web"]) == 0 {
		if c.HTTPHosts == nil {
			c.HTTPHosts = map[string][]string{}
		}
		c.HTTPHosts["web"] = []string{"app"}
	}
	if c.ComposeProject == "" {
		c.ComposeProject = DefaultCompose
	}
	if err := ValidateHostPattern("host", c.Host); err != nil {
		return err
	}
	if err := ValidateHostPattern("compose_project", c.ComposeProject); err != nil {
		return err
	}
	if len(c.Modes) == 0 {
		return errors.New("contract: modes must not be empty")
	}
	for name, mode := range c.Modes {
		if len(mode.ComposeFiles) == 0 {
			return fmt.Errorf("contract: mode %q missing compose_files", name)
		}
		if err := ValidateHostPattern("modes."+name+".host", mode.Host); err != nil {
			return err
		}
		if err := ValidateHostPattern("modes."+name+".compose_project", mode.ComposeProject); err != nil {
			return err
		}
		mode.ComposeEnvFiles = NormalizeComposeEnvFileList(mode.ComposeEnvFiles)
		c.Modes[name] = mode
	}
	return nil
}

// PrimaryBranchName returns the recipe primary branch for worktree start points.
func (c *Contract) PrimaryBranchName() string {
	if c == nil {
		return DefaultPrimaryBranch
	}
	if b := strings.TrimSpace(c.PrimaryBranch); b != "" {
		return b
	}
	return DefaultPrimaryBranch
}

// NormalizeComposeEnvFiles trims, slash-normalizes, and dedupes repo-relative
// compose --env-file paths. "none" / empty entries are dropped.
func NormalizeComposeEnvFiles(files []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range files {
		f := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		if f == "" || strings.EqualFold(f, "none") {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

func (c *Contract) Mode(name string) (Mode, error) {
	mode, ok := c.Modes[name]
	if !ok {
		return Mode{}, fmt.Errorf("unknown mode %q (have %s)", name, modeNames(c.Modes))
	}
	return mode, nil
}

// ModeNames returns recipe mode keys in sorted order.
func ModeNames(c *Contract) []string {
	if c == nil || len(c.Modes) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Modes))
	for n := range c.Modes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// PickMode uses the requested mode, or test, or the first mode if test is absent.
func PickMode(c *Contract, requested string) (string, error) {
	if requested == "" {
		requested = "test"
	}
	if _, err := c.Mode(requested); err == nil {
		return requested, nil
	}
	if requested != "test" {
		return "", fmt.Errorf("unknown mode %q (have %s)", requested, strings.Join(ModeNames(c), ", "))
	}
	names := ModeNames(c)
	if len(names) == 0 {
		return "", errors.New("contract: modes must not be empty")
	}
	return names[0], nil
}

// SlotEnvRel is the repo-relative env file for a mode. test keeps slot_env_file
// (.siding/env); other modes append ".<mode>" so two stacks can be up at once.
func SlotEnvRel(c *Contract, mode string) string {
	base := DefaultSlotEnv
	if c != nil && strings.TrimSpace(c.SlotEnvFile) != "" {
		base = c.SlotEnvFile
	}
	if mode == "" || mode == "test" {
		return base
	}
	return base + "." + mode
}

func (c *Contract) ExpandHost(stack, modeName string, mode Mode) (string, error) {
	pattern := c.Host
	if mode.Host != "" {
		pattern = mode.Host
	}
	if pattern == "" {
		pattern = DefaultHost
	}
	prefix := ""
	if c != nil {
		prefix = c.HostPrefix
	}
	if strings.TrimSpace(prefix) != "" && pattern == DefaultHost {
		pattern = PrefixedHost
	}
	_, rest := SplitHostPrefix(prefix)
	expanded := Expand(pattern, stack, c.Project, modeName, rest)
	if err := ValidateExpandedHostname(expanded); err != nil {
		return "", fmt.Errorf("contract: expanded host for mode %q: %w", modeName, err)
	}
	return CanonicalHost(expanded), nil
}

func (c *Contract) ExpandComposeProject(stack, modeName string, mode Mode) (string, error) {
	pattern := c.ComposeProject
	if mode.ComposeProject != "" {
		pattern = mode.ComposeProject
	}
	prefix := ""
	if c != nil {
		prefix = c.HostPrefix
	}
	expanded := Expand(pattern, stack, c.Project, modeName, prefix)
	if err := ValidateComposeProjectName(expanded); err != nil {
		return "", fmt.Errorf("contract: expanded compose_project for mode %q: %w", modeName, err)
	}
	return expanded, nil
}

// CanonicalHost lowercases, validates, and appends .localhost when missing.
// Invalid hosts return "" so callers can drop them before proxy generation.
func CanonicalHost(host string) string {
	out, err := CanonicalHostString(host)
	if err != nil {
		return ""
	}
	return out
}

// CanonicalHostString is CanonicalHost with an error when validation fails.
func CanonicalHostString(host string) (string, error) {
	h := strings.TrimSpace(strings.ToLower(host))
	if err := ValidateExpandedHostname(h); err != nil {
		return "", err
	}
	if strings.HasSuffix(h, ".localhost") {
		return h, nil
	}
	return h + ".localhost", nil
}

func modeNames(modes map[string]Mode) string {
	names := make([]string, 0, len(modes))
	for n := range modes {
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}
