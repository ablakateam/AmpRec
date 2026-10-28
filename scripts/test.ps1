$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go -and (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe')) {
    $go = Get-Item 'C:\Program Files\Go\bin\go.exe'
}
if (-not $go) { throw 'Go 1.26 or later is required.' }
$goExe = if ($go.Source) { $go.Source } else { $go.FullName }

Push-Location $repoRoot
try {
    & $goExe test -race ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
} finally {
    Pop-Location
}
