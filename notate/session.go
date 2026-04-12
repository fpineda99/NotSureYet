package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Session manages a Claude Code CLI session for note-taking.
type Session struct {
	ID      string // Claude Code session ID
	WorkDir string // Session folder (where notes.md lives)
}

// StartSession creates a new Claude Code session with a system prompt.
// It sends an initial message and captures the session ID from the output.
func StartSession(workDir, systemPrompt, initialMessage string) (*Session, error) {
	args := []string{
		"-p",
		"--system-prompt", systemPrompt,
		"--output-format", "json",
		"--max-turns", "5",
		"--dangerously-skip-permissions",
		initialMessage,
	}

	output, err := runClaude(workDir, args)
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	sessionID, err := parseSessionID(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse session ID: %w", err)
	}

	return &Session{ID: sessionID, WorkDir: workDir}, nil
}

// ResumeSession sends a message to an existing Claude Code session.
func (s *Session) Resume(message string) (string, error) {
	args := []string{
		"-p",
		"--resume", s.ID,
		"--output-format", "json",
		"--max-turns", "5",
		"--dangerously-skip-permissions",
		message,
	}

	output, err := runClaude(s.WorkDir, args)
	if err != nil {
		return "", fmt.Errorf("failed to resume session: %w", err)
	}

	return parseResultText(output), nil
}

// runClaude executes the claude CLI and returns its stdout.
func runClaude(workDir string, args []string) (string, error) {
	cmd := exec.Command("claude", args...)
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude command failed: %w", err)
	}

	return string(out), nil
}

// parseSessionID extracts the session ID from Claude Code's JSON output.
// The JSON output has a "session_id" field.
func parseSessionID(output string) (string, error) {
	// Claude's JSON output may have multiple JSON objects (one per message).
	// The session_id appears in the result message.
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Try parsing each line as JSON, look for session_id
	for _, line := range lines {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if id, ok := msg["session_id"].(string); ok && id != "" {
			return id, nil
		}
	}

	// Try parsing the whole output as a single JSON object
	var msg map[string]any
	if err := json.Unmarshal([]byte(output), &msg); err == nil {
		if id, ok := msg["session_id"].(string); ok && id != "" {
			return id, nil
		}
	}

	return "", fmt.Errorf("no session_id found in output")
}

// parseResultText extracts the result text from Claude Code's JSON output.
func parseResultText(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if result, ok := msg["result"].(string); ok {
			return result
		}
	}
	return output
}