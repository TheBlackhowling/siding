package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	EnvRoot = "SIDING_ROOT" // state file or directory override
	EnvData = "SIDING_DATA" // user data root override
)

// Paths locates siding machine state on the host.
type Paths struct {
	UserDataRoot string
	StateFile    string
	ProxyDir     string
}

// ResolvePaths returns paths to state.json and the shared proxy directory.
//
// Default layout:
//
//	{UserDataRoot}/state.json
//	{UserDataRoot}/siding-proxy/
//
// SIDING_ROOT / --root may be a directory (…/state.json inside) or a direct path to state.json.
func ResolvePaths(stateOverride string) (Paths, error) {
	if dir, ok, err := explicitStateOverride(stateOverride); ok {
		if err != nil {
			return Paths{}, err
		}
		return pathsFromStateLocation(dir)
	}

	userData, err := UserDataRoot("")
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		UserDataRoot: userData,
		StateFile:    filepath.Join(userData, "state.json"),
		ProxyDir:     filepath.Join(userData, "siding-proxy"),
	}, nil
}

func explicitStateOverride(stateOverride string) (string, bool, error) {
	if stateOverride != "" {
		path, err := Abs(stateOverride)
		return path, true, err
	}
	if env := strings.TrimSpace(os.Getenv(EnvRoot)); env != "" {
		path, err := Abs(env)
		return path, true, err
	}
	return "", false, nil
}

func pathsFromStateLocation(location string) (Paths, error) {
	if strings.HasSuffix(strings.ToLower(location), ".json") {
		return Paths{
			UserDataRoot: filepath.Dir(location),
			StateFile:    location,
			ProxyDir:     filepath.Join(filepath.Dir(location), "siding-proxy"),
		}, nil
	}
	return Paths{
		UserDataRoot: location,
		StateFile:    filepath.Join(location, "state.json"),
		ProxyDir:     filepath.Join(location, "siding-proxy"),
	}, nil
}

// CommandBinDir is where `siding install` places the binary so shells can find it.
// Windows: {UserDataRoot}/bin   Unix: ~/.local/bin
func CommandBinDir() (string, error) {
	return commandBinDirFor(runtime.GOOS)
}

func commandBinDirFor(goos string) (string, error) {
	if goos == "windows" {
		root, err := UserDataRoot("")
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "bin"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func UserDataRoot(override string) (string, error) {
	if override != "" {
		return Abs(override)
	}
	if env := strings.TrimSpace(os.Getenv(EnvData)); env != "" {
		return Abs(env)
	}
	return userDataRootFor(runtime.GOOS)
}

func userDataRootFor(goos string) (string, error) {
	switch goos {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(base, "siding"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "siding"), nil
	default:
		if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
			return filepath.Join(xdg, "siding"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "siding"), nil
	}
}
