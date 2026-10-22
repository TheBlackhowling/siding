package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TheBlackhowling/siding/internal/state"
)

func TestCollectBoardLinksAndLock(t *testing.T) {
	f := sampleState()
	state.ClaimLock(f, "acme", "betty", "test", "c:/source/acme-betty", "acme-betty-test")
	rows := CollectBoard(f)
	var betty, hello *BoardRow
	for i := range rows {
		switch rows[i].Stack {
		case "betty":
			betty = &rows[i]
		case "hello":
			hello = &rows[i]
		}
	}
	if betty == nil || hello == nil {
		t.Fatalf("rows=%#v", rows)
	}
	if !betty.Running {
		t.Fatal("betty should be up (lock held)")
	}
	if hello.Running {
		t.Fatal("hello should not be up")
	}
	got := map[string]string{}
	for _, l := range betty.Links {
		got[l.Label] = l.URL
	}
	if got["web"] != "http://betty.acme.test.localhost" || got["api"] != "http://api.betty.acme.test.localhost" {
		t.Fatalf("links=%#v", betty.Links)
	}
	payload := decodeBoardJSON(t, GenerateBoardJSON(f))
	if payload.Listen != 80 {
		t.Fatalf("listen=%d", payload.Listen)
	}
	if payload.GeneratedAt == "" {
		t.Fatal("generated_at")
	}
	var foundUp bool
	for _, row := range payload.Stacks {
		if row.Stack == "betty" && row.Running {
			foundUp = true
		}
	}
	if !foundUp {
		t.Fatalf("betty not up in json: %#v", payload.Stacks)
	}
}

func TestCollectBoardEntryPoints(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/acme-genie", "acme-genie", "genie", "test", state.ModeAllocation{
		Host:        "acme.genie.test",
		Ports:       map[string]int{"web": 3000, "api": 8000},
		HTTPHosts:   map[string][]string{"web": {"app"}},
		EntryPoints: map[string][]string{"web": {"app"}, "api": {"api"}},
	})
	rows := CollectBoard(f)
	if len(rows) != 1 {
		t.Fatalf("rows=%#v", rows)
	}
	got := map[string]string{}
	for _, l := range rows[0].Links {
		got[l.Label] = l.URL
	}
	if _, ok := got["web"]; ok {
		t.Fatalf("board should link app, not apex web: %#v", rows[0].Links)
	}
	if got["app"] != "http://app.acme.genie.test.localhost" {
		t.Fatalf("app=%q links=%#v", got["app"], rows[0].Links)
	}
	if got["api"] != "http://api.acme.genie.test.localhost" {
		t.Fatalf("api=%q links=%#v", got["api"], rows[0].Links)
	}
	raw := string(GenerateBoardJSON(f))
	if !strings.Contains(raw, `"url": "http://app.acme.genie.test.localhost"`) {
		t.Fatalf("app link missing: %s", raw)
	}
	if !strings.Contains(raw, `"url": "http://api.acme.genie.test.localhost"`) {
		t.Fatalf("api link missing: %s", raw)
	}
	if strings.Contains(raw, `"label": "web"`) {
		t.Fatalf("must not link apex web: %s", raw)
	}
}

func TestCollectBoardHTTPHostsFallback(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, "c:/source/acme-genie", "acme-genie", "genie", "test", state.ModeAllocation{
		Host:      "acme.genie.test",
		Ports:     map[string]int{"web": 3000, "api": 8000},
		HTTPHosts: map[string][]string{"web": {"app"}},
	})
	got := map[string]string{}
	for _, l := range CollectBoard(f)[0].Links {
		got[l.Label] = l.URL
	}
	if _, ok := got["web"]; ok {
		t.Fatalf("http_hosts.web=app should not also link the apex: %#v", got)
	}
	if got["app"] != "http://app.acme.genie.test.localhost" || got["api"] != "http://api.acme.genie.test.localhost" {
		t.Fatalf("got %#v", got)
	}
}

func TestGenerateBoardJSONEscapes(t *testing.T) {
	f := state.NewFile()
	state.PutCheckoutMode(f, `c:/source/<script>alert("x")</script>`, "hello", "hello", "test", state.ModeAllocation{
		Host:  `evil<script>.test`,
		Ports: map[string]int{"web": 18080},
	})
	raw := string(GenerateBoardJSON(f))
	if strings.Contains(raw, "<script>") {
		t.Fatalf("json must escape checkout/host: %s", raw)
	}
	if !strings.Contains(raw, `\u003cscript\u003e`) {
		t.Fatalf("expected unicode-escaped script: %s", raw)
	}
	payload := decodeBoardJSON(t, []byte(raw))
	if len(payload.Stacks) != 1 {
		t.Fatalf("stacks=%#v", payload.Stacks)
	}
	if !strings.Contains(payload.Stacks[0].Checkout, "<script>") {
		t.Fatalf("decoded checkout=%q", payload.Stacks[0].Checkout)
	}
}

func TestGenerateBoardJSONEmpty(t *testing.T) {
	raw := GenerateBoardJSON(nil)
	payload := decodeBoardJSON(t, raw)
	if payload.Stacks == nil || len(payload.Stacks) != 0 {
		t.Fatalf("empty stacks must be [] not null: %#v", payload.Stacks)
	}
	raw = GenerateBoardJSON(state.NewFile())
	payload = decodeBoardJSON(t, raw)
	if payload.Stacks == nil {
		t.Fatal("stacks null")
	}
}

func decodeBoardJSON(t *testing.T, raw []byte) BoardStatus {
	t.Helper()
	var payload BoardStatus
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	return payload
}
