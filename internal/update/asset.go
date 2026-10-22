package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func VersionNumber(v string) string {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	return s
}

func ArchiveExt(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func BinaryName(goos string) string {
	if goos == "windows" {
		return "siding.exe"
	}
	return "siding"
}

// AssetName is the GoReleaser archive for a GOOS/GOARCH, e.g.
// siding_0.1.3_windows_amd64.zip.
func AssetName(version, goos, goarch string) string {
	return "siding_" + VersionNumber(version) + "_" + goos + "_" + goarch + ArchiveExt(goos)
}

func ParseChecksum(checksums []byte, filename string) (string, error) {
	want := strings.TrimSpace(filename)
	for _, line := range strings.Split(string(checksums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, name, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, "*")
		if name != want {
			continue
		}
		sum = strings.ToLower(strings.TrimSpace(sum))
		if len(sum) != 64 {
			return "", fmt.Errorf("checksums.txt: bad sha256 for %s", filename)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf("checksums.txt: bad sha256 for %s", filename)
		}
		return sum, nil
	}
	return "", fmt.Errorf("checksums.txt: no entry for %s", filename)
}

func VerifySHA256(path, wantHex string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", path, got, wantHex)
	}
	return nil
}
