package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractBinary(archivePath, destDir, goos string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	want := BinaryName(goos)
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, destDir, want)
	case strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz"):
		return extractTarGz(archivePath, destDir, want)
	default:
		return "", fmt.Errorf("unsupported archive %s", filepath.Base(archivePath))
	}
}

func safeArchiveName(name, want string) bool {
	clean := filepath.ToSlash(name)
	if clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
		return false
	}
	return filepath.Base(clean) == want
}

func extractZip(archivePath, destDir, want string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !safeArchiveName(f.Name, want) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		dest := filepath.Join(destDir, want)
		err = writeExtracted(dest, rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("archive %s has no %s", filepath.Base(archivePath), want)
}

func extractTarGz(archivePath, destDir, want string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg || !safeArchiveName(hdr.Name, want) {
			continue
		}
		dest := filepath.Join(destDir, want)
		if err := writeExtracted(dest, tr); err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("archive %s has no %s", filepath.Base(archivePath), want)
}

func writeExtracted(dest string, r io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(r, MaxAssetBytes+1))
	if err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if n > MaxAssetBytes {
		_ = os.Remove(dest)
		return fmt.Errorf("archive entry exceeds %d bytes", MaxAssetBytes)
	}
	return nil
}
