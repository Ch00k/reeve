package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"github.com/charmbracelet/x/ansi"
)

// column is a column of the session table, in display order.
type column int

const (
	kindColumn column = iota
	stateColumn
	nameColumn
	dirColumn
	titleColumn
	ageColumn
	columnCount
)

var columnTitles = [columnCount]string{"KIND", "STATE", "NAME", "DIRECTORY", "TITLE", "AGE"}

// shrinkOrder lists the columns that give up width when the table is wider
// than the screen, the first of the widest giving first. The title column
// also takes any width left over.
var shrinkOrder = []column{titleColumn, dirColumn, nameColumn}

const (
	// minShrunkWidth is the narrowest a shrinking column gets.
	minShrunkWidth = 8
	// cellPadding is the horizontal padding the table puts around each cell.
	cellPadding = 2
)

// cells are a session's values in column order, before fitting them to
// their columns.
func (m model) cells(s Session) [columnCount]string {
	return [columnCount]string{
		kindColumn:  s.Kind,
		stateColumn: s.Activity(),
		nameColumn:  s.Name,
		dirColumn:   m.shortenHome(s.Cwd),
		titleColumn: s.Title,
		ageColumn:   formatAge(s.Started(), time.Now()),
	}
}

// tableWidth is the width of the table inside the session list's frame.
func (m model) tableWidth() int {
	return max(0, m.width-2)
}

// columnWidths fits the columns to the table width: each is as wide as its
// widest value or title, the title column takes what is left over, and when
// that is too wide the widest of shrinkOrder narrows until it fits.
func (m model) columnWidths() [columnCount]int {
	var widths [columnCount]int
	for c, title := range columnTitles {
		widths[c] = ansi.StringWidth(title)
	}
	for _, s := range m.sessions {
		for c, cell := range m.cells(s) {
			widths[c] = max(widths[c], ansi.StringWidth(cell))
		}
	}

	excess := int(columnCount)*cellPadding - m.tableWidth()
	for _, w := range widths {
		excess += w
	}

	for ; excess > 0; excess-- {
		widest := slices.MaxFunc(shrinkOrder, func(a, b column) int {
			// MaxFunc returns the last of equal maxima, so ties go to the earlier column.
			return cmp.Or(cmp.Compare(widths[a], widths[b]), cmp.Compare(b, a))
		})
		if widths[widest] <= minShrunkWidth {
			break
		}
		widths[widest]--
	}
	if excess < 0 {
		widths[titleColumn] -= excess
	}

	return widths
}

func (m *model) layoutTable() {
	widths := m.columnWidths()
	columns := make([]table.Column, columnCount)
	for c := range columnCount {
		columns[c] = table.Column{Title: columnTitles[c], Width: widths[c]}
	}

	m.table.SetColumns(columns)
	m.table.SetWidth(m.tableWidth())
	m.table.SetHeight(max(3, m.height-frameHeight-footerHeight))
	m.table.SetRows(m.rows(widths))
}

// rows are the table's rows. The table cuts a value too long for its column
// at the end; a directory is cut at the start instead, keeping its last
// components.
func (m model) rows(widths [columnCount]int) []table.Row {
	rows := make([]table.Row, 0, len(m.sessions))
	for _, s := range m.sessions {
		cells := m.cells(s)
		cells[dirColumn] = truncateLeft(cells[dirColumn], widths[dirColumn])
		rows = append(rows, cells[:])
	}
	return rows
}

func (m model) shortenHome(path string) string {
	if m.home != "" && (path == m.home || strings.HasPrefix(path, m.home+"/")) {
		return "~" + path[len(m.home):]
	}
	return path
}

// truncateLeft keeps the end of s, which is the informative part of a path.
func truncateLeft(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return "..." + string(runes[max(0, len(runes)-width+3):])
}

// sortSessions orders the sessions by the sort column, the youngest first
// among equals, so the order stays put between refreshes.
func (m *model) sortSessions() {
	slices.SortStableFunc(m.sessions, func(a, b Session) int {
		order := m.compareBy(m.sortColumn, a, b)
		if m.sortDescending {
			order = -order
		}
		return cmp.Or(order, cmp.Compare(b.StartedAt, a.StartedAt), cmp.Compare(a.SessionID, b.SessionID))
	})
}

// compareBy compares two sessions by a column's values, an age by its length.
func (m model) compareBy(c column, a, b Session) int {
	if c == ageColumn {
		return cmp.Compare(b.StartedAt, a.StartedAt)
	}
	return cmp.Compare(strings.ToLower(m.cells(a)[c]), strings.ToLower(m.cells(b)[c]))
}

// sortDescription names the sort column and direction for the title line.
func (m model) sortDescription() string {
	direction := "ascending"
	if m.sortDescending {
		direction = "descending"
	}
	return fmt.Sprintf("sorted by %s %s", strings.ToLower(columnTitles[m.sortColumn]), direction)
}

// formatAge is how long ago started was, at now, in its two largest units,
// such as 3h12m or 2d5h.
func formatAge(started, now time.Time) string {
	if started.IsZero() {
		return ""
	}

	d := max(0, now.Sub(started))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}
