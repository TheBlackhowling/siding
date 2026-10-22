//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableConsoleVT() {
	enableFileVT(os.Stdout)
	enableFileVT(os.Stderr)
}

func enableFileVT(f *os.File) {
	var mode uint32
	h := windows.Handle(f.Fd())
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return
	}
	_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
