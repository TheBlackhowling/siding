package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DownloadBinary fetches checksums.txt and the GOOS/GOARCH archive, verifies
// SHA-256, and extracts the siding binary into opt.WorkDir (or a temp dir).
func DownloadBinary(ctx context.Context, opt Options, rel Release) (string, error) {
	goos := opt.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opt.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	name := AssetName(rel.Tag, goos, goarch)
	archive, err := rel.Find(name)
	if err != nil {
		return "", err
	}
	sums, err := rel.Find("checksums.txt")
	if err != nil {
		return "", err
	}

	work := opt.WorkDir
	if work == "" {
		work, err = os.MkdirTemp("", "siding-update-")
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", err
	}

	sumBody, err := getAsset(ctx, opt, sums.URL, "application/octet-stream", 1<<20)
	if err != nil {
		return "", fmt.Errorf("download checksums.txt: %w", err)
	}
	sigAsset, err := rel.Find(checksumSigAsset)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", checksumSigAsset, err)
	}
	sigBody, err := getAsset(ctx, opt, sigAsset.URL, "application/octet-stream", 256)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", checksumSigAsset, err)
	}
	if err := VerifyChecksumsSignature(sumBody, sigBody); err != nil {
		return "", err
	}
	wantHex, err := ParseChecksum(sumBody, name)
	if err != nil {
		return "", err
	}

	archivePath := filepath.Join(work, name)
	body, err := getAsset(ctx, opt, archive.URL, "application/octet-stream", MaxAssetBytes)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	if err := os.WriteFile(archivePath, body, 0o644); err != nil {
		return "", err
	}
	if err := VerifySHA256(archivePath, wantHex); err != nil {
		_ = os.Remove(archivePath)
		return "", err
	}
	return ExtractBinary(archivePath, work, goos)
}
