# AmpRec Engineering Audit

## Scope and result

The Go/Win32 implementation was reviewed for capture isolation, file integrity, concurrency, native resource ownership, error recovery, privacy, and release portability. No unresolved high-severity source defect was found in the tested paths.

## Correctness and resource ownership

- WASAPI activation uses `AUDIOCLIENT_ACTIVATION_TYPE_PROCESS_LOOPBACK` with `PROCESS_LOOPBACK_MODE_INCLUDE_TARGET_PROCESS_TREE`; there is no endpoint-loopback fallback.
- Source discovery selects one independent Winamp, Chrome, Edge, Firefox, or Brave root. A browser integration test verified capture from its child renderer, and the isolation test excluded an unrelated renderer.
- The agile COM activation callback fixed an initial `E_ILLEGAL_METHOD_CALL` found by live testing.
- Capture uses event-driven packet draining and a bounded 32-packet producer/consumer channel. Saturation stops with an explicit error instead of dropping audio.
- Capture compensates Winamp and Windows session attenuation once, capped at +12 dB with PCM saturation. A controlled replay/recapture improved from -1.2 dB relative loss to within 0.2 dB of the source.
- COM interfaces, events, files, GDI objects, tray state, and worker goroutines have paired release paths. Race-enabled tests passed.
- WAV sizes are patched and flushed on close. Media Foundation samples carry monotonic time and duration. Successful files are atomically renamed from `.part`; recoverable failures use a `_recovered` name.
- Process loss, normal stop, panel close, tray exit, and session shutdown converge on the same finalization path.

## Security and privacy

AmpRec has no network code, telemetry, cloud service, updater, injection, virtual driver, administrator requirement, or microphone path. Settings are per-user and local. Filenames are generated internally, output files use owner-oriented permissions, and user-selected paths are validated as directories.

The sole external Go module is `golang.org/x/sys` v0.49.0, statically linked. `govulncheck` reported no vulnerabilities with the Go 1.27.3 release toolchain. The release build uses `CGO_ENABLED=0`, strips symbols, embeds an as-invoker manifest, and requires no Go runtime. The optional Inno Setup package installs per user with lowest privileges and includes a standard uninstaller; the portable build remains available.

## Performance

Capture requests stereo PCM16 at 44.1 kHz and lets shared-mode WASAPI perform supported conversion. Encoding is streamed; no whole-recording buffer exists. UI drawing is double-buffered and timer work is small. Process enumeration closes every snapshot handle; an idle multi-browser scan soak plateaued at 238-239 handles after initialization. Post-initialization memory and handles also stabilized during repeated recording cycles.

## Residual release considerations

The binary is unsigned, so Windows may show an unknown-publisher warning. Production distribution should add organization-controlled Authenticode signing. The classic WAV writer intentionally stops before the RIFF 4 GiB limit rather than emitting RF64. See `docs/test-report.md` for hardware and scenario coverage gaps.
