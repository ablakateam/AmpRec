# AmpRec Architecture

## Runtime flow

Process discovery groups `winamp.exe`, `chrome.exe`, `msedge.exe`, `firefox.exe`, and `brave.exe` into independent root trees. The persisted source choice and current root PID feed a deterministic application state machine: unavailable, ready, recording, finalizing, or error. The UI and tray invoke the same controller, so there is only one recording lifecycle.

The capture worker activates `VAD\Process_Loopback` with `AUDIOCLIENT_ACTIVATION_TYPE_PROCESS_LOOPBACK` and `PROCESS_LOOPBACK_MODE_INCLUDE_TARGET_PROCESS_TREE`. Browser renderer and audio subprocesses are therefore included while unrelated process trees remain excluded. WASAPI supplies 44.1 kHz, stereo PCM16 packets through an event-driven `IAudioCaptureClient`. For Winamp, AmpRec reads the player IPC volume and Windows audio-session scalar, then applies their bounded inverse once to prevent double attenuation on replay. PCM saturation protects the encoder from integer overflow. A 32-packet channel separates capture from storage and bounds memory use. Queue saturation stops with an explicit error rather than silently dropping audio.

## Encoding and storage

WAV output uses an explicit RIFF writer and patches chunk sizes after the final packet. MP3 output uses the inbox Media Foundation MP3 MFT through `IMFSinkWriter` at 128, 192, 256, or 320 kbps. Both formats write to a sibling `.part` file, flush and close it, and then atomically rename it to the visible destination.

## Native shell

The executable uses a custom-drawn, DPI-aware Win32 window and a native notification-area icon/menu. Settings are a small JSON document under `%LOCALAPPDATA%\AmpRec`. The optional startup entry is scoped to the current user at `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

## Ownership and shutdown

Every COM interface, event, GDI object, tray icon, file, and goroutine has one owner and a defined release path. Normal exit first signals capture, drains queued PCM, finalizes the encoder, renames the file, removes the tray icon, and destroys the window. A selected application exit follows the same finalization path and surfaces a truthful status.
