package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	refreshInterval = 3 * time.Second
	timeLayout      = "2006-01-02 15:04"

	// footerHeight is the status (statusLines, wrapped) and help lines below
	// the session list.
	statusLines  = 3
	footerHeight = statusLines + 1

	// headerSeparatorRow is the screen row of the line under the column
	// headers: below the frame's top edge and the headers.
	headerSeparatorRow = 2
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	listBorderColor = lipgloss.Color("240")
)

type sessionsMsg struct {
	sessions []Session
	err      error
}

type tickMsg struct{}

// action is something done to a session once the user confirms it.
type action int

const (
	noAction action = iota
	stopAction
	removeAction
)

type actionDoneMsg struct {
	action  action
	session Session
	err     error
}

type switchDoneMsg struct {
	session Session
	err     error
}

type model struct {
	table    table.Model
	sessions []Session
	width    int
	height   int
	// sortColumn and sortDescending order the sessions.
	sortColumn     column
	sortDescending bool
	// confirming is the action awaiting confirmation, on target.
	confirming  action
	target      Session
	status      string
	statusIsErr bool
	refreshedAt time.Time
	home        string
	// messageCount is how many last messages are loaded per session for the
	// messages window.
	messageCount int
	messages     messagesWindow
	// darkBackground is whether the terminal's background is dark, for
	// rendering Markdown; it is assumed until the terminal reports its colour.
	darkBackground bool
}

func newModel(messageCount int) model {
	home, _ := os.UserHomeDir()

	t := table.New(table.WithFocused(true))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Foreground(lipgloss.Color("245")).
		Border(lipgloss.NormalBorder(), false, false, true).BorderForeground(listBorderColor)
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("3"))
	t.SetStyles(styles)

	return model{table: t, home: home, sortColumn: ageColumn, messageCount: messageCount, darkBackground: true}
}

func (m model) loadCmd() tea.Msg {
	sessions, err := loadSessions(m.messageCount)
	return sessionsMsg{sessions: sessions, err: err}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func stopCmd(s Session) tea.Cmd {
	return func() tea.Msg {
		running, err := fetchSessions()
		if err != nil {
			return actionDoneMsg{action: stopAction, session: s, err: err}
		}
		return actionDoneMsg{action: stopAction, session: s, err: stopSession(s, running)}
	}
}

func removeCmd(s Session) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{action: removeAction, session: s, err: removeSession(s)}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd, tickCmd(), tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layoutTable()
		m.refreshMessages()
		return m, nil

	case tea.BackgroundColorMsg:
		m.darkBackground = msg.IsDark()
		m.refreshMessages()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadCmd, tickCmd())

	case sessionsMsg:
		m.applySessions(msg)
		return m, nil

	case actionDoneMsg:
		m.applyActionResult(msg)
		return m, m.loadCmd

	case switchDoneMsg:
		m.applySwitchResult(msg)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.confirming != noAction {
		switch msg.String() {
		case "y", "enter":
			return m.confirmAction()
		case "n", "esc":
			m.confirming = noAction
		}
		return m, nil
	}

	if m.messages.open {
		return m.handleMessagesKey(msg)
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "r":
		return m, m.loadCmd
	case "s":
		m.askToStop()
		return m, nil
	case "m":
		if s, ok := m.selected(); ok {
			m.openMessages(s)
		}
		return m, nil
	case "o":
		m.sortBy((m.sortColumn+1)%columnCount, false)
		return m, nil
	case "O":
		m.sortBy(m.sortColumn, !m.sortDescending)
		return m, nil
	case "t":
		return m, m.goToPane()
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// sortBy reorders the table, keeping the cursor on the session it was on.
func (m *model) sortBy(c column, descending bool) {
	selected, ok := m.selected()
	m.sortColumn, m.sortDescending = c, descending

	m.sortSessions()
	m.layoutTable()

	if ok {
		m.selectSession(selected.SessionID)
	}
}

// selected is the session under the table cursor.
func (m model) selected() (Session, bool) {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.sessions) {
		return Session{}, false
	}
	return m.sessions[cursor], true
}

// goToPane moves the tmux client this program runs in to the tmux pane of the
// selected session. Outside tmux it does nothing.
func (m *model) goToPane() tea.Cmd {
	s, ok := m.selected()
	if !ok || !insideTmux() {
		return nil
	}

	if s.Kind == "background" {
		m.setStatus(fmt.Sprintf("%s runs in the background; open it with claude attach %s", s.Name, s.ID), true)
		return nil
	}
	if s.TmuxPane == "" {
		m.setStatus(fmt.Sprintf("%s is not running in a tmux pane", s.Name), true)
		return nil
	}

	return func() tea.Msg { return switchDoneMsg{session: s, err: switchToPane(s.TmuxPane)} }
}

