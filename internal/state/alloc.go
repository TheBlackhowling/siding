package state

import (
	"fmt"
	"sort"
)

// PortStride is the offset applied to a claimed host port when that number is
// already taken (state.json, reserved proxy, or a running Docker publish).
const PortStride = 10

const maxPortSlots = 100

// PutCheckoutMode writes (or replaces) one mode allocation for a checkout.
func PutCheckoutMode(f *File, checkoutKey, project, stack, mode string, alloc ModeAllocation) {
	rec, ok := f.Checkouts[checkoutKey]
	if !ok {
		rec = Checkout{Modes: map[string]ModeAllocation{}}
	}
	rec.Project = project
	rec.Stack = stack
	if rec.Modes == nil {
		rec.Modes = map[string]ModeAllocation{}
	}
	rec.Modes[mode] = alloc
	f.Checkouts[checkoutKey] = rec
}

// AllocatePorts claims each key independently so a duplicate compose default
// (two services on 4443) or one busy bind cannot fail the whole set.
func AllocatePorts(defaults map[string]int, used map[int]struct{}) (map[string]int, error) {
	if len(defaults) == 0 {
		return map[string]int{}, nil
	}
	busy := map[int]struct{}{}
	for p := range used {
		busy[p] = struct{}{}
	}
	out := make(map[string]int, len(defaults))
	keys := make([]string, 0, len(defaults))
	for k := range defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p, err := allocateOne(defaults[k], busy)
		if err != nil {
			return nil, fmt.Errorf("port %s: %w", k, err)
		}
		out[k] = p
		busy[p] = struct{}{}
	}
	return out, nil
}

type ReconcileResult struct {
	Alloc      ModeAllocation
	Created    bool
	AddedPorts []string
}

// EnsureModeAllocation records a free port block when this checkout/mode has
// none yet. If an allocation exists, new recipe port keys are merged in and
// host/compose_project are refreshed from the current org recipe.
func EnsureModeAllocation(f *File, checkoutKey, project, stack, mode string, seed ModeAllocation, live map[int]struct{}) (ModeAllocation, bool, error) {
	res, err := ReconcileModeAllocation(f, checkoutKey, project, stack, mode, seed, live)
	if err != nil {
		return ModeAllocation{}, false, err
	}
	return res.Alloc, res.Created, nil
}

// ReconcileModeAllocation maps an org recipe seed onto this checkout's local
// allocation. Existing port numbers stay put; missing keys are claimed uniquely;
// host and compose project always follow the current recipe patterns.
//
// New claims use PreferHostPort so compose defaults (3000, 5432, …) are not the
// first try. live is extra busy host ports (running Docker publishes).
func ReconcileModeAllocation(f *File, checkoutKey, project, stack, mode string, seed ModeAllocation, live map[int]struct{}) (ReconcileResult, error) {
	existing, has := ModeAllocation{}, false
	if rec, ok := f.Checkouts[checkoutKey]; ok {
		if alloc, ok := rec.Modes[mode]; ok && len(alloc.Ports) > 0 {
			existing, has = alloc, true
		}
	}

	used := CollectUsedPorts(f)
	for p := range live {
		if p > 0 {
			used[p] = struct{}{}
		}
	}
	if has {
		existing.Ports = CollapsePortKeys(existing.Ports)
		for _, p := range existing.Ports {
			delete(used, p)
		}
	}

	wanted := LiftCommonPorts(CollapsePortKeys(seed.Ports))

	var (
		ports map[string]int
		added []string
		err   error
	)
	if !has {
		ports, err = AllocatePorts(wanted, used)
		if err != nil {
			return ReconcileResult{}, err
		}
	} else {
		ports, added, err = mergePorts(existing.Ports, wanted, used)
		if err != nil {
			return ReconcileResult{}, err
		}
	}

	host := seed.Host
	if host == "" {
		host = existing.Host
	}
	composeProject := seed.ComposeProject
	if composeProject == "" {
		composeProject = existing.ComposeProject
	}

	alloc := ModeAllocation{
		Host:           host,
		ComposeProject: composeProject,
		Ports:          ports,
		GitBranch:      existing.GitBranch,
		GitSHA:         existing.GitSHA,
		LastUpAt:       existing.LastUpAt,
		HTTPHosts:      seed.HTTPHosts,
		EntryPoints:    seed.EntryPoints,
	}
	PutCheckoutMode(f, checkoutKey, project, stack, mode, alloc)
	return ReconcileResult{Alloc: alloc, Created: !has, AddedPorts: added}, nil
}

// StampLastUp records git HEAD at a successful siding up (guess for the board).
func StampLastUp(f *File, checkoutKey, mode, branch, sha, at string) {
	if f == nil || checkoutKey == "" || mode == "" {
		return
	}
	rec, ok := f.Checkouts[checkoutKey]
	if !ok || rec.Modes == nil {
		return
	}
	alloc, ok := rec.Modes[mode]
	if !ok {
		return
	}
	alloc.GitBranch = branch
	alloc.GitSHA = sha
	alloc.LastUpAt = at
	rec.Modes[mode] = alloc
	f.Checkouts[checkoutKey] = rec
}

func mergePorts(existing, defaults map[string]int, used map[int]struct{}) (map[string]int, []string, error) {
	out := make(map[string]int, len(existing)+len(defaults))
	for k, v := range existing {
		out[k] = v
		if used != nil {
			used[v] = struct{}{}
		}
	}
	var added []string
	keys := make([]string, 0, len(defaults))
	for k := range defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := out[k]; ok {
			continue
		}
		p, err := allocateOne(defaults[k], used)
		if err != nil {
			return nil, nil, fmt.Errorf("port %s: %w", k, err)
		}
		out[k] = p
		if used != nil {
			used[p] = struct{}{}
		}
		added = append(added, k)
	}
	return out, added, nil
}

func allocateOne(want int, used map[int]struct{}) (int, error) {
	if used == nil {
		used = map[int]struct{}{}
	}
	for slot := 0; slot <= maxPortSlots; slot++ {
		p := want + PortStride*slot
		if p < 1 || p > 65535 {
			continue
		}
		if _, taken := used[p]; !taken {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port after %d strides of %d (start %d)", maxPortSlots, PortStride, want)
}
