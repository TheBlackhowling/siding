# Release

[Siding](../README.md) · [Commands](COMMANDS.md)

Source stays in this **private** repo. Binaries are published to the public
[siding-releases](https://github.com/TheBlackhowling/siding-releases) repo so
anyone can download them without access here.

You do not need local Go.

## What gets published

One `v*` tag releases **CLI binaries and the Go library together** — same
commit, same version. There is no separate library tag or release train.

| Deliverable | Where | How consumers get it |
|-------------|-------|----------------------|
| CLI binaries | [siding-releases](https://github.com/TheBlackhowling/siding-releases) | Download zip/tar.gz |
| Go library | This repo (private) | `go get github.com/TheBlackhowling/siding@vX.Y.Z` · import `siding/siding` |

Release CI runs [`scripts/verify-library.sh`](../scripts/verify-library.sh)
before GoReleaser uploads binaries. If the library does not compile for external
modules, the release fails and nothing is published.

GitHub Release assets on **siding-releases** for each of:

| OS | Arch | Archive |
|----|------|---------|
| Windows | amd64, arm64 | `.zip` (`siding.exe`) |
| macOS | amd64, arm64 | `.tar.gz` |
| Linux | amd64, arm64 | `.tar.gz` |

Checksums in `checksums.txt`. Version/commit/date are stamped into `siding version`.

CI: [`.github/workflows/release.yml`](../.github/workflows/release.yml) on tags `v*` in **this** repo. Config: [`.goreleaser.yaml`](../.goreleaser.yaml) (`release.github.name: siding-releases`).

## Release notes

Each tag creates two GitHub Releases. Same commit range (previous stable
`v*` tag → this tag). Old tags are left as they were.

| Where | Who sees it | Body |
|-------|-------------|------|
| **This repo** | people with access here | `gh release create --generate-notes` from [`scripts/release.ps1`](../scripts/release.ps1), then library import notes appended. Private PR links are fine. |
| **[siding-releases](https://github.com/TheBlackhowling/siding-releases)** | anyone | GoReleaser `git` changelog: 8-character SHAs, merge commits dropped. Header/footer note that the Go library ships at the same tag. No private PR URLs — the publish token cannot read this repo’s GitHub API. |

## One-time: secrets

On `TheBlackhowling/siding` Actions secrets:

**Publish** (`GORELEASER_GITHUB_TOKEN`): fine-grained PAT, resource
`TheBlackhowling/siding-releases` only, **Contents: Read and write**.
The default `GITHUB_TOKEN` cannot create a Release on another repo.

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

**Checksum signing** (`SIDING_CHECKSUM_SIGN_SEED`): 64-character hex encoding
of a 32-byte Ed25519 seed. Generate a keypair (no local Go required):

```powershell
.\docker\dev.ps1 gen-checksum-key
```

That bind-mounts the repo into a `golang:1.25` container and runs
`go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign`, so files
land under **`.docker/checksum-sign/`** on the host (gitignored):

Equivalent without `dev.ps1`:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.25 `
  go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign
```

| File | Use |
|------|-----|
| `checksum-sign.seed` | GitHub Actions secret `SIDING_CHECKSUM_SIGN_SEED` |
| `checksum-sign.pub.hex` | Paste into `internal/update/checksums_sig.go` (`mustDecodePubKey`) |

With Go installed locally: `go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign`

GoReleaser signs `checksums.txt` as `checksums.txt.sig`; `siding update`
refuses releases that omit or fail signature verification.

Apple notary JWTs last at most 20 minutes, so the job signs, submits both
darwin binaries, and does not wait for Apple to finish. Waiting longer is
rejected before sign starts. GoReleaser’s overall `--timeout` is 3 hours
for the rest of the release (six archives, checksums, upload).

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

The first tag is always `v0.1.0`. After that, the script uses the highest `vMAJOR.MINOR.PATCH` tag (prerelease tags like `v0.1.0-rc.1` are ignored). `-Force` tags the current branch instead of requiring `main`.

3. The script creates a GitHub Release on **this** repo (`--generate-notes`) and
   appends Go library import instructions (same tag as the CLI).
4. The Release workflow verifies the library (`verify-library` job), then runs
   GoReleaser and attaches the six archives to
   https://github.com/TheBlackhowling/siding-releases/releases
5. People download the zip for their OS, or on Windows run the exe through [install](COMMANDS.md#siding-install).
6. Go consumers (AEOS) pin `github.com/TheBlackhowling/siding` at the same tag —
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
