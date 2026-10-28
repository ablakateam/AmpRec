"""Locate a captured excerpt in its source and estimate capture gain."""

from __future__ import annotations

import argparse
import subprocess

import numpy as np
from scipy.signal import fftconvolve


ANALYSIS_RATE = 1000


def decode(path: str) -> np.ndarray:
    command = [
        "ffmpeg",
        "-v",
        "error",
        "-i",
        path,
        "-ac",
        "1",
        "-ar",
        str(ANALYSIS_RATE),
        "-f",
        "f32le",
        "pipe:1",
    ]
    return np.frombuffer(subprocess.check_output(command), dtype="<f4").astype(np.float64)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source")
    parser.add_argument("capture")
    args = parser.parse_args()

    source = decode(args.source)
    capture = decode(args.capture)
    capture -= np.mean(capture)
    if len(capture) >= len(source):
        raise SystemExit("capture must be shorter than source")

    # Remove very quiet edges that otherwise weaken the match.
    edge = min(250, len(capture) // 10)
    query = capture[edge:-edge] if edge else capture
    query -= np.mean(query)
    correlation = fftconvolve(source, query[::-1], mode="valid")
    offset = int(np.argmax(np.abs(correlation))) - edge
    offset = max(0, min(offset, len(source) - len(capture)))
    excerpt = source[offset : offset + len(capture)].copy()
    excerpt -= np.mean(excerpt)

    gain = float(np.dot(capture, excerpt) / np.dot(excerpt, excerpt))
    gain_db = 20 * np.log10(abs(gain))
    residual = capture - gain * excerpt
    similarity = 1 - float(np.dot(residual, residual) / np.dot(capture, capture))
    print(f"source_offset_seconds={offset / ANALYSIS_RATE:.3f}")
    print(f"capture_gain={gain:.6f}")
    print(f"capture_gain_db={gain_db:.2f}")
    print(f"explained_energy={similarity:.6f}")


if __name__ == "__main__":
    main()
