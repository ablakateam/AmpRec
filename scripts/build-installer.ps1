param(
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version = '1.1.0',
    [switch]$SkipAppBuild
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

if (-not $SkipAppBuild) {
    & (Join-Path $PSScriptRoot 'build.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Application build failed.' }
}

$compilerCandidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
    (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe')
)
$compiler = $compilerCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $compiler) {
    throw 'Inno Setup 6 was not found. Install JRSoftware.InnoSetup with winget.'
}

$script = Join-Path $repoRoot 'installer\AmpRec.iss'
& $compiler "/DAppVersion=$Version" $script
if ($LASTEXITCODE -ne 0) { throw 'Installer build failed.' }

$output = Join-Path $repoRoot "dist\AmpRec-Setup-$Version-win-x64.exe"
if (-not (Test-Path -LiteralPath $output)) { throw "Installer output was not created: $output" }
Write-Host "Built $output"
