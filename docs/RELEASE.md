# Release

[Siding](../README.md) · [Commands](COMMANDS.md)

One **`v*` tag** on this repo releases **CLI binaries, checksums, and the Go
module together** - same commit, same version.

You do not need local Go to cut a release (CI runs GoReleaser).

## What gets published

| Deliverable | Where | How consumers get it |
|-------------|-------|----------------------|
| CLI binaries | [GitHub Releases](https://github.com/TheBlackhowling/siding/releases) on **this repo** | Download zip/tar.gz |
| Go library | Same repo, same tag | `go get github.com/TheBlackhowling/siding@vX.Y.Z` · import `siding/siding` |

Release CI runs [`scripts/verify-library.sh`](../scripts/verify-library.sh)
before GoReleaser uploads assets. If the library does not compile for external
modules, the release fails.

Assets per tag:

| OS | Arch | Archive |
|----|------|---------|
| Windows | amd64, arm64 | `.zip` (`siding.exe`; use `install.ps1` from the repo) |
| macOS | amd64, arm64 | `.tar.gz` (includes `install.sh`) |
| Linux | amd64, arm64 | `.tar.gz` (includes `install.sh`) |

Checksums in `checksums.txt` (Ed25519 signature in `checksums.txt.sig`).
Version/commit/date are stamped into `siding version`.

CI: [`.github/workflows/release.yml`](../.github/workflows/release.yml) on tags
`v*`. Config: [`.goreleaser.yaml`](../.goreleaser.yaml) publishes assets to this
repo.

## Release notes

[`scripts/release.ps1`](../scripts/release.ps1) creates one **draft** GitHub
Release per tag (`--title vX.Y.Z`, `--generate-notes --draft`) and appends Go
library import notes. GoReleaser uses `use_existing_draft: true` so binaries
upload to that draft (never a second release on the same tag). After macOS sign
verification, CI runs `gh release edit --draft=false`. **Latest** updates only
when assets are attached. A failed workflow leaves a draft — re-run the Release
workflow on the tag, or delete the tag and both releases before retrying.

## One-time: secrets

On **`TheBlackhowling/siding`** → Settings → Secrets and variables → Actions:

**macOS sign + notarize** (all five, or none: empty skips signing):

| Secret | Value |
|--------|--------|
| `MACOS_SIGN_P12` | base64 of the Developer ID Application `.p12` |
| `MACOS_SIGN_PASSWORD` | password for that `.p12` |
| `MACOS_NOTARY_KEY` | base64 of the App Store Connect `.p8` |
| `MACOS_NOTARY_KEY_ID` | key id (also in the `.p8` filename) |
| `MACOS_NOTARY_ISSUER_ID` | issuer UUID on the API keys page |

Both `.p12` and `.p8` must be base64. PowerShell:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes((Resolve-Path .\developer-id.p12)))
[Convert]::ToBase64String([IO.File]::ReadAllBytes((Resolve-Path .\AuthKey_XXXXXXXXXX.p8)))
```

Do not commit those files.

**Proxy board image (GHCR)** — optional `GHCR_PACKAGES_TOKEN`: fine-grained or
classic PAT with **`write:packages`** on `ghcr.io/theblackhowling/siding-proxy`.
The **Proxy image** workflow uses it when set; otherwise `GITHUB_TOKEN`. If CI
logs `permission_denied: write_package`, either add this secret or in the org:
Actions → General → **Read and write** workflow permissions, and link the
`siding-proxy` package to this repo (Package → Settings → Manage Actions access).

**Checksum signing** (`SIDING_CHECKSUM_SIGN_SEED`): 64-character hex encoding
of a 32-byte Ed25519 seed. Generate a keypair (no local Go required):

```powershell
.\docker\dev.ps1 gen-checksum-key
```

That bind-mounts the repo into a `golang:1.26` container and runs
`go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign`, so files
land under **`.docker/checksum-sign/`** on the host (gitignored):

Equivalent without `dev.ps1`:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.26 `
  go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign
```

| File | Use |
|------|-----|
| `checksum-sign.seed` | GitHub Actions secret `SIDING_CHECKSUM_SIGN_SEED` |
| `checksum-sign.pub.hex` | Paste into `internal/update/checksums_sig.go` (`mustDecodePubKey`) |

With Go installed locally: `go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign`

GoReleaser signs `checksums.txt` as `checksums.txt.sig`; `siding update`
refuses releases that omit or fail signature verification.

GoReleaser waits up to **20 minutes** per darwin binary for notarization
(`wait: true`, Apple’s JWT cap). Signed-but-not-notarized uploads trigger
macOS Gatekeeper (“cannot verify…”) on downloaded tarballs. If notary times
out, re-run the failed Release workflow on the same tag (or cut a patch tag);
do not ship darwin assets from a job that logged `notarize still pending`
and continued. GoReleaser’s overall `--timeout` is 3 hours for the full job.

Bare Mach-O binaries cannot be stapled; after notarization, Gatekeeper uses
an online ticket check (or Right-click → Open once).

## Cut a release

1. Main is green ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml) tests on Ubuntu, Windows, macOS).
2. From a clean `main`, bump and push the next `v*` tag:

```powershell
git checkout main
.\scripts\release.ps1 patch    # 0.1.0 if none, else x.y.(z+1)
.\scripts\release.ps1 minor    # x.(y+1).0
.\scripts\release.ps1 major    # (x+1).0.0
.\scripts\release.ps1 patch -DryRun
```

The first tag on an empty repo is `v0.1.0`. After that, the script uses the highest `vMAJOR.MINOR.PATCH` tag (prerelease tags like `v0.1.0-rc.1` are ignored). `-Force` tags the current branch instead of requiring `main`.

**Continuing after `siding-releases`:** If binaries already shipped elsewhere (for example `v0.1.8` on `TheBlackhowling/siding-releases`) and this repo has no tags yet, seed the lineage so the next cut is not `v0.1.0`:

```powershell
git tag v0.1.8
git push origin v0.1.8
.\scripts\release.ps1 patch -DryRun   # expect v0.1.9
.\scripts\release.ps1 patch
```

The seed tag only needs to exist on the remote; it marks the previous public version. Say in release notes that downloads moved to this repo.

3. The script creates a GitHub Release on **this repo** (`--generate-notes`) and
   appends Go library import instructions (same tag as the CLI).
4. The Release workflow verifies the library (`verify-library` job), then runs
   GoReleaser and attaches the six archives to
   https://github.com/TheBlackhowling/siding/releases
5. People download the zip for their OS, or on Windows run the exe through [install](COMMANDS.md#siding-install).
6. Go consumers pin `github.com/TheBlackhowling/siding` at the same tag  - 
   see [LIBRARY.md](LIBRARY.md).

Prerelease tags (`v0.1.0-rc.1`) are marked prerelease automatically (`prerelease: auto`).

## Local dry-run (optional)

Need Go installed, or use Docker to produce unsigned `dist/` trees:

```powershell
.\docker\build.ps1 -All
Get-ChildItem dist
```

That does **not** stamp a real version (`siding version` stays `dev`) and does **not** upload. It is for checking that every GOOS/GOARCH compiles.

A tagged snapshot with GoReleaser (needs Go on the PATH):

```bash
goreleaser release --snapshot --clean
```

## After people download a Windows zip

Unzip, then either:

```powershell
.\siding.exe install
# new terminal:
siding version
```

or copy `siding.exe` somewhere already on PATH. PowerShell will not find `siding` in the current folder unless you type `.\siding.exe`.
