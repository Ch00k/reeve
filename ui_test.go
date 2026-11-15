package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	interactive = Session{
		Kind: "interactive", PID: 101, SessionID: "sid-1", Name: "tmp-4f", Cwd: "/home/u/tmp", Status: "idle",
		StartedAt: time.Now().Add(-(3*time.Hour + 12*time.Minute + 30*time.Second)).UnixMilli(),
		Title:     "Fix the login form",
		Messages: []Message{
			{Role: "user", Text: "fix the login form", Time: time.Date(2026, 11, 4, 5, 0, 0, 0, time.UTC)},
			{Role: "assistant", Text: "done\n\nall  good", Time: time.Date(2026, 11, 4, 5, 1, 0, 0, time.UTC)},
		},
	}
	background = Session{
		Kind:      "background",
		ID:        "a372fa6d",
		SessionID: "sid-2",
		Name:      "Clarify item",
		Cwd:       "/srv/docs",
		State:     "blocked",
		StartedAt: time.Now().Add(-26 * time.Hour).UnixMilli(),
	}
)

// sizedModel is a model laid out for a 160x30 terminal showing sessions.
func sizedModel(t *testing.T, sessions ...Session) model {
	t.Helper()

	m := newModel(3)
	m.home = "/home/u"
	m = update(t, m, tea.WindowSizeMsg{Width: 160, Height: 30})
	return update(t, m, sessionsMsg{sessions: sessions})
}

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()

	next, _ := m.Update(msg)
	return next.(model)
}

func key(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(text)[0], Text: text}
}

// TestSessionsSelectFirstRow puts the cursor on the first session once the
// first list arrives.
func TestSessionsSelectFirstRow(t *testing.T) {
	m := sizedModel(t, interactive, background)

	s, ok := m.selected()
	if !ok || s.SessionID != interactive.SessionID {
		t.Errorf("selected %+v, %v; want the first session", s, ok)
	}
}

// TestRefreshKeepsSelectedSession keeps the cursor on the same session when a
// refresh reorders the list.
func TestRefreshKeepsSelectedSession(t *testing.T) {
	m := sizedModel(t, interactive, background)
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})

	m = update(t, m, sessionsMsg{sessions: []Session{background, interactive}})

	if s, _ := m.selected(); s.SessionID != background.SessionID {
		t.Errorf("selected %q, want %q", s.SessionID, background.SessionID)
	}
}

// TestRefreshErrorKeepsSessions shows a failed refresh as an error and keeps
// the sessions already shown.
func TestRefreshErrorKeepsSessions(t *testing.T) {
	m := sizedModel(t, interactive)

	m = update(t, m, sessionsMsg{err: errors.New("claude agents --json: exit status 1")})

	if !m.statusIsErr || m.status != "claude agents --json: exit status 1" {
		t.Errorf("status %q (error %v)", m.status, m.statusIsErr)
	}
	if len(m.sessions) != 1 {
		t.Errorf("got %d sessions, want 1", len(m.sessions))
	}
}

// TestStopKeyAsksForConfirmation and cancelling with n or esc leaves the
// session running.
func TestStopKeyAsksForConfirmation(t *testing.T) {
	for _, cancel := range []tea.KeyPressMsg{key("n"), {Code: tea.KeyEscape}} {
		m := sizedModel(t, interactive)

		m = update(t, m, key("s"))
		if m.confirming != stopAction || m.target.SessionID != interactive.SessionID {
			t.Fatalf("s set confirming %v on %q; want stopping the selected session", m.confirming, m.target.SessionID)
		}

		next, cmd := m.Update(cancel)
		m = next.(model)
		if m.confirming != noAction || cmd != nil {
			t.Errorf("%s: confirming %v, command %v; want cancelled", cancel, m.confirming, cmd != nil)
		}
	}
}

