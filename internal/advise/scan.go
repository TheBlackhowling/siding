package advise

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/TheBlackhowling/siding/internal/contract"
)

type Finding struct {
	Kind    string // port_ok, port_hardcoded, public_url, missing_port_env, unowned_port
	Service string
	Detail  string
}

type Report struct {
	Findings   []Finding
	Ports      map[string]int    // inferred key → default host port
	PortEnv    map[string]string // key → env var
	HTTPHosts  map[string][]string
	HostPrefix string
	Network    NetworkPlan
	Tips       []string
}

type composeDoc struct {
	Services map[string]composeService `yaml:"services"`
	Networks map[string]any            `yaml:"networks"`
}

type composeService struct {
	Ports       []any  `yaml:"ports"`
	Environment any    `yaml:"environment"`
	Networks    any    `yaml:"networks"`
	NetworkMode string `yaml:"network_mode"`
}

var (
	envPortRef    = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)(?::-(\d+))?\}`)
	hostPrefix    = regexp.MustCompile(`^(?:\d{1,3}(?:\.\d{1,3}){3}|localhost|\[::\]):`)
	publishedSpec = regexp.MustCompile(`(?i)^(\d+)(?::(\d+))?(?:/(tcp|udp))?$`)
	publicEnvName = regexp.MustCompile(`(?i)^(NEXT_PUBLIC_|VITE_|PUBLIC_)[A-Z0-9_]+$`)
	portInDefault = regexp.MustCompile(`:\d{2,5}`)
)

func ScanRepo(repoRoot string, composeFiles []string, c *contract.Contract, mode string) Report {
	rep := Report{
		Ports:     map[string]int{},
		PortEnv:   map[string]string{},
		HTTPHosts: map[string][]string{},
	}
	for _, rel := range composeFiles {
		path := rel
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			rep.Findings = append(rep.Findings, Finding{Kind: "compose_missing", Detail: path + ": " + err.Error()})
			continue
		}
		scanCompose(&rep, rel, data)
		scanNetworkHints(&rep, rel, data)
		rep.Network = MergeNetworkPlans(rep.Network, AnalyzeNetworks(data))
	}
	if c != nil && mode != "" {
		if m, err := c.Mode(mode); err == nil {
			checkContractPorts(&rep, m)
			checkContractOwnsPublished(&rep, m)
			dropCoveredSlotEnv(&rep, m)
		}
	}
	rep.Tips = buildTips(rep, c)
	return rep
}

func scanCompose(rep *Report, file string, data []byte) {
	var doc composeDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		rep.Findings = append(rep.Findings, Finding{Kind: "compose_invalid", Detail: file + ": " + err.Error()})
		return
	}
	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		scanServicePorts(rep, file, name, doc.Services[name].Ports)
		if key, label := InferHTTPHostsFromService(name); key != "" {
			addHTTPHost(rep.HTTPHosts, key, label)
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := yamlAssign(line)
		if !ok || !isHostFacingKey(key) || skipAutoRewrite(key) {
			continue
		}
		if p := contract.InferHostPrefixFromHostname(hostnameFromValue(val)); p != "" {
			if strings.Count(p, ".") > strings.Count(rep.HostPrefix, ".") {
				rep.HostPrefix = p
			}
		}
		if key, label := InferHTTPHost(key, val); key != "" {
			addHTTPHost(rep.HTTPHosts, key, label)
		}
		if portInDefault.MatchString(val) || strings.Contains(val, "_PORT") || internalLocal.MatchString(val) {
			rep.Findings = append(rep.Findings, Finding{
				Kind:   "public_url",
				Detail: fmt.Sprintf("%s: %s default includes a host port (%s)", file, key, compact(val)),
			})
			continue
		}
		if envPlaceholder(val) {
			rep.Findings = append(rep.Findings, Finding{
				Kind:   "public_url",
				Detail: fmt.Sprintf("%s: %s needs .siding/env (%s)", file, key, compact(val)),
			})
		}
	}
}

func scanNetworkHints(rep *Report, file string, data []byte) {
	plan := AnalyzeNetworks(data)
	if plan.NeedsEnsure {
		for _, line := range FormatNetworkStatus(plan) {
			kind := "network_isolated"
			if strings.Contains(line, "network_mode: host") {
				kind = "host_network"
			}
			rep.Findings = append(rep.Findings, Finding{Kind: kind, Detail: file + ": " + line})
		}
	} else if len(plan.Services) >= 2 {
		rep.Findings = append(rep.Findings, Finding{
			Kind:   "network_ok",
			Detail: file + ": " + FormatNetworkStatus(plan)[0],
		})
	}
	skipUntil := -1
	serviceIndent := -1
	for _, line := range strings.Split(string(data), "\n") {
		if _, indent, ok := parseServiceHeader(line); ok {
			serviceIndent = indent
		}
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if skipUntil >= 0 {
			if indentWidth(line) <= skipUntil && !strings.HasPrefix(trim, "-") {
				skipUntil = -1
			} else {
				continue
			}
		}
		if isSkipBlock(trim, indentWidth(line), serviceIndent) {
			skipUntil = indentWidth(line)
			continue
		}
		key, val, ok := strings.Cut(trim, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if skipAutoRewrite(key) || isHostFacingKey(key) {
			continue
		}
		if internalLocal.MatchString(val) {
			rep.Findings = append(rep.Findings, Finding{
				Kind:   "localhost_internal",
				Detail: fmt.Sprintf("%s: %s uses localhost — in-stack traffic should use the service name on the Compose network", file, key),
			})
		}
	}
}

type hostPublish struct {
	Spec     string
	Env      string
	HostPort int
	Target   int
}

func scanServicePorts(rep *Report, file, service string, ports []any) {
	var pubs []hostPublish
	for _, raw := range ports {
		p, ok := parsePublish(raw)
		if !ok {
			continue
		}
		pubs = append(pubs, p)
	}
	hardIdx := 0
	total := len(pubs)
	for _, p := range pubs {
		var key, env string
		if p.Env != "" {
			key = portKeyFromEnv(p.Env)
			env = p.Env
		} else {
			key, env = hardcodedPortKey(service, p.Target, hardIdx, total)
			hardIdx++
		}
		_, env = finalizePublishKey(rep, key, env, p)
		if p.Env != "" {
			rep.Findings = append(rep.Findings, Finding{
				Kind:    "port_ok",
				Service: service,
				Detail:  fmt.Sprintf("%s %s publishes via ${%s}", file, service, p.Env),
			})
			continue
		}
		target := p.Target
		if target <= 0 {
			target = p.HostPort
		}
		rep.Findings = append(rep.Findings, Finding{
			Kind:    "port_hardcoded",
			Service: service,
			Detail:  fmt.Sprintf("%s %s publishes %s — use \"${%s:-%d}:%d\" so siding can move it", file, service, p.Spec, env, p.HostPort, target),
		})
	}
}

func portFromNumber(n int64) (int, bool) {
	if n < 1 || n > 65535 {
		return 0, false
	}
	return int(n), true
}

func parsePublish(raw any) (hostPublish, bool) {
	switch v := raw.(type) {
	case string:
		return parsePublishSpec(v)
	case int:
		return hostPublish{Spec: strconv.Itoa(v), HostPort: v, Target: v}, v > 0 && v <= 65535
	case int64:
		n, ok := portFromNumber(v)
		return hostPublish{Spec: strconv.Itoa(n), HostPort: n, Target: n}, ok
	case uint64:
		n, ok := portFromNumber(int64(v))
		return hostPublish{Spec: strconv.Itoa(n), HostPort: n, Target: n}, ok
	case float64:
		n, ok := portFromNumber(int64(v))
		return hostPublish{Spec: strconv.Itoa(n), HostPort: n, Target: n}, ok
	case map[string]any:
		if pub, ok := v["published"]; ok {
			target := v["target"]
			spec := strings.TrimSpace(fmt.Sprint(pub))
			if target != nil && fmt.Sprint(target) != "" && fmt.Sprint(target) != "<nil>" {
				spec = fmt.Sprintf("%v:%v", pub, target)
			}
			return parsePublishSpec(spec)
		}
		if len(v) == 1 {
			for k, val := range v {
				host, err := strconv.Atoi(strings.TrimSpace(k))
				target, ok := anyInt(val)
				if err == nil && ok && host > 0 {
					return hostPublish{Spec: fmt.Sprintf("%d:%d", host, target), HostPort: host, Target: target}, true
				}
			}
		}
	}
	return parsePublishSpec(fmt.Sprint(raw))
}

func anyInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return portFromNumber(int64(n))
	case int64:
		return portFromNumber(n)
	case uint64:
		return portFromNumber(int64(n))
	case float64:
		return portFromNumber(int64(n))
	case string:
		p, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return portFromNumber(int64(p))
	}
	return 0, false
}

func parsePublishSpec(spec string) (hostPublish, bool) {
	spec = strings.TrimSpace(strings.Trim(spec, `"'`))
	if spec == "" || spec == "<nil>" {
		return hostPublish{}, false
	}
	p := hostPublish{Spec: spec}
	if m := envPortRef.FindStringSubmatch(spec); m != nil {
		p.Env = m[1]
		if m[2] != "" {
			p.HostPort, _ = strconv.Atoi(m[2])
		}
		p.Target = trailingPort(spec)
		if p.HostPort <= 0 {
			p.HostPort = p.Target
		}
		return p, true
	}
	trimmed := hostPrefix.ReplaceAllString(spec, "")
	m := publishedSpec.FindStringSubmatch(trimmed)
	if m == nil {
		return hostPublish{}, false
	}
	host, _ := strconv.Atoi(m[1])
	target := host
	if m[2] != "" {
		target, _ = strconv.Atoi(m[2])
	}
	if host < 1 {
		return hostPublish{}, false
	}
	p.HostPort = host
	p.Target = target
	return p, true
}

