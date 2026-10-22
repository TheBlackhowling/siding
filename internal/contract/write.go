package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Write creates .siding/config.yml (and a .gitignore for generated env) at repoRoot.
func Write(repoRoot string, c *Contract) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(repoRoot, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte("env\nenv.*\nbackups/\n"), 0o644); err != nil {
			return "", err
		}
	}
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(Render(c)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func Render(c *Contract) string {
	var b strings.Builder
	b.WriteString("# Committed siding recipe. Slot names come from the checkout folder.\n")
	b.WriteString("version: 1\n")
	b.WriteString("project: " + yamlScalar(c.Project) + "\n")
	if c.FolderPrefix != "" {
		b.WriteString("folder_prefix: " + yamlScalar(c.FolderPrefix) + "\n")
	}
	if c.PrimaryBranch != "" {
		b.WriteString("primary_branch: " + yamlScalar(c.PrimaryBranch) + "\n")
	}
	if c.HostPrefix != "" {
		b.WriteString("host_prefix: " + yamlScalar(c.HostPrefix) + "\n")
	}
	host := c.Host
	if host == "" {
		if c.HostPrefix != "" {
			host = PrefixedHost
		} else {
			host = DefaultHost
		}
	}
	compose := c.ComposeProject
	if compose == "" {
		compose = DefaultCompose
	}
	b.WriteString("host: " + yamlScalar(host) + "\n")
	b.WriteString("compose_project: " + yamlScalar(compose) + "\n")
	b.WriteString("slot_env_file: " + yamlScalar(orDefault(c.SlotEnvFile, DefaultSlotEnv)) + "\n")
	writeHTTPHosts(&b, "", c.HTTPHosts)
	writeEntryPoints(&b, "", c.EntryPoints)
	b.WriteString("\nmodes:\n")

	names := make([]string, 0, len(c.Modes))
	for n := range c.Modes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		mode := c.Modes[name]
		b.WriteString("  " + name + ":\n")
		if mode.Host != "" {
			b.WriteString("    host: " + yamlScalar(mode.Host) + "\n")
		}
		if mode.ComposeProject != "" {
			b.WriteString("    compose_project: " + yamlScalar(mode.ComposeProject) + "\n")
		}
		b.WriteString("    compose_files:\n")
		for _, f := range mode.ComposeFiles {
			b.WriteString("      - " + yamlScalar(f) + "\n")
		}
		if envFiles := NormalizeComposeEnvFileList(mode.ComposeEnvFiles); len(envFiles) > 0 {
			b.WriteString("    compose_env_files:\n")
			for _, f := range envFiles {
				if f.Optional {
					b.WriteString("      - path: " + yamlScalar(f.Path) + "\n")
					b.WriteString("        optional: true\n")
				} else {
					b.WriteString("      - " + yamlScalar(f.Path) + "\n")
				}
			}
		}
		if len(mode.Ports) > 0 {
			b.WriteString("    ports:\n")
			keys := make([]string, 0, len(mode.Ports))
			for k := range mode.Ports {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("      " + k + ": " + strconv.Itoa(mode.Ports[k]) + "\n")
			}
		}
		if len(mode.PortEnv) > 0 {
			b.WriteString("    port_env:\n")
			keys := make([]string, 0, len(mode.PortEnv))
			for k := range mode.PortEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("      " + k + ": " + yamlScalar(mode.PortEnv[k]) + "\n")
			}
		}
		writeHTTPHosts(&b, "    ", mode.HTTPHosts)
		writeEntryPoints(&b, "    ", mode.EntryPoints)
		if len(mode.Env) > 0 {
			b.WriteString("    env:\n")
			keys := make([]string, 0, len(mode.Env))
			for k := range mode.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("      " + k + ": " + yamlScalar(mode.Env[k]) + "\n")
			}
		}
	}
	return b.String()
}

func writeEntryPoints(b *strings.Builder, indent string, entries map[string][]string) {
	keys := HTTPHostKeys(entries)
	if len(keys) == 0 {
		return
	}
	b.WriteString(indent + "entry_points:\n")
	for _, k := range keys {
		labels := NormalizeEntryLabels(entries[k])
		if len(labels) == 0 {
			continue
		}
		b.WriteString(indent + "  " + k + ":\n")
		for _, label := range labels {
			b.WriteString(indent + "    - " + yamlScalar(label) + "\n")
		}
	}
}

func writeHTTPHosts(b *strings.Builder, indent string, hosts map[string][]string) {
	keys := HTTPHostKeys(hosts)
	if len(keys) == 0 {
		return
	}
	b.WriteString(indent + "http_hosts:\n")
	for _, k := range keys {
		labels := NormalizeHTTPLabels(k, hosts[k])
		if len(labels) == 0 {
			continue
		}
		b.WriteString(indent + "  " + k + ":\n")
		for _, label := range labels {
			b.WriteString(indent + "    - " + yamlScalar(label) + "\n")
		}
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`,") || strings.Contains(s, "\n") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
