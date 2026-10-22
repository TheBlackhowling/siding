package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/update"
	"github.com/TheBlackhowling/siding/internal/version"
)

func updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [version]",
		Short: "Download a siding-releases binary onto PATH",
		Long: `Download the archive for this OS/arch from GitHub Releases
(TheBlackhowling/siding-releases), verify checksums.txt, and replace
the installed binary (not a random copy you launched from Downloads).

With no argument, installs the latest stable release. Pass a tag
(v0.1.4) to install that release. Already-current binaries print that
and exit 0 without downloading.

Windows:  %LOCALAPPDATA%\siding\bin\siding.exe
Unix:     ~/.local/bin/siding`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE:         runUpdate,
	}
}

func runUpdate(_ *cobra.Command, args []string) error {
	opt := update.Options{}
	if len(args) == 1 {
		if !update.IsReleaseVersion(args[0]) {
			return fmt.Errorf("update: %q is not a stable vMAJOR.MINOR.PATCH", args[0])
		}
		opt.Version = update.Display(args[0])
	}

	ctx, cancel := context.WithTimeout(context.Background(), update.DownloadTimeout)
	defer cancel()
	rel, err := update.FetchRelease(ctx, opt)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if isAlreadyCurrent(version.Version, rel.Tag, opt.Version) {
		printf("%s %s\n", dim("already current:"), update.Display(rel.Tag))
		return nil
	}

	work, err := os.MkdirTemp("", "siding-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	opt.WorkDir = work

	bin, err := update.DownloadBinary(ctx, opt, rel)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	dest, err := installedDest()
	if err != nil {
		return err
	}
	if err := replaceBinary(bin, dest); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	printf("%s %s\n", green("updated"), update.Display(rel.Tag))
	printf("%s %s\n", dim("installed"), dest)
	printNewBinaryVersion(dest)
	return printInstallPATHHelp(dest, false)
}

func isAlreadyCurrent(running, target, requested string) bool {
	if !update.IsReleaseVersion(running) {
		return false
	}
	if update.Display(running) == update.Display(target) {
		return true
	}
	if requested == "" && !update.Newer(running, target) {
		return true
	}
	return false
}

func printNewBinaryVersion(dest string) {
	ctx, cancel := context.WithTimeout(context.Background(), update.HTTPTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, dest, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	print(string(out))
}
