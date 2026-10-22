package advise

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TheBlackhowling/siding/internal/contract"
)

type Change struct {
	File   string
	Reason string
	Before string
	After  string
}

var (
	serviceHeader   = regexp.MustCompile(`^(\s{2,})([A-Za-z0-9._-]+):\s*(?:&[A-Za-z0-9._-]+)?\s*$`)
	hardPortLine    = regexp.MustCompile(`^(\s+-\s+)["']?(?:(?:\d{1,3}(?:\.\d{1,3}){3}|localhost|\[::\]):)?(\d+)(?::(\d+))?(?:/(tcp|udp))?["']?(?:\s+(#.*))?\s*$`)
	publishedAssign = regexp.MustCompile(`^(\s+published:\s*)["']?(\d+)["']?(\s*(?:#.*)?)?\s*$`)
	internalLocal   = regexp.MustCompile(`(?i)(https?://)(?:localhost|127\.0\.0\.1)(?::(\d+))?`)
	networkModeLine = regexp.MustCompile(`(?i)^\s+network_mode:\s*[\"']?host[\"']?\s*$`)
)

var nestedComposeKeys = map[string]struct{}{
	"ports": {}, "environment": {}, "env_file": {}, "networks": {}, "volumes": {},
	"depends_on": {}, "healthcheck": {}, "command": {}, "entrypoint": {},
	"image": {}, "build": {}, "restart": {}, "labels": {}, "expose": {},
	"extra_hosts": {}, "profiles": {}, "deploy": {}, "logging": {},
	"network_mode": {}, "hostname": {}, "user": {}, "working_dir": {},
	"container_name": {}, "privileged": {}, "cap_add": {}, "cap_drop": {},
	"ulimits": {}, "tmpfs": {}, "secrets": {}, "configs": {}, "extends": {},
	"platform": {}, "pull_policy": {}, "stdin_open": {}, "tty": {}, "init": {},
	"stop_grace_period": {}, "interval": {}, "timeout": {},
	"retries": {}, "start_period": {}, "target": {}, "published": {},
	"protocol": {}, "mode": {}, "services": {}, "name": {}, "x-siding": {},
	"aliases": {}, "ipv4_address": {}, "ipv6_address": {}, "driver": {},
	"ipam": {}, "external": {}, "attachable": {}, "default": {},
}

func parseServiceHeader(line string) (name string, indent int, ok bool) {
	m := serviceHeader.FindStringSubmatch(line)
	if m == nil {
		return "", 0, false
	}
	name = m[2]
	if strings.HasPrefix(name, "x-") {
		return "", 0, false
	}
	if _, nested := nestedComposeKeys[name]; nested {
		return "", 0, false
	}
	return name, len(m[1]), true
}

func isSkipBlock(trim string, indent, serviceIndent int) bool {
	if strings.HasPrefix(trim, "healthcheck:") || strings.HasPrefix(trim, "command:") || strings.HasPrefix(trim, "entrypoint:") {
		return true
	}
	if strings.HasPrefix(trim, "test:") && serviceIndent >= 0 && indent > serviceIndent {
		return true
	}
	return false
}

// composeCursor tracks the current service while walking compose YAML.
// Only keys at the first service indent under `services:` count as services,
// so networks.default.aliases does not steal later ports: (Genie web).
type composeCursor struct {
	service        string
	serviceIndent  int
	servicesIndent int
	inServices     bool
	skipUntil      int
}

func newComposeCursor() composeCursor {
	return composeCursor{skipUntil: -1, serviceIndent: -1, servicesIndent: -1}
}

func (c *composeCursor) feed(line string) bool {
	trim := strings.TrimSpace(line)
	ind := indentWidth(line)
	if c.skipUntil >= 0 {
		if trim != "" && !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, "-") && ind <= c.skipUntil {
			c.skipUntil = -1
		} else {
			return false
		}
	}
	if key, _, ok := yamlAssign(line); ok && key == "services" {
		c.inServices = true
		c.servicesIndent = ind
		c.serviceIndent = -1
		c.service = ""
		return false
	}
	if c.inServices && trim != "" && !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, "-") && ind <= c.servicesIndent {
		c.inServices = false
		c.service = ""
	}
	if c.inServices {
		if name, sind, ok := parseServiceHeader(line); ok {
			if c.serviceIndent < 0 && sind > c.servicesIndent {
				c.serviceIndent = sind
			}
			if c.serviceIndent >= 0 && sind == c.serviceIndent {
				c.service = name
			}
		}
		if isSkipBlock(trim, ind, c.serviceIndent) {
			c.skipUntil = ind
			return false
		}
	}
	return true
}

