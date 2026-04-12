"""
Transcribe WAV audio files using MLX Whisper (Apple Silicon optimized).

Takes WAV files produced by AudioCapture and saves transcripts alongside
them in the same session folder. Each transcript includes the full text
and timestamped segments for downstream processing.

Usage:
    python transcribe.py ~/AudioCapture/2026-04-12_13-45-30_lecture/
    python transcribe.py ~/AudioCapture/2026-04-12_13-45-30_lecture/recording.wav
    python transcribe.py ~/AudioCapture/
"""

import argparse
import json
import sys
from pathlib import Path

import mlx_whisper

# The Whisper model to use. "base" gives 100% accuracy on English
# benchmarks while running at 18x real-time on M4 Pro.
# Options: tiny, base, small, medium, large
DEFAULT_MODEL = "mlx-community/whisper-base-mlx"


def transcribe_file(wav_path: Path, model: str) -> dict:
    """Run Whisper on a single WAV file and return the result."""
    print(f"Transcribing: {wav_path.parent.name}/{wav_path.name}", file=sys.stderr)

    result = mlx_whisper.transcribe(
        str(wav_path),
        path_or_hf_repo=model,
        word_timestamps=True,
    )

    return result


def save_transcript(result: dict, wav_path: Path) -> Path:
    """Save transcription results next to the WAV file in the same folder."""
    output_dir = wav_path.parent

    # Build a clean output structure
    transcript = {
        "source": str(wav_path),
        "text": result.get("text", "").strip(),
        "segments": [],
    }

    for seg in result.get("segments", []):
        segment = {
            "start": round(seg["start"], 2),
            "end": round(seg["end"], 2),
            "text": seg["text"].strip(),
        }

        # Include word-level timestamps if available
        if "words" in seg:
            segment["words"] = [
                {
                    "word": w["word"].strip(),
                    "start": round(w["start"], 2),
                    "end": round(w["end"], 2),
                }
                for w in seg["words"]
            ]

        transcript["segments"].append(segment)

    # Save JSON (for pipeline)
    json_path = output_dir / "transcript.json"
    with open(json_path, "w") as f:
        json.dump(transcript, f, indent=2, ensure_ascii=False)

    # Save plain text (for humans)
    txt_path = output_dir / "transcript.txt"
    with open(txt_path, "w") as f:
        for seg in transcript["segments"]:
            f.write(seg["text"] + "\n")

    return json_path


def find_wav_files(path: Path) -> list[Path]:
    """Find WAV files from a path.

    Handles three cases:
      - A WAV file directly
      - A session folder containing recording.wav
      - A parent directory containing session folders
    """
    if path.is_file() and path.suffix.lower() == ".wav":
        return [path]
    elif path.is_dir():
        # Check if this is a session folder (contains a WAV directly)
        direct_wavs = sorted(path.glob("*.wav"))
        if direct_wavs:
            return direct_wavs

        # Otherwise search subdirectories (session folders)
        nested_wavs = sorted(path.glob("*/recording.wav"))
        if nested_wavs:
            return nested_wavs

        # Fallback: any WAV in any subfolder
        all_wavs = sorted(path.glob("**/*.wav"))
        if not all_wavs:
            print(f"No WAV files found in {path}", file=sys.stderr)
        return all_wavs
    else:
        print(f"Not a WAV file or directory: {path}", file=sys.stderr)
        return []


def main():
    parser = argparse.ArgumentParser(
        description="Transcribe WAV files using MLX Whisper."
    )
    parser.add_argument(
        "input",
        help="WAV file, session folder, or parent directory to transcribe.",
    )
    parser.add_argument(
        "-m", "--model",
        default=DEFAULT_MODEL,
        help=f"Whisper model to use (default: {DEFAULT_MODEL}).",
    )
    args = parser.parse_args()

    input_path = Path(args.input).expanduser().resolve()

    wav_files = find_wav_files(input_path)
    if not wav_files:
        sys.exit(1)

    print(f"Found {len(wav_files)} file(s) to transcribe.", file=sys.stderr)

    for wav_path in wav_files:
        result = transcribe_file(wav_path, args.model)
        json_path = save_transcript(result, wav_path)

        text = result.get("text", "").strip()
        preview = text[:100] + "..." if len(text) > 100 else text
        print(f"  → {json_path.parent.name}/transcript.json ({len(text)} chars)", file=sys.stderr)
        print(f"    \"{preview}\"", file=sys.stderr)

    print(f"\nDone. {len(wav_files)} file(s) transcribed.", file=sys.stderr)


if __name__ == "__main__":
    main()