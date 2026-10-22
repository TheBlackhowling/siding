package contract

import (
	"fmt"
	"strings"
)

// HTTPEntryKeys are the port keys Caddy and the board treat as HTTP sites.
var HTTPEntryKeys = []string{"web", "api"}

func IsHTTPEntryKey(key string) bool {
	_, ok := CanonicalHTTPEntryKey(key)
	return ok
}

// CanonicalHTTPEntryKey folds frontend/app/next → web and backend → api.
func CanonicalHTTPEntryKey(key string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "web", "frontend", "app", "next", "nextjs":
		return "web", true
	case "api", "backend":
		return "api", true
	default:
		return "", false
	}
}

// ResolveEntryPoints is mode entries over contract entries, then defaults
// (web → app when a host prefix is set, else apex; api → api).
func ResolveEntryPoints(c *Contract, mode Mode, ports map[string]int) map[string][]string {
	var base map[string][]string
	if c != nil {
		base = c.EntryPoints
	}
	out := MergeEntryPoints(base, mode.EntryPoints)
	if out == nil {
		out = map[string][]string{}
	}
	if _, ok := ports["web"]; ok && len(out["web"]) == 0 {
		if c != nil && strings.TrimSpace(c.HostPrefix) != "" {
			if labels := ResolveHTTPHosts(c, mode)["web"]; len(labels) > 0 {
				out["web"] = append([]string(nil), labels...)
			} else {
				out["web"] = []string{"app"}
			}
		} else if labels := ResolveHTTPHosts(c, mode)["web"]; len(labels) > 0 {
			out["web"] = append([]string(nil), labels...)
		} else {
			out["web"] = []string{"web"}
		}
	}
	if _, ok := ports["api"]; ok && len(out["api"]) == 0 {
		out["api"] = []string{"api"}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MergeEntryPoints(base, over map[string][]string) map[string][]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := map[string][]string{}
	for k, labels := range base {
		if n := NormalizeEntryLabels(labels); len(n) > 0 {
			out[strings.ToLower(strings.TrimSpace(k))] = n
		}
	}
	for k, labels := range over {
		if n := NormalizeEntryLabels(labels); len(n) > 0 {
			out[strings.ToLower(strings.TrimSpace(k))] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeEntryLabels(labels []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range labels {
		label := NormalizeHTTPLabel(raw)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

// ParseEntryFlag accepts KEY=label or KEY=label,label (e.g. web=app, www).
func ParseEntryFlag(raw string) (key string, labels []string, err error) {
	raw = strings.TrimSpace(raw)
	k, rest, ok := strings.Cut(raw, "=")
	if !ok {
		return "", nil, fmt.Errorf("entry must be KEY=LABEL (got %q)", raw)
	}
	key, ok = CanonicalHTTPEntryKey(k)
	if !ok {
		return "", nil, fmt.Errorf("entry key %q is not web or api", k)
	}
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.Index(part, "{"); i >= 0 {
			part = strings.TrimSpace(part[:i])
		}
		if n := NormalizeHTTPLabel(part); n != "" {
			labels = append(labels, n)
		}
	}
	labels = NormalizeEntryLabels(labels)
	if len(labels) == 0 {
		return "", nil, fmt.Errorf("entry %s has no labels", key)
	}
	return key, labels, nil
}

// ApplyEntryPoints sets recipe entry_points and mirrors web extras into http_hosts
// so Caddy serves app.{host} (api.{host} is always a site).
func ApplyEntryPoints(c *Contract, entries map[string][]string) {
	if c == nil {
		return
	}
	entries = MergeEntryPoints(entries, nil)
	c.EntryPoints = entries
	if c.HTTPHosts == nil {
		c.HTTPHosts = map[string][]string{}
	}
	if labels := entries["web"]; len(labels) > 0 {
		var extras []string
		for _, l := range labels {
			if l != "web" {
				extras = append(extras, l)
			}
		}
		if len(extras) > 0 {
			c.HTTPHosts["web"] = extras
		} else {
			delete(c.HTTPHosts, "web")
		}
	}
	if len(c.HTTPHosts) == 0 {
		c.HTTPHosts = nil
	}
	for name, mode := range c.Modes {
		if mode.HTTPHosts != nil {
			delete(mode.HTTPHosts, "web")
			if len(mode.HTTPHosts) == 0 {
				mode.HTTPHosts = nil
			}
		}
		mode.EntryPoints = nil
		c.Modes[name] = mode
	}
}

// EntryHost is the browser hostname for one entry label on a root host.
func EntryHost(root, label string) string {
	root = RootHost(root)
	label = NormalizeHTTPLabel(label)
	if label == "" || label == "web" {
		return CanonicalHost(root)
	}
	return extraHost(root, label)
}
