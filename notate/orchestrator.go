package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Orchestrator manages the pipeline of AudioCapture → transcribe → notate.
type Orchestrator struct {
	SessionDir string
	Label      string
	Profile    string
	Interval   time.Duration
	Duration   time.Duration

	captureCmd *exec.Cmd // background AudioCapture process
}

// RunListen starts AudioCapture and waits for Ctrl+C.
func (o *Orchestrator) RunListen() error {
	sessionDir, err := o.startCapture()
	if err != nil {
		return err
	}
	o.SessionDir = sessionDir

	fmt.Fprintf(os.Stderr, "Recording: %s\n", sessionDir)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	o.waitForSignal()
	o.stopCapture()

	fmt.Fprintf(os.Stderr, "\nSession: %s\n", sessionDir)
	return nil
}

// RunListenTranscribe starts AudioCapture, periodically transcribes, waits for Ctrl+C.
func (o *Orchestrator) RunListenTranscribe() error {
	sessionDir, err := o.startCapture()
	if err != nil {
		return err
	}
	o.SessionDir = sessionDir

	fmt.Fprintf(os.Stderr, "Recording + transcribing: %s\n", sessionDir)
	fmt.Fprintf(os.Stderr, "Interval: %s\n", o.Interval)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	stopCh := o.signalChannel()
	ticker := time.NewTicker(o.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.runTranscribe()
		case <-stopCh:
			o.stopCapture()
			fmt.Fprintf(os.Stderr, "\nFinal transcription...\n")
			o.runTranscribe()
			fmt.Fprintf(os.Stderr, "Session: %s\n", sessionDir)
			return nil
		}
	}
}

// RunListenTranscribeWrite starts the full pipeline with periodic processing.
func (o *Orchestrator) RunListenTranscribeWrite() error {
	prompt, err := LoadProfile(o.Profile)
	if err != nil {
		return err
	}

	sessionDir, err := o.startCapture()
	if err != nil {
		return err
	}
	o.SessionDir = sessionDir

	fmt.Fprintf(os.Stderr, "Full pipeline: %s\n", sessionDir)
	fmt.Fprintf(os.Stderr, "Profile: %s\n", o.Profile)
	fmt.Fprintf(os.Stderr, "Interval: %s\n", o.Interval)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	state := LoadState(sessionDir)
	state.Profile = o.Profile

	stopCh := o.signalChannel()
	ticker := time.NewTicker(o.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.runTranscribe()
			processNewChunks(sessionDir, prompt, &state)
		case <-stopCh:
			o.stopCapture()
			fmt.Fprintf(os.Stderr, "\nFinal transcription...\n")
			o.runTranscribe()
			fmt.Fprintf(os.Stderr, "Final notes pass...\n")
			processNewChunks(sessionDir, prompt, &state)
			SaveState(sessionDir, state)
			fmt.Fprintf(os.Stderr, "Session: %s\n", sessionDir)
			return nil
		}
	}
}

// RunTranscribe transcribes an existing session.
func (o *Orchestrator) RunTranscribe() error {
	fmt.Fprintf(os.Stderr, "Transcribing: %s\n", o.SessionDir)
	o.runTranscribe()
	fmt.Fprintf(os.Stderr, "Done.\n")
	return nil
}

// RunTranscribeWrite transcribes then generates notes.
func (o *Orchestrator) RunTranscribeWrite() error {
	fmt.Fprintf(os.Stderr, "Transcribing: %s\n", o.SessionDir)
	o.runTranscribe()

	fmt.Fprintf(os.Stderr, "Generating notes with profile: %s\n", o.Profile)
	return o.runNotateReview()
}

// RunWrite generates notes from an existing transcript.
func (o *Orchestrator) RunWrite() error {
	fmt.Fprintf(os.Stderr, "Generating notes: %s\n", o.SessionDir)
	return o.runNotateReview()
}

// startCapture launches AudioCapture as a background process.
// Returns the session directory path.
func (o *Orchestrator) startCapture() (string, error) {
	args := []string{}

	if o.Label != "" {
		args = append(args, "-l", o.Label)
	}

	if o.Duration > 0 {
		args = append(args, "-d", fmt.Sprintf("%.0f", o.Duration.Seconds()))
	}

	o.captureCmd = exec.Command("AudioCapture", args...)
	o.captureCmd.Stderr = os.Stderr

	if err := o.captureCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start AudioCapture: %w", err)
	}

	// Give AudioCapture a moment to create the session folder
	time.Sleep(1 * time.Second)

	// Find the session folder it just created
	return o.findLatestSession()
}

