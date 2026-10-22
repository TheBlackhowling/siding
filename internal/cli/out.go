package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/spf13/cobra"
)

// stdOut / stdErr are where command text goes. rootCmd binds them so
// execCLI (and any caller that SetOut/SetErr) captures the same lines a user sees.
var (
	stdOut io.Writer = os.Stdout
	stdErr io.Writer = os.Stderr
)

func bindStdOut(cmd *cobra.Command) {
	stdOut = cmd.OutOrStdout()
	stdErr = cmd.ErrOrStderr()
	enableVirtualTerminal()
}

func resetStdOut() {
	stdOut = os.Stdout
	stdErr = os.Stderr
}

func printf(format string, args ...any) {
	fmt.Fprintf(stdOut, format, args...)
}

func println(args ...any) {
	fmt.Fprintln(stdOut, args...)
}

func print(args ...any) {
	fmt.Fprint(stdOut, args...)
}

func warnf(format string, args ...any) {
	fmt.Fprint(stdErr, yellow(fmt.Sprintf(format, args...)))
}

func encodeJSON(v any) error {
	enc := json.NewEncoder(stdOut)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func flushOut() {
	if f, ok := stdOut.(*os.File); ok {
		_ = f.Sync()
	}
}

// ANSI roles. Codes are omitted when stdout is not a TTY, or when NO_COLOR
// / SIDING_NO_COLOR is set. FORCE_COLOR / SIDING_FORCE_COLOR turns them on
// even when piped (NO_COLOR still wins).
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("SIDING_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("SIDING_FORCE_COLOR") != "" {
		return true
	}
	f, ok := stdOut.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if s == "" || !colorEnabled() {
		return s
	}
	return code + s + ansiReset
}

func bold(s string) string   { return paint(ansiBold, s) }
func dim(s string) string    { return paint(ansiDim, s) }
func red(s string) string    { return paint(ansiRed, s) }
func green(s string) string  { return paint(ansiGreen, s) }
func yellow(s string) string { return paint(ansiYellow, s) }
func cyan(s string) string   { return paint(ansiCyan, s) }

func heading(s string) {
	println(bold(s))
}

var enableVTOnce sync.Once

func enableVirtualTerminal() {
	enableVTOnce.Do(enableConsoleVT)
}
