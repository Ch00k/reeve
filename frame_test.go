package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestRenderFrame draws the body between the frame's sides, padded, with the
// label and info in the top edge and the note in the bottom edge, every line
// as wide as the frame.
func TestRenderFrame(t *testing.T) {
	b := frameBorder

	out := ansi.Strip(renderFrame("body", 20, 1, lipgloss.Color("6"), "label", "info", "note"))

	want := []string{
		b.TopLeft + b.Top + " label " + strings.Repeat(b.Top, 3) + " info " + b.Top + b.TopRight,
		b.Left + " body" + strings.Repeat(" ", 13) + b.Right,
		b.BottomLeft + strings.Repeat(b.Bottom, 11) + " note " + b.Bottom + b.BottomRight,
	}
	if got := strings.Split(out, "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestFrameEdgeTooNarrow cuts the label short to keep the info, and leaves
// the info out when it alone does not fit, keeping the edge's width.
func TestFrameEdgeTooNarrow(t *testing.T) {
	b := frameBorder
	edge := lipgloss.NewStyle()

	cut := frameEdge(b.TopLeft, b.Top, b.TopRight, 20, "a long label", "info", edge)
	if want := b.TopLeft + b.Top + " a lon... " + " info " + b.Top + b.TopRight; cut != want {
		t.Errorf("got %q, want %q", cut, want)
	}

	bare := frameEdge(b.TopLeft, b.Top, b.TopRight, 6, "", "information", edge)
	if want := b.TopLeft + strings.Repeat(b.Top, 4) + b.TopRight; bare != want {
		t.Errorf("got %q, want %q", bare, want)
	}
}
