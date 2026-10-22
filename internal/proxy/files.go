package proxy

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/TheBlackhowling/siding/internal/state"
)

const (
	ContainerName   = "siding-proxy-caddy"
	CaddyfileName   = "Caddyfile"
	ComposeFileName = "docker-compose.yml"
	StatusFileName  = "status.json"
)

type Config struct {
	ProxyDir         string
	CaddyfilePath    string
	ComposePath      string
	Listen           int
	Routes           []Route
	SkippedPublishes []int
}

func WriteFiles(proxyDir string, st *state.File) (Config, error) {
	return WriteFilesSkip(proxyDir, st, nil)
}

func WriteFilesSkip(proxyDir string, st *state.File, skipPublish map[int]struct{}) (Config, error) {
	listen := state.ListenPort(st)
	routes := CollectRoutes(st)
	if err := os.MkdirAll(proxyDir, 0o755); err != nil {
		return Config{}, err
	}
	cfg := Config{
		ProxyDir:         proxyDir,
		CaddyfilePath:    filepath.Join(proxyDir, CaddyfileName),
		ComposePath:      filepath.Join(proxyDir, ComposeFileName),
		Listen:           listen,
		Routes:           routes,
		SkippedPublishes: skipKeys(skipPublish),
	}
	if err := os.WriteFile(cfg.CaddyfilePath, []byte(GenerateCaddyfile(routes, listen)), 0o644); err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(cfg.ComposePath, []byte(GenerateComposeSkip(listen, skipPublish)), 0o644); err != nil {
		return Config{}, err
	}
	boardDir := filepath.Join(proxyDir, "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(filepath.Join(boardDir, StatusFileName), GenerateBoardJSON(st), 0o644); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func WriteStatus(proxyDir string, listen int) error {
	if err := os.MkdirAll(proxyDir, 0o755); err != nil {
		return err
	}
	body := []byte("{\n  \"running\": true,\n  \"listen\": " + strconv.Itoa(listen) + "\n}\n")
	return os.WriteFile(filepath.Join(proxyDir, StatusFileName), body, 0o644)
}

func skipKeys(skip map[int]struct{}) []int {
	if len(skip) == 0 {
		return nil
	}
	keys := make([]int, 0, len(skip))
	for p := range skip {
		keys = append(keys, p)
	}
	sort.Ints(keys)
	return keys
}

func ClearStatus(proxyDir string) {
	_ = os.Remove(filepath.Join(proxyDir, StatusFileName))
}
