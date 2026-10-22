package compose

import (
	"path/filepath"
	"strconv"
	"strings"
)

// FileGroup is one independently runnable compose instance: own mode, ports,
// compose project, and env file. Override files stay with the primary base.
type FileGroup struct {
	Mode  string
	Files []string
}

// GroupFiles splits root compose filenames into instances that can run at the
// same time without sharing host ports or a Compose project.
//
// docker-compose.yml / compose.yaml is mode "test", with *override* files
// attached (Compose -f disables automatic override). Other docker-compose.*.yml
// files each become their own mode named from the suffix (prod-db, husky).
func GroupFiles(files []string) []FileGroup {
	if len(files) == 0 {
		return nil
	}
	var bases, primaryOverrides, rest []string
	var otherOverrides []string
	for _, f := range files {
		name := RelComposePath(f)
		switch {
		case isPrimaryOverrideName(name):
			primaryOverrides = append(primaryOverrides, name)
		case isOverrideComposeName(name):
			otherOverrides = append(otherOverrides, name)
		case isBaseComposeName(name):
			bases = append(bases, name)
		default:
			rest = append(rest, name)
		}
	}

	used := map[string]struct{}{}
	var groups []FileGroup
	if len(bases) > 0 {
		primary := []string{bases[0]}
		primary = append(primary, primaryOverrides...)
		groups = append(groups, FileGroup{Mode: uniqueModeName("test", used), Files: primary})
		for _, b := range bases[1:] {
			groups = append(groups, FileGroup{Mode: uniqueModeName(ModeNameFromFile(b), used), Files: []string{b}})
		}
	} else if len(primaryOverrides) > 0 {
		groups = append(groups, FileGroup{Mode: uniqueModeName("test", used), Files: primaryOverrides})
	}
	for i, f := range rest {
		mode := ModeNameFromFile(f)
		if len(groups) == 0 && i == 0 {
			mode = "test"
		}
		groups = append(groups, FileGroup{Mode: uniqueModeName(mode, used), Files: []string{f}})
	}
	for _, ov := range otherOverrides {
		mode := overrideAttachMode(ov)
		attached := false
		for i := range groups {
			if groups[i].Mode == mode {
				groups[i].Files = append(groups[i].Files, ov)
				attached = true
				break
			}
		}
		if !attached {
			groups = append(groups, FileGroup{Mode: uniqueModeName(mode, used), Files: []string{ov}})
		}
	}
	return groups
}

// ModeNameFromFile turns docker-compose.prod-db.yml into prod-db.
func ModeNameFromFile(name string) string {
	base := strings.ToLower(filepath.Base(RelComposePath(name)))
	base = strings.TrimSuffix(base, ".yaml")
	base = strings.TrimSuffix(base, ".yml")
	switch base {
	case "compose":
		return "compose"
	case "docker-compose":
		return "docker-compose"
	}
	for _, p := range []string{"docker-compose.", "compose."} {
		if strings.HasPrefix(base, p) {
			rest := strings.TrimPrefix(base, p)
			if rest == "" || rest == "override" || strings.HasPrefix(rest, "override.") {
				return "test"
			}
			return sanitizeModeName(rest)
		}
	}
	return sanitizeModeName(base)
}

// SanitizeModeName folds a wizard/flag suffix into a mode key (test, prod, …).
func SanitizeModeName(raw string) string {
	return sanitizeModeName(raw)
}

func isBaseComposeName(name string) bool {
	switch strings.ToLower(filepath.Base(name)) {
	case "compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml":
		return true
	}
	return false
}

func isOverrideComposeName(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	return strings.Contains(lower, "override")
}

func isPrimaryOverrideName(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	switch lower {
	case "docker-compose.override.yml", "docker-compose.override.yaml",
		"compose.override.yml", "compose.override.yaml":
		return true
	default:
		return strings.HasPrefix(lower, "docker-compose.override.") || strings.HasPrefix(lower, "compose.override.")
	}
}

// overrideAttachMode maps docker-compose.prod.override.yml onto mode "prod".
func overrideAttachMode(name string) string {
	mode := ModeNameFromFile(name)
	mode = strings.TrimSuffix(mode, "-override")
	mode = strings.TrimSuffix(mode, "_override")
	if mode == "" || mode == "override" {
		return "test"
	}
	return mode
}

func sanitizeModeName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		b.WriteRune(r)
		lastDash = r == '-'
	}
	name = strings.Trim(b.String(), "-")
	if name == "" {
		return "test"
	}
	return name
}

func uniqueModeName(want string, used map[string]struct{}) string {
	if want == "" {
		want = "test"
	}
	if _, ok := used[want]; !ok {
		used[want] = struct{}{}
		return want
	}
	for i := 2; i < 1000; i++ {
		n := want + "-" + strconv.Itoa(i)
		if _, ok := used[n]; !ok {
			used[n] = struct{}{}
			return n
		}
	}
	return want
}

// ApplyModeNames overlays FILE=suffix from the wizard or --mode-name.
// Lookup is by any file in the group. Empty names keep the current mode.
func ApplyModeNames(groups []FileGroup, byFile map[string]string) []FileGroup {
	if len(groups) == 0 {
		return nil
	}
	used := map[string]struct{}{}
	out := make([]FileGroup, 0, len(groups))
	for _, g := range groups {
		name := g.Mode
		for _, f := range g.Files {
			raw, ok := byFile[RelComposePath(f)]
			if !ok || strings.TrimSpace(raw) == "" {
				continue
			}
			name = sanitizeModeName(raw)
			break
		}
		out = append(out, FileGroup{Mode: uniqueModeName(name, used), Files: append([]string(nil), g.Files...)})
	}
	return out
}
