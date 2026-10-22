# Verify the public siding library builds the way an external Go module would.
# Used by CI, GoReleaser before hooks, and release docs.
#
#   .\scripts\verify-library.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

Write-Host "go test ./siding/..."
go test ./siding/...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "go build -C testdata/externalconsumer ."
go build -C testdata/externalconsumer -o $env:TEMP\siding-externalconsumer-test.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Library release checks passed."
