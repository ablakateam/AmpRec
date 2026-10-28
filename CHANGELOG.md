# Changelog

All notable AmpRec changes are documented here.

## [1.1.0] - 2026-10-28

### Added

- Source selector for Google Chrome, Microsoft Edge, Mozilla Firefox, and Brave.
- Browser root-process grouping with capture of renderer/audio child processes.
- Persistent application selection and application-specific tray/status messages.
- Per-user Windows installer with Start menu and optional desktop shortcuts.

### Verified

- Direct Winamp-style process capture to MP3.
- Browser child-renderer capture through WASAPI Process Loopback.
- Exclusion of unrelated application audio.
- Volume compensation regression, tray lifecycle, process loss, and atomic finalization.

## [1.0.1] - 2026-10-26

### Fixed

- Compensated Winamp and Windows session attenuation to prevent lower-volume recordings on replay.

## [1.0.0] - 2026-10-26

- Initial portable Windows release with Winamp process-loopback capture, MP3/WAV output, native UI, and tray controls.
