package contract

import (
	"fmt"
	"strings"

	"github.com/TheBlackhowling/siding/internal/paths"
)

// DefaultProject is the product id for a new recipe. It is shared by every
// clone: the folder prefix without a trailing hyphen (acme- → acme),
// else the folder name. The clone-specific suffix is the stack, not project —
// otherwise compose `{project}-{stack}-{mode}` becomes acme-genie2-genie3-test.
func DefaultProject(folder, prefix string) string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if stem := strings.Trim(prefix, "-_"); stem != "" {
		return stem
	}
	return paths.Basename(folder)
}

// ResolveStackName derives the stack key from the checkout folder.
//
// Priority: --stack → folder basename with folder_prefix stripped → basename.
// The contract does not list slot names; a new clone folder is a new stack.
func ResolveStackName(c *Contract, repoRoot, override string) (string, error) {
	if override != "" {
		return sanitizeStackName(override)
	}
	base := paths.Basename(repoRoot)
	prefix := strings.ToLower(strings.TrimSpace(c.FolderPrefix))
	if prefix != "" && strings.HasPrefix(base, prefix) {
		rest := strings.TrimPrefix(base, prefix)
		if rest != "" {
			return sanitizeStackName(rest)
		}
	}
	return sanitizeStackName(base)
}

func sanitizeStackName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r == '_' || r == '.':
			return '-'
		default:
			return -1
		}
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "", fmt.Errorf("stack name is empty after sanitizing %q", raw)
	}
	return name, nil
}

// Expand replaces {stack}, {project}, {mode}, {prefix} and drops adjacent
// duplicate segments so {stack}.{project}.{mode} becomes hello.test when
// stack == project. prefix is optional (host_prefix, e.g. app.acme).
func Expand(pattern, stack, project, mode string, prefix ...string) string {
	if pattern == "" {
		return ""
	}
	pre := ""
	if len(prefix) > 0 {
		pre = NormalizeHostPrefix(prefix[0])
	}
	repl := strings.NewReplacer(
		"{stack}", stack,
		"{project}", project,
		"{mode}", mode,
		"{prefix}", pre,
	)
	return collapseDuplicateSegments(repl.Replace(pattern))
}

func collapseDuplicateSegments(value string) string {
	sep := "."
	if !strings.Contains(value, ".") && strings.Contains(value, "-") {
		sep = "-"
	}
	parts := strings.Split(value, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == p {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, sep)
}
