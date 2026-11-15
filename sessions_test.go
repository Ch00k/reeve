package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeTranscript writes lines as a JSONL transcript and returns its path.
func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func record(t *testing.T, fields map[string]any) string {
	t.Helper()

	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func userText(text, timestamp string) map[string]any {
	return map[string]any{
		"type": "user", "timestamp": timestamp,
		"message": map[string]any{"role": "user", "content": text},
	}
}

func assistantBlocks(timestamp string, blocks ...map[string]any) map[string]any {
	return map[string]any{
		"type": "assistant", "timestamp": timestamp,
		"message": map[string]any{"role": "assistant", "content": blocks},
	}
}

// TestParseSessions decodes both kinds of session from the shape
// `claude agents --json` prints.
func TestParseSessions(t *testing.T) {
	data := []byte(`[
		{"id":"a372fa6d","cwd":"/home/u/docs","kind":"background","startedAt":1783451722926,
		 "sessionId":"a372fa6d-3657","name":"Clarify open item","state":"blocked"},
		{"pid":1943861,"cwd":"/home/u/tmp","kind":"interactive","startedAt":1793436221224,
		 "sessionId":"3537905d-4d26","name":"tmp-4f","status":"idle"}
	]`)

	sessions, err := parseSessions(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	bg, fg := sessions[0], sessions[1]
	if bg.ID != "a372fa6d" || bg.Kind != "background" || bg.Activity() != "blocked" || bg.Cwd != "/home/u/docs" {
		t.Errorf("background session decoded as %+v", bg)
	}
	if fg.PID != 1943861 || fg.Kind != "interactive" || fg.Activity() != "idle" || fg.Name != "tmp-4f" {
		t.Errorf("interactive session decoded as %+v", fg)
	}
}

// TestParseSessionsRejectsInvalidJSON reports output that is not a session list.
func TestParseSessionsRejectsInvalidJSON(t *testing.T) {
	if _, err := parseSessions([]byte("Error: not logged in")); err == nil {
		t.Fatal("expected an error")
	}
}

// TestTranscriptTailSkipsNonConversationalRecords returns the last typed or
// written text, passing over tool results, tool calls, thinking, meta and
// sidechain records, harness-injected tagged messages and non-message records.
func TestTranscriptTailSkipsNonConversationalRecords(t *testing.T) {
	path := writeTranscript(t,
		record(t, userText("first question", "2026-11-04T05:00:00Z")),
		record(t, assistantBlocks("2026-11-04T05:01:00Z",
			map[string]any{"type": "thinking", "thinking": "hmm"},
			map[string]any{"type": "text", "text": "the answer"},
			map[string]any{"type": "tool_use", "name": "Bash"},
		)),
		record(t, assistantBlocks("2026-11-04T05:02:00Z", map[string]any{"type": "tool_use", "name": "Bash"})),
		record(t, map[string]any{
			"type":      "user",
			"timestamp": "2026-11-04T05:03:00Z",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "tool_result", "content": "ok"}},
			},
		}),
		record(t, map[string]any{
			"type": "user", "isMeta": true, "timestamp": "2026-11-04T05:04:00Z",
			"message": map[string]any{"role": "user", "content": "meta text"},
		}),
		record(t, map[string]any{
			"type":        "assistant",
			"isSidechain": true,
			"timestamp":   "2026-11-04T05:05:00Z",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "subagent"}},
			},
		}),
		record(t, userText("<local-command-stdout>Reconnected</local-command-stdout>", "2026-11-04T05:06:00Z")),
		record(t, map[string]any{"type": "attachment", "timestamp": "2026-11-04T05:07:00Z"}),
		"not json",
	)

	_, messages, err := transcriptTail(path, 1)
	if err != nil {
		t.Fatal(err)
	}

	want := Message{Role: "assistant", Text: "the answer", Time: time.Date(2026, 11, 4, 5, 1, 0, 0, time.UTC)}
	if len(messages) != 1 {
		t.Fatalf("got %+v, want only %+v", messages, want)
	}
	if msg := messages[0]; msg.Role != want.Role || msg.Text != want.Text || !msg.Time.Equal(want.Time) {
		t.Errorf("got %+v, want %+v", msg, want)
	}
}

