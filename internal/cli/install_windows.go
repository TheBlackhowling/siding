//go:build windows

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// userEnvKeyPath is the HKCU key holding the user PATH. Tests point it at a
// scratch key so they never edit the real environment.
var userEnvKeyPath = `Environment`

func addWindowsUserPath(dir string) error {
	dir = filepath.Clean(dir)
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	raw, typ, err := readUnexpanded(k, "Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	for _, p := range strings.Split(raw, ";") {
		p = strings.TrimSpace(p)
		if p != "" && strings.EqualFold(filepath.Clean(p), dir) {
			return broadcastEnv()
		}
	}
	next := dir
	if raw != "" {
		next = dir + ";" + raw
	}
	if typ == registry.EXPAND_SZ || strings.Contains(next, "%") {
		if err := k.SetExpandStringValue("Path", next); err != nil {
			return err
		}
	} else {
		if err := k.SetStringValue("Path", next); err != nil {
			return err
		}
	}
	return broadcastEnv()
}

func readUnexpanded(k registry.Key, name string) (string, uint32, error) {
	n, typ, err := k.GetValue(name, nil)
	if err != nil {
		return "", 0, err
	}
	if n == 0 {
		return "", typ, nil
	}
	buf := make([]byte, n)
	n, typ, err = k.GetValue(name, buf)
	if err != nil {
		return "", typ, err
	}
	buf = buf[:n]
	if typ == registry.SZ || typ == registry.EXPAND_SZ {
		u16 := make([]uint16, len(buf)/2)
		for i := range u16 {
			u16[i] = uint16(buf[i*2]) | uint16(buf[i*2+1])<<8
		}
		return windows.UTF16ToString(u16), typ, nil
	}
	return string(buf), typ, nil
}

const (
	hwndBroadcast   = 0xffff
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var sendMessageTimeout = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW")

func broadcastEnv() error {
	env, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	var n uintptr
	_, _, _ = sendMessageTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(env)),
		smtoAbortIfHung,
		5000,
		uintptr(unsafe.Pointer(&n)),
	)
	return nil
}
