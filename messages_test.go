package main

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestMessagesWindow opens on m over the screen with the selected session's
// name, title and last messages, keeps the screen's line count, and closes on
// m or esc.
func TestMessagesWindow(t *testing.T) {
	for _, closeKey := range []tea.KeyPressMsg{key("m"), {Code: tea.KeyEscape}} {
		m := sizedModel(t, interactive, background)

		m = update(t, m, key("m"))
		lines := renderedLines(m)

		text := strings.Join(lines, "\n")
		for _, want := range []string{"tmp-4f  Fix the login form", "you, ", "fix the login form", "claude, ", "all  good"} {
			if !strings.Contains(text, want) {
				t.Errorf("window lacks %q", want)
			}
		}
		if len(lines) != 30 {
			t.Errorf("rendered %d lines, want 30", len(lines))
		}
		if got := lines[len(lines)-1]; got != "up/down scroll   pgup/pgdown page   home/end top/bottom   m/esc close" {
			t.Errorf("help line is %q", got)
		}

		m = update(t, m, closeKey)
		if m.messages.open || strings.Contains(ansi.Strip(m.render()), "you, ") {
			t.Errorf("%s did not close the window", closeKey)
		}
	}
}

// TestMessagesWindowRendersMarkdown renders Claude's replies as Markdown and
// shows a person's text as typed.
func TestMessagesWindowRendersMarkdown(t *testing.T) {
	s := interactive
	s.Messages = []Message{
		{Role: "user", Text: "keep **these** stars"},
		{Role: "assistant", Text: "# Result\n\nSome **bold** text and `code`."},
	}
	m := update(t, sizedModel(t, s), key("m"))

	text := ansi.Strip(m.renderMessagesWindow())

	yours, reply, _ := strings.Cut(text, "claude:")

	if !strings.Contains(yours, "keep **these** stars") {
		t.Error("the person's text was changed")
	}
	if strings.Contains(reply, "**") || strings.Contains(reply, "`") || strings.Contains(reply, "# Result") ||
		!strings.Contains(reply, "Some bold text and") {
		t.Errorf("reply not rendered as Markdown:\n%s", text)
	}
}

// longSession has a session whose messages are far longer than the screen,
// each line numbered.
var longSession = func() Session {
	s := interactive
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	s.Messages = []Message{{Role: "user", Text: strings.Join(lines, "\n")}}
	return s
}()

// TestMessagesWindowScrolls opens a window too long for the screen at its
// end, scrolls it with the pager keys, and says which lines it shows.
func TestMessagesWindowScrolls(t *testing.T) {
	m := update(t, sizedModel(t, longSession), key("m"))

	text := ansi.Strip(m.render())
	if !strings.Contains(text, "line 60") || strings.Contains(text, "line 1 ") || !strings.Contains(text, "of 61") {
		t.Errorf("window does not open at the end:\n%s", text)
	}
	lines := renderedLines(m)
	if len(lines) != 30 {
		t.Errorf("rendered %d lines, want 30", len(lines))
	}
	if !strings.Contains(lines[0], "Claude Code sessions") || !strings.HasPrefix(lines[29], "up/down scroll") {
		t.Errorf("window covers the title or help line: %q, %q", lines[0], lines[29])
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if text := ansi.Strip(m.render()); !strings.Contains(text, "lines 1-") || strings.Contains(text, "line 60") {
		t.Errorf("home did not scroll to the top:\n%s", text)
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if offset := m.messages.viewport.YOffset(); offset != 1 {
		t.Errorf("down scrolled to %d, want 1", offset)
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if offset := m.messages.viewport.YOffset(); offset <= 1 {
		t.Errorf("pgdown scrolled to %d", offset)
	}

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m.messages.viewport.AtBottom() {
		t.Error("end did not scroll to the bottom")
	}
}

// TestMessagesWindowKeepsSelection leaves the table's cursor where it is
// while the up and down keys scroll the window.
func TestMessagesWindowKeepsSelection(t *testing.T) {
	m := update(t, sizedModel(t, longSession, background), key("m"))

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})

	if s, _ := m.selected(); s.SessionID != longSession.SessionID {
		t.Errorf("selected %q, want the window's session", s.SessionID)
	}
}

// TestMessagesRefreshKeepsScroll keeps the window where it was scrolled when
// the sessions refresh, and follows new messages when it was at the end.
func TestMessagesRefreshKeepsScroll(t *testing.T) {
	m := update(t, sizedModel(t, longSession), key("m"))
	grown := longSession
	grown.Messages = append(
		append([]Message(nil), longSession.Messages...),
		Message{Role: "assistant", Text: "the newest"},
	)

	atEnd := update(t, m, sessionsMsg{sessions: []Session{grown}})
	if text := ansi.Strip(atEnd.render()); !strings.Contains(text, "the newest") {
		t.Error("a window at its end does not show the new message")
	}

	scrolled := update(t, update(t, m, tea.KeyPressMsg{Code: tea.KeyHome}), sessionsMsg{sessions: []Session{grown}})
	if offset := scrolled.messages.viewport.YOffset(); offset != 0 {
		t.Errorf("a refresh moved the window from the top to %d", offset)
	}
}

// TestMessagesWindowFollowsResize fits the window to a resized screen,
// leaving its title and help lines uncovered.
func TestMessagesWindowFollowsResize(t *testing.T) {
	m := update(t, sizedModel(t, longSession), key("m"))

	m = update(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})

	window := strings.Split(ansi.Strip(m.renderMessagesWindow()), "\n")
	if len(window) != 18 {
		t.Errorf("window is %d lines high, want 18", len(window))
	}
	if got := ansi.StringWidth(window[0]); got != 60 {
		t.Errorf("window is %d wide, want 60", got)
	}
}

// TestMessagesWindowClosesWhenSessionGone closes the window once a refresh
// no longer lists its session.
func TestMessagesWindowClosesWhenSessionGone(t *testing.T) {
	m := update(t, sizedModel(t, interactive, background), key("m"))

	m = update(t, m, sessionsMsg{sessions: []Session{background}})

	if m.messages.open {
		t.Error("window still open after its session went away")
	}
}

// TestMessagesWindowWithoutMessages says the session has none.
func TestMessagesWindowWithoutMessages(t *testing.T) {
	s := background
	m := update(t, sizedModel(t, s), key("m"))

	if text := ansi.Strip(m.renderMessagesWindow()); !strings.Contains(text, "No messages found.") {
		t.Errorf("window lacks the notice:\n%s", text)
	}
}

// TestMessagesKeyWithoutSessions opens nothing.
func TestMessagesKeyWithoutSessions(t *testing.T) {
	m := sizedModel(t)

	if m = update(t, m, key("m")); m.messages.open {
		t.Error("opened the messages window without sessions")
	}
}

// TestBackgroundColour records whether the terminal reports a dark
// background, for the Markdown style.
func TestBackgroundColour(t *testing.T) {
	m := sizedModel(t, interactive)

	if m = update(t, m, tea.BackgroundColorMsg{Color: color.White}); m.darkBackground {
		t.Error("white taken for a dark background")
	}
	if m = update(t, m, tea.BackgroundColorMsg{Color: color.Black}); !m.darkBackground {
		t.Error("black taken for a light background")
	}
}
