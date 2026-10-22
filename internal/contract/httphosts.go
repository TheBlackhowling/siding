package contract

import (
	"sort"
	"strings"
)

// ResolveHTTPHosts is mode labels over contract labels. Extra Caddy names are
// `{label}.{host}` for that port key. The port key itself is always a site.
func ResolveHTTPHosts(c *Contract, mode Mode) map[string][]string {
	var base map[string][]string
	if c != nil {
		base = c.HTTPHosts
	}
	return MergeHTTPHosts(base, mode.HTTPHosts)
}

func MergeHTTPHosts(base, over map[string][]string) map[string][]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := map[string][]string{}
	for k, labels := range base {
		if n := NormalizeHTTPLabels(k, labels); len(n) > 0 {
			out[k] = n
		}
	}
	for k, labels := range over {
		if n := NormalizeHTTPLabels(k, labels); len(n) > 0 {
			out[k] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeHTTPLabels(portKey string, labels []string) []string {
	portKey = strings.ToLower(strings.TrimSpace(portKey))
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range labels {
		label := NormalizeHTTPLabel(raw)
		if label == "" || label == portKey {
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

func NormalizeHTTPLabel(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimSuffix(s, ".")
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if s == "" || s == "localhost" {
		return ""
	}
	for _, r := range s {
		if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return s
}

var webRoleLabels = map[string]struct{}{
	"app": {}, "www": {}, "web": {}, "ui": {}, "frontend": {}, "client": {}, "shop": {},
}

// PublicHost is the browser hostname for a port key: first extra label, else
// ServiceHost (apex for web; sibling or prepended label for api).
func PublicHost(canonical, portKey string, hosts map[string][]string) string {
	canonical = CanonicalHost(canonical)
	portKey = strings.ToLower(strings.TrimSpace(portKey))
	if labels := hosts[portKey]; len(labels) > 0 {
		return extraHost(canonical, labels[0])
	}
	return ServiceHost(canonical, portKey)
}

func extraHost(canonical, label string) string {
	canonical = CanonicalHost(canonical)
	label = NormalizeHTTPLabel(label)
	if label == "" {
		return canonical
	}
	h := strings.TrimSuffix(canonical, ".localhost")
	if strings.HasPrefix(h+".", label+".") {
		return canonical
	}
	return ServiceHost(canonical, label)
}

// RootHost strips a leading frontend role (app, www, …) so NEXT_PUBLIC_ROOT_DOMAIN
// is acme.genie.test.localhost when the public URL is app.acme.genie.test.localhost.
func RootHost(canonical string) string {
	canonical = CanonicalHost(canonical)
	h := strings.TrimSuffix(canonical, ".localhost")
	parts := strings.Split(h, ".")
	if len(parts) >= 2 {
		if _, role := webRoleLabels[parts[0]]; role {
			return CanonicalHost(strings.Join(parts[1:], "."))
		}
	}
	return canonical
}

// SplitHostPrefix splits app.acme into role "app" and product "acme".
func SplitHostPrefix(prefix string) (role, rest string) {
	prefix = NormalizeHostPrefix(prefix)
	if prefix == "" {
		return "", ""
	}
	parts := strings.Split(prefix, ".")
	if _, ok := webRoleLabels[parts[0]]; ok {
		return parts[0], strings.Join(parts[1:], ".")
	}
	return "", prefix
}

// ServiceHost is the Caddy site for a port key on a canonical host.
//
// web stays at the apex. If the first label is a frontend role (app, www, …)
// it is replaced: app.acme.genie.test → api.acme.genie.test. Otherwise the
// key is prepended: betty.acme.test → api.betty.acme.test.
func ServiceHost(canonical, portKey string) string {
	canonical = CanonicalHost(canonical)
	portKey = NormalizeHTTPLabel(portKey)
	if portKey == "" || portKey == "web" {
		return canonical
	}
	h := strings.TrimSuffix(canonical, ".localhost")
	parts := strings.Split(h, ".")
	if len(parts) >= 2 {
		if _, role := webRoleLabels[parts[0]]; role {
			parts[0] = portKey
			return CanonicalHost(strings.Join(parts, "."))
		}
	}
	return CanonicalHost(portKey + "." + h)
}

// NormalizeHostPrefix keeps dotted labels (app.acme). Trailing .localhost /
// .test / .local / .dev are stripped so a pasted host still yields a prefix.
func NormalizeHostPrefix(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.Trim(s, ".")
	if s == "" || s == "localhost" || s == "-" || strings.EqualFold(s, "none") {
		return ""
	}
	if i := strings.Index(s, "{"); i >= 0 {
		s = strings.TrimSpace(s[:i])
		s = strings.Trim(s, ".")
	}
	s = strings.TrimSuffix(s, ".localhost")
	for _, tld := range []string{".test", ".local", ".dev"} {
		if strings.HasSuffix(s, tld) {
			s = strings.TrimSuffix(s, tld)
			break
		}
	}
	var parts []string
	for _, p := range strings.Split(s, ".") {
		n := NormalizeHTTPLabel(p)
		if n == "" {
			return ""
		}
		parts = append(parts, n)
	}
	return strings.Join(parts, ".")
}

// InferHostPrefixFromHostname turns compose defaults like app.example.test
// into example (app./www. are not part of the prefix).
func InferHostPrefixFromHostname(host string) string {
	p := NormalizeHostPrefix(host)
	if p == "" {
		return ""
	}
	role, rest := SplitHostPrefix(p)
	if role != "" && rest != "" {
		return rest
	}
	if role == "" && strings.Contains(p, ".") {
		return p
	}
	return ""
}

// ParseHostPrefixInput accepts "example", "app.example", or "none".
// A leading app./www. is dropped so the stored prefix is the product name.
func ParseHostPrefixInput(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, ","); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	p := NormalizeHostPrefix(s)
	role, rest := SplitHostPrefix(p)
	if role != "" && rest != "" {
		return rest
	}
	return p
}

// ApplyHostPrefix sets host_prefix (product only) and host: {prefix}.{stack}.{mode}.
// app. is the default public web subdomain, not part of the prefix.
func ApplyHostPrefix(c *Contract, prefix string) {
	if c == nil {
		return
	}
	prefix = ParseHostPrefixInput(prefix)
	c.HostPrefix = prefix
	if prefix != "" {
		c.Host = PrefixedHost
		if c.HTTPHosts == nil {
			c.HTTPHosts = map[string][]string{}
		}
		c.HTTPHosts["web"] = []string{"app"}
		for name, mode := range c.Modes {
			if mode.HTTPHosts == nil {
				continue
			}
			delete(mode.HTTPHosts, "web")
			if len(mode.HTTPHosts) == 0 {
				mode.HTTPHosts = nil
			}
			c.Modes[name] = mode
		}
		return
	}
	if c.Host == "" || c.Host == PrefixedHost || strings.Contains(c.Host, "{prefix}") {
		c.Host = DefaultHost
	}
	clearWebHTTPHosts(c)
}

func clearWebHTTPHosts(c *Contract) {
	if c.HTTPHosts != nil {
		delete(c.HTTPHosts, "web")
		if len(c.HTTPHosts) == 0 {
			c.HTTPHosts = nil
		}
	}
	for name, mode := range c.Modes {
		if mode.HTTPHosts == nil {
			continue
		}
		delete(mode.HTTPHosts, "web")
		if len(mode.HTTPHosts) == 0 {
			mode.HTTPHosts = nil
		}
		c.Modes[name] = mode
	}
}

// ParseHTTPPrefixInput accepts "app", "app,www", "app.{host}", or "none".
func ParseHTTPPrefixInput(raw string) []string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.EqualFold(s, "none") || s == "-" || s == "." {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.Index(part, "{"); i >= 0 {
			part = strings.TrimSpace(part[:i])
		}
		part = strings.TrimSuffix(part, ".")
		if n := NormalizeHTTPLabels("web", []string{part}); len(n) > 0 {
			out = append(out, n...)
		}
	}
	return NormalizeHTTPLabels("web", out)
}

// ApplyHTTPPrefixes sets product-wide http_hosts.web and clears per-mode web
// labels so the user's choice is what Caddy and {web_host} use.
func ApplyHTTPPrefixes(c *Contract, labels []string) {
	if c == nil {
		return
	}
	labels = NormalizeHTTPLabels("web", labels)
	if c.HTTPHosts == nil {
		c.HTTPHosts = map[string][]string{}
	}
	if len(labels) == 0 {
		delete(c.HTTPHosts, "web")
		if len(c.HTTPHosts) == 0 {
			c.HTTPHosts = nil
		}
	} else {
		c.HTTPHosts["web"] = labels
	}
	for name, mode := range c.Modes {
		if mode.HTTPHosts == nil {
			continue
		}
		delete(mode.HTTPHosts, "web")
		if len(mode.HTTPHosts) == 0 {
			mode.HTTPHosts = nil
		}
		c.Modes[name] = mode
	}
}

func HTTPHostKeys(hosts map[string][]string) []string {
	keys := make([]string, 0, len(hosts))
	for k := range hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
