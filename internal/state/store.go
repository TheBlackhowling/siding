package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheBlackhowling/siding/internal/paths"
)

type Store struct {
	StateFile string
	ProxyDir  string
}

func NewStore(p paths.Paths) *Store {
	return &Store{
		StateFile: p.StateFile,
		ProxyDir:  p.ProxyDir,
	}
}

func (s *Store) Read() (*File, error) {
	data, err := os.ReadFile(s.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return NewFile(), nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.StateFile, err)
	}
	if f.Checkouts == nil {
		f.Checkouts = map[string]Checkout{}
	}
	if f.Locks == nil {
		f.Locks = map[string]Lock{}
	}
	if f.Version == 0 {
		f.Version = 1
	}
	MigrateOffDefaultPorts(&f)
	return &f, nil
}

func (s *Store) Write(f *File) error {
	if err := os.MkdirAll(filepath.Dir(s.StateFile), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(s.StateFile, data)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err == nil {
		return nil
	}
	bak := path + ".bak"
	_ = os.Remove(bak)
	if err := os.Rename(path, bak); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(bak, path)
		_ = os.Remove(tmpName)
		return err
	}
	_ = os.Remove(bak)
	return nil
}

func (s *Store) ModeAllocation(f *File, checkoutKey, mode string) (ModeAllocation, bool) {
	rec, ok := f.Checkouts[checkoutKey]
	if !ok {
		return ModeAllocation{}, false
	}
	m, ok := rec.Modes[mode]
	return m, ok
}

func CollectUsedPorts(f *File) map[int]struct{} {
	used := map[int]struct{}{}
	for _, checkout := range f.Checkouts {
		for _, mode := range checkout.Modes {
			for _, port := range mode.Ports {
				used[port] = struct{}{}
			}
		}
	}
	for _, p := range ReservedProxyPorts(f) {
		if p > 0 {
			used[p] = struct{}{}
		}
	}
	return used
}

// ReservedProxyPorts are host binds owned by siding-proxy-caddy, not stacks.
func ReservedProxyPorts(f *File) []int {
	seen := map[int]struct{}{
		ProxyHTTPListen:  {},
		ProxyAdminListen: {},
	}
	if listen := ListenPort(f); listen > 0 {
		seen[listen] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

func AssertLock(f *File, project, stack, mode, repoRoot string) error {
	key := LockKey(project, stack, mode)
	entry, ok := f.Locks[key]
	if !ok {
		return nil
	}
	if paths.NormalizeLockPath(entry.Checkout) != paths.NormalizeLockPath(repoRoot) {
		return fmt.Errorf("lock %s held by %s (project %s)", key, entry.Checkout, entry.ComposeProject)
	}
	return nil
}

func ClaimLock(f *File, project, stack, mode, repoRoot, composeProject string) (bool, error) {
	key := LockKey(project, stack, mode)
	newly := f.Locks[key].Checkout == ""
	if err := AssertLock(f, project, stack, mode, repoRoot); err != nil {
		return false, err
	}
	f.Locks[key] = Lock{
		Checkout:       paths.NormalizeLockPath(repoRoot),
		ComposeProject: composeProject,
	}
	return newly, nil
}

func ReleaseLock(f *File, project, stack, mode, repoRoot string) {
	key := LockKey(project, stack, mode)
	entry, ok := f.Locks[key]
	if !ok {
		return
	}
	if paths.NormalizeLockPath(entry.Checkout) == paths.NormalizeLockPath(repoRoot) {
		delete(f.Locks, key)
	}
}
