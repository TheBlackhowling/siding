package update

import (
	"fmt"
	"net/url"
	"strings"
)

func assertGitHubAssetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("github releases: bad asset URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("github releases: asset URL must be https")
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "github.com", "api.github.com", "objects.githubusercontent.com",
		"release-assets.githubusercontent.com", "github-releases.githubusercontent.com":
		return nil
	}
	if strings.HasSuffix(host, ".githubusercontent.com") {
		return nil
	}
	return fmt.Errorf("github releases: refusing asset host %s", host)
}
