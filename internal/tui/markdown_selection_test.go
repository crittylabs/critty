package tui

import (
	"strings"
	"testing"
)

func TestCopyMarkdownSource(t *testing.T) {
	for _, tt := range []struct {
		name, source, want             string
		startLine, start, endLine, end int
	}{
		{"list prefix", "- item", "- item", 1, 0, 1, 6},
		{"code", "before `word` after", "`word`", 1, 8, 1, 12},
		{"code prefix", "`long word`", "`long`", 1, 1, 1, 5},
		{"multiple ticks", "`` `word` ``", "`` `word` ``", 1, 1, 1, 7},
		{"strong", "before **bold** after", "**bold**", 1, 7, 1, 11},
		{"strong prefix", "**bold text**", "**bold**", 1, 0, 1, 4},
		{"nested prefix", "***bold text***", "***bold***", 1, 0, 1, 4},
		{"strong middle", "**bold text** after", "ol", 1, 1, 1, 3},
		{"strong suffix", "**bold text** after", "text after", 1, 5, 1, 15},
		{"strong trailing space", "**bold text**", "**bold** ", 1, 0, 1, 5},
		{"nested link prefix", "[**long label**](https://example.org)", "[**long**](https://example.org)", 1, 0, 1, 4},
		{"autolink prefix", "<https://example.org/page>", "[https](<https://example.org/page>)", 1, 0, 1, 5},
		{"autolink middle", "<https://example.org/page>", "[example](<https://example.org/page>)", 1, 8, 1, 15},
		{"autolink whole", "<https://example.org/page>", "<https://example.org/page>", 1, 0, 1, 24},
		{"bare URL middle", "https://example.org/page", "[example](<https://example.org/page>)", 1, 8, 1, 15},
		{"link", "before [label](https://example.org/a_(b) \"title\") after", "[label](https://example.org/a_(b) \"title\")", 1, 7, 1, 12},
		{"link prefix", "[long label](https://example.org/a_(b))", "[long](https://example.org/a_(b))", 1, 0, 1, 4},
		{"link middle", "[long label](<https://example.org/a> 'title')", "[ong](<https://example.org/a> 'title')", 1, 1, 1, 4},
		{"multiline emphasis", "**first\nsecond**", "**first\nsecond**", 1, 0, 2, 6},
		{"multiline prefix", "**first\nsecond**", "**first\nsec**", 1, 0, 2, 3},
		{"multiline code", "`first\nsecond`", "`first\nsecond`", 1, 1, 2, 6},
		{"entity", "a &amp; b", "&amp;", 1, 2, 1, 7},
		{"escape", "a \\* b", "\\*", 1, 2, 1, 4},
		{"tab", "a\tb", "\t", 1, 1, 1, 5},
		{"fence prefix", "```go\nfirst\nsecond\n```", "```go\nfirst\n```", 1, 0, 2, 5},
		{"closed fence", "```go\nfirst\n```", "```go\nfirst\n```", 1, 0, 3, 3},
		{"fence body", "```go\nfirst\nsecond\n```", "first", 2, 0, 2, 5},
		{"quoted fence", "> ```go\n> first\n> second\n> ```", "> ```go\n> first\n> ```", 1, 0, 2, 7},
		{"empty quoted fence", "> ```\n> ```", "> ```\n> ```", 1, 0, 2, 5},
		{"fence opener only", "```go\nbody\n```", "```go\n```", 1, 0, 1, 5},
		{"fence inside literal body", "````md\n```go\nbody\n````", "```go\nbody", 2, 0, 3, 4},
		{"unclosed fence", "~~~go\nfirst", "~~~go\nfirst\n~~~", 1, 0, 2, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := parseMarkdownSource(strings.Split(tt.source, "\n"), nil)
			if got := source.copyRange(tt.startLine, tt.start, tt.endLine, tt.end); got != tt.want {
				t.Fatalf("copied = %q, want %q", got, tt.want)
			}
		})
	}
}