func SuggestedPublicTemplate(key string) string {
	u := strings.ToUpper(key)
	switch {
	case strings.Contains(u, "ROOT_DOMAIN") || strings.HasSuffix(u, "_DOMAIN"):
		return "{host}{proxy_suffix}"
	case strings.HasSuffix(u, "_HOST") && !strings.Contains(u, "GCS"):
		return "{host}{proxy_suffix}"
	case strings.Contains(u, "FRONTEND") || strings.Contains(u, "APP_URL") || strings.Contains(u, "WEB_URL"):
		return "http://{web_host}{web_suffix}"
	case strings.Contains(u, "API"):
		return "http://{api_host}{api_suffix}"
	default:
		return "{url}"
	}
}

// RewriteCompose updates published ports, strips :port defaults from public URL
// env vars, and rewrites internal localhost URLs to Compose service DNS.
func RewriteCompose(body string, containerPortToService map[int]string) (string, []Change, map[string]string) {
	body = normalizeNewlines(body)
	var changes []Change
	publicEnv := map[string]string{}
	lines := strings.Split(body, "\n")
	envByPublish := publishEnvIndex(body)
	cur := newComposeCursor()
	for i, line := range lines {
		if !cur.feed(line) {
			continue
		}
		service := cur.service
		if m := hardPortLine.FindStringSubmatch(line); m != nil {
			host := m[2]
			target := m[3]
			if target == "" {
				target = host
			}
			hostN, _ := strconv.Atoi(host)
			targetN, _ := strconv.Atoi(target)
			env := lookupPublishEnv(envByPublish, service, hostN, targetN)
			after := fmt.Sprintf("%s\"${%s:-%s}:%s\"", m[1], env, host, target)
			if m[4] != "" {
				after += "/" + m[4]
			}
			if m[5] != "" {
				after += "  " + strings.TrimSpace(m[5])
			}
			changes = append(changes, Change{Reason: "published port → siding env", Before: strings.TrimSpace(line), After: strings.TrimSpace(after)})
			lines[i] = after
			continue
		}
		if m := publishedAssign.FindStringSubmatch(line); m != nil && service != "" {
			hostN, _ := strconv.Atoi(m[2])
			env := lookupPublishEnv(envByPublish, service, hostN, 0)
			after := fmt.Sprintf("%s\"${%s:-%s}\"%s", m[1], env, m[2], m[3])
			changes = append(changes, Change{Reason: "published port → siding env", Before: strings.TrimSpace(line), After: strings.TrimSpace(after)})
			lines[i] = after
			continue
		}
		if key, val, ok := yamlAssign(line); ok && isHostFacingKey(key) && !skipAutoRewrite(key) {
			if portInDefault.MatchString(val) || strings.Contains(val, "_PORT") || internalLocal.MatchString(val) {
				after := indentOf(line) + key + ": " + wrapEnvKeepDefault(key, val)
				changes = append(changes, Change{Reason: "public URL default must come from .siding/env", Before: strings.TrimSpace(line), After: strings.TrimSpace(after)})
				lines[i] = after
				publicEnv[key] = SuggestedPublicTemplate(key)
			}
			continue
		}
		if key, _, ok := yamlAssign(line); ok && (isHostFacingKey(key) || skipAutoRewrite(key)) {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "localhost") && !strings.Contains(line, "127.0.0.1") {
			continue
		}
		if !internalLocal.MatchString(line) {
			continue
		}
		rewritten := internalLocal.ReplaceAllStringFunc(line, func(match string) string {
			parts := internalLocal.FindStringSubmatch(match)
			scheme := parts[1]
			port := 0
			if parts[2] != "" {
				fmt.Sscanf(parts[2], "%d", &port)
			}
			svc := containerPortToService[port]
			if svc == "" {
				return match
			}
			if port > 0 {
				return fmt.Sprintf("%s%s:%d", scheme, svc, port)
			}
			return scheme + svc
		})
		if rewritten != line {
			changes = append(changes, Change{Reason: "in-stack URL should use Compose service DNS, not localhost", Before: strings.TrimSpace(line), After: strings.TrimSpace(rewritten)})
			lines[i] = rewritten
		}
	}
	body = strings.Join(lines, "\n")
	next, netChanges := EnsureSharedNetwork(body)
	changes = append(changes, netChanges...)
	return next, changes, publicEnv
}

