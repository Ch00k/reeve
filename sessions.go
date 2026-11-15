package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// transcriptTailChunk is how much of a transcript's end is read first when
// looking for its title and last messages; the window doubles until they are found.
const transcriptTailChunk = 256 * 1024

// serviceStartTimeout bounds the wait for a started background service to
// accept connections, and servicePollInterval is how often it is checked.
const (
	serviceStartTimeout = 10 * time.Second
	servicePollInterval = 200 * time.Millisecond
)

// Session is one running Claude Code session as reported by
// `claude agents --json`, plus the title and last messages of its transcript.
type Session struct {
	ID        string `json:"id"`  // short id, background sessions only
	PID       int    `json:"pid"` // interactive sessions only
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"` // "interactive" or "background"
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Status    string `json:"status"`    // interactive sessions: idle, busy
	State     string `json:"state"`     // background sessions: blocked, done, ...
	StartedAt int64  `json:"startedAt"` // Unix milliseconds
	// Title is the latest title Claude Code generated for the conversation,
	// the one Mac notifications show, and Messages its last messages, oldest first.
	Title    string    `json:"-"`
	Messages []Message `json:"-"`
	// TmuxPane is the id of the tmux pane an interactive session runs in, and
	// TmuxLocation that pane as session:window.pane; both are empty for a
	// session outside tmux.
	TmuxPane     string `json:"-"`
	TmuxLocation string `json:"-"`
}

// Activity is the session's current state, whichever field its kind reports it in.
func (s Session) Activity() string {
	if s.Kind == "background" {
		return s.State
	}
	return s.Status
}

// Started is when the session started, or the zero time when it is unknown.
func (s Session) Started() time.Time {
	if s.StartedAt == 0 {
		return time.Time{}
	}
	return time.UnixMilli(s.StartedAt)
}

// Message is one conversational message of a transcript.
type Message struct {
	Role string // "user" or "assistant"
	Text string
	Time time.Time
}

// claudeConfigDir is where Claude Code keeps its data, honouring CLAUDE_CONFIG_DIR.
func claudeConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// fetchSessions lists the running sessions, without their titles and messages.
func fetchSessions() ([]Session, error) {
	out, err := exec.Command("claude", "agents", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("claude agents --json: %w", err)
	}
	return parseSessions(out)
}

func parseSessions(data []byte) ([]Session, error) {
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("parse session list: %w", err)
	}
	return sessions, nil
}

// loadSessions lists the running sessions with the title and up to
// messageCount last messages of each and, for interactive sessions, the tmux
// pane they run in.
func loadSessions(messageCount int) ([]Session, error) {
	sessions, err := fetchSessions()
	if err != nil {
		return nil, err
	}

	configDir, err := claudeConfigDir()
	if err != nil {
		return nil, err
	}
	projectsDir := filepath.Join(configDir, "projects")
	panes := tmuxPanes()

	for i := range sessions {
		if sessions[i].Kind == "interactive" {
			pane := recordedTmuxPane(configDir, sessions[i].PID)
			if location, ok := panes[pane]; ok {
				sessions[i].TmuxPane, sessions[i].TmuxLocation = pane, location
			}
		}

		path, err := transcriptPath(projectsDir, sessions[i].SessionID)
		if err != nil || path == "" {
			continue
		}
		sessions[i].Title, sessions[i].Messages, err = transcriptTail(path, messageCount)
		if err != nil {
			return nil, err
		}
	}

	return sessions, nil
}

// transcriptPath finds a session's transcript in any project directory, since a
// background session's transcript can live under a directory other than its cwd.
// It returns "" when the session has no transcript yet.
func transcriptPath(projectsDir, sessionID string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return "", err
	}
	return matches[0], nil
}

// transcriptTail returns a transcript's latest title and up to count of its
// last conversational messages, oldest first, reading it from the end. The
// title is "" until Claude Code has generated one.
func transcriptTail(path string, count int) (string, []Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return "", nil, err
	}
	size := info.Size()

	for window := int64(transcriptTailChunk); ; window *= 2 {
		start := max(0, size-window)
		buf := make([]byte, size-start)
		if _, err := f.ReadAt(buf, start); err != nil {
			return "", nil, fmt.Errorf("read %s: %w", path, err)
		}

		lines := bytes.Split(buf, []byte("\n"))
		if start > 0 {
			// The window can begin mid-line.
			lines = lines[1:]
		}

		var title string
		var messages []Message
		for _, line := range slices.Backward(lines) {
			if title != "" && len(messages) == count {
				break
			}
			record, ok := parseRecord(line)
			if !ok {
				continue
			}
			if record.Type == "ai-title" && title == "" {
				title = record.AITitle
			}
			if msg, ok := record.message(); ok && len(messages) < count {
				messages = append(messages, msg)
			}
		}

		if (title != "" && len(messages) == count) || start == 0 {
			slices.Reverse(messages)
			return title, messages, nil
		}
	}
}

