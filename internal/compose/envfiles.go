package compose

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/TheBlackhowling/siding/internal/contract"
)

// GuessComposeEnvFiles finds repo env files a mode should pass as extra
// docker compose --env-file flags (interpolation). Only existing docker/.env*
// (or root .env.*) paths are returned. Service env_file entries like
// apps/backend/.env are ignored — those load inside the container.
func GuessComposeEnvFiles(repoRoot, mode string, composeFiles []string) []string {
	_, guessed := ListComposeEnvFiles(repoRoot, mode, composeFiles)
	return guessed
}

// ListComposeEnvFiles returns env files that belong to this mode only:
// paths declared in that mode's compose files, docker/.env.<compose-suffix>,
// and docker/.env.<mode>. Other modes' files are not listed.
func ListComposeEnvFiles(repoRoot, mode string, composeFiles []string) (all, guessed []string) {
	var candidates []string
	for _, f := range composeFiles {
		candidates = append(candidates, envFilesDeclaredInCompose(repoRoot, f)...)
		if suf := interpolationSuffix(f); suf != "" {
			candidates = append(candidates, "docker/.env."+suf)
		}
	}
	if mode != "" && mode != "test" {
		candidates = append(candidates, "docker/.env."+mode)
	}
	guessed = existingComposeEnvFiles(repoRoot, candidates)
	return guessed, guessed
}

// DefaultEnvFileSelection is the wizard default: guessed file numbers in
// override order, or none.
func DefaultEnvFileSelection(all, guessed []string) string {
	if len(guessed) == 0 || len(all) == 0 {
		return "none"
	}
	var nums []string
	for _, g := range guessed {
		for i, a := range all {
			if a == g {
				nums = append(nums, strconv.Itoa(i+1))
				break
			}
		}
	}
	if len(nums) == 0 {
		return "none"
	}
	return strings.Join(nums, ",")
}

func interpolationSuffix(composeFile string) string {
	suf := ModeNameFromFile(composeFile)
	switch suf {
	case "", "test", "docker-compose", "compose":
		return ""
	default:
		return suf
	}
}

func envFilesDeclaredInCompose(repoRoot, composeFile string) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(composeFile)))
	if err != nil {
		// Rewritten copies live under .siding/compose/; originals may still be at the root.
		data, err = os.ReadFile(filepath.Join(repoRoot, filepath.Base(composeFile)))
		if err != nil {
			return nil
		}
	}
	var doc struct {
		Services map[string]struct {
			EnvFile any `yaml:"env_file"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []string
	for _, svc := range doc.Services {
		for _, p := range flattenEnvFile(svc.EnvFile) {
			if keepGuessedEnvFile(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func flattenEnvFile(v any) []string {
	switch t := v.(type) {
	case string:
		return contract.NormalizeComposeEnvFiles([]string{t})
	case []any:
		var out []string
		for _, item := range t {
			switch row := item.(type) {
			case string:
				out = append(out, row)
			case map[string]any:
				if p, ok := row["path"].(string); ok {
					out = append(out, p)
				}
			}
		}
		return contract.NormalizeComposeEnvFiles(out)
	default:
		return nil
	}
}

func keepGuessedEnvFile(rel string) bool {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" || strings.HasPrefix(rel, ".siding/") {
		return false
	}
	base := filepath.Base(rel)
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return false
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	return dir == "." || dir == "" || dir == "docker"
}

func existingComposeEnvFiles(repoRoot string, candidates []string) []string {
	var exist []string
	for _, rel := range contract.NormalizeComposeEnvFiles(candidates) {
		if !keepGuessedEnvFile(rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); err != nil {
			continue
		}
		exist = append(exist, rel)
	}
	return contract.NormalizeComposeEnvFiles(exist)
}