// TestConfirmStartsStop confirms with y or enter, which starts stopping the
// selected session.
func TestConfirmStartsStop(t *testing.T) {
	for _, confirm := range []tea.KeyPressMsg{key("y"), {Code: tea.KeyEnter}} {
		m := sizedModel(t, interactive)
		m = update(t, m, key("s"))

		next, cmd := m.Update(confirm)
		m = next.(model)

		if m.confirming != noAction || cmd == nil {
			t.Errorf("%s: confirming %v, command %v; want a stop command", confirm, m.confirming, cmd != nil)
		}
		if m.status != "Stopping tmp-4f..." {
			t.Errorf("%s: status %q", confirm, m.status)
		}
	}
}

// stopFailed is the result of `claude stop` failing on a background session
// whose process is gone.
var stopFailed = actionDoneMsg{
	action:  stopAction,
	session: background,
	err:     errors.New("claude stop a372fa6d: exit status 1: couldn't confirm a372fa6d was stopped"),
}

// TestFailedBackgroundStopOffersRemoval asks whether to remove a background
// session that could not be stopped, selecting it even when the cursor moved
// while the stop ran, and keeps the error shown.
func TestFailedBackgroundStopOffersRemoval(t *testing.T) {
	m := sizedModel(t, interactive, background)

	m = update(t, m, stopFailed)

	if m.confirming != removeAction || m.target.SessionID != background.SessionID {
		t.Errorf("confirming %v on %q; want removing the background session", m.confirming, m.target.SessionID)
	}
	if s, _ := m.selected(); s.SessionID != background.SessionID {
		t.Errorf("selected %q, want the background session", s.SessionID)
	}
	if !m.statusIsErr || m.status != stopFailed.err.Error() {
		t.Errorf("status %q (error %v)", m.status, m.statusIsErr)
	}
}

// TestRemovalOfferSurvivesRefresh keeps asking while the session is still
// listed, as a dead session stays listed.
func TestRemovalOfferSurvivesRefresh(t *testing.T) {
	m := sizedModel(t, interactive, background)
	m = update(t, m, stopFailed)

	m = update(t, m, sessionsMsg{sessions: []Session{interactive, background}})

	if m.confirming != removeAction {
		t.Errorf("confirming %v after a refresh, want removing", m.confirming)
	}
}

// TestConfirmRemovalStartsRemove removes the session the question named on y,
// and n leaves it alone.
func TestConfirmRemovalStartsRemove(t *testing.T) {
	m := sizedModel(t, interactive, background)
	m = update(t, m, stopFailed)

	next, cmd := m.Update(key("y"))
	confirmed := next.(model)
	if confirmed.confirming != noAction || cmd == nil || confirmed.status != "Removing Clarify item..." {
		t.Errorf(
			"confirming %v, command %v, status %q; want removing",
			confirmed.confirming,
			cmd != nil,
			confirmed.status,
		)
	}

	next, cmd = m.Update(key("n"))
	if next.(model).confirming != noAction || cmd != nil {
		t.Error("n did not cancel the removal")
	}
}

// TestFailedInteractiveStopOffersNothing only reports the error, since an
// interactive session has no record to remove.
func TestFailedInteractiveStopOffersNothing(t *testing.T) {
	m := sizedModel(t, interactive)

	m = update(
		t,
		m,
		actionDoneMsg{action: stopAction, session: interactive, err: errors.New("signal pid 101: no such process")},
	)

	if m.confirming != noAction {
		t.Errorf("confirming %v, want nothing", m.confirming)
	}
}

// TestFailedRemovalOffersNothingMore only reports the error.
func TestFailedRemovalOffersNothingMore(t *testing.T) {
	m := sizedModel(t, background)

	m = update(
		t,
		m,
		actionDoneMsg{action: removeAction, session: background, err: errors.New("claude rm a372fa6d: exit status 1")},
	)

	if m.confirming != noAction {
		t.Errorf("confirming %v, want nothing", m.confirming)
	}
}

// TestStopKeyWithoutSessions does nothing when no session is listed.
func TestStopKeyWithoutSessions(t *testing.T) {
	m := sizedModel(t)

	m = update(t, m, key("s"))

	if m.confirming != noAction {
		t.Error("asked to confirm stopping no session")
	}
}