func (m *model) applySwitchResult(msg switchDoneMsg) {
	if msg.err != nil {
		m.setStatus(msg.err.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("Switched to %s (tmux %s)", msg.session.Name, msg.session.TmuxLocation), false)
}

func (m *model) askToStop() {
	if s, ok := m.selected(); ok {
		m.confirming, m.target = stopAction, s
	}
}

// askToRemove asks whether to remove s, selecting it so the cursor is on the
// session the question is about, and closing the messages window so the
// table shows it.
func (m *model) askToRemove(s Session) {
	m.confirming, m.target = removeAction, s
	m.messages = messagesWindow{}
	m.selectSession(s.SessionID)
}

func (m model) confirmAction() (tea.Model, tea.Cmd) {
	confirmed, s := m.confirming, m.target
	m.confirming = noAction

	if confirmed == removeAction {
		m.setStatus(fmt.Sprintf("Removing %s...", s.Name), false)
		return m, removeCmd(s)
	}
	m.setStatus(fmt.Sprintf("Stopping %s...", s.Name), false)
	return m, stopCmd(s)
}

func (m *model) setStatus(text string, isErr bool) {
	m.status, m.statusIsErr = text, isErr
}

// applySessions shows a fresh session list in the sort order, keeping the
// cursor on the session it was on.
func (m *model) applySessions(msg sessionsMsg) {
	if msg.err != nil {
		m.setStatus(msg.err.Error(), true)
		return
	}

	previous, hadSelection := m.selected()
	m.sessions = msg.sessions
	m.refreshedAt = time.Now()
	m.sortSessions()
	m.layoutTable()

	if !hadSelection || !m.selectSession(previous.SessionID) {
		// The table's cursor is -1 while it has no rows and stays there when rows arrive.
		m.table.SetCursor(max(0, m.table.Cursor()))
	}

	m.refreshMessages()
	if m.confirming != noAction && !m.listed(m.target.SessionID) {
		m.confirming = noAction
	}
}

// selectSession moves the cursor to the session with sessionID, reporting
// whether it is listed.
func (m *model) selectSession(sessionID string) bool {
	for i, s := range m.sessions {
		if s.SessionID == sessionID {
			m.table.SetCursor(i)
			return true
		}
	}
	return false
}

func (m model) listed(sessionID string) bool {
	return slices.ContainsFunc(m.sessions, func(s Session) bool { return s.SessionID == sessionID })
}

// applyActionResult reports how an action went. A background session that
// could not be stopped is offered for removal.
func (m *model) applyActionResult(msg actionDoneMsg) {
	s := msg.session

	if msg.err != nil {
		m.setStatus(msg.err.Error(), true)
		if msg.action == stopAction && s.Kind == "background" {
			m.askToRemove(s)
		}
		return
	}

	if msg.action == removeAction {
		m.setStatus(fmt.Sprintf("Removed %s (claude rm %s)", s.Name, s.ID), false)
		return
	}
	if s.Kind == "background" {
		m.setStatus(fmt.Sprintf("Stopped %s (claude stop %s)", s.Name, s.ID), false)
		return
	}
	m.setStatus(fmt.Sprintf("Sent SIGTERM to %s (pid %d)", msg.session.Name, msg.session.PID), false)
}

func roleLabel(role string) string {
	if role == "assistant" {
		return "claude"
	}
	return "you"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format(timeLayout)
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Claude Code sessions"
	return v
}

// render draws the whole screen, with the messages window over it when open.
func (m model) render() string {
	label := titleStyle.Render(fmt.Sprintf("Claude Code sessions: %d", len(m.sessions)))
	info := m.sortDescription()
	if !m.refreshedAt.IsZero() {
		info += ", refreshed " + m.refreshedAt.Format("15:04:05")
	}
	list := renderFrame(m.table.View(), m.width, 0, listBorderColor, label, dimStyle.Render(info), "")

	lines := strings.Split(list, "\n")
	// The line under the column headers joins the frame's sides.
	lines[headerSeparatorRow] = frameEdge(
		frameBorder.MiddleLeft,
		frameBorder.Top,
		frameBorder.MiddleRight,
		m.width,
		"",
		"",
		lipgloss.NewStyle().Foreground(listBorderColor),
	)
	lines = append(lines, m.renderStatus()...)
	lines = append(lines, m.renderHelp())
	screen := strings.Join(lines, "\n")

	if !m.messages.open {
		return screen
	}

	window := m.renderMessagesWindow()
	x := max(0, (m.width-lipgloss.Width(window))/2)
	y := max(1, (m.height-lipgloss.Height(window))/2)
	return lipgloss.NewCompositor(lipgloss.NewLayer(screen), lipgloss.NewLayer(window).X(x).Y(y).Z(1)).Render()
}

// renderStatus draws the status wrapped to the screen width in exactly
// statusLines lines.
func (m model) renderStatus() []string {
	style := lipgloss.NewStyle()
	if m.statusIsErr {
		style = errorStyle
	}

	lines := make([]string, statusLines)
	if m.status != "" {
		copy(lines, strings.Split(style.Width(max(1, m.width)).Render(m.status), "\n"))
	}
	return lines
}

// renderHelp draws the bottom line: the keys that apply, or the pending
// action's question with the keys that answer it.
func (m model) renderHelp() string {
	answers := renderKeyHints(keyHint{"y/enter", "yes"}, keyHint{"n/esc", "cancel"})
	switch m.confirming {
	case stopAction:
		return promptStyle.Render(fmt.Sprintf("Stop %s?", m.target.Name)) + "  " + answers
	case removeAction:
		return promptStyle.Render(
			fmt.Sprintf("Remove %s instead? It can't be resumed afterwards.", m.target.Name),
		) + "  " + answers
	}

	if m.messages.open {
		return renderKeyHints(keyHint{"up/down", "scroll"}, keyHint{"pgup/pgdown", "page"},
			keyHint{"home/end", "top/bottom"}, keyHint{"m/esc", "close"})
	}

	hints := []keyHint{{"up/down", "select"}, {"o", "sort"}, {"O", "reverse"}, {"m", "messages"}}
	if insideTmux() {
		hints = append(hints, keyHint{"t", "go to tmux pane"})
	}
	hints = append(hints, keyHint{"s", "stop"}, keyHint{"r", "refresh"}, keyHint{"q", "quit"})
	return renderKeyHints(hints...)
}

// keyHint is a key and the action it triggers.
type keyHint struct {
	key    string
	action string
}

// renderKeyHints draws key hints with each key coloured apart from its action.
func renderKeyHints(hints ...keyHint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = keyStyle.Render(h.key) + " " + dimStyle.Render(h.action)
	}
	return strings.Join(parts, "   ")
}
