package update

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	if AssetName("v0.1.4", "windows", "amd64") != "siding_0.1.4_windows_amd64.zip" {
		t.Fatal(AssetName("v0.1.4", "windows", "amd64"))
	}
	if AssetName("0.1.3", "darwin", "arm64") != "siding_0.1.3_darwin_arm64.tar.gz" {
		t.Fatal(AssetName("0.1.3", "darwin", "arm64"))
	}
	if BinaryName("windows") != "siding.exe" || BinaryName("linux") != "siding" {
		t.Fatal("binary name")
	}
}

func TestParseAndVerifyChecksum(t *testing.T) {
	payload := []byte("hello")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])
	txt := "# comment\n" + hexSum + "  siding_0.1.4_linux_amd64.tar.gz\n"
	got, err := ParseChecksum([]byte(txt), "siding_0.1.4_linux_amd64.tar.gz")
	if err != nil || got != hexSum {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := ParseChecksum([]byte(txt), "nope.zip"); err == nil {
		t.Fatal("missing name")
	}
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(path, hexSum); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("mismatch")
	}
}

func TestGitHubOnlyRedirect(t *testing.T) {
	ok, _ := http.NewRequest(http.MethodGet, "https://github.com/TheBlackhowling/siding-releases/x", nil)
	if err := githubOnlyRedirect(ok, nil); err != nil {
		t.Fatal(err)
	}
	ok, _ = http.NewRequest(http.MethodGet, "https://objects.githubusercontent.com/foo", nil)
	if err := githubOnlyRedirect(ok, nil); err != nil {
		t.Fatal(err)
	}
	bad, _ := http.NewRequest(http.MethodGet, "https://evil.example/payload", nil)
	if err := githubOnlyRedirect(bad, nil); err == nil {
		t.Fatal("allowed off-site redirect")
	}
}
