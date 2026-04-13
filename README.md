# AutoNote

Capture system audio, transcribe it, and extract notes — all automated. Includes a CLI pipeline and a native macOS desktop app.

## How It Works

```
Audio In → AudioCapture → Transcriber → Claude Code → Notes
(48kHz WAV)   (MLX Whisper)   (LLM note-taking)
```

Each session gets its own folder with everything together:

```
~/AudioCapture/2026-04-12_lecture/
├── recording.wav       ← captured audio
├── transcript.json     ← timestamped transcript (for pipeline)
├── transcript.txt      ← plain text transcript (for you)
└── notes.md            ← extracted notes (by Claude)
```

## Quick Start

```bash
# Full pipeline — record, transcribe, and take notes
autonote LTW -l lecture -p lecture

# Just record audio
autonote L -l meeting

# Record and transcribe (no notes)
autonote LT -l call

# Generate notes from an existing session
autonote W ~/AudioCapture/2026-04-12_lecture/ -p lecture

# List available profiles
autonote profiles
```

## Installation

### Prerequisites

- macOS 15+
- Screen Recording permission (prompted on first run)
- Python 3.10+
- Go 1.21+
- Node.js + Bun (for the desktop app)

### Build & Install

```bash
# 1. AudioCapture (Swift)
swift build -c release
sudo cp .build/release/AudioCapture /usr/local/bin/AudioCapture
sudo codesign --force --sign - /usr/local/bin/AudioCapture

# 2. Transcriber (Python)
cd transcriber
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
sudo ln -sf $(pwd)/transcribe /usr/local/bin/transcribe

# 3. Autonote (Go)
cd notate
go build -o autonote .
sudo cp autonote /usr/local/bin/autonote
sudo codesign --force --sign - /usr/local/bin/autonote

# 4. Install profiles
mkdir -p ~/.notate/profiles
cp profiles/*.md ~/.notate/profiles/

# 5. Desktop app (optional)
cd app
bun install
bun run tauri dev
```

## Project Structure

```
├── Sources/AudioCapture/    Swift CLI — captures macOS system audio
├── transcriber/             Python — MLX Whisper speech-to-text
├── notate/                  Go — orchestrator + Claude Code integration
├── profiles/                Context profiles for different scenarios
├── app/                     Tauri + React desktop app
│   ├── src/                 React frontend
│   ├── src-tauri/           Rust backend
│   └── tray.html            System tray panel
└── README.md
```

## CLI Commands

| Command | What It Does |
|---------|-------------|
| `autonote L` | Record audio only |
| `autonote LT` | Record + transcribe |
| `autonote LTW` | Record + transcribe + notes (full pipeline) |
| `autonote T <dir>` | Transcribe existing recording |
| `autonote TW <dir>` | Transcribe + notes |
| `autonote W <dir>` | Notes from existing transcript |
| `autonote profiles` | List available profiles |
| `autonote delete <dir>` | Delete a session |

### Options

```
-l, --label <name>         Session label (e.g., lecture, meeting)
-p, --profile <name>       Context profile (default: lecture)
-e, --extra-prompt <text>  Additional instructions for the note-taker
-i, --interval <dur>       Processing interval (default: 60s)
-d, --duration <dur>       Recording duration (default: until Ctrl+C)
```

## Profiles

Profiles tell the LLM what to extract. Stored in `~/.notate/profiles/`:

| Profile | Purpose |
|---------|---------|
| `lecture` | Assignments, exam info, key concepts |
| `personal` | Places, plans, preferences, things to remember |
| `meeting` | Action items, decisions, follow-ups |
| `interview` | Questions asked, key topics, next steps |

Create custom profiles by adding a `.md` file to `~/.notate/profiles/`.

## Desktop App

The Tauri app provides a visual interface for managing sessions:

- **Left sidebar** — browse sessions, search, manage profiles
- **Center** — rich text editor (TipTap) for viewing/editing notes
- **Right sidebar** — embedded terminal for Claude Code chat, session info
- **System tray** — start/stop recordings from the menu bar

### Run the app

```bash
cd app && bun run tauri dev
```

## Audio Format

48kHz, mono, 16-bit PCM WAV — native system quality. Whisper handles resampling internally.

## Tests

```bash
# Go tests
cd notate && go test ./...

# Python tests
cd transcriber && source .venv/bin/activate && python test_transcribe.py
```

## Requirements

- macOS 15+ (Apple Silicon recommended)
- Screen Recording permission
- Python 3.10+ with MLX Whisper
- Claude Code CLI (for note generation)
