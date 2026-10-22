package signcheck

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHasCodeSignatureUnsignedDarwin(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "siding")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cross-compile darwin: %v\n%s", err, out)
	}
	ok, err := HasCodeSignature(bin)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ad-hoc go darwin build must not count as Developer ID signed")
	}
	if err := RequireSigned([]string{bin}); err == nil {
		t.Fatal("RequireSigned must fail on an unsigned darwin binary")
	}
}

func TestSuperBlobHasCMS(t *testing.T) {
	// SuperBlob, big-endian: magic, length, count=1, slot CMS at offset 20, blob len 16.
	blob := make([]byte, 36)
	binary.BigEndian.PutUint32(blob[0:4], csMagicEmbedded)
	binary.BigEndian.PutUint32(blob[4:8], 36)
	binary.BigEndian.PutUint32(blob[8:12], 1)
	binary.BigEndian.PutUint32(blob[12:16], csSlotCMS)
	binary.BigEndian.PutUint32(blob[16:20], 20)
	binary.BigEndian.PutUint32(blob[20:24], 0xfade0b01)
	binary.BigEndian.PutUint32(blob[24:28], 16)
	if !superBlobHasCMS(blob) {
		t.Fatal("CMS slot must count as signed")
	}
	// CodeDirectory-only SuperBlob (Go ad-hoc) is not Developer ID.
	adhoc := make([]byte, 20)
	binary.BigEndian.PutUint32(adhoc[0:4], csMagicEmbedded)
	binary.BigEndian.PutUint32(adhoc[4:8], 20)
	binary.BigEndian.PutUint32(adhoc[8:12], 1)
	binary.BigEndian.PutUint32(adhoc[12:16], 0) // CSSLOT_CODEDIRECTORY
	binary.BigEndian.PutUint32(adhoc[16:20], 12)
	if superBlobHasCMS(adhoc) {
		t.Fatal("ad-hoc CodeDirectory-only blob must not count as signed")
	}
}

func TestHasCodeSignatureRejectsNonMacho(t *testing.T) {
	p := filepath.Join(t.TempDir(), "not-macho")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := HasCodeSignature(p); err == nil {
		t.Fatal("expected error for a non-Mach-O file")
	}
}

func TestGoreleaserNotarizeConfigured(t *testing.T) {
	root := findRepoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"notarize:",
		"MACOS_SIGN_P12",
		"MACOS_SIGN_PASSWORD",
		"MACOS_NOTARY_KEY",
		"MACOS_NOTARY_KEY_ID",
		"MACOS_NOTARY_ISSUER_ID",
		"wait: false",
		"timeout: 20m",
	} {
		if !strings.Contains(s, want) {
			t.Errorf(".goreleaser.yaml missing %q", want)
		}
	}
}

// goreleaserChangelog is the changelog/release subset we lock for public notes.
type goreleaserChangelog struct {
	Changelog struct {
		Use     string `yaml:"use"`
		Abbrev  int    `yaml:"abbrev"`
		Filters struct {
			Exclude []string `yaml:"exclude"`
		} `yaml:"filters"`
	} `yaml:"changelog"`
	Release struct {
		Header string `yaml:"header"`
	} `yaml:"release"`
}

func loadGoreleaserChangelog(t *testing.T) goreleaserChangelog {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepoRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg goreleaserChangelog
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func changelogExcluded(excludes []string, subject string) (bool, error) {
	for _, p := range excludes {
		re, err := regexp.Compile(p)
		if err != nil {
			return false, err
		}
		if re.MatchString(subject) {
			return true, nil
		}
	}
	return false, nil
}

func TestGoreleaserChangelogConfigured(t *testing.T) {
	cfg := loadGoreleaserChangelog(t)
	if cfg.Changelog.Use != "git" {
		t.Fatalf("changelog.use must be git so notes stay non-empty without the private GitHub API, got %q", cfg.Changelog.Use)
	}
	if cfg.Changelog.Abbrev < 1 {
		t.Fatalf("changelog.abbrev must shorten SHAs, got %d", cfg.Changelog.Abbrev)
	}
	if !strings.Contains(strings.ToLower(cfg.Release.Header), "binar") {
		t.Fatalf("release.header must say this repo is binaries-only, got %q", cfg.Release.Header)
	}

	ex := cfg.Changelog.Filters.Exclude
	if len(ex) == 0 {
		t.Fatal("changelog.filters.exclude must drop merge commits")
	}
	for _, subject := range []string{
		"Merge pull request #6 from TheBlackhowling/feat/notarize-within-apple-token-limit",
		"Merge branch 'feat/b1-changelog'",
		"Merge remote-tracking branch 'origin/main'",
	} {
		ok, err := changelogExcluded(ex, subject)
		if err != nil {
			t.Fatalf("exclude regexp: %v", err)
		}
		if !ok {
			t.Errorf("exclude must drop %q", subject)
		}
	}
	for _, subject := range []string{
		"Submit macOS notarization without waiting past Apple's 20m token.",
		"Add a siding recipe so sibling worktrees get their own compose projects.",
		"Serve the status board as a React SPA from a published Caddy image.",
	} {
		ok, err := changelogExcluded(ex, subject)
		if err != nil {
			t.Fatalf("exclude regexp: %v", err)
		}
		if ok {
			t.Errorf("exclude must keep %q", subject)
		}
	}
}

func TestResolveDistRelativeToRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := ResolveDist("dist", root)
	if !ok || got != filepath.Join(root, "dist") {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := ResolveDist("missing", root); ok {
		t.Fatal("missing dist must not resolve")
	}
}

func TestDistBinariesSigned(t *testing.T) {
	dist := os.Getenv("SIDING_DIST")
	if dist == "" {
		t.Skip("SIDING_DIST not set")
	}
	resolved, ok := ResolveDist(dist, findRepoRoot(t))
	if !ok {
		t.Fatalf("SIDING_DIST %s is not a directory", dist)
	}
	dist = resolved
	paths, err := DarwinBinaries(dist)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SIDING_REQUIRE_SIGNED") == "1" {
		if err := RequireSigned(paths); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Logf("found %d darwin binaries (signing not required)", len(paths))
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".goreleaser.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func TestDarwinBinariesWalk(t *testing.T) {
	if runtime.GOOS == "" {
		t.Fatal("goos")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "siding_darwin_arm64", "siding")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", nested, src)
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DarwinBinaries(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != nested {
		t.Fatalf("got %#v", got)
	}
}
