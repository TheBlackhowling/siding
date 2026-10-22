# Sync user-facing docs from this private repo to siding-releases.
# Strips developer-only sections and rewrites examples for public readers.
param(
    [string]$Dest = (Join-Path (Split-Path $PSScriptRoot -Parent) "..\siding-releases")
)

$ErrorActionPreference = "Stop"
$SrcRoot = Join-Path $PSScriptRoot ".." | Resolve-Path
$SrcDocs = Join-Path $SrcRoot "docs"
$Dest = $Dest | Resolve-Path
$DestDocs = Join-Path $Dest "docs"

if (-not (Test-Path $DestDocs)) {
    throw "siding-releases docs not found at $DestDocs"
}

function Write-PublicDoc {
    param([string]$Name, [string]$Content)
    $path = Join-Path $DestDocs $Name
    $utf8 = New-Object System.Text.UTF8Encoding $false
    [IO.File]::WriteAllText($path, (Normalize-PublicText $Content).TrimEnd() + "`n", $utf8)
    Write-Host "  $Name"
}

function Normalize-PublicText {
    param([string]$Text)
    $Text = $Text -replace [char]0x2019, "'"
    $Text = $Text -replace [char]0x2014, ' - '
    $Text = $Text -replace [char]0x2013, '-'
    return $Text
}

function Convert-PublicExamples {
    param([string]$Text)
    $Text = $Text -replace 'C:\\source\\my-app(?:-other)?', 'my-app'
    $Text = $Text -replace 'C:\\source\\acme-(\w+)', 'my-app-$1'
    $Text = $Text -replace 'acme-', 'acme-'
    $Text = $Text -replace '\bacme\b', 'acme'
    $Text = $Text -replace 'acme', 'acme'
    $Text = $Text -replace 'betty\.acme', 'shop.acme'
    $Text = $Text -replace '\bbetty\b', 'shop'
    $Text = $Text -replace 'genie3', 'shop'
    return $Text
}

Write-Host "Syncing docs -> $DestDocs"

function Read-SourceDoc {
    param([string]$Name)
    return Get-Content (Join-Path $SrcDocs $Name) -Raw -Encoding UTF8
}

# --- COMMANDS.md ---
$commands = Read-SourceDoc "COMMANDS.md"
$commands = $commands -replace '\[Release\]\(RELEASE\.md\)', '[Releases](https://github.com/TheBlackhowling/siding-releases/releases)'
$commands = $commands -replace '(?s)\r?\n---\r?\n\r?\n## Develop the siding CLI.*\z', ''

$publicInstallUpdate = @'
## `siding install`

Copy **this** executable to a user bin directory and, on Windows, add that directory to your user PATH.

PowerShell does not search the current folder, so `siding` only works after the exe is on PATH (or you type `.\siding.exe`).

| Where | Path |
|-------|------|
| Windows | `%LOCALAPPDATA%\siding\bin\siding.exe` |
| macOS / Linux | `~/.local/bin/siding` |

