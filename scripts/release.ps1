# Cut the next GitHub Release from this private repo.
#
# Finds the highest vMAJOR.MINOR.PATCH tag, increments it, tags HEAD, and
# pushes. No prior tag → v0.1.0 (any bump). Then creates a GitHub Release
# on this repo (generate-notes; private PR links stay here). The Release
# workflow publishes archives to TheBlackhowling/siding-releases.
#
#   .\scripts\release.ps1 patch
#   .\scripts\release.ps1 minor
#   .\scripts\release.ps1 major
#   .\scripts\release.ps1 patch -DryRun

param(
    [Parameter(Position = 0, Mandatory = $true)]
    [ValidateSet("major", "minor", "patch")]
    [string] $Bump,

    [switch] $DryRun,

    [switch] $Force
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

function Get-LatestStableVersion {
    $best = $null
    $tags = @(git tag -l "v*")
    foreach ($t in $tags) {
        if ($t -notmatch '^v(\d+)\.(\d+)\.(\d+)$') {
            continue
        }
        $v = [version]"$($Matches[1]).$($Matches[2]).$($Matches[3])"
        if (-not $best -or $v -gt $best) {
            $best = $v
        }
    }
    return $best
}

function Get-NextVersion([version] $Current, [string] $Kind) {
    if (-not $Current) {
        return [version]"0.1.0"
    }
    switch ($Kind) {
        "major" { return [version]::new($Current.Major + 1, 0, 0) }
        "minor" { return [version]::new($Current.Major, $Current.Minor + 1, 0) }
        "patch" { return [version]::new($Current.Major, $Current.Minor, $Current.Build + 1) }
        default { throw "unknown bump $Kind" }
    }
}

$status = git status --porcelain
if ($status) {
    throw "working tree is not clean; commit or stash first"
}

Write-Host "Fetching tags and origin/main ..."
git fetch origin --tags --prune
git fetch origin main

$branch = (git branch --show-current).Trim()
if ($branch -ne "main" -and -not $Force) {
    throw "release from main (now on '$branch'). Pass -Force to tag this HEAD anyway."
}

if ($branch -eq "main") {
    git merge --ff-only origin/main
}

$current = Get-LatestStableVersion
$next = Get-NextVersion $current $Bump
$tag = "v$($next.Major).$($next.Minor).$($next.Build)"

if ($current) {
    Write-Host "Previous: v$($current.Major).$($current.Minor).$($current.Build)"
} else {
    Write-Host "Previous: (none: first release is v0.1.0)"
}
Write-Host "Next:     $tag  ($Bump)"

if ($DryRun) {
    Write-Host "dry run: no tag created"
    Write-Host "dry run: would create GitHub Release $tag on this repo (--generate-notes)"
    exit 0
}

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    throw "gh is required to create the private GitHub Release on this repo"
}

if (git rev-parse -q --verify "refs/tags/$tag") {
    throw "tag $tag already exists"
}

git tag -a $tag -m "siding $tag"
git push origin $tag

$ghArgs = @(
    "release", "create", $tag,
    "--repo", "TheBlackhowling/siding",
    "--title", "siding $tag",
    "--generate-notes"
)
if ($current) {
    $prev = "v$($current.Major).$($current.Minor).$($current.Build)"
    $ghArgs += @("--notes-start-tag", $prev)
}
& gh @ghArgs
if ($LASTEXITCODE -ne 0) {
    Write-Host "Tag $tag is on origin. Create the private Release manually:"
    Write-Host "  gh release create $tag --repo TheBlackhowling/siding --generate-notes"
    throw "gh release create failed"
}

$libraryAppend = @"

## Go library (lockstep with CLI)

Same tag ``$tag`` — the public Go module is released with these CLI binaries. AEOS and other Go CLIs import in-process:

``````go
require github.com/TheBlackhowling/siding $tag

import "github.com/TheBlackhowling/siding/siding"
``````

Set ``GOPRIVATE=github.com/TheBlackhowling/*``. See ``docs/LIBRARY.md``.
"@
$body = gh release view $tag --repo TheBlackhowling/siding --json body -q .body
$notesFile = Join-Path $env:TEMP "siding-release-notes-$tag.md"
Set-Content -Path $notesFile -Value ($body + $libraryAppend) -NoNewline
gh release edit $tag --repo TheBlackhowling/siding --notes-file $notesFile
if ($LASTEXITCODE -ne 0) {
    Write-Host "Warning: could not append library notes to private release $tag"
}

Write-Host ""
Write-Host "Pushed $tag. Watch: https://github.com/TheBlackhowling/siding/actions"
Write-Host "Notes:    https://github.com/TheBlackhowling/siding/releases/tag/$tag"
Write-Host "Binaries: https://github.com/TheBlackhowling/siding-releases/releases/tag/$tag"
Write-Host "Library:  go get github.com/TheBlackhowling/siding@$tag"
