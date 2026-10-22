package update

import "testing"

func TestAssertGitHubAssetURL(t *testing.T) {
	for _, u := range []string{
		"https://github.com/o/r/releases/download/v1/bin.zip",
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/2/3",
	} {
		if err := assertGitHubAssetURL(u); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	if err := assertGitHubAssetURL("https://evil.example/asset"); err == nil {
		t.Fatal("expected refusal")
	}
}
