package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPaintOffByDefaultOnBuffer(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("SIDING_NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("SIDING_FORCE_COLOR", "")
	old := stdOut
	t.Cleanup(func() { stdOut = old })
	stdOut = &bytes.Buffer{}
	if bold("x") != "x" || green("ok") != "ok" || cyan("http://x") != "http://x" {
		t.Fatalf("buffer output must stay plain: %q %q", bold("x"), green("ok"))
	}
}

func TestPaintNoColorWins(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	if yellow("warn") != "warn" {
		t.Fatalf("NO_COLOR must win: %q", yellow("warn"))
	}
}

func TestPaintForceColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("SIDING_NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	got := red("FAIL")
	if got == "FAIL" || !strings.Contains(got, "FAIL") || !strings.HasPrefix(got, ansiRed) {
		t.Fatalf("FORCE_COLOR should wrap: %q", got)
	}
	if !strings.HasSuffix(got, ansiReset) {
		t.Fatalf("missing reset: %q", got)
	}
}
