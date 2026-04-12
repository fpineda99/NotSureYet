package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Transcript matches the JSON structure from our transcriber.
type Transcript struct {
	Source   string    `json:"source"`
	Text     string   `json:"text"`
	Segments []Segment `json:"segments"`
}

type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// State tracks what we've already processed so we don't repeat ourselves.
type State struct {
	SessionID        string `json:"session_id"`
	LastSegmentIndex int    `json:"last_segment_index"`
	LastProcessedAt  string `json:"last_processed_at"`
	Profile          string `json:"profile"`
}

const stateFile = ".notate-state.json"

// ReadTranscript loads transcript.json from a session folder.
func ReadTranscript(sessionDir string) (*Transcript, error) {
	path := filepath.Join(sessionDir, "transcript.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read transcript: %w", err)
	}

	var t Transcript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("could not parse transcript: %w", err)
	}
	return &t, nil
}

// LoadState reads the watcher state from the session folder.
// Returns a zero state if the file doesn't exist yet.
func LoadState(sessionDir string) State {
	path := filepath.Join(sessionDir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return State{LastSegmentIndex: -1}
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{LastSegmentIndex: -1}
	}
	return s
}

// SaveState writes the watcher state to the session folder.
func SaveState(sessionDir string, state State) error {
	state.LastProcessedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionDir, stateFile), data, 0644)
}

// GetNewSegments returns segments that haven't been processed yet.
func GetNewSegments(t *Transcript, lastIndex int) []Segment {
	start := lastIndex + 1
	if start >= len(t.Segments) {
		return nil
	}
	return t.Segments[start:]
}

// GetRecentSegments returns segments from the last N seconds.
func GetRecentSegments(t *Transcript, lastSeconds float64) []Segment {
	if len(t.Segments) == 0 {
		return nil
	}

	cutoff := t.Segments[len(t.Segments)-1].End - lastSeconds
	var result []Segment
	for _, seg := range t.Segments {
		if seg.Start >= cutoff {
			result = append(result, seg)
		}
	}
	return result
}

// FormatChunk turns segments into a readable prompt for Claude.
func FormatChunk(segments []Segment) string {
	var b strings.Builder
	for _, seg := range segments {
		fmt.Fprintf(&b, "[%s] %s\n", formatTimestamp(seg.Start), seg.Text)
	}
	return b.String()
}

// formatTimestamp converts seconds to MM:SS format.
func formatTimestamp(seconds float64) string {
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}
