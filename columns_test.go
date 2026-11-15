package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tableWidth is the width the columns take with their padding.
func tableWidth(widths [columnCount]int) int {
	total := int(columnCount) * cellPadding
	for _, w := range widths {
		total += w
	}
	return total
}

// resized is m after the terminal changed to width columns.
func resized(t *testing.T, m model, width int) model {
	t.Helper()

	return update(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
}

// TestRows shows each session's kind, state, name, directory with the home
// directory shortened, title and age.
func TestRows(t *testing.T) {
	m := sizedModel(t, interactive, background)

	rows := m.rows(m.columnWidths())

	want := [][]string{
		{"interactive", "idle", "tmp-4f", "~/tmp", "Fix the login form", "3h12m"},
		{"background", "blocked", "Clarify item", "/srv/docs", "", "1d2h"},
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}

// TestColumnWidthsFillScreen makes each column as wide as its widest value or
// title and gives the rest of the frame's inside to the title column.
func TestColumnWidthsFillScreen(t *testing.T) {
	m := sizedModel(t, interactive, background)

	widths := m.columnWidths()

	want := [columnCount]int{len("interactive"), len("blocked"), len("Clarify item"), len("DIRECTORY"), 0, len("3h12m")}
	want[titleColumn] = 160 - 2 - tableWidth(want)
	if widths != want {
		t.Errorf("widths %v, want %v", widths, want)
	}
}

// TestColumnsWidenForNewSession fits the columns to a session that appears
// with a longer name and directory, leaving its values whole while the
// screen has room.
func TestColumnsWidenForNewSession(t *testing.T) {
	m := sizedModel(t, interactive)
	newcomer := background
	newcomer.Name = "a considerably longer session name"
	newcomer.Cwd = "/srv/a/much/deeper/working/directory"

	m = update(t, m, sessionsMsg{sessions: []Session{interactive, newcomer}})

	widths := m.columnWidths()
	row := m.rows(widths)[1]
	if row[nameColumn] != newcomer.Name || row[dirColumn] != newcomer.Cwd {
		t.Errorf("row %q, want the whole name and directory", row)
	}
	if widths[nameColumn] < len(newcomer.Name) || widths[dirColumn] < len(newcomer.Cwd) {
		t.Errorf("widths %v too narrow for the new session", widths)
	}
}

// TestColumnWidthsShrinkWidest narrows the widest of the title, directory and
// name columns first when the values do not fit, so the table still fills
// its frame.
func TestColumnWidthsShrinkWidest(t *testing.T) {
	long := interactive
	long.Name = strings.Repeat("n", 30)
	long.Cwd = "/srv/" + strings.Repeat("d", 45)
	long.Title = strings.Repeat("t", 60)
	m := resized(t, sizedModel(t, long), 122)

	widths := m.columnWidths()

	if got := tableWidth(widths); got != 120 {
		t.Errorf("table is %d wide, want 120", got)
	}
	// 30+50+60 values in 120-12-(11+5+5) columns: the title and directory
	// shrink to the name's width, then all three shrink alike.
	if widths[nameColumn] != 29 || widths[dirColumn] != 29 || widths[titleColumn] != 29 {
		t.Errorf("name, directory and title widths %d, %d, %d; want 29 each",
			widths[nameColumn], widths[dirColumn], widths[titleColumn])
	}
}

// TestColumnWidthsStopAtMinimum leaves the shrinking columns at
// minShrunkWidth on a screen too narrow for the table.
func TestColumnWidthsStopAtMinimum(t *testing.T) {
	m := resized(t, sizedModel(t, interactive, background), 40)

	widths := m.columnWidths()

	for _, c := range shrinkOrder {
		if widths[c] != minShrunkWidth {
			t.Errorf("%s is %d wide, want %d", columnTitles[c], widths[c], minShrunkWidth)
		}
	}
}

// TestResizeRefitsTable lays the table out again for a new terminal width,
// as a resized window or tmux pane reports it, keeping the framed header
// within the screen and the age column at the frame's right side.
func TestResizeRefitsTable(t *testing.T) {
	m := sizedModel(t, interactive, background)

	for _, width := range []int{200, 90, 160} {
		m = resized(t, m, width)

		header := renderedLines(m)[1]
		if got := ansi.StringWidth(header); got != width {
			t.Errorf("width %d: header is %d wide", width, got)
		}
		inside := strings.TrimSuffix(header, frameBorder.Right)
		if !strings.HasSuffix(strings.TrimRight(inside, " "), "AGE") {
			t.Errorf("width %d: header %q does not end with AGE", width, header)
		}
	}
}

// TestDirectoryCutAtStart keeps the end of a directory too long for its
// column.
func TestDirectoryCutAtStart(t *testing.T) {
	long := interactive
	long.Cwd = "/srv/" + strings.Repeat("d", 80) + "/project"
	m := resized(t, sizedModel(t, long), 100)

	widths := m.columnWidths()
	dir := m.rows(widths)[0][dirColumn]

	if len(dir) != widths[dirColumn] || !strings.HasPrefix(dir, "...") || !strings.HasSuffix(dir, "/project") {
		t.Errorf("directory %q in a %d-wide column", dir, widths[dirColumn])
	}
}

// TestSortSessions orders by each column's values, case-insensitively, an
// age by its length, reversed when descending, and the youngest first among
// equals.
func TestSortSessions(t *testing.T) {
	now := time.Now()
	sessions := []Session{
		{Name: "beta", Kind: "interactive", SessionID: "1", StartedAt: now.Add(-time.Hour).UnixMilli()},
		{Name: "Alpha", Kind: "background", SessionID: "2", StartedAt: now.Add(-3 * time.Hour).UnixMilli()},
		{Name: "gamma", Kind: "interactive", SessionID: "3", StartedAt: now.Add(-2 * time.Hour).UnixMilli()},
	}

	cases := []struct {
		column     column
		descending bool
		want       string
	}{
		{ageColumn, false, "beta,gamma,Alpha"},
		{ageColumn, true, "Alpha,gamma,beta"},
		{nameColumn, false, "Alpha,beta,gamma"},
		{nameColumn, true, "gamma,beta,Alpha"},
		{kindColumn, false, "Alpha,beta,gamma"},
		{kindColumn, true, "beta,gamma,Alpha"},
	}
	for _, c := range cases {
		m := model{sessions: append([]Session(nil), sessions...), sortColumn: c.column, sortDescending: c.descending}

		m.sortSessions()

		if got := sessionNames(m); got != c.want {
			t.Errorf("by %s (descending %v): %q, want %q", columnTitles[c.column], c.descending, got, c.want)
		}
	}
}

// TestShortenHome replaces only a leading home directory component.
func TestShortenHome(t *testing.T) {
	m := model{home: "/home/u"}

	for path, want := range map[string]string{
		"/home/u":       "~",
		"/home/u/tmp":   "~/tmp",
		"/home/user2/x": "/home/user2/x",
		"/srv":          "/srv",
	} {
		if got := m.shortenHome(path); got != want {
			t.Errorf("shortenHome(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestTruncateLeft keeps the end of a long string within the width.
func TestTruncateLeft(t *testing.T) {
	if got := truncateLeft("~/projects/zytlyn/identity-access", 16); got != "...entity-access" {
		t.Errorf("got %q, want %q", got, "...entity-access")
	}
	if got := truncateLeft("~/tmp", 16); got != "~/tmp" {
		t.Errorf("got %q, want it unchanged", got)
	}
}

// TestFormatAge shows the two largest units of the time since the start, and
// nothing for an unknown start.
func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

	for age, want := range map[time.Duration]string{
		45 * time.Second:                              "45s",
		12*time.Minute + 59*time.Second:               "12m",
		3*time.Hour + 12*time.Minute:                  "3h12m",
		2*24*time.Hour + 5*time.Hour + 59*time.Minute: "2d5h",
		-time.Second:                                  "0s",
	} {
		if got := formatAge(now.Add(-age), now); got != want {
			t.Errorf("formatAge(%v ago) = %q, want %q", age, got, want)
		}
	}
	if got := formatAge(time.Time{}, now); got != "" {
		t.Errorf("formatAge(zero) = %q, want empty", got)
	}
}