// TestConfirmationDroppedWhenSessionGone cancels the confirmation when a
// refresh no longer lists the session it was about.
func TestConfirmationDroppedWhenSessionGone(t *testing.T) {
	m := sizedModel(t, interactive, background)
	m = update(t, m, key("s"))

	m = update(t, m, sessionsMsg{sessions: []Session{background}})

	if m.confirming != noAction {
		t.Error("still confirming after the session went away")
	}
}

// TestActionResultStatus reports how each kind of session was stopped or
// removed, and a failure as an error.
func TestActionResultStatus(t *testing.T) {
	cases := []struct {
		msg    actionDoneMsg
		status string
		isErr  bool
	}{
		{actionDoneMsg{action: stopAction, session: interactive}, "Sent SIGTERM to tmp-4f (pid 101)", false},
		{actionDoneMsg{action: stopAction, session: background}, "Stopped Clarify item (claude stop a372fa6d)", false},
		{actionDoneMsg{action: removeAction, session: background}, "Removed Clarify item (claude rm a372fa6d)", false},
		{
			actionDoneMsg{
				action:  stopAction,
				session: interactive,
				err:     errors.New("session tmp-4f (pid 101) is no longer running"),
			},
			"session tmp-4f (pid 101) is no longer running",
			true,
		},
		{
			actionDoneMsg{
				action:  removeAction,
				session: background,
				err:     errors.New("claude rm a372fa6d: exit status 1: unpushed commits"),
			},
			"claude rm a372fa6d: exit status 1: unpushed commits",
			true,
		},
	}

	for _, c := range cases {
		m := sizedModel(t, interactive, background)

		next, cmd := m.Update(c.msg)
		m = next.(model)

		if m.status != c.status || m.statusIsErr != c.isErr {
			t.Errorf("status %q (error %v), want %q (error %v)", m.status, m.statusIsErr, c.status, c.isErr)
		}
		if cmd == nil {
			t.Error("no refresh after stopping")
		}
	}
}

// renderedLines returns the plain text of the rendered screen's lines.
func renderedLines(m model) []string {
	return strings.Split(ansi.Strip(m.render()), "\n")
}

// TestViewLeavesMouseToTerminal keeps mouse reporting off, so the terminal
// handles text selection.
func TestViewLeavesMouseToTerminal(t *testing.T) {
	m := sizedModel(t, interactive)

	if mode := m.View().MouseMode; mode != tea.MouseModeNone {
		t.Errorf("mouse mode %v, want none", mode)
	}
}

// TestFrameTopEdge counts the sessions at the start of the session list's
// top edge and names the sort order and refresh time at its end.
func TestFrameTopEdge(t *testing.T) {
	m := sizedModel(t, interactive, background)

	line := renderedLines(m)[0]

	start := frameBorder.TopLeft + frameBorder.Top + " Claude Code sessions: 2 " + frameBorder.Top
	if !strings.HasPrefix(line, start) || !strings.Contains(line, " sorted by age ascending, refreshed ") ||
		!strings.HasSuffix(line, frameBorder.Top+frameBorder.TopRight) {
		t.Errorf("top edge is %q", line)
	}
	if got := ansi.StringWidth(line); got != 160 {
		t.Errorf("top edge is %d wide, want 160", got)
	}
}

// TestHeaderSeparatorJoinsFrame draws the line under the column headers
// across the frame, joined to its sides.
func TestHeaderSeparatorJoinsFrame(t *testing.T) {
	m := sizedModel(t, interactive)

	line := renderedLines(m)[headerSeparatorRow]

	if want := frameBorder.MiddleLeft + strings.Repeat(frameBorder.Top, 158) + frameBorder.MiddleRight; line != want {
		t.Errorf("separator is %q", line)
	}
}