// OwnPublished unions every scanned compose publish into ports/port_env.
// Existing keys win (flags or recipe); scan fills holes so nothing stays at a
// shared compose default across checkouts.
func OwnPublished(ports map[string]int, env map[string]string, scan Report) (map[string]int, map[string]string) {
	scanPorts := collapseIntKeys(scan.Ports)
	scanEnv := collapseStrKeys(scan.PortEnv)
	outPorts := collapseIntKeys(ports)
	outEnv := collapseStrKeys(env)
	keys := make([]string, 0, len(scanPorts)+len(scanEnv))
	seen := map[string]struct{}{}
	for k := range scanPorts {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range scanEnv {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := outPorts[k]; !ok {
			if p := scanPorts[k]; p > 0 {
				outPorts[k] = p
			}
		}
		if outEnv[k] == "" && scanEnv[k] != "" {
			outEnv[k] = scanEnv[k]
		}
	}
	for k := range outPorts {
		if outEnv[k] == "" {
			outEnv[k] = EnvNameForKey(k)
		}
	}
	return outPorts, outEnv
}

func foldPortKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	return strings.ReplaceAll(key, "-", "_")
}

func collapseIntKeys(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]int, len(in))
	for _, k := range keys {
		nk := foldPortKey(k)
		if nk == "" {
			continue
		}
		if v := in[k]; v > 0 {
			if _, ok := out[nk]; !ok {
				out[nk] = v
			}
		}
	}
	return out
}

