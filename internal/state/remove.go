package state

import (
	"path/filepath"
	"sort"

	"github.com/TheBlackhowling/siding/internal/paths"
)

// FindCheckout walks up from startPath looking for a state.json checkout key.
func FindCheckout(f *File, startPath string) (key string, rec Checkout, ok bool) {
	if f == nil || len(f.Checkouts) == 0 {
		return "", Checkout{}, false
	}
	dir, err := filepath.Abs(startPath)
	if err != nil || dir == "" {
		return "", Checkout{}, false
	}
	for {
		key = paths.NormalizeLockPath(dir)
		if rec, ok = f.Checkouts[key]; ok {
			return key, rec, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", Checkout{}, false
		}
		dir = parent
	}
}

// CheckoutModeNames is recipe modes plus any extra modes recorded on this checkout.
func CheckoutModeNames(recipeModes []string, rec Checkout, only string) []string {
	if only != "" {
		return []string{only}
	}
	seen := map[string]struct{}{}
	var names []string
	for _, n := range recipeModes {
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	var extra []string
	for n := range rec.Modes {
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		extra = append(extra, n)
	}
	sort.Strings(extra)
	return append(names, extra...)
}

func ReleaseLocksForCheckout(f *File, checkoutKey string) int {
	if f == nil || f.Locks == nil {
		return 0
	}
	want := paths.NormalizeLockPath(checkoutKey)
	n := 0
	for k, lock := range f.Locks {
		if paths.NormalizeLockPath(lock.Checkout) == want {
			delete(f.Locks, k)
			n++
		}
	}
	return n
}

// RemoveMode drops one mode allocation. If that was the last mode, the checkout
// row is deleted. Matching locks for that mode (or the whole checkout) go too.
func RemoveMode(f *File, checkoutKey, mode string) bool {
	if f == nil || f.Checkouts == nil || mode == "" {
		return false
	}
	key := paths.NormalizeLockPath(checkoutKey)
	rec, ok := f.Checkouts[key]
	if !ok || rec.Modes == nil {
		return false
	}
	if _, ok := rec.Modes[mode]; !ok {
		return false
	}
	delete(rec.Modes, mode)
	if rec.Project != "" && rec.Stack != "" {
		delete(f.Locks, LockKey(rec.Project, rec.Stack, mode))
	}
	if len(rec.Modes) == 0 {
		delete(f.Checkouts, key)
		ReleaseLocksForCheckout(f, key)
	} else {
		f.Checkouts[key] = rec
	}
	return true
}

// RemoveCheckout deletes the checkout row and every lock that points at it.
func RemoveCheckout(f *File, checkoutKey string) bool {
	if f == nil || f.Checkouts == nil {
		return false
	}
	key := paths.NormalizeLockPath(checkoutKey)
	if _, ok := f.Checkouts[key]; !ok {
		n := ReleaseLocksForCheckout(f, key)
		return n > 0
	}
	delete(f.Checkouts, key)
	ReleaseLocksForCheckout(f, key)
	return true
}
