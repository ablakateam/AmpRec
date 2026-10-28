# Repository Guidelines

## Project Structure & Module Organization

AmpRec is a portable Windows 11 x64 recorder for process-specific Winamp and browser audio. Treat `AmpRec_Windows_11_Product_Design_and_Codex_Spec.pdf` and the screens in `assets/mockups/` as the baseline behavioral and visual source of truth.

- `cmd/amprec/`: executable entry point, manifest, and Windows resources.
- `internal/recording/`: WASAPI capture, Media Foundation MP3, WAV output, and lifecycle control.
- `internal/ui/`: custom Win32 window, drawing, input, and tray behavior.
- `internal/config/` and `internal/win32/`: settings and native API helpers.
- `testtools/`: deterministic audio and window harnesses used by integration tests.
- `scripts/`: repeatable build and test entry points.
- `docs/`: architecture, validation results, and audit notes.

Generated files belong under `build/`, `dist/`, or `tmp/`. Do not commit local recordings.

## Build, Test, and Development Commands

Use 64-bit PowerShell on Windows:

```powershell
.\scripts\test.ps1
.\scripts\test-integration.ps1
.\scripts\build.ps1
.\scripts\build-installer.ps1
```

The first command runs unit, race, WAV, and MP3 integrity tests. The integration script proves process isolation and child-renderer capture through a browser-shaped process tree. The build scripts create the standalone executable and the per-user Inno Setup package.

## Coding Style & Naming Conventions

Use Go 1.27 or later (latest patch release) and run `gofmt` on every Go change. Follow standard Go naming: exported identifiers use `PascalCase`, package-private identifiers use `camelCase`, and package names are short lowercase words. Keep Windows-only files suffixed `_windows.go`. Wrap each COM interface, handle, file, GDI object, and goroutine with explicit ownership and cleanup. Avoid `unsafe` outside the native boundary.

## Testing Guidelines

Name tests `TestBehavior` in `_test.go` files. Never claim real-machine behavior without running it. Validate output with FFprobe when available. Changes to capture must rerun `test-integration.ps1`; UI changes require screenshots at 100% and 150% DPI plus tray close/restore checks. Record manual gaps in `docs/test-report.md`.

## Commit & Pull Request Guidelines

Use short, imperative subjects, optionally scoped, such as `capture: handle Winamp process exit`. Pull requests must describe user-visible changes, list exact automated and manual checks, link issues, and include screenshots for UI work. Call out capture, encoder, file-recovery, or compatibility risks.

## Security & Product Constraints

Do not add telemetry, networking, administrator requirements, injection, virtual drivers, or system-wide loopback fallback. Preserve atomic finalization and protected-content restrictions.
