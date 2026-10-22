// Package update checks siding-releases for a newer stable GitHub Release.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDisable = "SIDING_NO_UPDATE_CHECK"

	ReleasesOwner = "TheBlackhowling"
	ReleasesRepo  = "siding-releases"
	ReleasesPage  = "https://github.com/TheBlackhowling/siding-releases/releases/latest"

	CacheFileName = "update-check.json"
	DefaultTTL    = 1 * time.Hour
	HTTPTimeout   = 3 * time.Second
	DownloadTimeout = 3 * time.Minute
	MaxAssetBytes   = 64 << 20
)

const apiRepo = "https://api.github.com/repos/" + ReleasesOwner + "/" + ReleasesRepo

// LatestURL is the GitHub Releases API for the latest non-prerelease.
// Tests replace it with an httptest server.
var LatestURL = apiRepo + "/releases/latest"

// TagsURLPrefix is the GitHub Releases API prefix for a tag (no trailing slash).
// Tests replace it. FetchRelease appends /vMAJOR.MINOR.PATCH.
var TagsURLPrefix = apiRepo + "/releases/tags"

// HTTPClient is used when Options.Client is nil. Tests replace it.
var HTTPClient = &http.Client{Timeout: HTTPTimeout}

// DownloadClient fetches release assets. Tests replace it.
var DownloadClient = &http.Client{Timeout: DownloadTimeout, CheckRedirect: githubOnlyRedirect}

// AllowAnyAssetHostForTest skips GitHub host checks (cli/update integration tests).
var AllowAnyAssetHostForTest bool

type Options struct {
	Current      string
	Version      string // empty = latest; used by FetchRelease
	CacheDir     string
	Now          time.Time
	TTL          time.Duration
	Client       *http.Client
	LatestURL    string
	TagURLPrefix string
	GOOS         string
	GOARCH       string
	WorkDir      string
	// AllowAnyAssetHost skips GitHub host allowlisting (httptest only).
	AllowAnyAssetHost bool
}

type Result struct {
	Current string
	Latest  string
	Newer   bool
}

type cacheFile struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Disabled() bool {
	return strings.TrimSpace(os.Getenv(EnvDisable)) != ""
}

// IsReleaseVersion is true for a stable vMAJOR.MINOR.PATCH (optional v).
func IsReleaseVersion(v string) bool {
	_, ok := parseStable(v)
	return ok
}

func parseStable(v string) ([3]int, bool) {
	var zero [3]int
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" || strings.EqualFold(s, "dev") || strings.EqualFold(s, "none") {
		return zero, false
	}
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		return zero, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return zero, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || strconv.Itoa(n) != p {
			return zero, false
		}
		out[i] = n
	}
	return out, true
}

func Display(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		return s
	}
	return "v" + s
}

// Newer reports whether latest is a higher stable semver than current.
func Newer(current, latest string) bool {
	c, okc := parseStable(current)
	l, okl := parseStable(latest)
	if !okc || !okl {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func Check(ctx context.Context, opt Options) (Result, error) {
	res := Result{Current: strings.TrimSpace(opt.Current)}
	if !IsReleaseVersion(res.Current) {
		return res, nil
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	ttl := opt.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if cached, ok := readCache(opt.CacheDir, opt.Now, ttl); ok {
		res.Latest = cached
		res.Newer = Newer(res.Current, cached)
		return res, nil
	}

	latest, err := fetchLatest(ctx, opt)
	if err != nil {
		return res, err
	}
	res.Latest = latest
	res.Newer = Newer(res.Current, latest)
	writeCache(opt.CacheDir, cacheFile{CheckedAt: opt.Now, Latest: latest})
	return res, nil
}

func fetchLatest(ctx context.Context, opt Options) (string, error) {
	rel, err := FetchRelease(ctx, opt)
	if err != nil {
		return "", err
	}
	return rel.Tag, nil
}

func readCache(dir string, now time.Time, ttl time.Duration) (string, bool) {
	if dir == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, CacheFileName))
	if err != nil {
		return "", false
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return "", false
	}
	if c.Latest == "" || !IsReleaseVersion(c.Latest) {
		return "", false
	}
	if now.Sub(c.CheckedAt) >= ttl || now.Before(c.CheckedAt) {
		return "", false
	}
	return Display(c.Latest), true
}

func writeCache(dir string, c cacheFile) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, CacheFileName), data, 0o644)
}