// TestTranscriptTailTitleAndMessages returns the latest generated title and
// the last count messages, oldest first.
func TestTranscriptTailTitleAndMessages(t *testing.T) {
	path := writeTranscript(t,
		record(t, userText("one", "2026-11-04T05:00:00Z")),
		record(t, map[string]any{"type": "ai-title", "aiTitle": "First title"}),
		record(t, userText("two", "2026-11-04T05:01:00Z")),
		record(t, map[string]any{"type": "ai-title", "aiTitle": "Latest title"}),
		record(t, userText("three", "2026-11-04T05:02:00Z")),
		record(t, userText("four", "2026-11-04T05:03:00Z")),
	)

	title, messages, err := transcriptTail(path, 3)
	if err != nil {
		t.Fatal(err)
	}

	if title != "Latest title" {
		t.Errorf("title %q, want %q", title, "Latest title")
	}
	var texts []string
	for _, msg := range messages {
		texts = append(texts, msg.Text)
	}
	if got := strings.Join(texts, "|"); got != "two|three|four" {
		t.Errorf("messages %q, want two|three|four", got)
	}
}

// TestTranscriptTailJoinsTextBlocks joins all text blocks of a message and
// trims surrounding whitespace.
func TestTranscriptTailJoinsTextBlocks(t *testing.T) {
	path := writeTranscript(t, record(t, assistantBlocks("2026-11-04T05:01:00Z",
		map[string]any{"type": "text", "text": "  part one"},
		map[string]any{"type": "tool_use", "name": "Bash"},
		map[string]any{"type": "text", "text": "part two\n"},
	)))

	_, messages, err := transcriptTail(path, 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || messages[0].Text != "part one\npart two" {
		t.Errorf("got %+v", messages)
	}
}

// TestTranscriptTailBeyondFirstWindow finds a title and a message that lie
// before the first tail window by widening the window, and ignores the partial
// line the window starts in.
func TestTranscriptTailBeyondFirstWindow(t *testing.T) {
	lines := []string{
		record(t, map[string]any{"type": "ai-title", "aiTitle": "Early title"}),
		record(t, userText("early message", "2026-11-04T05:00:00Z")),
	}

	// Tool results totalling over three windows push the message out of the first two.
	filler := record(t, map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "tool_result", "content": strings.Repeat("x", 4096)}},
		},
	})
	for range 3 * transcriptTailChunk / len(filler) {
		lines = append(lines, filler)
	}
	path := writeTranscript(t, lines...)

	title, messages, err := transcriptTail(path, 1)
	if err != nil {
		t.Fatal(err)
	}

	if title != "Early title" || len(messages) != 1 || messages[0].Text != "early message" {
		t.Errorf("got title %q, messages %+v", title, messages)
	}
}

// TestTranscriptTailWithoutTitleOrMessages returns neither.
func TestTranscriptTailWithoutTitleOrMessages(t *testing.T) {
	path := writeTranscript(t, record(t, map[string]any{"type": "permission-mode"}))

	title, messages, err := transcriptTail(path, 3)
	if err != nil {
		t.Fatal(err)
	}

	if title != "" || len(messages) != 0 {
		t.Errorf("got title %q, messages %+v; want neither", title, messages)
	}
}

// TestTranscriptTailOfMissingFile reports the error.
func TestTranscriptTailOfMissingFile(t *testing.T) {
	_, _, err := transcriptTail(filepath.Join(t.TempDir(), "missing.jsonl"), 1)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("got %v, want a not-exist error", err)
	}
}

// TestTranscriptPath finds a transcript under any project directory and
// returns "" when there is none.
func TestTranscriptPath(t *testing.T) {
	projects := t.TempDir()
	dir := filepath.Join(projects, "-home-u-elsewhere")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "abc-123.jsonl")
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := transcriptPath(projects, "abc-123")
	if err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}

	got, err = transcriptPath(projects, "other")
	if err != nil || got != "" {
		t.Errorf("got %q, %v; want no transcript", got, err)
	}
}

