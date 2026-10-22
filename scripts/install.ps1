# Install the host siding.exe so this PowerShell can run `siding` (no .\ prefix).
#
# PowerShell does not search the current directory. This copies the exe to
# %LOCALAPPDATA%\siding\bin, puts that folder on your user PATH, and prepends
# it for THIS session so `siding version` works immediately.
#
#   .\scripts\install.ps1
#   .\scripts\install.ps1 -Build

param(
    [switch] $Build
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$binExe = Join-Path $Root "bin\siding.exe"
$dist = Join-Path $Root "dist\siding_windows_amd64.exe"
if ($Build) {
    & (Join-Path $Root "docker\build.ps1")
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
$src = $null
if (Test-Path $binExe) { $src = $binExe }
elseif (Test-Path $dist) { $src = $dist }
if (-not $src) {
    Write-Error "missing bin\siding.exe - run .\docker\build.ps1 first"
}

$bin = Join-Path $env:LOCALAPPDATA "siding\bin"
New-Item -ItemType Directory -Path $bin -Force | Out-Null
$dest = Join-Path $bin "siding.exe"
Copy-Item -Path $src -Destination $dest -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = @()
if ($userPath) {
    $parts = @($userPath -split ";" | Where-Object { $_ -ne "" })
}
$onPath = $false
foreach ($p in $parts) {
    if ([string]::Equals($p, $bin, [System.StringComparison]::OrdinalIgnoreCase)) {
        $onPath = $true
        break
    }
}
if (-not $onPath) {
    if ($userPath) {
        $new = $bin + ";" + $userPath
    } else {
        $new = $bin
    }
    [Environment]::SetEnvironmentVariable("Path", $new, "User")
}

$env:Path = $bin + ";" + $env:Path

Write-Host "Installed $dest"
Write-Host ""
& $dest version
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host ""
Write-Host "This session:  siding version"
Write-Host "New terminals pick up PATH automatically."
