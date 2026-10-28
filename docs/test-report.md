# AmpRec Test Report

- Date: 2026-10-28
- Host: Windows 11 x64, 150% display scaling
- Toolchain: Go 1.27.3, `golang.org/x/sys` v0.49.0, FFmpeg/FFprobe available

## Automated verification

- `scripts/test.ps1`: passed all packages under the Go race detector. Tests cover settings normalization, PCM peak calculation, output naming, RIFF header finalization, block alignment, Media Foundation MP3 encoding, and WAV/MP3 duration validation through FFprobe.
- `scripts/test-integration.ps1`: passed. Two native renderers played 440 Hz and 880 Hz simultaneously. Process-loopback captured only the selected renderer; the latest measured Goertzel energy ratio was 21,918,277.3:1. Separate end-to-end tests produced finalized 320 kbps MP3s from a direct renderer and from a browser-shaped root whose child renderer owned the audio stream.
- `go vet -unsafeptr=false ./...`: passed. The disabled analyzer reports intentional COM vtable conversions at the native boundary.
- `govulncheck ./...`: no vulnerabilities found after upgrading to Go 1.27.3 and `x/sys` v0.49.0.
- Inno Setup 6.7.3 compiled the per-user 1.1.0 installer. A silent isolated install returned exit code 0, produced an `AmpRec.exe` matching the release SHA-256 exactly, registered an uninstaller, and removed the test installation cleanly with exit code 0. The user's existing portable AmpRec process remained running throughout.
- Volume-regression test with the reported recording: the current build replay/recapture measured -33.8 LUFS and -20.4 dBFS peak from a -32.6 LUFS, -19.1 dBFS source. With compensation, the result measured -32.4 LUFS and the original -19.1 dBFS peak.

## Native UI and lifecycle verification

The release executable was run on the host at 150% DPI. Captured frames were compared with every supplied mockup. Ready, recording, saved, settings, format menu, multiple-source menu, keyboard focus, and process-exit states rendered without clipping or overlap.

Verified behaviors:

- Winamp detection before launch and detection of a second `winamp.exe` after launch.
- Root-process grouping for Winamp, Chrome, Edge, Firefox, and Brave; browser child processes are not shown as separate sources.
- The source selector recognized the installed Chrome and Firefox roots among dozens of subprocesses, persisted Chrome selection, and rendered correctly at 150% DPI.
- Single-instance activation.
- Start/stop and valid MP3 finalization at approximately 320 kbps.
- Closing the panel during capture kept recording active; tray restore showed an advanced timer.
- Tray Exit during capture finalized the file and left no `.part` file.
- Selected-process exit finalized safely and displayed an explicit status.
- Ten repeated MP3 cycles stabilized at 20-25 threads, 763-771 handles, and about 31-32 MB private memory after Media Foundation initialization.
- A 45-second idle browser-discovery soak plateaued at 238-239 handles after runtime initialization; shutdown left no AmpRec process.
- The final 1.1.0 Windows ZIP was extracted, launched, rendered, and exited successfully.

## Not verified on this host

- The installed Winamp instance was idle. Real Winamp playback of MP3, FLAC, and internet radio was not available for content-specific testing.
- Playback from installed Chrome, Edge, Firefox, and Brave was not manually exercised. Browser process-tree behavior was verified with native `chrome.exe` root and child-renderer fixtures.
- Disk-full and access-denied faults were not induced. Invalid output paths are covered automatically.
- 100% DPI, multi-monitor DPI transitions, classic 4 GiB WAV exhaustion, protected streams, and unusual exclusive-mode output plugins were not exercised.
- The executable is not Authenticode-signed.