// TestClaudeConfigDir honours CLAUDE_CONFIG_DIR and falls back to ~/.claude.
func TestClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	if dir, err := claudeConfigDir(); err != nil || dir != "/custom/claude" {
		t.Errorf("got %q, %v", dir, err)
	}

	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "/home/someone")
	if dir, err := claudeConfigDir(); err != nil || dir != "/home/someone/.claude" {
		t.Errorf("got %q, %v", dir, err)
	}
}

// startProcess starts a real long-running child process standing in for an
// interactive session, and returns it with a channel that receives how it ended.
func startProcess(t *testing.T) (*exec.Cmd, chan error) {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	return cmd, done
}

// TestStopInteractiveSessionSendsSIGTERM terminates a running process listed
// with the same pid and session id, by SIGTERM.
func TestStopInteractiveSessionSendsSIGTERM(t *testing.T) {
	cmd, done := startProcess(t)
	s := Session{Kind: "interactive", Name: "s", PID: cmd.Process.Pid, SessionID: "sid"}

	if err := stopSession(s, []Session{s}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("process ended with %v", err)
		}
		status := exitErr.Sys().(syscall.WaitStatus)
		if !status.Signaled() || status.Signal() != syscall.SIGTERM {
			t.Errorf("process ended with %v, want SIGTERM", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process still running")
	}
}

// TestStopInteractiveSessionNoLongerListed leaves the process alone when the
// fresh listing has no session with that pid and session id, as happens when
// the session exited and its pid was reused.
func TestStopInteractiveSessionNoLongerListed(t *testing.T) {
	cmd, done := startProcess(t)
	s := Session{Kind: "interactive", Name: "s", PID: cmd.Process.Pid, SessionID: "sid"}
	reused := Session{Kind: "interactive", PID: cmd.Process.Pid, SessionID: "another"}

	err := stopSession(s, []Session{reused})
	if err == nil || !strings.Contains(err.Error(), "no longer running") {
		t.Fatalf("got %v, want a no-longer-running error", err)
	}

	select {
	case err := <-done:
		t.Fatalf("process ended with %v", err)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestBackgroundActionsReportClaudeOutput runs the real `claude stop` and
// `claude rm` on an id no session has, and reports the command with its
// output when it fails. Changes nothing.
func TestBackgroundActionsReportClaudeOutput(t *testing.T) {
	requireClaude(t)
	s := Session{Kind: "background", ID: "zzzz0000", Name: "missing"}

	err := stopSession(s, nil)
	if err == nil ||
		!strings.HasPrefix(err.Error(), "claude stop zzzz0000: exit status 1: No job matching 'zzzz0000'") {
		t.Errorf("stop: got %v", err)
	}

	err = removeSession(s)
	if err == nil || err.Error() != "claude rm zzzz0000: exit status 1: No job matching 'zzzz0000'" {
		t.Errorf("rm: got %v", err)
	}
}

// TestWithBackgroundService runs the action while Claude Code's real
// background service is up, and leaves the service as it found it: shut down
// again when it was down before, still running when another client had it up.
func TestWithBackgroundService(t *testing.T) {
	requireClaude(t)
	before := backgroundServicePID()

	var during int
	err := withBackgroundService(func() error {
		during = backgroundServicePID()
		return errors.New("action failed")
	})

	if err == nil || err.Error() != "action failed" {
		t.Errorf("got %v, want the action's error", err)
	}
	if during == 0 {
		t.Error("background service was not running during the action")
	}
	if before != 0 && during != before {
		t.Errorf("service pid %d during the action, want the running %d", during, before)
	}

	if after := backgroundServicePID(); (after != 0) != (before != 0) {
		t.Errorf("service pid %d after the action, %d before", after, before)
	}
}

// requireClaude skips a test that runs the real claude CLI where it is not
// installed, as on CI.
func requireClaude(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not on PATH")
	}
}
