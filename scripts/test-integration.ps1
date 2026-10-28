$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go -and (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe')) {
    $go = Get-Item 'C:\Program Files\Go\bin\go.exe'
}
if (-not $go) { throw 'Go 1.27 or later is required.' }
$goExe = if ($go.Source) { $go.Source } else { $go.FullName }
$testDir = Join-Path $repoRoot 'build\integration'
New-Item -ItemType Directory -Force -Path $testDir | Out-Null

Push-Location $repoRoot
try {
    & $goExe build -trimpath -o (Join-Path $testDir 'winamp.exe') .\testtools\toneplayer
    if ($LASTEXITCODE -ne 0) { throw 'Could not build the Winamp test renderer.' }
    Copy-Item -LiteralPath (Join-Path $testDir 'winamp.exe') -Destination (Join-Path $testDir 'other-audio.exe') -Force
	Copy-Item -LiteralPath (Join-Path $testDir 'winamp.exe') -Destination (Join-Path $testDir 'browser-renderer.exe') -Force
	& $goExe build -trimpath -o (Join-Path $testDir 'chrome.exe') .\testtools\browserhost
	if ($LASTEXITCODE -ne 0) { throw 'Could not build the browser process-tree fixture.' }
    $env:AMPREC_TEST_WINAMP = Join-Path $testDir 'winamp.exe'
    $env:AMPREC_TEST_OTHER_AUDIO = Join-Path $testDir 'other-audio.exe'
    $env:AMPREC_TEST_BROWSER = Join-Path $testDir 'chrome.exe'
    $env:AMPREC_TEST_RENDERER = Join-Path $testDir 'browser-renderer.exe'
    & $goExe test -v -run 'Test(BrowserProcessTreeMP3Capture|ProcessLoopbackExcludesOtherProcess|RecorderMP3ProcessCapture)' .\internal\recording
    if ($LASTEXITCODE -ne 0) { throw 'Process-loopback integration test failed.' }
} finally {
    Remove-Item Env:AMPREC_TEST_WINAMP -ErrorAction SilentlyContinue
    Remove-Item Env:AMPREC_TEST_OTHER_AUDIO -ErrorAction SilentlyContinue
	Remove-Item Env:AMPREC_TEST_BROWSER -ErrorAction SilentlyContinue
	Remove-Item Env:AMPREC_TEST_RENDERER -ErrorAction SilentlyContinue
    Pop-Location
}
