package contract

import (
	"fmt"
	"strings"
	"unicode"
)

// hostPatternPlaceholders are the only braced tokens allowed in host / compose_project patterns.
var hostPatternPlaceholders = map[string]struct{}{
	"{stack}": {}, "{project}": {}, "{mode}": {}, "{prefix}": {},
}

// ValidateHostPattern checks a recipe host or compose_project template before expansion.
func ValidateHostPattern(field, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	for _, r := range pattern {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return fmt.Errorf("contract: %s must not contain whitespace", field)
		case r == '"' || r == '`' || r == '\\':
			return fmt.Errorf("contract: %s contains invalid character %q", field, r)
		case unicode.IsControl(r):
			return fmt.Errorf("contract: %s contains control character", field)
		}
	}
	// Validate braced segments are exactly known placeholders.
	i := 0
	for i < len(pattern) {
		if pattern[i] != '{' {
			i++
			continue
		}
		j := strings.IndexByte(pattern[i:], '}')
		if j < 0 {
			return fmt.Errorf("contract: %s has unclosed placeholder", field)
		}
		token := pattern[i : i+j+1]
		if _, ok := hostPatternPlaceholders[token]; !ok {
			return fmt.Errorf("contract: %s has unknown placeholder %s", field, token)
		}
		i += j + 1
	}
	return nil
}

// ValidateProjectID is the product id in the recipe (also substituted into patterns).
func ValidateProjectID(project string) error {
	project = strings.TrimSpace(strings.ToLower(project))
	if project == "" {
		return fmt.Errorf("contract: project is required")
	}
	return validateLabelRun("project", project, '.', '-', '_')
}

// ValidateExpandedHostname checks a fully expanded browser hostname (with or without .localhost).
// A leading *. is allowed for Caddy wildcard site matchers (e.g. *.hello.test.localhost).
func ValidateExpandedHostname(host string) error {
	h := strings.TrimSpace(strings.ToLower(host))
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return fmt.Errorf("contract: host is empty")
	}
	if strings.ContainsAny(h, " \t\n\r\"'`\\{}/") {
		return fmt.Errorf("contract: host %q contains invalid characters", host)
	}
	h = strings.TrimPrefix(h, "*.")
	base := strings.TrimSuffix(h, ".localhost")
	if base == "" {
		return fmt.Errorf("contract: host is only .localhost")
	}
	for _, label := range strings.Split(base, ".") {
		if err := validateDNSLabel("host", label); err != nil {
			return err
		}
	}
	return nil
}

// ValidateComposeProjectName checks an expanded docker compose project name.
func ValidateComposeProjectName(name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return fmt.Errorf("contract: compose project is empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("contract: compose project name too long")
	}
	return validateLabelRun("compose_project", name, '-', '_')
}

func validateLabelRun(field, value string, extra ...rune) error {
	allowedExtra := map[rune]struct{}{}
	for _, r := range extra {
		allowedExtra[r] = struct{}{}
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			if _, ok := allowedExtra[r]; ok {
				continue
			}
			return fmt.Errorf("contract: %s %q has invalid character at position %d", field, value, i)
		}
	}
	return nil
}

func validateDNSLabel(field, label string) error {
	if label == "" {
		return fmt.Errorf("contract: %s has empty label", field)
	}
	if len(label) > 63 {
		return fmt.Errorf("contract: %s label %q too long", field, label)
	}
	for i, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-':
			if i == 0 || i == len(label)-1 {
				return fmt.Errorf("contract: %s label %q has leading or trailing hyphen", field, label)
			}
		default:
			return fmt.Errorf("contract: %s label %q has invalid character", field, label)
		}
	}
	return nil
}
