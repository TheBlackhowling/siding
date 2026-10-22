# Siding library API (private)

AEOS and other Go CLIs should import **`github.com/TheBlackhowling/siding/siding`**, not
`internal/*`. The human `siding` binary in `cmd/siding` is a thin cobra wrapper around
this package.

## Module

```go
require github.com/TheBlackhowling/siding vX.Y.Z

import "github.com/TheBlackhowling/siding/siding"
```

Pin a release tag. Set `GOPRIVATE=github.com/TheBlackhowling/*` for private fetches.

## Versioning (lockstep with CLI)

Every `v*` tag is **one release** for both the `siding` binary and this module.
Cut releases with [`scripts/release.ps1`](../scripts/release.ps1) the same way as
CLI-only releases — there is no separate library version.

```go
// AEOS go.mod — same tag as the CLI zip on siding-releases
require github.com/TheBlackhowling/siding v0.2.0
```

Release CI runs `scripts/verify-library.sh` before GoReleaser publishes binaries.
If the external-consumer compile fails, the whole release is blocked.

## Client

```go
client := siding.New(stateRoot) // empty = default user data dir (SIDING_ROOT)
```

| Method | CLI equivalent | Notes |
|--------|----------------|-------|
| `Resolve` | (internal) | Checkout slot without writes |
| `Plan` | `siding plan` | Slot env + tips, no containers |
| `Add` | `siding add` | Pre-allocate ports in state.json |
| `Sync` | `siding sync` | Recipe → local state + env files |
| `Up` | `siding up` | docker compose up -d on host |
| `Down` | `siding down` | docker compose down + release lock |
| `ProxyUp` | `siding proxy up` | Shared Caddy on loopback |
| `ProxyDown` | `siding proxy down` | Stop Caddy |
| `ProxyReload` | `siding proxy reload` | Rewrite routes from state.json |

All methods take `context.Context`. Stack methods use `Target{StartPath, Stack, Mode}`.

## Out of scope (CLI only)

Install/update wizard, doctor, worktree helpers, cobra flags, and TUI output stay in
`internal/cli`. Library callers handle their own UX and logging.

## Stability

Breaking changes to exported types or method signatures require a tagged release bump.
`internal/*` may change freely.

## Verify external import

```powershell
.\scripts\verify-library.ps1
# or: go build -C testdata/externalconsumer .
```
