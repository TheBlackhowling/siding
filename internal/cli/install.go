package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/update"
)

// addUserPath persists the bin dir on the Windows user PATH. Tests replace it
// so `siding install` never touches the real registry.
var addUserPath = addWindowsUserPath

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Copy this binary onto your PATH so PowerShell can run `siding`",
		Long: `Install the current executable as "siding" in a user bin directory
and, on Windows, add that directory to your user PATH.

PowerShell does not search the current folder. After install, prepend the
bin dir to $env:Path in this session (or start a new Windows Terminal /
cmd window — IDE terminals keep the IDE environment until the IDE restarts):

  siding version

Windows:  %LOCALAPPDATA%\siding\bin\siding.exe
Unix:     ~/.local/bin/siding`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dest, err := installSelf()
			if err != nil {
				return err
			}
			printf("%s %s\n", green("installed"), dest)
			return printInstallPATHHelp(dest, true)
		},
	}
}

func installedDest() (string, error) {
	dir, err := paths.CommandBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, update.BinaryName(runtime.GOOS)), nil
}

func installSelf() (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", err
	}
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return "", err
	}
	dest, err := installedDest()
	if err != nil {
		return "", err
	}
	if sameFile(src, dest) {
		return dest, nil
	}
	if err := replaceBinary(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func replaceBinary(src, dest string) error {
	tmp := dest + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	old := dest + ".old"
	if _, err := os.Stat(dest); err == nil {
		_ = os.Remove(old)
		if err := os.Rename(dest, old); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("replace running binary: %w", err)
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Rename(old, dest)
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(old)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dest, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func printInstallPATHHelp(dest string, force bool) error {
	if runtime.GOOS == "windows" {
		if err := addUserPath(filepath.Dir(dest)); err != nil {
			return fmt.Errorf("could not update user PATH: %w", err)
		}
		printf("%s %s\n", dim("user PATH:"), green("updated"))
		if !force && dirOnPATH(filepath.Dir(dest)) {
			return nil
		}
		println("new Windows Terminal / cmd windows see `siding`.")
		println(dim("IDE terminals keep the IDE's environment until the IDE restarts."))
		println()
		println(bold("this PowerShell session still needs:"))
		printf("  %s\n", cyan(fmt.Sprintf("$env:Path = %q + ';' + $env:Path", filepath.Dir(dest))))
		printf("  %s\n", cyan("siding version"))
		println()
		println(dim("or run  .\\scripts\\install.ps1  which does that for you."))
		return nil
	}
	if force || !dirOnPATH(filepath.Dir(dest)) {
		println(dim("ensure the directory is on PATH (often true for ~/.local/bin)."))
	}
	return nil
}

func dirOnPATH(dir string) bool {
	want, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	want = filepath.Clean(want)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		got, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if strings.EqualFold(filepath.Clean(got), want) {
			return true
		}
	}
	return false
}

func sameFile(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
