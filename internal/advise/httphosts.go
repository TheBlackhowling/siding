package advise

import (
	"net/url"
	"strings"

	"github.com/TheBlackhowling/siding/internal/contract"
)

var (
	webSubLabels = map[string]struct{}{
		"app": {}, "www": {}, "web": {}, "ui": {}, "frontend": {}, "client": {}, "shop": {},
	}
	apiSubLabels = map[string]struct{}{
		"api": {}, "graphql": {},
	}
)

func addHTTPHost(dst map[string][]string, portKey, label string) {
	label = contract.NormalizeHTTPLabel(label)
	if label == "" || portKey == "" {
		return
	}
	for _, have := range dst[portKey] {
		if have == label {
			return
		}
	}
	dst[portKey] = append(dst[portKey], label)
}

func InferHTTPHost(envKey, raw string) (portKey, label string) {
	host := hostnameFromValue(raw)
	if host == "" {
		return "", ""
	}
	label = contract.NormalizeHTTPLabel(strings.Split(host, ".")[0])
	if label == "" {
		return "", ""
	}
	key := strings.ToUpper(strings.TrimSpace(envKey))
	switch {
	case strings.Contains(key, "API") || strings.Contains(key, "GRAPHQL"):
		if _, ok := apiSubLabels[label]; ok {
			return "api", label
		}
		return "", ""
	case strings.Contains(key, "FRONTEND") || strings.Contains(key, "APP_URL") || strings.Contains(key, "WEB_URL") ||
		strings.Contains(key, "ROOT_DOMAIN") || strings.HasSuffix(key, "_DOMAIN") || strings.Contains(key, "HOST"):
		if _, ok := webSubLabels[label]; ok {
			return "web", label
		}
		return "", ""
	default:
		if _, ok := webSubLabels[label]; ok {
			return "web", label
		}
		if _, ok := apiSubLabels[label]; ok {
			return "api", label
		}
		return "", ""
	}
}

func hostnameFromValue(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ":-"); i >= 0 {
		s = s[i+2:]
		s = strings.TrimRight(s, "}")
	}
	s = strings.Trim(s, `"'`)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" {
			return ""
		}
		s = u.Hostname()
	} else if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "localhost" || strings.Count(s, ".") == 0 {
		return ""
	}
	return s
}

func InferHTTPHostsFromService(name string) (portKey, label string) {
	key := portKeyFromService(name)
	label = contract.NormalizeHTTPLabel(name)
	if label == "" {
		return "", ""
	}
	switch key {
	case "web":
		if _, ok := webSubLabels[label]; ok && label != "web" {
			return "web", label
		}
	case "api":
		if _, ok := apiSubLabels[label]; ok && label != "api" {
			return "api", label
		}
	}
	return "", ""
}