// TestConfirmationPrompt replaces the help line with a question naming the
// session and the keys that answer it.
func TestConfirmationPrompt(t *testing.T) {
	m := sizedModel(t, interactive)
	m = update(t, m, key("s"))

	lines := renderedLines(m)

	if got := lines[len(lines)-1]; got != "Stop tmp-4f?  y/enter yes   n/esc cancel" {
		t.Errorf("help line is %q", got)
	}
}

// TestRemovalPrompt names the session, warns that removal is final, and shows
// the keys that answer it.
func TestRemovalPrompt(t *testing.T) {
	m := sizedModel(t, background)
	m = update(t, m, stopFailed)

	lines := renderedLines(m)

	want := "Remove Clarify item instead? It can't be resumed afterwards.  y/enter yes   n/esc cancel"
	if got := lines[len(lines)-1]; got != want {
		t.Errorf("help line is %q, want %q", got, want)
	}
}

// TestHelpLine lists the keys and their actions on the last line, each key
// styled apart from its action. The tmux key is listed only inside tmux.
func TestHelpLine(t *testing.T) {
	cases := map[string]string{
		"/tmp/tmux-1000/default,1,0": "up/down select   o sort   O reverse   m messages   t go to tmux pane   s stop   r refresh   q quit",
		"":                           "up/down select   o sort   O reverse   m messages   s stop   r refresh   q quit",
	}
	for tmuxEnv, want := range cases {
		t.Setenv("TMUX", tmuxEnv)
		m := sizedModel(t, interactive)

		lines := renderedLines(m)

		if got := lines[len(lines)-1]; got != want {
			t.Errorf("TMUX=%q: help line is %q, want %q", tmuxEnv, got, want)
		}
	}
	if got := renderKeyHints(keyHint{"s", "stop"}); got != keyStyle.Render("s")+" "+dimStyle.Render("stop") {
		t.Errorf("key hint rendered as %q", got)
	}
}

// sessionNames lists the names of the sessions in table order.
func sessionNames(m model) string {
	names := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		names[i] = s.Name
	}
	return strings.Join(names, ",")
}

// TestSortKeys sorts by age, youngest first, until o moves the sort to the
// next column and O reverses it, keeping the cursor on the selected session.
func TestSortKeys(t *testing.T) {
	m := sizedModel(t, background, interactive)
	if got := sessionNames(m); got != "tmp-4f,Clarify item" {
		t.Fatalf("sessions in order %q, want the youngest first", got)
	}

	m = update(t, m, key("o"))
	if m.sortColumn != kindColumn || m.sortDescending || sessionNames(m) != "Clarify item,tmp-4f" {
		t.Errorf(
			"after o: sorted by %v (descending %v) as %q; want by kind",
			m.sortColumn,
			m.sortDescending,
			sessionNames(m),
		)
	}
	if s, _ := m.selected(); s.SessionID != interactive.SessionID {
		t.Errorf("selected %q after sorting, want the session selected before", s.SessionID)
	}

	m = update(t, m, key("O"))
	if !m.sortDescending || sessionNames(m) != "tmp-4f,Clarify item" {
		t.Errorf("after O: descending %v, order %q; want by kind reversed", m.sortDescending, sessionNames(m))
	}

	m = update(t, m, key("o"))
	if m.sortColumn != stateColumn || m.sortDescending {
		t.Errorf("after o: sorted by %v (descending %v); want by state ascending", m.sortColumn, m.sortDescending)
	}
}

// TestSortWrapsAround moves the sort from the last column back to the first.
func TestSortWrapsAround(t *testing.T) {
	m := sizedModel(t, interactive)

	m = update(t, m, key("o"))

	if m.sortColumn != kindColumn {
		t.Errorf("sorted by %v after the age column, want kind", m.sortColumn)
	}
}

// TestRefreshKeepsSortOrder sorts a freshly loaded list the way the table
// is sorted.
func TestRefreshKeepsSortOrder(t *testing.T) {
	m := sizedModel(t, interactive, background)
	m = update(t, m, key("O"))

	m = update(t, m, sessionsMsg{sessions: []Session{interactive, background}})

	if got := sessionNames(m); got != "Clarify item,tmp-4f" {
		t.Errorf("sessions in order %q, want the oldest first", got)
	}
}

