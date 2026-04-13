package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadProfile reads a profile prompt from disk.
// Searches in order: ~/.notate/profiles/, ./profiles/, ../profiles/
func LoadProfile(name string) (string, error) {
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}

	searchPaths := []string{
		filepath.Join(homeDir(), ".notate", "profiles", name),
		filepath.Join("profiles", name),
		filepath.Join("..", "profiles", name),
	}

	for _, path := range searchPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}

	return "", fmt.Errorf("profile %q not found (searched: %s)",
		name, strings.Join(searchPaths, ", "))
}

// ProfileInfo holds a profile name and its description.
type ProfileInfo struct {
	Name        string
	Description string
}

// ListProfiles returns all available profiles with descriptions.
func ListProfiles() []ProfileInfo {
	var profiles []ProfileInfo
	seen := make(map[string]bool)

	searchDirs := []string{
		filepath.Join(homeDir(), ".notate", "profiles"),
		"profiles",
		filepath.Join("..", "profiles"),
	}

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") && !seen[e.Name()] {
				seen[e.Name()] = true
				name := strings.TrimSuffix(e.Name(), ".md")
				desc := parseDescription(filepath.Join(dir, e.Name()))
				profiles = append(profiles, ProfileInfo{Name: name, Description: desc})
			}
		}
	}
	return profiles
}

// parseDescription reads the first line of a profile file for an HTML comment description.
// Expected format: <!-- Description text here -->
func parseDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	firstLine := strings.SplitN(string(data), "\n", 2)[0]
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "<!--") && strings.HasSuffix(firstLine, "-->") {
		desc := strings.TrimPrefix(firstLine, "<!--")
		desc = strings.TrimSuffix(desc, "-->")
		return strings.TrimSpace(desc)
	}
	return ""
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// findExecutable looks for a command in common paths.
// Falls back to the bare name (relying on PATH) if not found.
func findExecutable(name string) string {
	searchPaths := []string{
		filepath.Join("/usr/local/bin", name),
		filepath.Join("/opt/homebrew/bin", name),
		filepath.Join(homeDir(), ".local/bin", name),
	}
	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Fall back to PATH lookup
	return name
}