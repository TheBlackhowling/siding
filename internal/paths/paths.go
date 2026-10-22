package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Abs cleans and absolutizes a path. Empty path uses the current working directory.
func Abs(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	return filepath.Abs(filepath.Clean(path))
}

// Basename is the lowercased final path segment, without trailing separators.
func Basename(path string) string {
	clean := strings.TrimRight(strings.ReplaceAll(path, "\\", "/"), "/")
	if clean == "" {
		return ""
	}
	parts := strings.Split(clean, "/")
	return strings.ToLower(parts[len(parts)-1])
}

// Parent returns the directory containing the checkout.
func Parent(checkoutPath string) string {
	return filepath.Dir(checkoutPath)
}

// NormalizeLockPath compares lock claimants across Windows and POSIX spellings.
func NormalizeLockPath(checkoutPath string) string {
	value := strings.TrimRight(strings.ReplaceAll(checkoutPath, "\\", "/"), "/")
	if len(value) >= 2 && value[1] == ':' {
		value = strings.ToLower(string(value[0])) + value[1:]
	}
	return value
}