// inTmux is the interactive session running in a tmux pane.
var inTmux = func() Session {
	s := interactive
	s.TmuxPane, s.TmuxLocation = "%263", "misc:6.2"
	return s
}()

// TestTmuxKeyGoesToTmuxPane starts switching to the selected session's pane
// from inside tmux. The command is not run, as it would move the tmux client
// running the tests.
func TestTmuxKeyGoesToTmuxPane(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	m := sizedModel(t, inTmux)

	next, cmd := m.Update(key("t"))

	if cmd == nil {
		t.Error("no command to switch")
	}
	if next.(model).statusIsErr {
		t.Errorf("status %q", next.(model).status)
	}
}

// TestTmuxKeyOutsideTmux does nothing when this program runs outside tmux.
func TestTmuxKeyOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	m := sizedModel(t, inTmux)

	next, cmd := m.Update(key("t"))

	if cmd != nil || next.(model).status != "" {
		t.Errorf("command %v, status %q; want nothing", cmd != nil, next.(model).status)
	}
}

// TestTmuxKeyWithoutTmuxPane explains why there is nowhere to go: a background
// session has no pane, and an interactive one may run outside tmux.
func TestTmuxKeyWithoutTmuxPane(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	cases := map[string]Session{
		"Clarify item runs in the background; open it with claude attach a372fa6d": background,
		"tmp-4f is not running in a tmux pane":                                     interactive,
	}

	for want, s := range cases {
		m := sizedModel(t, s)

		next, cmd := m.Update(key("t"))
		m = next.(model)

		if cmd != nil || !m.statusIsErr || m.status != want {
			t.Errorf("command %v, status %q (error %v); want %q", cmd != nil, m.status, m.statusIsErr, want)
		}
	}
}

// TestSwitchResultStatus reports where the switch went, or why it failed.
func TestSwitchResultStatus(t *testing.T) {
	m := sizedModel(t, inTmux)

	ok := update(t, m, switchDoneMsg{session: inTmux})
	if ok.status != "Switched to tmp-4f (tmux misc:6.2)" || ok.statusIsErr {
		t.Errorf("status %q (error %v)", ok.status, ok.statusIsErr)
	}

	failed := update(
		t,
		m,
		switchDoneMsg{
			session: inTmux,
			err:     errors.New("tmux switch-client -t %263: exit status 1: can't find pane: %263"),
		},
	)
	if failed.status != "tmux switch-client -t %263: exit status 1: can't find pane: %263" || !failed.statusIsErr {
		t.Errorf("status %q (error %v)", failed.status, failed.statusIsErr)
	}
}

// TestRenderFillsScreen draws exactly as many lines as the terminal has, so
// the footer stays at the bottom.
func TestRenderFillsScreen(t *testing.T) {
	m := sizedModel(t, interactive, background)

	if got := len(renderedLines(m)); got != 30 {
		t.Errorf("rendered %d lines, want 30", got)
	}
}

// TestLongStatusWraps shows a status wider than the screen in full over the
// status lines, keeping the screen's line count.
func TestLongStatusWraps(t *testing.T) {
	m := newModel(3)
	m = update(t, m, tea.WindowSizeMsg{Width: 60, Height: 30})
	m = update(t, m, sessionsMsg{sessions: []Session{background}})
	errText := "claude stop a372fa6d: exit status 1: couldn't confirm a372fa6d was stopped, " +
		"the background service may be restarting. Try again in a moment."
	m = update(t, m, actionDoneMsg{action: stopAction, session: background, err: errors.New(errText)})

	lines := renderedLines(m)

	if len(lines) != 30 {
		t.Fatalf("rendered %d lines, want 30", len(lines))
	}
	status := lines[len(lines)-footerHeight : len(lines)-1]
	joined := strings.Join(strings.Fields(strings.Join(status, " ")), " ")
	if joined != errText {
		t.Errorf("status lines %q, want the whole error", status)
	}
}
