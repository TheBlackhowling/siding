# Cross-compile siding via Docker — no local Go required.
#
#   .\docker\build.ps1                 # Windows amd64 -> bin\siding.exe
#   .\docker\build.ps1 -All            # all OS/arch -> dist\ + bin\siding.exe
#   .\docker\build.ps1 linux amd64     # one target -> bin\siding
#   .\docker\build.ps1 -ProxyImage     # also docker build siding-proxy:local
#   .\scripts\install.ps1              # put bin\siding.exe on PATH

param(
    [Parameter(Position = 0)]
    [ValidateSet("windows", "linux", "darwin")]
    [string] $Platform = "windows",

    [Parameter(Position = 1)]
    [ValidateSet("amd64", "arm64")]
    [string] $Arch = "amd64",

    [switch] $All,
    [switch] $ProxyImage
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

function Build-ProxyImage {
    Write-Host "Building siding-proxy:local from docker/proxy/Dockerfile ..."
    & docker build -t siding-proxy:local -f docker/proxy/Dockerfile $Root
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "Built: siding-proxy:local"
}

$BinDir = Join-Path $Root "bin"
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $Root ".docker\siding-data") -Force | Out-Null

if ($All) {
    New-Item -ItemType Directory -Path (Join-Path $Root "dist") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $Root ".docker") -Force | Out-Null
    Write-Host "Building all platforms -> dist\ + bin\siding.exe ..."
    $srcScript = Join-Path $Root "docker\build-all.sh"
    $innerScript = Join-Path $Root ".docker\build-all.sh"
    $text = [System.IO.File]::ReadAllText($srcScript) -replace "`r`n", "`n" -replace "`r", "`n"
    $utf8 = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($innerScript, $text, $utf8)
    & docker compose -f docker-compose.dev.yml run --rm dev sh /src/.docker/build-all.sh
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $distWin = Join-Path $Root "dist\siding_windows_amd64.exe"
    $binWin = Join-Path $BinDir "siding.exe"
    if (Test-Path $distWin) {
        try {
            Copy-Item $distWin $binWin -Force
        } catch {
            Write-Host "note: could not refresh bin\siding.exe (close a running siding.exe). dist\siding_windows_amd64.exe is current."
        }
    }
    Write-Host ""
    Get-ChildItem (Join-Path $Root "dist") | ForEach-Object { Write-Host ("  dist\" + $_.Name) }
    Write-Host ""
    Write-Host "Windows host:  .\scripts\install.ps1"
    Write-Host "Launch zips:   tag vX.Y.Z and push. See docs/RELEASE.md"
    if ($ProxyImage) { Build-ProxyImage }
    exit 0
}

$outName = if ($Platform -eq "windows") { "siding.exe" } else { "siding" }
$outPath = "/src/bin/$outName"
$inner = "go mod tidy; CGO_ENABLED=0 GOOS=$Platform GOARCH=$Arch go build -ldflags='-s -w' -o $outPath ./cmd/siding"

Write-Host "Building $Platform/$Arch -> bin\$outName ..."
& docker compose -f docker-compose.dev.yml run --rm dev @("sh", "-c", $inner)
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$hostPath = Join-Path $BinDir $outName
Write-Host ""
Write-Host "Built: $hostPath"
Write-Host ""
Write-Host "PowerShell cannot run binaries from the current folder by name."
Write-Host "  .\bin\$outName version"
Write-Host "  .\scripts\install.ps1          # then: siding version"
if ($ProxyImage) { Build-ProxyImage }