// stopCapture sends SIGINT to the AudioCapture process.
func (o *Orchestrator) stopCapture() {
	if o.captureCmd != nil && o.captureCmd.Process != nil {
		o.captureCmd.Process.Signal(syscall.SIGINT)
		o.captureCmd.Wait()
	}
}

// runTranscribe calls the transcribe CLI on the session folder.
func (o *Orchestrator) runTranscribe() error {
	cmd := exec.Command("transcribe", o.SessionDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: transcription failed: %v\n", err)
		return err
	}
	return nil
}

// runNotateReview uses the existing notate logic to generate notes.
func (o *Orchestrator) runNotateReview() error {
	prompt, err := LoadProfile(o.Profile)
	if err != nil {
		return err
	}

	t, err := ReadTranscript(o.SessionDir)
	if err != nil {
		return err
	}

	if len(t.Segments) == 0 {
		fmt.Fprintf(os.Stderr, "Transcript is empty.\n")
		return nil
	}

	fullText := FormatChunk(t.Segments)
	message := fmt.Sprintf(
		"Here is the full transcript of a conversation. Create comprehensive notes in notes.md.\n\n%s",
		fullText,
	)

	session, err := StartSession(o.SessionDir, prompt, message)
	if err != nil {
		return err
	}

	state := State{
		SessionID:        session.ID,
		LastSegmentIndex: len(t.Segments) - 1,
		Profile:          o.Profile,
	}
	SaveState(o.SessionDir, state)

	fmt.Fprintf(os.Stderr, "Done. Session ID: %s\n", session.ID)
	return nil
}

// processNewChunks checks for new transcript segments and sends them to Claude.
func processNewChunks(sessionDir, prompt string, state *State) {
	t, err := ReadTranscript(sessionDir)
	if err != nil {
		return
	}

	segments := GetNewSegments(t, state.LastSegmentIndex)
	if len(segments) == 0 {
		return
	}

	chunk := FormatChunk(segments)
	fmt.Fprintf(os.Stderr, "[%s] Processing %d new segments...\n",
		time.Now().Format("15:04:05"), len(segments))

	err = sendToSession(sessionDir, prompt, state, chunk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing chunk: %v\n", err)
		return
	}

	state.LastSegmentIndex = state.LastSegmentIndex + len(segments)
	SaveState(sessionDir, *state)
}

// sendToSession sends a transcript chunk to Claude, starting or resuming a session.
func sendToSession(sessionDir, prompt string, state *State, chunk string) error {
	message := fmt.Sprintf(
		"Here is new transcript from the conversation. Update notes.md with any important information.\n\n%s",
		chunk,
	)

	if state.SessionID == "" {
		session, err := StartSession(sessionDir, prompt, message)
		if err != nil {
			return err
		}
		state.SessionID = session.ID
	} else {
		session := &Session{ID: state.SessionID, WorkDir: sessionDir}
		_, err := session.Resume(message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not resume session, starting new one...\n")
			newSession, err := StartSession(sessionDir, prompt, message)
			if err != nil {
				return err
			}
			state.SessionID = newSession.ID
		}
	}

	return nil
}

// findLatestSession finds the most recently created session folder in ~/AudioCapture/.
func (o *Orchestrator) findLatestSession() (string, error) {
	baseDir := filepath.Join(homeDir(), "AudioCapture")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("could not read AudioCapture directory: %w", err)
	}

	var latest string
	var latestTime time.Time

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = e.Name()
		}
	}

	if latest == "" {
		return "", fmt.Errorf("no session folder found in %s", baseDir)
	}

	return filepath.Join(baseDir, latest), nil
}

// signalChannel returns a channel that receives on SIGINT/SIGTERM.
func (o *Orchestrator) signalChannel() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}

// waitForSignal blocks until SIGINT or SIGTERM.
func (o *Orchestrator) waitForSignal() {
	ch := o.signalChannel()
	<-ch
}