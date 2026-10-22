# Run siding dev commands inside Docker (no local Go required).
#
#   .\docker\dev.ps1 test
#   .\docker\dev.ps1 run version
#   .\docker\dev.ps1 run plan --path examples/hello --json
#   .\docker\dev.ps1 doctor --path examples/hello
#   .\docker\dev.ps1 shell
#   .\docker\dev.ps1 gen-checksum-key   # writes .docker/checksum-sign/ (gitignored)

param(
    [Parameter(Position = 0)]
    [ValidateSet("test", "run", "doctor", "shell", "build", "tidy", "gen-checksum-key")]
    [string] $Command = "test",

    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $Rest
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$DataDir = Join-Path $Root ".docker\siding-data"
if (-not (Test-Path $DataDir)) {
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
}

function Invoke-Compose {
    param(
        [string] $Service,
        [string[]] $ComposeArgs
    )
    # Do not name this $Args — that shadows PowerShell's automatic $args.
    & docker compose -f docker-compose.dev.yml run --rm $Service @ComposeArgs
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

switch ($Command) {
    "test" {
        Invoke-Compose "dev" @("sh", "-c", "go mod tidy && go test ./...")
    }
    "tidy" {
        Invoke-Compose "dev" @("sh", "-c", "go mod tidy")
    }
    "build" {
        & (Join-Path $Root "docker\build.ps1")
    }
    "run" {
        if ($Rest.Count -eq 0) {
            Write-Error "Usage: .\docker\dev.ps1 run <siding-args...>  e.g. run plan --path examples/hello"
        }
        $argsLine = ($Rest | ForEach-Object {
            if ($_ -match '\s') { "'$_'" } else { $_ }
        }) -join " "
        Invoke-Compose "dev" @("sh", "-c", "go mod tidy && go run ./cmd/siding $argsLine")
    }
    "doctor" {
        $argsLine = if ($Rest.Count -gt 0) {
            ($Rest | ForEach-Object {
                if ($_ -match '\s') { "'$_'" } else { $_ }
            }) -join " "
        } else {
            "--path examples/hello"
        }
        Invoke-Compose "doctor" @("sh", "-c", "go mod tidy && go run ./cmd/siding doctor $argsLine")
    }
    "shell" {
        Invoke-Compose "dev" @("sh")
    }
    "gen-checksum-key" {
        $Out = Join-Path $Root ".docker\checksum-sign"
        if (-not (Test-Path $Out)) {
            New-Item -ItemType Directory -Path $Out -Force | Out-Null
        }
        # Bind-mount the repo so -out .docker/checksum-sign lands on the host (gitignored).
        # Uses stock golang:1.25 so this works before `docker compose build dev`.
        & docker run --rm `
            -v "${Root}:/src" `
            -w /src `
            golang:1.25 `
            go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        Write-Host "Host files: $Out"
    }
}