func collapseStrKeys(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(in))
	for _, k := range keys {
		nk := foldPortKey(k)
		if nk == "" {
			continue
		}
		if v := strings.TrimSpace(in[k]); v != "" {
			if _, ok := out[nk]; !ok {
				out[nk] = v
			}
		}
	}
	return out
}

func portKeyFromEnv(env string) string {
	name := foldPortKey(env)
	name = strings.TrimSuffix(name, "_port")
	switch name {
	case "backend":
		return "api"
	case "frontend", "next", "nextjs", "app":
		return "web"
	default:
		return name
	}
}

func portKeyFromService(service string) string {
	s := foldPortKey(service)
	switch s {
	case "frontend", "next", "nextjs", "app":
		return "web"
	case "backend":
		return "api"
	default:
		return s
	}
}

func EnvNameForKey(key string) string {
	return strings.ToUpper(foldPortKey(key)) + "_PORT"
}

func publishEnvIndex(body string) map[string]string {
	var doc composeDoc
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil || len(doc.Services) == 0 {
		return map[string]string{}
	}
	fake := &Report{Ports: map[string]int{}, PortEnv: map[string]string{}}
	out := map[string]string{}
	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		var pubs []hostPublish
		for _, raw := range doc.Services[name].Ports {
			if p, ok := parsePublish(raw); ok {
				pubs = append(pubs, p)
			}
		}
		hardIdx := 0
		total := len(pubs)
		for _, p := range pubs {
			var key, env string
			if p.Env != "" {
				key = portKeyFromEnv(p.Env)
				env = p.Env
			} else {
				key, env = hardcodedPortKey(name, p.Target, hardIdx, total)
				hardIdx++
			}
			_, env = finalizePublishKey(fake, key, env, p)
			if p.Env != "" {
				continue
			}
			out[publishLookupKey(name, p.HostPort, p.Target)] = env
			out[publishLookupKey(name, p.HostPort, 0)] = env
		}
	}
	return out
}

