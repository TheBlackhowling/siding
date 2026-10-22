package main

import (
	"os"
	"testing"
)

func TestMainVersion(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"siding", "version"}
	t.Cleanup(func() { os.Args = oldArgs; exitFunc = os.Exit })
	exitFunc = func(code int) {
		t.Fatalf("version should not exit %d", code)
	}
	main()
}

func TestMainUnknownCommand(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"siding", "not-a-command"}
	t.Cleanup(func() { os.Args = oldArgs; exitFunc = os.Exit })
	var code int
	exitFunc = func(c int) { code = c }
	main()
	if code != 1 {
		t.Fatalf("exit %d want 1", code)
	}
}
