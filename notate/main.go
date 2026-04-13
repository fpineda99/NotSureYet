package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := strings.ToUpper(os.Args[1])

	switch command {
	case "LTW":
		runCommand(modeLTW)
	case "LT":
		runCommand(modeLT)
	case "L":
		runCommand(modeL)
	case "TW":
		runCommand(modeTW)
	case "T":
		runCommand(modeT)
	case "W":
		runCommand(modeW)
	case "PROFILES":
		runProfilesList()
	case "DELETE":
		runDelete()
	case "HELP", "-H", "--HELP":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

type mode int

const (
	modeL mode = iota
	modeLT
	modeLTW
	modeT
	modeTW
	modeW
)

func runCommand(m mode) {
	label, profile, extraPrompt, interval, duration, sessionDir := parseFlags()

	o := &Orchestrator{
		SessionDir:  sessionDir,
		Label:       label,
		Profile:     profile,
		ExtraPrompt: extraPrompt,
		Interval:    interval,
		Duration:    duration,
	}

	// Validate: T, TW, W require an existing session directory
	needsSessionDir := m == modeT || m == modeTW || m == modeW
	if needsSessionDir && sessionDir == "" {
		fmt.Fprintf(os.Stderr, "Error: session directory is required for this command\n")
		fmt.Fprintf(os.Stderr, "Usage: autonote %s <session-dir> [options]\n", os.Args[1])
		os.Exit(1)
	}

	// Validate: LTW, TW, W require a profile
	needsProfile := m == modeLTW || m == modeTW || m == modeW
	if needsProfile && profile == "" {
		fmt.Fprintf(os.Stderr, "Error: --profile is required for this command\n")
		os.Exit(1)
	}

	var err error
	switch m {
	case modeL:
		err = o.RunListen()
	case modeLT:
		err = o.RunListenTranscribe()
	case modeLTW:
		err = o.RunListenTranscribeWrite()
	case modeT:
		err = o.RunTranscribe()
	case modeTW:
		err = o.RunTranscribeWrite()
	case modeW:
		err = o.RunWrite()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDelete() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Error: session directory is required\n")
		fmt.Fprintf(os.Stderr, "Usage: autonote delete <session-dir>\n")
		os.Exit(1)
	}

	sessionDir := os.Args[2]

	// Verify it's inside ~/AudioCapture/ to prevent accidental deletion of other folders
	home := homeDir()
	baseDir := home + "/AudioCapture/"
	absPath, _ := filepath.Abs(sessionDir)
	if !strings.HasPrefix(absPath, baseDir) {
		fmt.Fprintf(os.Stderr, "Error: can only delete session folders inside ~/AudioCapture/\n")
		os.Exit(1)
	}

	// Check it exists
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a valid session directory\n", sessionDir)
		os.Exit(1)
	}

	// Delete the Claude Code session if one exists
	state := LoadState(absPath)
	if state.SessionID != "" {
		deleteClaudeSession(absPath, state.SessionID)
	}

	// Show what will be deleted from the session folder
	entries, _ := os.ReadDir(absPath)
	fmt.Fprintf(os.Stderr, "Deleting session: %s\n", filepath.Base(absPath))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			fmt.Fprintf(os.Stderr, "  %s\n", e.Name())
		}
	}

	if err := os.RemoveAll(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Deleted.\n")
}

// deleteClaudeSession removes the Claude Code session file for this notate session.
// NOTE: The path encoding here (replacing / and _ with -) is reverse-engineered from
// observed behavior. If Claude Code changes its encoding scheme, this may break.
// Worst case: the session file isn't deleted but nothing else breaks.
func deleteClaudeSession(sessionDir string, sessionID string) {
	// Validate session ID to prevent path traversal
	for _, c := range sessionID {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			fmt.Fprintf(os.Stderr, "Warning: invalid session ID format, skipping cleanup\n")
			return
		}
	}

	home := homeDir()
	encoded := strings.ReplaceAll(sessionDir, "/", "-")
	encoded = strings.ReplaceAll(encoded, "_", "-")
	claudeProjectDir := filepath.Join(home, ".claude", "projects", encoded)

	sessionFile := filepath.Join(claudeProjectDir, sessionID+".jsonl")
	if _, err := os.Stat(sessionFile); err == nil {
		os.Remove(sessionFile)
		fmt.Fprintf(os.Stderr, "Deleted Claude Code session: %s\n", sessionID)
	}

	// Clean up the project directory if it's now empty
	entries, err := os.ReadDir(claudeProjectDir)
	if err == nil && len(entries) == 0 {
		os.Remove(claudeProjectDir)
	}
}

func runProfilesList() {
	profiles := ListProfiles()
	if len(profiles) == 0 {
		fmt.Println("No profiles found.")
		fmt.Println("Create profiles in ~/.notate/profiles/ or ./profiles/")
		return
	}

	fmt.Println("Available profiles:")
	for _, p := range profiles {
		if p.Description != "" {
			fmt.Printf("  %-12s — %s\n", p.Name, p.Description)
		} else {
			fmt.Printf("  %s\n", p.Name)
		}
	}
}

func parseFlags() (label, profile, extraPrompt string, interval, duration time.Duration, sessionDir string) {
	profile = "lecture"
	interval = 60 * time.Second

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-l", "--label":
			if i+1 < len(args) {
				label = args[i+1]
				i++
			}
		case "-p", "--profile":
			if i+1 < len(args) {
				profile = args[i+1]
				i++
			}
		case "-e", "--extra-prompt":
			if i+1 < len(args) {
				extraPrompt = args[i+1]
				i++
			}
		case "-i", "--interval":
			if i+1 < len(args) {
				d, err := time.ParseDuration(args[i+1])
				if err == nil {
					interval = d
				}
				i++
			}
		case "-d", "--duration":
			if i+1 < len(args) {
				d, err := time.ParseDuration(args[i+1])
				if err == nil {
					duration = d
				}
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") && sessionDir == "" {
				sessionDir = args[i]
			}
		}
	}
	return
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `autonote — Capture audio, transcribe, and extract notes

Commands:
  L       Listen only (record audio)
  LT      Listen + Transcribe
  LTW     Listen + Transcribe + Write notes (full pipeline)
  T       Transcribe existing recording
  TW      Transcribe + Write notes
  W       Write notes from existing transcript
  profiles  List available context profiles
  delete    Delete a session folder

Options:
  -l, --label <name>         Session label (e.g., lecture, meeting)
  -p, --profile <name>       Context profile for note-taking (default: lecture)
  -e, --extra-prompt <text>  Extra instructions appended to the profile prompt
  -i, --interval <dur>       Processing interval for live modes (default: 60s)
  -d, --duration <dur>       Recording duration (default: until Ctrl+C)

Examples:
  autonote LTW -l lecture -p lecture
  autonote LT -l meeting
  autonote L -l quick-note
  autonote W ~/AudioCapture/2026-04-12_lecture/ -p lecture
  autonote TW ~/AudioCapture/2026-04-12_meeting/ -p meeting
  autonote profiles
`)
}