package main

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// messagesMaxWidth caps the width of the messages window, borders included.
const messagesMaxWidth = 100

// windowBorderColor sets the messages window apart from the session list
// under it, and windowPadding is the space between its sides and the messages.
var windowBorderColor = lipgloss.Color("6")

const windowPadding = 1

// messagesWindow is the floating window showing a session's last messages,
// scrolled with the keys of a pager. A refresh keeps it scrolled where it
// was, or at the end when it was there.
type messagesWindow struct {
	open     bool
	session  Session
	viewport viewport.Model
}

// openMessages opens the window on s, scrolled to its end.
func (m *model) openMessages(s Session) {
	m.messages = messagesWindow{open: true, session: s, viewport: viewport.New()}
	m.layoutMessages()
	m.messages.viewport.GotoBottom()
}

// refreshMessages shows the window's session as freshly loaded, closing the
// window when the session is gone.
func (m *model) refreshMessages() {
	if !m.messages.open {
		return
	}

	i := slices.IndexFunc(m.sessions, func(s Session) bool { return s.SessionID == m.messages.session.SessionID })
	if i < 0 {
		m.messages = messagesWindow{}
		return
	}

	m.messages.session = m.sessions[i]
	m.layoutMessages()
}

// layoutMessages sizes the window to the screen and its content, and fills it.
func (m *model) layoutMessages() {
	w := &m.messages
	atBottom := w.viewport.AtBottom()

	width := min(m.width, messagesMaxWidth)
	innerWidth := max(1, width-2-2*windowPadding)
	content := m.renderMessageList(w.session.Messages, innerWidth)

	// The window leaves the screen's first and last lines uncovered, the top
	// of the session list's frame and the help line.
	available := m.height - 2 - frameHeight

	w.viewport.SetWidth(innerWidth)
	w.viewport.SetHeight(max(1, min(lipgloss.Height(content), available)))
	w.viewport.SetContent(content)
	if atBottom {
		w.viewport.GotoBottom()
	}
}

// handleMessagesKey scrolls or closes the window.
func (m model) handleMessagesKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	vp := &m.messages.viewport

	switch msg.String() {
	case "m", "esc":
		m.messages = messagesWindow{}
	case "home", "g":
		vp.GotoTop()
	case "end", "G":
		vp.GotoBottom()
	default:
		*vp, _ = vp.Update(msg)
	}

	return m, nil
}

// renderMessagesWindow draws the window: the session's name and title in its
// top edge, its messages, and in its bottom edge where they are scrolled to
// when they do not fit.
func (m model) renderMessagesWindow() string {
	w := m.messages

	label := titleStyle.Render(w.session.Name)
	if w.session.Title != "" {
		label += "  " + w.session.Title
	}

	var position string
	if total := w.viewport.TotalLineCount(); total > w.viewport.Height() {
		first := w.viewport.YOffset() + 1
		last := w.viewport.YOffset() + w.viewport.VisibleLineCount()
		position = dimStyle.Render(fmt.Sprintf("lines %d-%d of %d", first, last, total))
	}

	width := w.viewport.Width() + 2 + 2*windowPadding
	return renderFrame(w.viewport.View(), width, windowPadding, windowBorderColor, label, "", position)
}

// renderMessageList draws messages one under another, each headed by who
// wrote it and when. Claude's replies are rendered as Markdown; a person's
// text is shown as typed.
func (m model) renderMessageList(messages []Message, width int) string {
	if len(messages) == 0 {
		return dimStyle.Render("No messages found.")
	}

	plain := lipgloss.NewStyle().Width(width)
	blocks := make([]string, len(messages))
	for i, msg := range messages {
		text := plain.Render(msg.Text)
		if msg.Role == "assistant" {
			text = m.renderMarkdown(msg.Text, width)
		}
		heading := roleLabel(msg.Role)
		if !msg.Time.IsZero() {
			heading += ", " + formatTime(msg.Time)
		}
		blocks[i] = dimStyle.Render(heading+":") + "\n" + text
	}
	return strings.Join(blocks, "\n\n")
}