func finalizePublishKey(rep *Report, key, env string, p hostPublish) (string, string) {
	key = uniquePortKey(rep, key, p.Target)
	if p.Env == "" {
		env = EnvNameForKey(key)
	}
	if p.HostPort > 0 && rep.Ports[key] == 0 {
		rep.Ports[key] = p.HostPort
	}
	if env != "" && rep.PortEnv[key] == "" {
		rep.PortEnv[key] = env
	}
	return key, env
}

func publishLookupKey(service string, host, target int) string {
	return fmt.Sprintf("%s\t%d\t%d", service, host, target)
}

func lookupPublishEnv(idx map[string]string, service string, host, target int) string {
	if e := idx[publishLookupKey(service, host, target)]; e != "" {
		return e
	}
	if e := idx[publishLookupKey(service, host, 0)]; e != "" {
		return e
	}
	if target > 0 {
		if e := idx[publishLookupKey(service, target, target)]; e != "" {
			return e
		}
	}
	_, env := hardcodedPortKey(service, target, 0, 1)
	return env
}

func hardcodedPortKey(service string, target, index, total int) (key, env string) {
	base := portKeyFromService(service)
	key = base
	if total > 1 {
		if (base == "web" || base == "api") && index == 0 {
			key = base
		} else if target > 0 {
			key = fmt.Sprintf("%s_%d", base, target)
		} else {
			key = fmt.Sprintf("%s_%d", base, index+1)
		}
	}
	return key, EnvNameForKey(key)
}

func uniquePortKey(rep *Report, want string, target int) string {
	want = foldPortKey(want)
	taken := func(k string) bool {
		_, inPorts := rep.Ports[k]
		_, inEnv := rep.PortEnv[k]
		return inPorts || inEnv
	}
	if !taken(want) {
		return want
	}
	if target > 0 {
		alt := fmt.Sprintf("%s_%d", want, target)
		if !taken(alt) {
			return alt
		}
	}
	for i := 2; i < 1000; i++ {
		alt := fmt.Sprintf("%s_%d", want, i)
		if !taken(alt) {
			return alt
		}
	}
	return want
}

func checkContractPorts(rep *Report, mode contract.Mode) {
	for key, envName := range mode.PortEnv {
		found := false
		for _, f := range rep.Findings {
			if f.Kind == "port_ok" && strings.Contains(f.Detail, "${"+envName+"}") {
				found = true
				break
			}
		}
		if !found {
			rep.Findings = append(rep.Findings, Finding{
				Kind:   "missing_port_env",
				Detail: fmt.Sprintf("contract port_env.%s=%s is not referenced as ${%s} in compose ports", key, envName, envName),
			})
		}
	}
}

