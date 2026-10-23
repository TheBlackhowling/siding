//go:build !windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const unixPathMarker = "# added by siding install"

func init() {
	addUserPath = addUnixUserPath
}

func addUnixUserPath(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	dir = filepath.Clean(dir)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	line := fmt.Sprintf(`export PATH=%q:$PATH`, dir)
	for _, name := range unixShellConfigFiles() {
		path := filepath.Join(home, name)
		if err := appendUnixPathLine(path, line); err != nil {
			return err
		}
	}
	return nil
}

func unixShellConfigFiles() []string {
	if runtime.GOOS == "darwin" {
		return []string{".zprofile", ".zshrc"}
	}
	return []string{".profile", ".bashrc"}
}

func appendUnixPathLine(path, exportLine string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		body = nil
	}
	if bytesContainsPathEntry(body, exportLine) {
		return nil
	}
	var b strings.Builder
	if len(body) > 0 {
		b.Write(body)
		if body[len(body)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n")
	b.WriteString(unixPathMarker)
	b.WriteByte('\n')
	b.WriteString(exportLine)
	b.WriteByte('\n')
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func bytesContainsPathEntry(body []byte, exportLine string) bool {
	s := string(body)
	if strings.Contains(s, unixPathMarker) {
		return true
	}
	// Same directory already exported (manual or older install).
	dir := strings.TrimPrefix(exportLine, `export PATH="`)
	if i := strings.Index(dir, `"`); i >= 0 {
		dir = dir[:i]
	}
	return dir != "" && strings.Contains(s, dir)
}
