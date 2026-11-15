package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// sampleMarkdown has one of each element markdownStyle styles.
const sampleMarkdown = "# Heading one\n\nSome **bold** and `inline code`.\n\n## Heading two\n\n- one\n- two\n\n> quoted\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"hi\", 42)\n}\n```"

// TestMarkdownLikeClaudeCode renders headings without their # marks, -
// list markers, a bar before quotes, and inline code in its own colour with
// no background anywhere.
func TestMarkdownLikeClaudeCode(t *testing.T) {
	m := newModel(3)

	out := m.renderMarkdown(sampleMarkdown, 60)
	text := ansi.Strip(out)

	for _, want := range []string{"Heading one", "Heading two", "- one", "\u258e quoted", "inline code", "    fmt.Println(\"hi\", 42)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"#", "`", "\t"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("%q left in:\n%s", unwanted, text)
		}
	}
	if !strings.Contains(out, "\x1b[38;2;177;185;249minline code") {
		t.Errorf("inline code not in its colour:\n%q", out)
	}
	if strings.Contains(out, "[48;") || strings.Contains(out, ";48;") {
		t.Errorf("a background colour is set:\n%q", out)
	}
}

// TestMarkdownCodeInTerminalColours highlights code with the terminal's own
// colours: keywords blue, functions yellow, strings red, numbers green.
func TestMarkdownCodeInTerminalColours(t *testing.T) {
	out := newModel(3).renderMarkdown(sampleMarkdown, 60)

	for _, want := range []string{"\x1b[34mfunc", "\x1b[33mmain", "\x1b[31m\"hi\"", "\x1b[32m42"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%q", want, out)
		}
	}
}

// TestMarkdownOnLightBackground colours inline code darker on a light
// terminal background.
func TestMarkdownOnLightBackground(t *testing.T) {
	m := newModel(3)
	m.darkBackground = false

	if out := m.renderMarkdown("`code`", 60); !strings.Contains(out, "\x1b[38;2;87;105;247mcode") {
		t.Errorf("inline code rendered as %q", out)
	}
}
