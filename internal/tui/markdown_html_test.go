package tui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownHTMLHighlightsSyntaxAndPreservesSource(t *testing.T) {
	t.Parallel()
	const inline = `Read <span title="A > B" lang=en>this</span>.`
	const block = "<div\n  class='sample'>\n  Body &amp; text\n</div>"
	const script = "<script>const text = '<fake>';\n</script>"
	for _, tc := range []struct {
		name, source string
		want         map[string]chroma.TokenType
	}{
		{"inline", inline, map[string]chroma.TokenType{"span": chroma.NameTag, "title": chroma.NameAttribute, `"A > B"`: chroma.LiteralString, "lang": chroma.NameAttribute, "en": chroma.LiteralString}},
		{"block", block, map[string]chroma.TokenType{"div": chroma.NameTag, "class": chroma.NameAttribute, "'sample'": chroma.LiteralString}},
		{"script", script, map[string]chroma.TokenType{"script": chroma.NameTag}},
		{"escaped tag and code span", "Read \\<span> or `<div>`.", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseMarkdownSource(strings.Split(tc.source, "\n"), nil)
			if got := doc.copySourceRange(0, len(tc.source)); got != tc.source {
				t.Fatalf("copied source = %q, want %q", got, tc.source)
			}
			found := make(map[string]chroma.TokenType)
			for _, line := range doc.lines {
				if tc.want != nil {
					view := line.render(nil, lipgloss.Style{}, lipgloss.Style{}, nil)
					if got := ansi.Strip(view); got != line.source {
						t.Errorf("rendered text = %q, want %q", got, line.source)
					}
				}
				for _, token := range line.tokens {
					if token.style&markdownHTML == 0 || token.syntax == 0 {
						continue
					}
					switch token.syntax {
					case chroma.Punctuation, chroma.Operator:
						continue
					}
					found[token.text] = token.syntax
				}
			}
			for value, kind := range tc.want {
				if found[value] != kind {
					t.Errorf("%q has syntax %s, want %s", value, found[value], kind)
				}
			}
			if len(found) != len(tc.want) {
				t.Errorf("unexpected HTML tokens: %v", found)
			}
		})
	}
}

func TestMarkdownHTMLPaletteAdaptsToBackground(t *testing.T) {
	previousDark, previousBackground := terminalIsDark, terminalBackground
	t.Cleanup(func() {
		initAdaptiveStyles(previousDark)
		terminalBackground = previousBackground
	})
	const source = `Read <span lang="en">this</span>.`
	doc := parseMarkdownSource([]string{source}, nil)
	var previous color.Color
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		tag := markdownHTMLStyle(lipgloss.Style{}, chroma.NameTag).GetForeground()
		attr := markdownHTMLStyle(lipgloss.Style{}, chroma.NameAttribute).GetForeground()
		value := markdownHTMLStyle(lipgloss.Style{}, chroma.LiteralString).GetForeground()
		if sameColor(tag, attr) || sameColor(attr, value) || sameColor(tag, value) || sameColor(previous, tag) {
			t.Fatalf("dark=%t: HTML styles lack distinct colors", dark)
		}
		for _, view := range []string{
			doc.lines[0].render(nil, lipgloss.Style{}, lipgloss.Style{}, nil),
			highlightInline(source),
		} {
			if !hasForeground(view, tag, "span") || !hasForeground(view, attr, "lang") || !hasForeground(view, value, `"en"`) {
				t.Errorf("dark=%t: HTML palette missing from render", dark)
			}
		}
		previous = tag
	}
	terminalBackground = lipgloss.Color("#000000")
	if sameColor(previous, markdownHTMLStyle(lipgloss.Style{}, chroma.NameTag).GetForeground()) {
		t.Error("a different terminal background did not adjust the tag color")
	}
}
