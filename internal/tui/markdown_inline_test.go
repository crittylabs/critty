package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestInlineMarkdownParser(t *testing.T) {
	for _, tc := range []struct {
		source, text, uri string
	}{
		{"", "", ""},
		{"   ", "   ", ""},
		{"**bold `literal ** stars` end**", "bold  literal ** stars  end", ""},
		{"``a ` b``", " a ` b ", ""},
		{"[**label**](https://example.org/a_(b))", "label", "https://example.org/a_(b)"},
		{"https://example.org/plain.", "https://example.org/plain.", "https://example.org/plain"},
		{"`https://example.org/code`", " https://example.org/code ", ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			got := highlightInline(tc.source)
			if text := ansi.Strip(got); text != tc.text {
				t.Fatalf("text = %q, want %q; raw=%q", text, tc.text, got)
			}
			if tc.uri != "" {
				if !strings.Contains(got, ansi.SetHyperlink(tc.uri)) {
					t.Fatalf("missing hyperlink %q in %q", tc.uri, got)
				}
			} else if strings.Contains(got, "\x1b]8;") {
				t.Fatalf("unexpected hyperlink in %q", got)
			}
		})
	}
}

func TestInlineMarkdownPreservesImageLinks(t *testing.T) {
	uri := imageTarget{url: "https://example.org/a**b**.png"}.uri()
	got := highlightInline(linkedText(uri, "![picture](https://example.org/a**b**.png)"))
	if !strings.Contains(got, ansi.SetHyperlink(uri)) {
		t.Fatalf("image target changed: %q", got)
	}
}
