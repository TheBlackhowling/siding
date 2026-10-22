package advise

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const SharedNetworkName = "siding"

type NetworkService struct {
	Name     string
	Mode     string   // host | none | ""
	Networks []string // empty + no mode → implicit default
}

type NetworkPlan struct {
	Services    []NetworkService
	TopLevel    []string
	Shared      string   // network everyone should use; "default" is the Compose project network
	Isolated    []string // services that cannot reach Shared today
	HostMode    []string
	NeedsEnsure bool
}

// MergeNetworkPlans combines base + override scans so the last file does not
// drop services that only exist in an earlier compose file.
func MergeNetworkPlans(base, over NetworkPlan) NetworkPlan {
	if len(base.Services) == 0 {
		return over
	}
	if len(over.Services) == 0 {
		return base
	}
	byName := map[string]NetworkService{}
	for _, s := range base.Services {
		byName[s.Name] = s
	}
	for _, s := range over.Services {
		byName[s.Name] = s
	}
	top := map[string]struct{}{}
	for _, n := range base.TopLevel {
		top[n] = struct{}{}
	}
	for _, n := range over.TopLevel {
		top[n] = struct{}{}
	}
	plan := NetworkPlan{Shared: "default"}
	for n := range top {
		plan.TopLevel = append(plan.TopLevel, n)
	}
	sort.Strings(plan.TopLevel)
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	count := map[string]int{}
	for _, name := range names {
		ns := byName[name]
		plan.Services = append(plan.Services, ns)
		if ns.Mode == "host" {
			plan.HostMode = append(plan.HostMode, name)
			continue
		}
		for _, n := range networksOf(ns) {
			count[n]++
		}
	}
	if len(count) > 0 {
		plan.Shared = pickSharedNetwork(count)
	}
	for _, ns := range plan.Services {
		if ns.Mode == "host" || ns.Mode == "none" {
			plan.Isolated = append(plan.Isolated, ns.Name)
			continue
		}
		if !containsFold(networksOf(ns), plan.Shared) {
			plan.Isolated = append(plan.Isolated, ns.Name)
		}
	}
	plan.NeedsEnsure = len(plan.Isolated) > 0
	return plan
}

func AnalyzeNetworks(data []byte) NetworkPlan {
	plan := NetworkPlan{Shared: "default"}
	var doc composeDoc
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Services) == 0 {
		return plan
	}
	for name := range doc.Networks {
		plan.TopLevel = append(plan.TopLevel, name)
	}
	sort.Strings(plan.TopLevel)

	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)

	count := map[string]int{}
	for _, name := range names {
		svc := doc.Services[name]
		ns := NetworkService{
			Name:     name,
			Mode:     strings.TrimSpace(strings.ToLower(svc.NetworkMode)),
			Networks: serviceNetworkNames(svc.Networks),
		}
		plan.Services = append(plan.Services, ns)
		if ns.Mode == "host" {
			plan.HostMode = append(plan.HostMode, name)
			continue
		}
		for _, n := range networksOf(ns) {
			count[n]++
		}
	}

	if len(count) > 0 {
		plan.Shared = pickSharedNetwork(count)
	}
	for _, ns := range plan.Services {
		if ns.Mode == "host" || ns.Mode == "none" {
			plan.Isolated = append(plan.Isolated, ns.Name)
			continue
		}
		if !containsFold(networksOf(ns), plan.Shared) {
			plan.Isolated = append(plan.Isolated, ns.Name)
		}
	}
	plan.NeedsEnsure = len(plan.Isolated) > 0
	return plan
}

func networksOf(ns NetworkService) []string {
	if ns.Mode == "host" || ns.Mode == "none" {
		return nil
	}
	if len(ns.Networks) == 0 {
		return []string{"default"}
	}
	return ns.Networks
}

func pickSharedNetwork(count map[string]int) string {
	best := ""
	bestN := -1
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n := count[k]
		if n > bestN || (n == bestN && k == "default") {
			best, bestN = k, n
		}
	}
	if best == "" {
		return "default"
	}
	return best
}

func serviceNetworkNames(raw any) []string {
	if raw == nil {
		return nil
	}
	var names []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				names = append(names, s)
			}
		}
	case map[string]any:
		for k := range v {
			if k != "" {
				names = append(names, k)
			}
		}
		sort.Strings(names)
	}
	return names
}

// EnsureSharedNetwork comments out network_mode: host and attaches isolated
// services to a shared Compose network so frontend can reach backend by DNS.
func EnsureSharedNetwork(body string) (string, []Change) {
	body = normalizeNewlines(body)
	plan := AnalyzeNetworks([]byte(body))
	if !plan.NeedsEnsure {
		return body, nil
	}

	shared := plan.Shared
	createSiding := false
	if shared == "" || (len(plan.Isolated) > 0 && majorityIsSplit(plan)) {
		shared = SharedNetworkName
		createSiding = true
	}
	// host-mode services join default after the mode line is removed
	if shared == "default" && !createSiding {
		return commentHostNetwork(body)
	}

	var changes []Change
	next, hostChanges := commentHostNetwork(body)
	changes = append(changes, hostChanges...)
	body = next
	lines := strings.Split(body, "\n")

	targets := plan.Isolated
	if createSiding {
		targets = make([]string, 0, len(plan.Services))
		for _, ns := range plan.Services {
			targets = append(targets, ns.Name)
		}
	}
	for _, name := range targets {
		var added bool
		lines, added = attachServiceNetwork(lines, name, shared)
		if added {
			changes = append(changes, Change{
				Reason: "attach service to the shared Compose network",
				Before: name + " was isolated from " + shared,
				After:  name + " joins " + shared,
			})
		}
	}
	if createSiding || (shared != "default" && !containsFold(plan.TopLevel, shared)) {
		var added bool
		lines, added = ensureTopLevelNetwork(lines, shared)
		if added {
			changes = append(changes, Change{
				Reason: "declare shared Compose network",
				Before: "no top-level network " + shared,
				After:  "networks." + shared,
			})
		}
	}
	return strings.Join(lines, "\n"), changes
}