Download the archive from [Releases](https://github.com/TheBlackhowling/siding-releases/releases/latest) first. Windows amd64/arm64 are zips; macOS and Linux are `.tar.gz` (macOS is Developer ID signed).

The install process cannot change **this** PowerShell session’s PATH (a child process cannot). After `siding install`, prepend the bin dir in this session (or open a new Windows Terminal):

```powershell
.\siding.exe install
$env:Path = "$env:LOCALAPPDATA\siding\bin;" + $env:Path
siding version
```

**macOS / Linux:** unpack `siding` and copy it onto PATH yourself (install still works if you run it from that copy):

```bash
tar -xzf siding_*_darwin_arm64.tar.gz   # or linux / amd64
install -m 0755 siding ~/.local/bin/siding
siding version
```

No extra flags.

---

## `siding update`

Download the archive for **this** OS/arch from
[siding-releases](https://github.com/TheBlackhowling/siding-releases/releases/latest),
verify `checksums.txt`, and replace the binary in the same directory as
[`siding install`](#siding-install), not a copy you launched from Downloads.

```powershell
siding update          # latest stable
siding update v0.1.4   # that tag
```

Already current (same stable tag as this binary) prints that and exits 0
without downloading. Network / checksum failures exit non-zero and leave
the installed file unchanged. `SIDING_NO_UPDATE_CHECK` does not apply;
this command is explicit.

After a successful replace, siding prints the new `siding version` output
when the new file can run, plus the same Windows PATH reminder as `install`
if that directory is not already on PATH.

No extra flags.

---
'@

$commands = [regex]::Replace(
    $commands,
    '(?s)## `siding install`.*?## `siding init`',
    ($publicInstallUpdate.TrimEnd() + "`n`n## ``siding init``")
)
$commands = Convert-PublicExamples $commands
Write-PublicDoc "COMMANDS.md" $commands

# --- CONTRACT.md ---
$contract = Read-SourceDoc "CONTRACT.md"
$contract = Convert-PublicExamples $contract
# Keep public-style multi-clone example naming
Write-PublicDoc "CONTRACT.md" $contract

# --- STATE.md ---
$state = Read-SourceDoc "STATE.md"
$state = Convert-PublicExamples $state
$state = $state -replace 'c:/source/acme-shop', 'C:/Users/you/Projects/acme-shop'
$state = $state -replace '"checkout": "c:/source/acme-shop"', '"checkout": "C:/Users/you/Projects/acme-shop"'
$oldStack = '4. Stack = ``--stack`` ' + [char]0x2192 + ' folder basename minus ``folder_prefix`` ' + [char]0x2192 + ' basename'
$newStack = '4. Stack = ``--stack``, else folder basename with ``folder_prefix`` trimmed when set, else the folder basename as-is'
$state = $state -replace [regex]::Escape($oldStack), $newStack
Write-PublicDoc "STATE.md" $state

# --- QUICKSTART.md ---
$quick = Read-SourceDoc "QUICKSTART.md"
$quick = $quick -replace '(?s)^# Quick start.*?## 1\. Point at the app repo',
@'
# Quick start

[Siding](../README.md) · [Commands](COMMANDS.md) · [Recipe](CONTRACT.md) · [State](STATE.md)

Get one checkout running and check in the org recipe so everyone else can
`sync`. Docker Desktop must be running.

Download a binary from
[Releases](https://github.com/TheBlackhowling/siding-releases/releases/latest)
(Windows `.zip`, macOS/Linux `.tar.gz`). macOS builds are Developer ID signed.

**Windows:** PowerShell will not run `siding` from the current folder (no
implicit `.\`). Install once:

```powershell
.\siding.exe install
$env:Path = "$env:LOCALAPPDATA\siding\bin;" + $env:Path
siding version
```

That copies the exe to `%LOCALAPPDATA%\siding\bin` and updates your user PATH.
New Windows Terminal windows pick it up; Cursor/IDE terminals keep the old
PATH until the IDE restarts.

**macOS / Linux:** unpack `siding` onto PATH (`~/.local/bin` is typical), or
run `./siding install` from the unpacked binary.

See [Commands → install](COMMANDS.md#siding-install).

Work from the app checkout for the rest of this page. Pass `--path` only when
you are somewhere else.

```powershell
cd my-app
```

## 1. Point at the app repo
'@

$quick = Convert-PublicExamples $quick
$quick = $quick -replace 'siding doctor --path my-app', 'siding doctor'
$quick = $quick -replace 'siding init --path my-app', 'siding init'
$quick = $quick -replace 'siding up --path my-app', 'siding up'
$quick = $quick -replace 'siding down --path my-app', 'siding down'
$quick = $quick -replace 'siding ps --path my-app', 'siding ps'
$quick = $quick -replace 'cd my-app\r?\n', ''
$quick = $quick -replace '(?s)## Next clone of the same product.*?(?=## If it fights you)',
@'
## Next clone of the same product

No wizard. The recipe is already in git:

```powershell
cd my-app-other
siding init
siding proxy up    # if this machine does not have it yet
siding up
```

`init` here is `sync`: map the committed recipe onto this folder’s ports and
`.siding/env`.

## Git worktrees

Sibling folders for parallel branches (same repo, different stack names):

```powershell
siding worktree add genie feat/my-work
siding worktree list
siding worktree sync          # adopt a plain git worktree add
siding worktree remove genie  # from the main checkout
```

Close the linked folder as a Cursor workspace before `worktree remove` on
Windows — an open workspace can block deleting the directory.

'@

Write-PublicDoc "QUICKSTART.md" $quick

Write-Host "Done. Review diff in siding-releases, then commit there."
