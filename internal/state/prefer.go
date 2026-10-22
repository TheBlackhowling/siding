package state

import (
	"sort"
	"strings"
)

// PortLift is added to compose/recipe defaults that sit on well-known ports
// (3000, 5432, 8080, …) so the first claim is less likely to hit whatever is
// already running on this machine outside state.json.
const PortLift = 15000

const commonPortCeiling = 10000

var extraCommonPorts = map[int]struct{}{
	9200:  {},
	9300:  {},
	11211: {},
	15672: {},
	27017: {},
	27018: {},
	50051: {},
}

// IsCommonDefault is a port apps and databases conventionally publish.
func IsCommonDefault(p int) bool {
	if p < 1 || p > 65535 {
		return false
	}
	if p < commonPortCeiling {
		return true
	}
	_, ok := extraCommonPorts[p]
	return ok
}

// PreferHostPort maps a compose/recipe default onto siding's preferred first
// claim. High ports already in the siding band are left alone.
func PreferHostPort(p int) int {
	if !IsCommonDefault(p) {
		return p
	}
	lifted := p + PortLift
	if lifted < 1 || lifted > 65535 {
		return p
	}
	return lifted
}

// LiftCommonPorts returns a copy with PreferHostPort applied to every value.
func LiftCommonPorts(ports map[string]int) map[string]int {
	out := make(map[string]int, len(ports))
	for k, v := range ports {
		out[k] = PreferHostPort(v)
	}
	return out
}

// NormalizePortKey folds hyphen/underscore so gcs-emulator and gcs_emulator
// are the same allocation key.
func NormalizePortKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	return strings.ReplaceAll(key, "-", "_")
}

// CollapsePortKeys keeps one entry per normalized key (sorted original name wins).
func CollapsePortKeys(ports map[string]int) map[string]int {
	if len(ports) == 0 {
		return map[string]int{}
	}
	keys := make([]string, 0, len(ports))
	for k := range ports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]int, len(ports))
	for _, k := range keys {
		nk := NormalizePortKey(k)
		if nk == "" {
			continue
		}
		if _, ok := out[nk]; ok {
			continue
		}
		out[nk] = ports[k]
	}
	return out
}
