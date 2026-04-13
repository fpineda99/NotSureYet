package main

import (
	"os"
	"testing"
)

func TestFindExecutable(t *testing.T) {
	// Should find AudioCapture if installed
	path := findExecutable("AudioCapture")
	if path == "AudioCapture" {
		t.Skip("AudioCapture not installed in known paths")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findExecutable returned path that doesn't exist: %s", path)
	}
}

func TestFindExecutableAudioCapture(t *testing.T) {
	path := findExecutable("AudioCapture")
	if path == "AudioCapture" {
		t.Skip("AudioCapture not installed in known paths")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("AudioCapture path doesn't exist: %s", path)
	}
}

func TestFindExecutableTranscribe(t *testing.T) {
	path := findExecutable("transcribe")
	if path == "transcribe" {
		t.Skip("transcribe not installed in known paths")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("transcribe path doesn't exist: %s", path)
	}
}

func TestFindExecutableClaude(t *testing.T) {
	path := findExecutable("claude")
	if path == "claude" {
		t.Skip("claude not installed in known paths")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("claude path doesn't exist: %s", path)
	}
}

func TestParseSessionID(t *testing.T) {
	// Valid JSON with session_id
	output := `{"session_id": "abc-123-def", "result": "done"}`
	id, err := parseSessionID(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc-123-def" {
		t.Errorf("expected 'abc-123-def', got '%s'", id)
	}
}

func TestParseSessionIDMultiline(t *testing.T) {
	output := `{"type": "progress"}
{"session_id": "xyz-789", "result": "ok"}`
	id, err := parseSessionID(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "xyz-789" {
		t.Errorf("expected 'xyz-789', got '%s'", id)
	}
}

func TestParseSessionIDMissing(t *testing.T) {
	output := `{"result": "done"}`
	_, err := parseSessionID(output)
	if err == nil {
		t.Error("expected error for missing session_id")
	}
}

func TestParseResultText(t *testing.T) {
	output := `{"result": "notes updated successfully"}`
	result := parseResultText(output)
	if result != "notes updated successfully" {
		t.Errorf("expected 'notes updated successfully', got '%s'", result)
	}
}

func TestParseResultTextFallback(t *testing.T) {
	output := "plain text response"
	result := parseResultText(output)
	if result != output {
		t.Errorf("expected raw output as fallback, got '%s'", result)
	}
}

func TestSessionIDValidation(t *testing.T) {
	// Valid session IDs
	validIDs := []string{"abc-123", "a1b2c3", "ABC-def-456"}
	for _, id := range validIDs {
		valid := true
		for _, c := range id {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				valid = false
				break
			}
		}
		if !valid {
			t.Errorf("ID '%s' should be valid", id)
		}
	}

	// Invalid session IDs (path traversal)
	invalidIDs := []string{"../etc/passwd", "abc/../def", "id;rm -rf /"}
	for _, id := range invalidIDs {
		valid := true
		for _, c := range id {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				valid = false
				break
			}
		}
		if valid {
			t.Errorf("ID '%s' should be invalid", id)
		}
	}
}

