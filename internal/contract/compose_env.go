package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeEnvFile is one repo-relative docker compose --env-file for a mode.
type ComposeEnvFile struct {
	Path     string
	Optional bool
}

// ComposeEnvFileList is modes.<name>.compose_env_files in the recipe.
type ComposeEnvFileList []ComposeEnvFile

func (l *ComposeEnvFileList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		*l = nil
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("compose_env_files must be a list")
	}
	var out ComposeEnvFileList
	for _, item := range value.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			path := normalizeComposeEnvPath(item.Value)
			if path == "" {
				continue
			}
			out = append(out, ComposeEnvFile{Path: path})
		case yaml.MappingNode:
			var row struct {
				Path     string `yaml:"path"`
				Optional bool   `yaml:"optional"`
			}
			if err := item.Decode(&row); err != nil {
				return fmt.Errorf("compose_env_files entry: %w", err)
			}
			path := normalizeComposeEnvPath(row.Path)
			if path == "" {
				continue
			}
			out = append(out, ComposeEnvFile{Path: path, Optional: row.Optional})
		default:
			return fmt.Errorf("compose_env_files entries must be strings or mappings")
		}
	}
	*l = NormalizeComposeEnvFileList(out)
	return nil
}

func normalizeComposeEnvPath(raw string) string {
	f := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if f == "" || strings.EqualFold(f, "none") {
		return ""
	}
	return f
}

// NormalizeComposeEnvFileList trims paths and dedupes by path (first wins).
func NormalizeComposeEnvFileList(list ComposeEnvFileList) ComposeEnvFileList {
	seen := map[string]struct{}{}
	var out ComposeEnvFileList
	for _, e := range list {
		path := normalizeComposeEnvPath(e.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, ComposeEnvFile{Path: path, Optional: e.Optional})
	}
	return out
}

// ComposeEnvFilesFromPaths builds a required-only list from CLI flags.
func ComposeEnvFilesFromPaths(paths []string) ComposeEnvFileList {
	paths = NormalizeComposeEnvFiles(paths)
	out := make(ComposeEnvFileList, 0, len(paths))
	for _, p := range paths {
		out = append(out, ComposeEnvFile{Path: p})
	}
	return out
}

// Paths returns configured repo-relative paths in recipe order.
func (l ComposeEnvFileList) Paths() []string {
	l = NormalizeComposeEnvFileList(l)
	out := make([]string, 0, len(l))
	for _, e := range l {
		out = append(out, e.Path)
	}
	return out
}

// SelectComposeEnvPaths returns paths to pass as docker compose --env-file.
// Optional entries missing on disk are omitted (listed in skipped).
func SelectComposeEnvPaths(repoRoot string, list ComposeEnvFileList) (included, skipped []string, err error) {
	list = NormalizeComposeEnvFileList(list)
	for _, e := range list {
		if e.Path == "" {
			continue
		}
		full := filepath.Join(repoRoot, filepath.FromSlash(e.Path))
		st, statErr := os.Stat(full)
		present := statErr == nil && st.Mode().IsRegular()
		if present {
			included = append(included, e.Path)
			continue
		}
		if e.Optional {
			skipped = append(skipped, e.Path)
			continue
		}
		return nil, nil, fmt.Errorf("required compose env file missing: %s", e.Path)
	}
	return included, skipped, nil
}
