package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// frameHeight is the rows a frame adds around its body: its top and bottom edges.
const frameHeight = 2

// frameBorder is the rounded border frames are drawn with.
var frameBorder = lipgloss.RoundedBorder()

// renderFrame draws body in a rounded frame width columns wide, with padding
// columns between its sides and the body, label at the start of its top edge,
// info at the end of its top edge and note at the end of its bottom edge.
func renderFrame(body string, width, padding int, borderColor color.Color, label, info, note string) string {
	edge := lipgloss.NewStyle().Foreground(borderColor)
	sides := lipgloss.NewStyle().
		Border(frameBorder, false, true).
		BorderForeground(borderColor).
		Padding(0, padding).
		Width(width)

	top := frameEdge(frameBorder.TopLeft, frameBorder.Top, frameBorder.TopRight, width, label, info, edge)
	bottom := frameEdge(frameBorder.BottomLeft, frameBorder.Bottom, frameBorder.BottomRight, width, "", note, edge)
	return top + "\n" + sides.Render(body) + "\n" + bottom
}

// frameEdge draws a top or bottom edge width columns wide between its corners,
// with label near its start and info near its end, each set off by a space on
// either side. The label is cut short when both do not fit, and the info is
// left out when it alone does not.
func frameEdge(leftCorner, line, rightCorner string, width int, label, info string, edge lipgloss.Style) string {
	inner := max(0, width-2)

	end := edge.Render(rightCorner)
	if info != "" && ansi.StringWidth(info)+3 <= inner {
		end = " " + info + " " + edge.Render(line+rightCorner)
		inner -= ansi.StringWidth(info) + 3
	}

	start := edge.Render(leftCorner)
	if room := inner - 3; label != "" && room > 0 {
		label = ansi.Truncate(label, room, "...")
		start = edge.Render(leftCorner+line) + " " + label + " "
		inner -= ansi.StringWidth(label) + 3
	}

	return start + edge.Render(strings.Repeat(line, inner)) + end
}
