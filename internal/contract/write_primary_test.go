package contract

import "testing"

func TestRenderPrimaryBranch(t *testing.T) {
	text := Render(&Contract{
		Project:       "hello",
		PrimaryBranch: "main",
		Modes: map[string]Mode{
			"test": {ComposeFiles: []string{"docker-compose.yml"}},
		},
	})
	if !containsLine(text, "primary_branch: main") {
		t.Fatalf("missing primary_branch:\n%s", text)
	}
}

func containsLine(text, line string) bool {
	for _, row := range splitLines(text) {
		if row == line {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, text[start:i])
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}
