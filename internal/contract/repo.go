package contract

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRepoRoot walks up from startPath until .siding/config.yml (or legacy .siding.yml) is found.
func FindRepoRoot(startPath string) (string, error) {
	start, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}
	if start == "" {
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	dir := start
	for {
		if HasContract(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found at or above %s (run siding init)", FileName, start)
		}
		dir = parent
	}
}