func wrapEnvKeepDefault(key, val string) string {
	val = strings.TrimSpace(val)
	prefix := "${" + key
	if strings.HasPrefix(val, prefix) {
		if strings.HasPrefix(val, prefix+":-") && strings.HasSuffix(val, "}") {
			return val
		}
		if val == prefix+"}" {
			return val
		}
	}
	return "${" + key + ":-" + val + "}"
}

func yamlAssign(line string) (key, val string, ok bool) {
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "-") {
		return "", "", false
	}
	k, v, cut := strings.Cut(trim, ":")
	if !cut {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if k == "" || strings.ContainsAny(k, " \t") {
		return "", "", false
	}
	return k, strings.TrimSpace(v), true
}

func ContainerPortIndex(body string) map[int]string {
	idx := map[int]string{}
	cur := newComposeCursor()
	for _, line := range strings.Split(body, "\n") {
		if !cur.feed(line) {
			continue
		}
		service := cur.service
		if m := envPortRef.FindStringSubmatch(line); m != nil {
			// ${WEB_PORT:-3000}:80  → container 80
			if i := strings.LastIndex(line, ":"); i >= 0 {
				tail := strings.Trim(line[i+1:], " \"'/\t")
				tail = strings.TrimSuffix(tail, "tcp")
				var p int
				if _, err := fmt.Sscanf(tail, "%d", &p); err == nil && p > 0 && service != "" {
					idx[p] = service
				}
			}
			continue
		}
		if m := hardPortLine.FindStringSubmatch(line); m != nil && service != "" {
			var p int
			fmt.Sscanf(m[3], "%d", &p)
			if p > 0 {
				idx[p] = service
			}
		}
	}
	return idx
}

// WriteSidingCompose writes a rewritten copy under .siding/compose/. The
// original compose file is not modified.
func WriteSidingCompose(repoRoot, sourceRel, body string) (destRel string, err error) {
	destRel = contract.SidingComposeRel(sourceRel)
	dest := filepath.Join(repoRoot, filepath.FromSlash(destRel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err == nil {
		backupDir := filepath.Join(repoRoot, ".siding", "backups")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return "", err
		}
		prev, readErr := os.ReadFile(dest)
		if readErr == nil {
			stamp := time.Now().UTC().Format("20060102-150405")
			bak := filepath.Join(backupDir, filepath.Base(dest)+"."+stamp)
			_ = os.WriteFile(bak, prev, 0o644)
		}
	}
	if err := os.WriteFile(dest, []byte(WithSidingHeader(body, sourceRel)), 0o644); err != nil {
		return "", err
	}
	return destRel, nil
}

func WithSidingHeader(body, sourceRel string) string {
	sourceRel = filepath.ToSlash(sourceRel)
	header := "# Generated by siding from " + sourceRel + ". The original file is unchanged.\n" +
		"# Regenerate with: siding init --force --rewrite-compose\n"
	body = strings.TrimPrefix(body, "\ufeff")
	if strings.HasPrefix(body, "# Generated by siding from ") {
		if i := strings.Index(body, "\n"); i >= 0 {
			body = body[i+1:]
			if strings.HasPrefix(body, "# Regenerat") {
				if j := strings.Index(body, "\n"); j >= 0 {
					body = body[j+1:]
				}
			}
		}
	}
	return header + body
}
