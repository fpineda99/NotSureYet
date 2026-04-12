# AudioCapture

A macOS CLI tool that captures system audio and saves it as a WAV file optimized for speech transcription.

## How It Works

```
System Audio (48kHz Float32)
     |
     v
AudioTapManager ---- Creates a "tap" on all system audio via Core Audio Taps API.
     |                Audio still plays through speakers normally.
     v
AudioProcessor ----- Receives raw audio on a real-time thread, dispatches to a
     |                background queue, converts 48kHz Float32 -> 16kHz Int16 mono.
     v
AudioWriter -------- Writes converted PCM data to a WAV file (or raw to stdout).
                     WAV header gets finalized on shutdown with actual sizes.
```

The entry point (`AudioCapture.swift`) wires these three together and handles CLI args + signal handling (Ctrl+C).

## Files

| File | Role |
|------|------|
| `AudioCapture.swift` | Entry point. Parses CLI args, wires modules, handles shutdown. |
| `AudioTapManager.swift` | Talks to macOS Core Audio. Creates tap + aggregate device. |
| `AudioProcessor.swift` | Converts audio format. Bridges real-time thread to background queue. |
| `AudioWriter.swift` | `AudioWriting` protocol + WAV file writer + stdout writer. |
| `Errors.swift` | All error types with human-readable messages. |

## Usage

```bash
# Build
swift build

# Install as system CLI (one-time)
swift build -c release && sudo cp .build/release/AudioCapture /usr/local/bin/AudioCapture

# Record until Ctrl+C (auto-named, saved to ~/AudioCapture/recordings/)
AudioCapture

# Record for 30 seconds
AudioCapture -d 30

# Record with a label for easy identification
AudioCapture -d 60 -l lecture
AudioCapture -d 30 -l meeting
AudioCapture -l girlfriend

# Custom output path (overrides default directory)
AudioCapture -o ~/Desktop/custom.wav -d 10

# Verbose mode (shows format info)
AudioCapture -d 10 -v

# Stream raw PCM to stdout (for piping to other tools)
AudioCapture --raw-stdout | python transcribe.py

# Show all options
AudioCapture -h
```

## Default Output

Recordings are saved to `~/AudioCapture/recordings/` with timestamp-based names:

```
~/AudioCapture/recordings/
├── 2026-04-12_13-45-30.wav
├── 2026-04-12_14-20-00_lecture.wav
├── 2026-04-12_19-30-15_girlfriend.wav
```

## Output Format

16kHz, mono, 16-bit PCM WAV — the standard input format for Whisper and most speech-to-text engines.

## Requirements

- macOS 15+
- Screen Recording permission (prompted on first run)