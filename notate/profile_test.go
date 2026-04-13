package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfileFromHome(t *testing.T) {
	// This test requires profiles to exist in ~/.notate/profiles/
	home, _ := os.UserHomeDir()
	profilePath := filepath.Join(home, ".notate", "profiles", "lecture.md")
	if _, err := os.Stat(profilePath); err != nil {
		t.Skip("lecture.md not installed at ~/.notate/profiles/")
	}

	content, err := LoadProfile("lecture")
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}
	if len(content) < 10 {
		t.Error("profile content seems too short")
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	_, err := LoadProfile("nonexistent-profile-xyz")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}


func TestListProfiles(t *testing.T) {
	profiles := ListProfiles()
	// Should find at least one profile if installed
	if len(profiles) == 0 {
		t.Skip("no profiles installed")
	}

	for _, p := range profiles {
		if p.Name == "" {
			t.Error("profile name should not be empty")
		}
	}
}

func TestParseDescription(t *testing.T) {
	dir := t.TempDir()

	// File with description
	withDesc := filepath.Join(dir, "with.md")
	os.WriteFile(withDesc, []byte("<!-- My description -->\nPrompt content"), 0644)

	desc := parseDescription(withDesc)
	if desc != "My description" {
		t.Errorf("expected 'My description', got '%s'", desc)
	}

	// File without description
	withoutDesc := filepath.Join(dir, "without.md")
	os.WriteFile(withoutDesc, []byte("Just prompt content"), 0644)

	desc = parseDescription(withoutDesc)
	if desc != "" {
		t.Errorf("expected empty description, got '%s'", desc)
	}

	// Missing file
	desc = parseDescription(filepath.Join(dir, "missing.md"))
	if desc != "" {
		t.Errorf("expected empty description for missing file, got '%s'", desc)
	}
}


func TestFindExecutableInstalled(t *testing.T) {
	// autonote should exist in /usr/local/bin if installed
	path := findExecutable("autonote")
	if path == "autonote" {
		t.Skip("autonote not installed in known paths")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findExecutable('autonote') returned invalid path: %s", path)
	}
}

func TestFindExecutableFallback(t *testing.T) {
	// Non-existent binary should fall back to bare name
	path := findExecutable("definitely-not-a-real-binary")
	if path != "definitely-not-a-real-binary" {
		t.Errorf("expected fallback to bare name, got '%s'", path)
	}
}
