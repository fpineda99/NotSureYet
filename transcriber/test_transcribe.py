"""Tests for the transcriber module."""

import json
import os
import tempfile
from pathlib import Path

# Import from the transcriber module
import sys
sys.path.insert(0, os.path.dirname(__file__))
from transcribe import find_wav_files, save_transcript, _atomic_write


def test_find_wav_file_direct():
    """find_wav_files should return a single file when given a WAV path."""
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        f.write(b"fake wav data")
        path = Path(f.name)

    try:
        result = find_wav_files(path)
        assert len(result) == 1
        assert result[0] == path
    finally:
        path.unlink()


def test_find_wav_files_in_directory():
    """find_wav_files should find WAVs in a directory."""
    with tempfile.TemporaryDirectory() as d:
        wav1 = Path(d) / "test1.wav"
        wav2 = Path(d) / "test2.wav"
        txt = Path(d) / "notes.txt"
        wav1.write_bytes(b"wav1")
        wav2.write_bytes(b"wav2")
        txt.write_text("not a wav")

        result = find_wav_files(Path(d))
        assert len(result) == 2
        names = {r.name for r in result}
        assert "test1.wav" in names
        assert "test2.wav" in names


def test_find_wav_files_nested():
    """find_wav_files should find recording.wav in subdirectories."""
    with tempfile.TemporaryDirectory() as d:
        session = Path(d) / "2026-04-12_session"
        session.mkdir()
        (session / "recording.wav").write_bytes(b"wav")

        result = find_wav_files(Path(d))
        assert len(result) == 1
        assert result[0].name == "recording.wav"


def test_find_wav_files_empty():
    """find_wav_files should return empty list for empty directory."""
    with tempfile.TemporaryDirectory() as d:
        result = find_wav_files(Path(d))
        assert len(result) == 0


def test_find_wav_files_not_a_wav():
    """find_wav_files should reject non-WAV files."""
    with tempfile.NamedTemporaryFile(suffix=".txt", delete=False) as f:
        path = Path(f.name)

    try:
        result = find_wav_files(path)
        assert len(result) == 0
    finally:
        path.unlink()


def test_save_transcript_creates_files():
    """save_transcript should create both JSON and TXT files."""
    with tempfile.TemporaryDirectory() as d:
        wav_path = Path(d) / "recording.wav"
        wav_path.write_bytes(b"fake")

        result = {
            "text": "hello world",
            "segments": [
                {"start": 0.0, "end": 2.5, "text": "hello"},
                {"start": 2.5, "end": 5.0, "text": "world"},
            ],
        }

        json_path = save_transcript(result, wav_path)

        assert json_path.exists()
        assert (Path(d) / "transcript.txt").exists()

        # Verify JSON structure
        with open(json_path) as f:
            data = json.load(f)
        assert data["text"] == "hello world"
        assert len(data["segments"]) == 2
        assert data["source"] == str(wav_path)

        # Verify TXT content
        txt_content = (Path(d) / "transcript.txt").read_text()
        assert "hello" in txt_content
        assert "world" in txt_content


def test_save_transcript_with_word_timestamps():
    """save_transcript should include word-level timestamps if available."""
    with tempfile.TemporaryDirectory() as d:
        wav_path = Path(d) / "recording.wav"
        wav_path.write_bytes(b"fake")

        result = {
            "text": "hello world",
            "segments": [
                {
                    "start": 0.0,
                    "end": 2.5,
                    "text": "hello world",
                    "words": [
                        {"word": "hello", "start": 0.0, "end": 1.2},
                        {"word": "world", "start": 1.3, "end": 2.5},
                    ],
                },
            ],
        }

        json_path = save_transcript(result, wav_path)
        with open(json_path) as f:
            data = json.load(f)

        assert "words" in data["segments"][0]
        assert len(data["segments"][0]["words"]) == 2


def test_atomic_write_success():
    """_atomic_write should create the file with correct content."""
    with tempfile.TemporaryDirectory() as d:
        path = Path(d) / "test.txt"
        _atomic_write(path, "hello world")

        assert path.exists()
        assert path.read_text() == "hello world"


if __name__ == "__main__":
    # Simple test runner
    tests = [
        test_find_wav_file_direct,
        test_find_wav_files_in_directory,
        test_find_wav_files_nested,
        test_find_wav_files_empty,
        test_find_wav_files_not_a_wav,
        test_save_transcript_creates_files,
        test_save_transcript_with_word_timestamps,
        test_atomic_write_success,
    ]

    passed = 0
    failed = 0
    for test in tests:
        try:
            test()
            print(f"  ✓ {test.__name__}")
            passed += 1
        except Exception as e:
            print(f"  ✗ {test.__name__}: {e}")
            failed += 1

    print(f"\n{passed} passed, {failed} failed")
    sys.exit(1 if failed > 0 else 0)