func majorityIsSplit(plan NetworkPlan) bool {
	if plan.Shared == "default" {
		return false
	}
	onShared := 0
	for _, ns := range plan.Services {
		if containsFold(networksOf(ns), plan.Shared) {
			onShared++
		}
	}
	return onShared <= len(plan.Services)/2
}

func commentHostNetwork(body string) (string, []Change) {
	lines := strings.Split(body, "\n")
	var changes []Change
	for i, line := range lines {
		if !networkModeLine.MatchString(line) {
			continue
		}
		after := indentOf(line) + "# " + strings.TrimSpace(line) + "  # siding: use the Compose project network"
		changes = append(changes, Change{
			Reason: "network_mode: host blocks Compose DNS",
			Before: strings.TrimSpace(line),
			After:  strings.TrimSpace(after),
		})
		lines[i] = after
	}
	if len(changes) == 0 {
		return body, nil
	}
	return strings.Join(lines, "\n"), changes
}

func attachServiceNetwork(lines []string, service, network string) ([]string, bool) {
	start, end, ok := serviceRange(lines, service)
	if !ok {
		return lines, false
	}
	netIdx := childKeyIndex(lines, start+1, end, "networks")
	if netIdx < 0 {
		insert := []string{
			"    networks:",
			"      - " + network,
		}
		return insertLines(lines, end, insert), true
	}
	netEnd := childBlockEnd(lines, netIdx, end)
	if networkListed(lines[netIdx:netEnd], network) {
		return lines, false
	}
	style := networkBlockStyle(lines[netIdx:netEnd])
	var extra string
	if style == "map" {
		extra = "      " + network + ":"
	} else {
		extra = "      - " + network
	}
	return insertLines(lines, netEnd, []string{extra}), true
}

func ensureTopLevelNetwork(lines []string, network string) ([]string, bool) {
	for i, line := range lines {
		if !strings.HasPrefix(line, "networks:") {
			continue
		}
		end := topLevelBlockEnd(lines, i)
		if networkListed(lines[i:end], network) {
			return lines, false
		}
		return insertLines(lines, end, []string{"  " + network + ":"}), true
	}
	extra := []string{"", "networks:", "  " + network + ":"}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		extra = extra[1:]
	}
	return append(lines, extra...), true
}

func serviceRange(lines []string, name string) (start, end int, ok bool) {
	want := "  " + name + ":"
	for i, line := range lines {
		if strings.TrimRight(line, " \t\r") != want {
			continue
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if serviceHeader.MatchString(lines[j]) || topLevelKey(lines[j]) {
				end = j
				break
			}
		}
		return i, end, true
	}
	return 0, 0, false
}

func childKeyIndex(lines []string, start, end int, key string) int {
	prefix := "    " + key + ":"
	for i := start; i < end; i++ {
		if strings.HasPrefix(strings.TrimRight(lines[i], " \t"), prefix) {
			return i
		}
	}
	return -1
}

func childBlockEnd(lines []string, keyIdx, serviceEnd int) int {
	for i := keyIdx + 1; i < serviceEnd; i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if indentWidth(lines[i]) <= indentWidth(lines[keyIdx]) {
			return i
		}
	}
	return serviceEnd
}

func topLevelBlockEnd(lines []string, keyIdx int) int {
	for i := keyIdx + 1; i < len(lines); i++ {
		if topLevelKey(lines[i]) {
			return i
		}
	}
	return len(lines)
}

func topLevelKey(line string) bool {
	if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	return strings.Contains(line, ":")
}

func networkListed(block []string, network string) bool {
	for _, line := range block {
		trim := strings.TrimSpace(line)
		if trim == network+":" || trim == "- "+network || strings.HasPrefix(trim, "- "+network+" ") {
			return true
		}
	}
	return false
}

func networkBlockStyle(block []string) string {
	for _, line := range block[1:] {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			return "list"
		}
		return "map"
	}
	return "list"
}

func insertLines(lines []string, at int, extra []string) []string {
	out := make([]string, 0, len(lines)+len(extra))
	out = append(out, lines[:at]...)
	out = append(out, extra...)
	out = append(out, lines[at:]...)
	return out
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func indentWidth(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' {
			n++
			continue
		}
		if r == '\t' {
			n += 2
			continue
		}
		break
	}
	return n
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func FormatNetworkStatus(plan NetworkPlan) []string {
	if len(plan.Services) < 2 && len(plan.HostMode) == 0 {
		return []string{"single service — Compose default network is enough"}
	}
	if !plan.NeedsEnsure {
		return []string{fmt.Sprintf("services share the %q network (frontend reaches backend as http://<service>:<container-port>)", plan.Shared)}
	}
	var out []string
	if len(plan.HostMode) > 0 {
		out = append(out, "network_mode: host on "+strings.Join(plan.HostMode, ", ")+" — containers cannot use Compose DNS")
	}
	if len(plan.Isolated) > 0 {
		out = append(out, "isolated from "+plan.Shared+": "+strings.Join(plan.Isolated, ", "))
	}
	return out
}
