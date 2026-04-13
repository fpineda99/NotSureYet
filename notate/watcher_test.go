package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTranscript(t *testing.T) {
	dir := t.TempDir()

	transcript := Transcript{
		Source: "test.wav",
		Text:   "hello world",
		Segments: []Segment{
			{Start: 0.0, End: 2.5, Text: "hello"},
			{Start: 2.5, End: 5.0, Text: "world"},
		},
	}

	data, _ := json.MarshalIndent(transcript, "", "  ")
	os.WriteFile(filepath.Join(dir, "transcript.json"), data, 0644)

	result, err := ReadTranscript(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result.Text)
	}
	if len(result.Segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(result.Segments))
	}
}

func TestReadTranscriptMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadTranscript(dir)
	if err == nil {
		t.Error("expected error for missing transcript")
	}
}

func TestReadTranscriptCorrupted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "transcript.json"), []byte("not json"), 0644)

	_, err := ReadTranscript(dir)
	if err == nil {
		t.Error("expected error for corrupted transcript")
	}
}

func TestGetNewSegments(t *testing.T) {
	transcript := &Transcript{
		Segments: []Segment{
			{Start: 0.0, End: 2.0, Text: "one"},
			{Start: 2.0, End: 4.0, Text: "two"},
			{Start: 4.0, End: 6.0, Text: "three"},
			{Start: 6.0, End: 8.0, Text: "four"},
		},
	}

	// First call — get all segments
	segments := GetNewSegments(transcript, -1)
	if len(segments) != 4 {
		t.Errorf("expected 4 segments, got %d", len(segments))
	}

	// After processing 2 segments
	segments = GetNewSegments(transcript, 1)
	if len(segments) != 2 {
		t.Errorf("expected 2 new segments, got %d", len(segments))
	}
	if segments[0].Text != "three" {
		t.Errorf("expected 'three', got '%s'", segments[0].Text)
	}

	// All processed
	segments = GetNewSegments(transcript, 3)
	if len(segments) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segments))
	}
}

func TestFormatChunk(t *testing.T) {
	segments := []Segment{
		{Start: 0.0, End: 5.0, Text: "hello"},
		{Start: 65.0, End: 70.0, Text: "world"},
	}

	result := FormatChunk(segments)
	if result == "" {
		t.Error("FormatChunk returned empty string")
	}
	// Check timestamp format
	if !contains(result, "[0:00]") {
		t.Error("expected [0:00] timestamp")
	}
	if !contains(result, "[1:05]") {
		t.Error("expected [1:05] timestamp")
	}
	if !contains(result, "hello") {
		t.Error("expected 'hello' in output")
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{60, "1:00"},
		{65, "1:05"},
		{3600, "60:00"},
	}

	for _, tt := range tests {
		result := formatTimestamp(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatTimestamp(%v) = '%s', want '%s'", tt.seconds, result, tt.expected)
		}
	}
}

func TestLoadStateMissing(t *testing.T) {
	dir := t.TempDir()
	state := LoadState(dir)
	if state.LastSegmentIndex != -1 {
		t.Errorf("expected -1, got %d", state.LastSegmentIndex)
	}
	if state.SessionID != "" {
		t.Errorf("expected empty session ID, got '%s'", state.SessionID)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()

	state := State{
		SessionID:        "test-session-123",
		LastSegmentIndex: 5,
		Profile:          "lecture",
	}

	err := SaveState(dir, state)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded := LoadState(dir)
	if loaded.SessionID != "test-session-123" {
		t.Errorf("expected 'test-session-123', got '%s'", loaded.SessionID)
	}
	if loaded.LastSegmentIndex != 5 {
		t.Errorf("expected 5, got %d", loaded.LastSegmentIndex)
	}
	if loaded.Profile != "lecture" {
		t.Errorf("expected 'lecture', got '%s'", loaded.Profile)
	}
	if loaded.LastProcessedAt == "" {
		t.Error("expected LastProcessedAt to be set")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCheck(s, substr))
}

func containsCheck(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
