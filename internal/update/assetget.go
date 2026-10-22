package update

import "context"

func getAsset(ctx context.Context, opt Options, rawURL, accept string, limit int64) ([]byte, error) {
	if !opt.AllowAnyAssetHost && !AllowAnyAssetHostForTest {
		if err := assertGitHubAssetURL(rawURL); err != nil {
			return nil, err
		}
	}
	return get(ctx, assetClient(opt), rawURL, accept, limit)
}