func checkContractOwnsPublished(rep *Report, mode contract.Mode) {
	owned := map[string]struct{}{}
	for k := range mode.Ports {
		if nk := foldPortKey(k); nk != "" {
			owned[nk] = struct{}{}
		}
	}
	for k := range mode.PortEnv {
		if nk := foldPortKey(k); nk != "" {
			owned[nk] = struct{}{}
		}
	}
	keys := make([]string, 0, len(rep.PortEnv)+len(rep.Ports))
	seen := map[string]struct{}{}
	for k := range rep.PortEnv {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range rep.Ports {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := owned[foldPortKey(k)]; ok {
			continue
		}
		env := rep.PortEnv[k]
		if env == "" {
			env = strings.ToUpper(k) + "_PORT"
		}
		rep.Findings = append(rep.Findings, Finding{
			Kind:   "unowned_port",
			Detail: fmt.Sprintf("compose publishes %s (${%s}) but the recipe does not own it — add ports.%s so parallel checkouts cannot collide", k, env, k),
		})
	}
}

func trailingPort(spec string) int {
	spec = strings.Trim(spec, `"'`)
	i := strings.LastIndex(spec, ":")
	if i < 0 || i+1 >= len(spec) {
		return 0
	}
	tail := spec[i+1:]
	if slash := strings.IndexByte(tail, '/'); slash >= 0 {
		tail = tail[:slash]
	}
	n, err := strconv.Atoi(strings.TrimSpace(tail))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func buildTips(rep Report, c *contract.Contract) []string {
	var tips []string
	hard, pub, hostNet, local, isolated, unowned := 0, 0, 0, 0, 0, 0
	for _, f := range rep.Findings {
		switch f.Kind {
		case "port_hardcoded":
			hard++
		case "public_url":
			pub++
		case "host_network":
			hostNet++
		case "localhost_internal":
			local++
		case "network_isolated":
			isolated++
		case "unowned_port":
			unowned++
		}
	}
	if hard > 0 {
		tips = append(tips, "Publish host ports as \"${WEB_PORT:-3000}:3000\", not \"3000:3000\". Siding allocates per checkout; a hardcoded bind cannot move. Prefer a copy under .siding/compose/ so the original compose file still works without siding.")
	}
	if unowned > 0 {
		tips = append(tips, "Every published compose port must be in modes.*.ports / port_env. Unowned binds stay at the compose default and collide across checkouts. Caddy only HTTP-proxies web/api; redis/postgres are still allocated.")
	}
	if hostNet > 0 || local > 0 || isolated > 0 {
		tips = append(tips, "Keep services on one Compose network. Frontend→backend should be http://backend:8000 (service DNS), not localhost. Browsers use *.localhost via Caddy on :80; containers do not.")
	}
	if pub > 0 {
		tips = append(tips,
			"NEXT_PUBLIC_* / VITE_* values are baked into the browser. With the siding proxy they are portless (app.my.stack.localhost), not host:3000 — otherwise redirects skip Caddy.",
			"Without the proxy, include the allocated web port (host:18000). Caddy defaults to host :80.",
			"Set those vars in .siding/env (siding writes SIDING_HOST / SIDING_URL) or under modes.*.env so compose ${NEXT_PUBLIC_...} picks up the slot instead of the :port default in docker-compose.yml.",
		)
	} else if c != nil {
		tips = append(tips, "If the app uses Next.js public URLs, put NEXT_PUBLIC_* in .siding/env from SIDING_HOST / SIDING_URL. Proxy on :80 means those values stay portless.")
	}
	return tips
}

func isHostFacingKey(key string) bool {
	u := strings.ToUpper(strings.TrimSpace(key))
	if publicEnvName.MatchString(u) {
		return true
	}
	switch u {
	case "DEV_FRONTEND_URL", "DEV_API_URL", "DEV_WEB_URL":
		return true
	}
	if strings.Contains(u, "PUBLIC") {
		return true
	}
	if strings.Contains(u, "FRONTEND") && strings.Contains(u, "URL") {
		return true
	}
	return false
}

func skipAutoRewrite(key string) bool {
	u := strings.ToUpper(strings.TrimSpace(key))
	for _, part := range []string{"GCS", "EMULATOR", "ASSET", "HEALTH"} {
		if strings.Contains(u, part) {
			return true
		}
	}
	return false
}

func envPlaceholder(val string) bool {
	val = strings.Trim(val, `"'`)
	return strings.HasPrefix(val, "${") && strings.HasSuffix(val, "}")
}

func dropCoveredSlotEnv(rep *Report, mode contract.Mode) {
	if len(mode.Env) == 0 {
		return
	}
	out := make([]Finding, 0, len(rep.Findings))
	for _, f := range rep.Findings {
		if f.Kind == "public_url" && strings.Contains(f.Detail, "needs .siding/env") {
			parts := strings.SplitN(f.Detail, ": ", 3)
			if len(parts) >= 2 {
				key := strings.Fields(parts[1])[0]
				if _, ok := mode.Env[key]; ok {
					continue
				}
			}
		}
		out = append(out, f)
	}
	rep.Findings = out
}

func compact(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func HasProblems(r Report) bool {
	for _, f := range r.Findings {
		if f.Kind == "port_hardcoded" || f.Kind == "public_url" || f.Kind == "missing_port_env" || f.Kind == "unowned_port" || f.Kind == "compose_missing" || f.Kind == "host_network" || f.Kind == "localhost_internal" || f.Kind == "network_isolated" {
			return true
		}
	}
	return false
}
