#!/usr/bin/env python3
"""Evaluate repeatable WAV-to-APRS text decoding, without live radio or gateway I/O."""

import argparse
from collections import Counter
import json
from pathlib import Path
import statistics
import subprocess
import sys
import time
import wave


def decoded_frames(output: str) -> list[str]:
    """multimon-ng prints decoded TNC2 frames on lines prefixed with APRS:."""
    frames = []
    for line in output.splitlines():
        line = line.strip()
        if line.startswith("APRS:"):
            frame = line.removeprefix("APRS:").strip()
            if frame:
                frames.append(frame)
    return frames


def compare(expected: list[str], observed: list[str]) -> dict:
    # Multisets preserve repeated packets while ignoring decoder print ordering.
    correct = sum((Counter(expected) & Counter(observed)).values())
    missed = len(expected) - correct
    extra = len(observed) - correct
    return {
        "expected": len(expected),
        "decoded": len(observed),
        "correct": correct,
        "missed": missed,
        "extra": extra,
        "recall": correct / len(expected) if expected else None,
        "precision": correct / len(observed) if observed else None,
    }


def run_decoder(decoder: str, wav_path: Path, timeout: float) -> tuple[list[str], float]:
    # No shell, substitutions, network calls, or invocation of Sparky itself.
    args = [decoder, "-a", "AFSK1200", "-A", "-t", "wav", str(wav_path)]
    started = time.perf_counter()
    proc = subprocess.run(
        args, capture_output=True, text=True, errors="replace", timeout=timeout, check=False
    )
    elapsed_ms = (time.perf_counter() - started) * 1000
    if proc.returncode != 0:
        raise RuntimeError(
            f"decoder exited {proc.returncode}: {proc.stderr[-400:].strip()}"
        )
    return decoded_frames(proc.stdout), elapsed_ms


def evaluate(wav_path: Path, expected: list[str], decoder: str, repeats: int,
             timeout: float) -> dict:
    with wave.open(str(wav_path), "rb") as audio:
        if audio.getnframes() == 0:
            raise ValueError("WAV must contain audio frames")
        audio_meta = {
            "sample_rate_hz": audio.getframerate(),
            "channels": audio.getnchannels(),
            "sample_width_bytes": audio.getsampwidth(),
            "duration_seconds": audio.getnframes() / audio.getframerate(),
        }
    runs = []
    for _ in range(repeats):
        frames, elapsed_ms = run_decoder(decoder, wav_path, timeout)
        runs.append({"elapsed_ms": round(elapsed_ms, 3), **compare(expected, frames)})
    return {
        "format_version": 1,
        "wav": str(wav_path),
        "decoder": decoder,
        "repeats": repeats,
        "audio": audio_meta,
        "runs": runs,
        "median_process_elapsed_ms": round(
            statistics.median(run["elapsed_ms"] for run in runs), 3
        ),
        "consistent_frames_across_runs": all(
            (r["correct"], r["missed"], r["extra"], r["decoded"])
            == (runs[0]["correct"], runs[0]["missed"], runs[0]["extra"], runs[0]["decoded"])
            for r in runs
        ),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wav", type=Path, required=True, help="Existing PCM WAV recording")
    parser.add_argument("--expected", type=Path, required=True,
                        help="UTF-8 text file: one expected TNC2 APRS frame per line")
    parser.add_argument("--decoder", default="multimon-ng",
                        help="Path/name of multimon-ng-compatible executable")
    parser.add_argument("--repeat", type=int, default=5,
                        help="Number of identical whole-file decode runs (default: 5)")
    parser.add_argument("--timeout", type=float, default=60,
                        help="Timeout in seconds per decode run (default: 60)")
    parser.add_argument("--output", type=Path, help="Write JSON report to this file")
    args = parser.parse_args(argv)
    if not 1 <= args.repeat <= 100:
        parser.error("--repeat must be in 1..100")
    if not 0 < args.timeout <= 3600:
        parser.error("--timeout must be in (0, 3600]")
    try:
        expected = [line.strip() for line in args.expected.read_text(encoding="utf-8").splitlines()
                    if line.strip()]
        if not expected:
            raise ValueError("expected packet file must contain at least one frame")
        result = evaluate(args.wav, expected, args.decoder, args.repeat, args.timeout)
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired, wave.Error) as exc:
        print(f"replay error: {exc}", file=sys.stderr)
        return 2
    output = json.dumps(result, indent=2, sort_keys=True) + "\n"
    if args.output:
        try:
            args.output.write_text(output, encoding="utf-8")
        except OSError as exc:
            print(f"unable to write result: {exc}", file=sys.stderr)
            return 2
    else:
        print(output, end="")
    # Nonzero when the WAV does not decode as the expected multiset.
    return 0 if all(run["missed"] == 0 and run["extra"] == 0 for run in result["runs"]) else 1


if __name__ == "__main__":
    sys.exit(main())
