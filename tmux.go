package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// tmuxPanes maps the id of every pane of the tmux server to its location as
// session:window.pane. It is empty when no tmux server runs.
func tmuxPanes() map[string]string {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}").
		Output()
	if err != nil {
		return nil
	}
	return parseTmuxPanes(string(out))
}

func parseTmuxPanes(out string) map[string]string {
	panes := make(map[string]string)
	for line := range strings.Lines(out) {
		if id, location, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			panes[id] = location
		}
	}
	return panes
}

// recordedTmuxPane returns the id of the tmux pane an interactive session
// started in, as Claude Code records it in the session's file, or "" when it
// did not start inside tmux.
func recordedTmuxPane(configDir string, pid int) string {
	data, err := os.ReadFile(filepath.Join(configDir, "sessions", fmt.Sprintf("%d.json", pid)))
	if err != nil {
		return ""
	}

	var record struct {
		Tmux string `json:"tmux"`
	}
	if json.Unmarshal(data, &record) != nil {
		return ""
	}
	return paneID(record.Tmux)
}

// paneID extracts the pane id from a recorded location such as
// "misc:@87.%263", whose session name can change while the pane id cannot.
func paneID(location string) string {
	i := strings.LastIndex(location, "%")
	if i < 0 {
		return ""
	}
	return location[i:]
}

// insideTmux reports whether this program runs in a tmux pane.
func insideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// switchToPane moves the tmux client this program runs in to a pane, making
// its session, window and pane current.
func switchToPane(pane string) error {
	out, err := exec.Command("tmux", "switch-client", "-t", pane).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux switch-client -t %s: %w: %s", pane, err, strings.TrimSpace(string(out)))
	}
	return nil
}
