package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Release struct {
	Tag    string
	Assets []Asset
}

type Asset struct {
	Name string
	URL  string
}

func (r Release) Find(name string) (Asset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no asset %q", r.Tag, name)
}

// FetchRelease loads latest (opt.Version empty) or a specific stable tag.
// It does not use the background-check cache.
func FetchRelease(ctx context.Context, opt Options) (Release, error) {
	rawURL, err := releaseAPIURL(opt)
	if err != nil {
		return Release{}, err
	}
	body, err := get(ctx, apiClient(opt), rawURL, "application/vnd.github+json", 1<<20)
	if err != nil {
		return Release{}, err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return Release{}, err
	}
	if rel.Prerelease || !IsReleaseVersion(rel.TagName) {
		return Release{}, fmt.Errorf("github releases: not a stable tag %q", rel.TagName)
	}
	out := Release{Tag: Display(rel.TagName)}
	for _, a := range rel.Assets {
		if a.Name == "" || a.BrowserDownloadURL == "" {
			continue
		}
		if !opt.AllowAnyAssetHost && !AllowAnyAssetHostForTest {
			if err := assertGitHubAssetURL(a.BrowserDownloadURL); err != nil {
				return Release{}, err
			}
		}
		out.Assets = append(out.Assets, Asset{Name: a.Name, URL: a.BrowserDownloadURL})
	}
	return out, nil
}

func releaseAPIURL(opt Options) (string, error) {
	if strings.TrimSpace(opt.Version) == "" {
		if opt.LatestURL != "" {
			return opt.LatestURL, nil
		}
		return LatestURL, nil
	}
	tag := Display(opt.Version)
	if !IsReleaseVersion(tag) {
		return "", fmt.Errorf("not a stable tag %q", opt.Version)
	}
	prefix := opt.TagURLPrefix
	if prefix == "" {
		prefix = TagsURLPrefix
	}
	return strings.TrimRight(prefix, "/") + "/" + url.PathEscape(tag), nil
}

func apiClient(opt Options) *http.Client {
	if opt.Client != nil {
		return opt.Client
	}
	return HTTPClient
}

func assetClient(opt Options) *http.Client {
	if opt.Client != nil {
		return opt.Client
	}
	return DownloadClient
}

func get(ctx context.Context, client *http.Client, rawURL, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "siding")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("github releases: response too large")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases: %s", resp.Status)
	}
	return body, nil
}

func githubOnlyRedirect(req *http.Request, _ []*http.Request) error {
	if req == nil || req.URL == nil {
		return fmt.Errorf("refusing redirect: empty URL")
	}
	host := strings.ToLower(req.URL.Hostname())
	switch host {
	case "github.com", "api.github.com", "objects.githubusercontent.com",
		"release-assets.githubusercontent.com", "github-releases.githubusercontent.com":
		return nil
	}
	if strings.HasSuffix(host, ".githubusercontent.com") {
		return nil
	}
	return fmt.Errorf("refusing redirect to %s", host)
}
