package proxy

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/state"
)

// BoardHost is the local status page (running stacks + open links).
const BoardHost = "siding.localhost"

// ProxyImage is the published Caddy + board SPA. Tag latest so the UI can
// move without a new CLI version. siding proxy up pulls always.
const ProxyImage = "ghcr.io/theblackhowling/siding-proxy:latest"

type BoardLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type BoardRow struct {
	Project        string      `json:"project"`
	Stack          string      `json:"stack"`
	Mode           string      `json:"mode"`
	Checkout       string      `json:"checkout"`
	Host           string      `json:"host"`
	ComposeProject string      `json:"compose_project"`
	Running        bool        `json:"running"`
	Branch         string      `json:"branch"`
	SHA            string      `json:"sha"`
	LastUpAt       string      `json:"last_up_at"`
	Links          []BoardLink `json:"links"`
}

type BoardStatus struct {
	GeneratedAt string     `json:"generated_at"`
	Listen      int        `json:"listen"`
	Stacks      []BoardRow `json:"stacks"`
}

func lockHeld(f *state.File, project, stack, mode, checkout string) bool {
	if f == nil || f.Locks == nil {
		return false
	}
	entry, ok := f.Locks[state.LockKey(project, stack, mode)]
	if !ok {
		return false
	}
	return paths.NormalizeLockPath(entry.Checkout) == paths.NormalizeLockPath(checkout)
}

// CollectBoard lists every allocated mode, with Caddy links and last-up git.
func CollectBoard(f *state.File) []BoardRow {
	if f == nil {
		return nil
	}
	listen := state.ListenPort(f)
	checkouts := make([]string, 0, len(f.Checkouts))
	for k := range f.Checkouts {
		checkouts = append(checkouts, k)
	}
	sort.Strings(checkouts)
	var rows []BoardRow
	for _, checkout := range checkouts {
		rec := f.Checkouts[checkout]
		modes := make([]string, 0, len(rec.Modes))
		for m := range rec.Modes {
			modes = append(modes, m)
		}
		sort.Strings(modes)
		for _, mode := range modes {
			alloc := rec.Modes[mode]
			if strings.TrimSpace(alloc.Host) == "" && len(alloc.Ports) == 0 {
				continue
			}
			host := ""
			if alloc.Host != "" {
				host = contract.CanonicalHost(alloc.Host)
			}
			row := BoardRow{
				Project:        rec.Project,
				Stack:          rec.Stack,
				Mode:           mode,
				Checkout:       checkout,
				Host:           host,
				ComposeProject: alloc.ComposeProject,
				Running:        lockHeld(f, rec.Project, rec.Stack, mode, checkout),
				Branch:         alloc.GitBranch,
				SHA:            alloc.GitSHA,
				LastUpAt:       alloc.LastUpAt,
				Links:          []BoardLink{},
			}
			if host != "" {
				for _, key := range contract.HTTPEntryKeys {
					if _, ok := alloc.Ports[key]; !ok {
						continue
					}
					for _, label := range boardEntryLabels(alloc, key) {
						epHost := contract.EntryHost(host, label)
						row.Links = append(row.Links, BoardLink{
							Label: label,
							URL:   SiteAddress(epHost, listen),
						})
					}
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// boardEntryLabels is the clickable board hosts for one HTTP port key.
// Recipe entry_points win; otherwise http_hosts extras (app, www) or the key itself (web / api).
func boardEntryLabels(alloc state.ModeAllocation, key string) []string {
	if labels := contract.NormalizeEntryLabels(alloc.EntryPoints[key]); len(labels) > 0 {
		return labels
	}
	if extras := alloc.HTTPHosts[key]; len(extras) > 0 {
		var out []string
		if key == "api" {
			out = append(out, "api")
		}
		for _, label := range extras {
			label = contract.NormalizeHTTPLabel(label)
			if label == "" || label == key {
				continue
			}
			out = append(out, label)
		}
		if len(out) > 0 {
			return contract.NormalizeEntryLabels(out)
		}
	}
	if key == "" {
		return nil
	}
	return []string{key}
}

func GenerateBoardJSON(f *state.File) []byte {
	stacks := CollectBoard(f)
	if stacks == nil {
		stacks = []BoardRow{}
	}
	body, err := json.MarshalIndent(BoardStatus{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Listen:      state.ListenPort(f),
		Stacks:      stacks,
	}, "", "  ")
	if err != nil {
		return []byte("{\n  \"generated_at\": \"\",\n  \"listen\": 80,\n  \"stacks\": []\n}\n")
	}
	return append(body, '\n')
}
