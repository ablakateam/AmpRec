# Contributing to AmpRec

Thank you for helping improve AmpRec. Keep changes focused, local-only, and compatible with its process-isolated capture model.

## Development setup

Use Windows x64, Go 1.26 or later, and a functional Windows audio endpoint. Run:

```powershell
.\scripts\test.ps1
.\scripts\test-integration.ps1
.\scripts\build.ps1
```

Run `gofmt` on Go changes. Keep Windows-only files suffixed `_windows.go`, pair every native handle or COM reference with a clear release path, and avoid new dependencies unless they materially reduce risk.

## Pull requests

- Explain the user-visible behavior and technical approach.
- Link related issues.
- List the exact automated and manual checks performed.
- Include 100% and 150% DPI screenshots for UI changes.
- Call out capture, encoding, file-recovery, privacy, or compatibility risks.
- Never report real-machine capture as passing unless it was actually exercised.

AmpRec will not accept telemetry, cloud requirements, administrator-only operation, injection, virtual audio drivers, microphone capture, or endpoint-wide loopback fallback.
