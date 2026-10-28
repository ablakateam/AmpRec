$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go -and (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe')) {
    $go = Get-Item 'C:\Program Files\Go\bin\go.exe'
}
if (-not $go) { throw 'Go 1.27 or later is required. Install the latest patch release from https://go.dev/dl/.' }
$goExe = if ($go.Source) { $go.Source } else { $go.FullName }

$dist = Join-Path $repoRoot 'dist'
New-Item -ItemType Directory -Force -Path $dist | Out-Null

& $goExe test ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }

$previousCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    & $goExe build -trimpath -buildvcs=false -ldflags '-s -w -H=windowsgui -X main.version=1.1.0' -o (Join-Path $dist 'AmpRec.exe') ./cmd/amprec
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
} finally {
    if ($null -eq $previousCgo) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    } else {
        $env:CGO_ENABLED = $previousCgo
    }
}

Write-Host "Built $dist\AmpRec.exe"