// transcriptRecord is the part of a transcript line this program reads.
type transcriptRecord struct {
	Type        string    `json:"type"`
	AITitle     string    `json:"aiTitle"`
	IsMeta      bool      `json:"isMeta"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func parseRecord(line []byte) (transcriptRecord, bool) {
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return transcriptRecord{}, false
	}
	return record, true
}

// message returns the record as a Message when it is text a person typed or
// Claude wrote in the main conversation. Tool calls and results, subagent
// (sidechain) records, and harness-injected messages such as slash-command
// output, task notifications and reminders (which are wrapped in XML-like
// tags) are skipped.
func (r transcriptRecord) message() (Message, bool) {
	if r.Type != "user" && r.Type != "assistant" {
		return Message{}, false
	}
	if r.IsMeta || r.IsSidechain {
		return Message{}, false
	}

	text := strings.TrimSpace(contentText(r.Message.Content))
	if text == "" || strings.HasPrefix(text, "<") {
		return Message{}, false
	}

	return Message{Role: r.Message.Role, Text: text, Time: r.Timestamp}, true
}

// contentText extracts the text of a message's content, which is either a
// plain string or a list of blocks of which only "text" blocks carry text.
func contentText(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}

	var parts []string
	for _, block := range blocks {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// stopSession ends a session gracefully. A background session is stopped
// through `claude stop`, which keeps its conversation resumable, with the
// background service up. An
// interactive session gets SIGTERM, on which Claude Code cleans up and exits;
// it is signalled only while running still lists its pid with the same session
// id, so a pid reused by another process is never signalled.
func stopSession(s Session, running []Session) error {
	if s.Kind == "background" {
		return withBackgroundService(func() error { return runClaude("stop", s.ID) })
	}

	stillRunning := slices.ContainsFunc(running, func(r Session) bool {
		return r.PID == s.PID && r.SessionID == s.SessionID
	})
	if !stillRunning {
		return fmt.Errorf("session %s (pid %d) is no longer running", s.Name, s.PID)
	}

	if err := syscall.Kill(s.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", s.PID, err)
	}
	return nil
}

// removeSession deletes a background session and its worktree through
// `claude rm`, which also works when the session's process is gone. Claude
// Code refuses to delete a worktree holding unpushed or uncommitted work, and
// that refusal is returned as the error.
func removeSession(s Session) error {
	return withBackgroundService(func() error { return runClaude("rm", s.ID) })
}

// withBackgroundService runs action while Claude Code's background service is
// up. `claude stop` and `claude rm` confirm a session ended only through that
// service, and do not start it. The service runs on demand and exits once no
// client or live background session needs it, so it is down exactly when a
// session's process has died while its record still lists it as running.
//
// When the service is down, a supervisor is started for the action and shut
// down afterwards with --keep-workers, which leaves running any background
// session started through it in the meantime. A supervisor another client
// started first is left alone.
func withBackgroundService(action func() error) error {
	if backgroundServicePID() != 0 {
		return action()
	}

	service := exec.Command("claude", "daemon", "run")
	if err := service.Start(); err != nil {
		return fmt.Errorf("start the background service: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- service.Wait() }()

	pid, err := awaitBackgroundService(exited)
	if err != nil {
		_ = service.Process.Signal(syscall.SIGTERM)
		return err
	}

	actionErr := action()

	if pid != service.Process.Pid {
		return actionErr
	}
	if err := runClaude("daemon", "stop", "--any", "--keep-workers"); err != nil {
		_ = service.Process.Signal(syscall.SIGTERM)
		return errors.Join(actionErr, err)
	}
	<-exited //nolint:errcheck // the stopped supervisor's exit status says nothing about the action

	return actionErr
}

// awaitBackgroundService waits until a background service accepts
// connections and returns its pid. The supervisor started for the action
// exits at once when another client's service got there first, which then
// is the one returned.
func awaitBackgroundService(exited <-chan error) (int, error) {
	deadline := time.Now().Add(serviceStartTimeout)

	for time.Now().Before(deadline) {
		if pid := backgroundServicePID(); pid != 0 {
			return pid, nil
		}

		select {
		case err := <-exited:
			if pid := backgroundServicePID(); pid != 0 {
				return pid, nil
			}
			return 0, fmt.Errorf("the background service exited on start: %v", err)
		case <-time.After(servicePollInterval):
		}
	}

	return 0, errors.New("the background service did not start")
}

// backgroundServicePID returns the pid of Claude Code's background service,
// or 0 when it is not running.
func backgroundServicePID() int {
	out, err := exec.Command("claude", "daemon", "status").Output()
	if err != nil {
		return 0
	}

	for line := range strings.Lines(string(out)) {
		if value, ok := strings.CutPrefix(line, "pid:"); ok {
			pid, _ := strconv.Atoi(strings.TrimSpace(value))
			return pid
		}
	}
	return 0
}

// runClaude runs a claude subcommand, putting its output in the error when it fails.
func runClaude(args ...string) error {
	out, err := exec.Command("claude", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
