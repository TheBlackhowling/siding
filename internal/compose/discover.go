package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ListRootComposeFiles returns compose YAML filenames in repoRoot (not nested).
// Matches Compose's usual names: compose.yaml, docker-compose.yml, and
// docker-compose.*.yml / compose.*.yml overlays.
func ListRootComposeFiles(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isRootComposeName(name) {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := composeRank(names[i]), composeRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names, nil
}

func isRootComposeName(name string) bool {
	lower := strings.ToLower(name)
	if lower == "compose.yml" || lower == "compose.yaml" ||
		lower == "docker-compose.yml" || lower == "docker-compose.yaml" {
		return true
	}
	if !(strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")) {
		return false
	}
	return strings.HasPrefix(lower, "docker-compose.") || strings.HasPrefix(lower, "compose.")
}

func composeRank(name string) int {
	switch strings.ToLower(name) {
	case "compose.yaml":
		return 0
	case "compose.yml":
		return 1
	case "docker-compose.yaml":
		return 2
	case "docker-compose.yml":
		return 3
	default:
		return 10
	}
}

// ParseComposeSelection maps a wizard answer onto files.
// Empty, "all", or "*" means every file. Otherwise 1-based indexes, comma-separated.
func ParseComposeSelection(raw string, files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no compose files to select")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "all") || raw == "*" {
		return append([]string(nil), files...), nil
	}
	return parseNumberedSelection(raw, files, "compose")
}

// ParseEnvFileSelection maps a wizard answer onto extra compose --env-file
// paths. Order is override order. none / 0 / empty skips. all uses list order.
func ParseEnvFileSelection(raw string, files []string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "none") || raw == "0" {
		return nil, nil
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no env files to select")
	}
	if strings.EqualFold(raw, "all") || raw == "*" {
		return append([]string(nil), files...), nil
	}
	return parseNumberedSelection(raw, files, "env file")
}

func parseNumberedSelection(raw string, files []string, kind string) ([]string, error) {
	seen := map[int]struct{}{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, "all") || part == "*" {
			return append([]string(nil), files...), nil
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > len(files) {
			return nil, fmt.Errorf("%s selection %q: use numbers 1-%d, comma-separated (order is override order), all, or none", kind, part, len(files))
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, files[n-1])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("select %s numbers, all, or none", kind)
	}
	return out, nil
}

// RelComposePath is the repo-relative slash path for a root compose filename.
func RelComposePath(name string) string {
	n := filepath.ToSlash(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "./")
	if n == "" {
		return n
	}
	if strings.Contains(n, "/") {
		return n
	}
	return filepath.ToSlash(filepath.Base(n))
}
