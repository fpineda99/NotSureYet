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
	SessionDir  string
	Label       string
	Profile     string
	ExtraPrompt string
	Interval    time.Duration
	Duration    time.Duration

	captureCmd *exec.Cmd // background AudioCapture process
}

// globalPrompt is the system-level identity prompt that always goes first.
// It tells the LLM what it is, what product it's part of, and how to behave.
const globalPrompt = `You are AutoNote's note-taking assistant. You are part of a live audio capture and transcription pipeline. Your job is to process transcript chunks and maintain a clean, organized notes document (notes.md) in the current directory.

Key rules:
- You will receive transcript chunks periodically during a live recording session. The user is NOT interacting with you during recording — do not ask questions or wait for input.
- Always produce output by creating or updating notes.md. Never respond with just text — use the Write or Edit tool to modify notes.md.
- Keep notes concise, organized with clear markdown headings, and include timestamps like [12:30] where relevant.
- Preserve all previous notes when updating — only add new information, never remove existing content.
- Do not include filler words, repetitions, or irrelevant chatter from the transcript.
- Do not mention that you are an AI, that you are processing a transcript, or reference the pipeline. Just write the notes as if a skilled human note-taker wrote them.
- If the transcript chunk contains nothing noteworthy, do not modify notes.md.`

// loadPrompt builds the full prompt: global + profile + extra.
func (o *Orchestrator) loadPrompt() (string, error) {
	profilePrompt, err := LoadProfile(o.Profile)
	if err != nil {
		return "", err
	}

	prompt := globalPrompt + "\n\n" + profilePrompt

	if o.ExtraPrompt != "" {
		prompt += "\n\n" + o.ExtraPrompt
	}
	return prompt, nil
}

// startTimer launches a goroutine that prints an updating elapsed time to stderr.
// Close the returned channel to stop it.
func startTimer() chan struct{} {
	done := make(chan struct{})
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(start)
				mins := int(elapsed.Minutes())
				secs := int(elapsed.Seconds()) % 60
				fmt.Fprintf(os.Stderr, "\rRecording: %d:%02d", mins, secs)
			case <-done:
				fmt.Fprintf(os.Stderr, "\r")
				return
			}
		}
	}()
	return done
}

// RunListen starts AudioCapture and waits for Ctrl+C.
func (o *Orchestrator) RunListen() error {
	sessionDir, err := o.startCapture()
	if err != nil {
		return err
	}
	o.SessionDir = sessionDir

	fmt.Fprintf(os.Stderr, "✓ Recording: %s\n", filepath.Base(sessionDir))

	timerDone := startTimer()
	o.waitForSignal()
	close(timerDone)

	o.stopCapture()
	fmt.Fprintf(os.Stderr, "\n✓ Saved\n")

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

	fmt.Fprintf(os.Stderr, "✓ Recording: %s\n", filepath.Base(sessionDir))

	timerDone := startTimer()
	stopCh := o.signalChannel()
	ticker := time.NewTicker(o.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\n⟳ Transcribing...\n")
			if err := o.runTranscribe(); err == nil {
				fmt.Fprintf(os.Stderr, "✓ Transcript updated\n")
			}
		case <-stopCh:
			close(timerDone)
			o.stopCapture()
			fmt.Fprintf(os.Stderr, "\n⟳ Final transcription...\n")
			o.runTranscribe()
			fmt.Fprintf(os.Stderr, "Session: %s\n", sessionDir)
			return nil
		}
	}
}

// RunListenTranscribeWrite starts the full pipeline with periodic processing.
func (o *Orchestrator) RunListenTranscribeWrite() error {
	prompt, err := o.loadPrompt()
	if err != nil {
		return err
	}

	sessionDir, err := o.startCapture()
	if err != nil {
		return err
	}
	o.SessionDir = sessionDir

	fmt.Fprintf(os.Stderr, "✓ Recording: %s\n", filepath.Base(sessionDir))
	fmt.Fprintf(os.Stderr, "  Profile: %s\n", o.Profile)

	timerDone := startTimer()
	state := LoadState(sessionDir)
	state.Profile = o.Profile

	stopCh := o.signalChannel()
	ticker := time.NewTicker(o.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\n⟳ Transcribing...\n")
			if err := o.runTranscribe(); err == nil {
				fmt.Fprintf(os.Stderr, "⟳ Generating notes...\n")
				processNewChunks(sessionDir, prompt, &state)
				fmt.Fprintf(os.Stderr, "✓ Notes updated\n")
			}
		case <-stopCh:
			close(timerDone)
			o.stopCapture()
			fmt.Fprintf(os.Stderr, "\n⟳ Final transcription...\n")
			if err := o.runTranscribe(); err == nil {
				fmt.Fprintf(os.Stderr, "⟳ Generating notes...\n")
				processNewChunks(sessionDir, prompt, &state)
			}
			SaveState(sessionDir, state)
			fmt.Fprintf(os.Stderr, "✓ Done\n")
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

	// Use full path so it works from any environment (including Tauri app)
	capturePath := findExecutable("AudioCapture")
	o.captureCmd = exec.Command(capturePath, args...)
	o.captureCmd.Stderr = os.Stderr

	if err := o.captureCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start AudioCapture: %w", err)
	}

	// Poll until AudioCapture creates the session folder (up to 10 seconds)
	for attempts := 0; attempts < 20; attempts++ {
		time.Sleep(500 * time.Millisecond)
		dir, err := o.findLatestSession()
		if err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("timed out waiting for AudioCapture to create session folder")
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
	transcribePath := findExecutable("transcribe")
	cmd := exec.Command(transcribePath, o.SessionDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: transcription failed: %v\n", err)
		return err
	}
	return nil
}

// runNotateReview uses the existing notate logic to generate notes.
func (o *Orchestrator) runNotateReview() error {
	prompt, err := o.loadPrompt()
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

	notesPath := filepath.Join(o.SessionDir, "notes.md")
	fullText := FormatChunk(t.Segments)
	message := fmt.Sprintf(
		"Here is the full transcript of a conversation. Create comprehensive notes at %s.\n\n%s",
		notesPath, fullText,
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
// Only advances state if processing succeeds — failed chunks will be retried.
func processNewChunks(sessionDir, prompt string, state *State) {
	t, err := ReadTranscript(sessionDir)
	if err != nil {
		// Transcript may not exist yet during live recording — only warn if
		// we've processed before (meaning the file existed and is now broken)
		if state.LastSegmentIndex >= 0 {
			fmt.Fprintf(os.Stderr, "[%s] Warning: could not read transcript: %v\n",
				time.Now().Format("15:04:05"), err)
		}
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
		// Don't advance state — these chunks will be retried next tick
		return
	}

	state.LastSegmentIndex = state.LastSegmentIndex + len(segments)
	SaveState(sessionDir, *state)
}

// sendToSession sends a transcript chunk to Claude, starting or resuming a session.
func sendToSession(sessionDir, prompt string, state *State, chunk string) error {
	notesPath := filepath.Join(sessionDir, "notes.md")
	message := fmt.Sprintf(
		"Here is new transcript from the conversation. Update the notes file at %s with any important information.\n\n%s",
		notesPath, chunk,
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