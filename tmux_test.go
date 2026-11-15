package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseTmuxPanes maps each pane id to its session:window.pane location,
// including session names with spaces.
func TestParseTmuxPanes(t *testing.T) {
	panes := parseTmuxPanes("%263 misc:6.2\n%277 personal:3.1\n%9 my notes:0.0\n")

	want := map[string]string{"%263": "misc:6.2", "%277": "personal:3.1", "%9": "my notes:0.0"}
	if len(panes) != len(want) {
		t.Fatalf("got %v, want %v", panes, want)
	}
	for id, location := range want {
		if panes[id] != location {
			t.Errorf("pane %s at %q, want %q", id, panes[id], location)
		}
	}
}

// TestPaneID extracts the pane id from a recorded tmux location.
func TestPaneID(t *testing.T) {
	for location, want := range map[string]string{
		"misc:@87.%263": "%263",
		"%5":            "%5",
		"":              "",
		"misc:@87":      "",
	} {
		if got := paneID(location); got != want {
			t.Errorf("paneID(%q) = %q, want %q", location, got, want)
		}
	}
}

// TestRecordedTmuxPane reads the pane from a session's file, and returns ""
// for a session that did not start in tmux or has no readable file.
func TestRecordedTmuxPane(t *testing.T) {
	configDir := t.TempDir()
	sessionsDir := filepath.Join(configDir, "sessions")
	if err := os.Mkdir(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"101.json": `{"pid":101,"tmux":"misc:@87.%263","kind":"interactive"}`,
		"102.json": `{"pid":102,"kind":"interactive"}`,
		"103.json": `not json`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sessionsDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for pid, want := range map[int]string{101: "%263", 102: "", 103: "", 104: ""} {
		if got := recordedTmuxPane(configDir, pid); got != want {
			t.Errorf("pid %d: got %q, want %q", pid, got, want)
		}
	}
}
