# Siding

**Park a stack on a siding.** Each git checkout gets its own Docker Compose stack - its own ports, compose project name, and browser-friendly `*.localhost` host - without copying allocator scripts into every repo.

If you run **more than one clone** of the same app, or several apps at once, you have already hit the wall: everything defaults to `3000`, `8000`, and `5432`. Per-repo PowerShell helpers drift. Editing the hosts file does not scale. Siding is a small CLI that keeps a **recipe in git** and **ports on your machine** separate, so a second folder can `up` without fighting the first.

Start here: [Quick start](docs/QUICKSTART.md). When you need every flag: [Commands](docs/COMMANDS.md). The committed contract is [`.siding/config.yml`](docs/CONTRACT.md); machine state lives in [STATE.md](docs/STATE.md).

## How it thinks about your repos

You commit a **recipe** at `.siding/config.yml`: modes (`test`, `prod`, …), which compose files to use, which port keys exist, how hosts should look. That file describes *kinds* of stack, not a list of developer names.

The **checkout folder** is the slot. `acme-betty` with `folder_prefix: acme-` is stack `betty`. Open another clone as `acme-changelog` and you get another stack without touching the recipe.

On disk, siding writes `.siding/env` (generated, gitignored) and records allocations in `%LOCALAPPDATA%\siding\state.json` on Windows (see [STATE.md](docs/STATE.md) for other OS paths). Same recipe in git; different numbers per machine and per folder.

Compose always runs **on the host**, not inside a dev container, so bind mounts are real paths on your machine. Commands like `up`, `down`, `proxy`, and `ps` should use an installed `siding` binary for that reason.

Optional **shared proxy**: one Caddy on loopback `:80` routes `http://app.betty.acme.test.localhost` (and friends) to whatever port siding picked for that checkout. Redis and Postgres stay ordinary host ports - siding still allocates them so two stacks never grab the same `5432`.

## What it is good at (and what it is not)

It tries to **own every port your compose file publishes**, not just the web tier, so collisions show up at `init`/`up` instead of as mysterious Docker errors. HTTP goes through Caddy when the proxy is up; everything else is “pick a free port and put it in env.”

It is **not** a cloud orchestrator, not a replacement for Compose, and not a general TCP router for databases. It is local slot bookkeeping plus an optional HTTP front door for browsers.

Embedding in another Go CLI: [Library API](docs/LIBRARY.md) (`github.com/TheBlackhowling/siding/siding`, same tags as the binary).

## Get the binary

Releases: [GitHub Releases](https://github.com/TheBlackhowling/siding/releases) on this repo. See [RELEASE.md](docs/RELEASE.md) for tagging and CI.

Windows: unzip, then `siding install` (or use [install.ps1](scripts/install.ps1) from a dev build). PowerShell will not run `.\siding.exe` from the current directory by name - install puts it on PATH.

## Hack on siding itself

You only need Docker Desktop (no local Go). Build in a container, install the exe, run tests:

```powershell
.\docker\build.ps1
.\scripts\install.ps1
siding version
.\docker\dev.ps1 test
```

Do not run `docker\dev.ps1 run up` for app stacks - compose would see paths inside the container. All platforms: `.\docker\build.ps1 -All` → `dist\`. More detail: [Commands → develop](docs/COMMANDS.md#develop-the-siding-cli). With Go 1.26+ on PATH: `go test ./...` / `go run ./cmd/siding`.

## License

MIT - see [LICENSE](LICENSE).
