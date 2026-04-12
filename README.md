# NotSureYet

A personal productivity tool that captures system audio, transcribes it, and (soon) extracts key notes via LLM agents.

## Pipeline

Each recording gets its own session folder with all related files together:

```
~/AudioCapture/2026-04-12_14-20-00_lecture/
├── recording.wav       ← AudioCapture (Phase 1)
├── transcript.json     ← Transcriber (Phase 2) — for pipeline
├── transcript.txt      ← Transcriber (Phase 2) — for you
└── notes.json          ← Phase 3 (coming) — extracted key points
```

## Phase 1: AudioCapture

A macOS CLI tool that captures system audio via Core Audio Taps API.

### How It Works

```
System Audio (48kHz Float32)
     |
     v
AudioTapManager ---- Creates a "tap" on all system audio.
     |                Audio still plays through speakers normally.
     v
AudioProcessor ----- Receives raw audio on a real-time thread, dispatches to a
     |                background queue, converts 48kHz Float32 -> 16kHz Int16 mono.
     v
AudioWriter -------- Writes converted PCM data to a WAV file (or raw to stdout).
```

### Files

| File | Role |
|------|------|
| `AudioCapture.swift` | Entry point. Parses CLI args, wires modules, handles shutdown. |
| `AudioTapManager.swift` | Talks to macOS Core Audio. Creates tap + aggregate device. |
| `AudioProcessor.swift` | Converts audio format. Bridges real-time thread to background queue. |
| `AudioWriter.swift` | `AudioWriting` protocol + WAV file writer + stdout writer. |
| `Errors.swift` | All error types with human-readable messages. |

### Usage

```bash
# Build and install (one-time)
swift build -c release && sudo cp .build/release/AudioCapture /usr/local/bin/AudioCapture

# Record until Ctrl+C
AudioCapture

# Record for 30 seconds
AudioCapture -d 30

# Record with a label for easy identification
AudioCapture -d 60 -l teams
AudioCapture -d 30 -l discord
AudioCapture -l facetime

# Custom output path (overrides session folder)
AudioCapture -o ~/Desktop/custom.wav -d 10

# Verbose mode (shows format info)
AudioCapture -d 10 -v

# Stream raw PCM to stdout (for piping to other tools)
AudioCapture --raw-stdout | python transcribe.py

# Show all options
AudioCapture -h
```

### Default Output

Each recording creates a session folder in `~/AudioCapture/`:

```
~/AudioCapture/
├── 2026-04-12_13-45-30/
│   └── recording.wav
├── 2026-04-12_14-20-00_lecture/
│   └── recording.wav
├── 2026-04-12_19-30-15_facetime/
│   └── recording.wav
```

## Phase 2: Transcriber

A Python script that transcribes WAV files using MLX Whisper (Apple Silicon optimized).

### Setup

```bash
cd transcriber
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

### Usage

```bash
# Activate the virtual environment first
source transcriber/.venv/bin/activate

# Transcribe a session folder
python transcriber/transcribe.py ~/AudioCapture/2026-04-12_14-20-00_lecture/

# Transcribe all sessions
python transcriber/transcribe.py ~/AudioCapture/

# Use a different model (tiny, base, small, medium, large)
python transcriber/transcribe.py -m mlx-community/whisper-small-mlx ~/AudioCapture/2026-04-12_14-20-00_lecture/

# Show all options
python transcriber/transcribe.py -h
```

### Output

Transcripts are saved alongside the recording in the same session folder:

```json
{
  "source": "/path/to/recording.wav",
  "text": "the full transcript as one string",
  "segments": [
    {
      "start": 0.0,
      "end": 4.5,
      "text": "what was said in this chunk",
      "words": [{"word": "each", "start": 0.1, "end": 0.3}]
    }
  ]
}
```

## Audio Format

16kHz, mono, 16-bit PCM WAV — the standard input for Whisper.

## Requirements

- macOS 15+
- Screen Recording permission (prompted on first run)
- Python 3.10+ (for transcriber)