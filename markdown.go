package main

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
)

// Inline code takes Claude Code's "permission" colour for the background.
const (
	inlineCodeColorDark  = "#B1B9F9"
	inlineCodeColorLight = "#5769F7"
)

// codeBlockFormatter colours code with the terminal's own 16 colours, which
// the #ansi colour names in markdownStyle pick.
const codeBlockFormatter = "terminal16"

// tabWidth is how many spaces a tab in a code block takes, since the window
// measures each character as one column.
const tabWidth = 4

// renderMarkdown renders Markdown the way Claude Code shows its replies,
// wrapped to width, falling back to the source wrapped when it cannot be
// rendered.
func (m model) renderMarkdown(source string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle(m.darkBackground)),
		glamour.WithChromaFormatter(codeBlockFormatter),
		glamour.WithWordWrap(width),
	)
	if err == nil {
		if out, err := renderer.Render(source); err == nil {
			return strings.ReplaceAll(strings.Trim(out, "\n"), "\t", strings.Repeat(" ", tabWidth))
		}
	}
	return lipgloss.NewStyle().Width(width).Render(source)
}

// markdownStyle follows Claude Code's own rendering: text in the terminal's
// colour on no background, headings without their # marks (the first level
// bold, italic and underlined, the others bold), inline code in a colour of
// its own, code blocks highlighted with the terminal's colours, - and 1.
// list markers, and quotes in italics behind a bar.
func markdownStyle(darkBackground bool) ansi.StyleConfig {
	inlineCodeColor := inlineCodeColorDark
	if !darkBackground {
		inlineCodeColor = inlineCodeColorLight
	}

	return ansi.StyleConfig{
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Italic: ptr(true)},
			Indent:         ptr(uint(1)),
			IndentToken:    ptr("\u258e "),
		},
		List: ansi.StyleList{LevelIndent: 2},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Bold: ptr(true)},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Italic: ptr(true), Underline: ptr(true)},
		},
		Strikethrough:  ansi.StylePrimitive{CrossedOut: ptr(true)},
		Emph:           ansi.StylePrimitive{Italic: ptr(true)},
		Strong:         ansi.StylePrimitive{Bold: ptr(true)},
		HorizontalRule: ansi.StylePrimitive{Format: "\n---\n"},
		Item:           ansi.StylePrimitive{BlockPrefix: "- "},
		Enumeration:    ansi.StylePrimitive{BlockPrefix: ". "},
		Task:           ansi.StyleTask{Ticked: "[x] ", Unticked: "[ ] "},
		Link:           ansi.StylePrimitive{Underline: ptr(true)},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: ptr(inlineCodeColor)},
		},
		CodeBlock: ansi.StyleCodeBlock{
			Chroma: &ansi.Chroma{
				Comment:         ansi.StylePrimitive{Color: ptr("#ansidarkgreen")},
				CommentPreproc:  ansi.StylePrimitive{Color: ptr("#ansidarkgray")},
				Keyword:         ansi.StylePrimitive{Color: ptr("#ansidarkblue")},
				KeywordReserved: ansi.StylePrimitive{Color: ptr("#ansidarkblue")},
				KeywordType:     ansi.StylePrimitive{Color: ptr("#ansiteal")},
				NameBuiltin:     ansi.StylePrimitive{Color: ptr("#ansiteal")},
				NameTag:         ansi.StylePrimitive{Color: ptr("#ansidarkgray")},
				NameAttribute:   ansi.StylePrimitive{Color: ptr("#ansiteal")},
				NameClass:       ansi.StylePrimitive{Color: ptr("#ansidarkblue")},
				NameDecorator:   ansi.StylePrimitive{Color: ptr("#ansidarkgray")},
				NameFunction:    ansi.StylePrimitive{Color: ptr("#ansibrown")},
				LiteralNumber:   ansi.StylePrimitive{Color: ptr("#ansidarkgreen")},
				LiteralString:   ansi.StylePrimitive{Color: ptr("#ansidarkred")},
				GenericDeleted:  ansi.StylePrimitive{Color: ptr("#ansidarkred")},
				GenericInserted: ansi.StylePrimitive{Color: ptr("#ansidarkgreen")},
				GenericEmph:     ansi.StylePrimitive{Italic: ptr(true)},
				GenericStrong:   ansi.StylePrimitive{Bold: ptr(true)},
			},
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}
