package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestMarkdownCodeBlocks(t *testing.T) {
	for _, tt := range []struct {
		name, source, highlighted string
	}{
		{"go", "```go\nvar count = 42\n```", "var"},
		{"alias and info", "~~~js title=example\nconst count = 42;\n~~~", "const"},
		{"attributes", "```{#example .go .numberLines startFrom=\"10\"}\nvar count = 42\n```", "var"},
		{"quoted attributes", "```{title=\".python and spaces\" .go .numberLines}\nvar count = 42\n```", "var"},
		{"invalid attributes", "```{.go title=\"unclosed}\nvar count = 42\n```", ""},
		{"quote", "> ```python\n> count = 42\n> ```", "42"},
		{"list", "- Example\n\n  ```go\n  var count = 42\n  ```", "var"},
		{"Unicode and tabs", "```go\n\tvar café = \"é\"\n```", "var"},
		{"CRLF", "```go\r\nvar count = 42\r\n```\r", "var"},
		{"unclosed", "```go\nvar count = 42", "var"},
		{"empty", "```go\n```", ""},
		{"unknown", "```unknown-language\n**literal** &amp;\n```", ""},
		{"no language", "```\n**literal** &amp;\n```", ""},
		{"indented", "    **literal** &amp;", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseMarkdownSource(strings.Split(tt.source, "\n"), nil)
			found := false
			for _, line := range parsed.lines {
				if got := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)); got != displaySourceText(line.source) {
					t.Fatalf("rendered = %q, want %q", got, line.source)
				}
				for _, token := range line.tokens {
					if line.source[token.start:token.end] != token.text {
						t.Fatalf("token lost source coordinates: %+v", token)
					}
					if token.syntax != 0 && strings.Contains(token.text, tt.highlighted) {
						found = true
					}
				}
			}
			if found != (tt.highlighted != "") {
				t.Fatalf("syntax highlighting = %t, want %q", found, tt.highlighted)
			}
		})
	}
}

func TestMarkdownCodeFenceStyles(t *testing.T) {
	for _, tt := range []struct {
		name, opening, language string
		known                   bool
	}{
		{"supported", "```go", "go", true},
		{"unknown", "```unknown-language", "unknown-language", false},
		{"attributes after language", "~~~js title=\"Example code\"", "js", true},
		{"attribute list", "```{#example .go .numberLines startFrom=\"10\"}", "go", true},
		{"quoted value", "```{title=\".python\" .go .numberLines}", "go", true},
		{"unknown class", "```{.unknown-language #example}", "unknown-language", false},
		{"quote and CRLF", "> ```go\r", "go", true},
		{"empty", "```", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prefix := ""
			if strings.HasPrefix(tt.opening, "> ") {
				prefix = "> "
			}
			marker := "````"
			if strings.Contains(tt.opening, "~~~") {
				marker = "~~~~"
			}
			parsed := parseMarkdownSource([]string{tt.opening, prefix + "var count = 42", prefix + marker}, nil)
			for _, row := range []int{0, 2} {
				line := parsed.lines[row]
				foundLanguage := false
				for _, token := range line.tokens {
					if strings.Contains(token.text, marker[:3]) && token.style != markdownDelimiter {
						t.Fatalf("fence is not muted: %+v", token)
					}
					if row == 0 && token.text == tt.language && tt.language != "" {
						foundLanguage = true
						want := markdownDelimiter
						if tt.known {
							want = markdownFenceLanguage
						}
						if token.style != want {
							t.Fatalf("language style = %d, want %d", token.style, want)
						}
					}
					if strings.Contains(token.text, "title=") && token.style != markdownFenceAttribute {
						t.Fatalf("attribute is not muted: %+v", token)
					}
				}
				if row == 0 && tt.language != "" && !foundLanguage {
					t.Fatal("language source span missing")
				}
				if got := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)); got != displaySourceText(line.source) {
					t.Fatalf("rendered = %q, want %q", got, line.source)
				}
			}
		})
	}
}

func TestMarkdownCodeBlockMultiline(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { terminalIsDark = previous })
	const source = "```go\n/* comment\ncontinued */\nvar count = 42\n```\n\n```go\nvar next = 1\n```"
	parsed := parseMarkdownSource(strings.Split(source, "\n"), nil)
	for _, row := range []int{1, 2} {
		if tokens := parsed.lines[row].tokens; !tokens[1].syntax.InCategory(chroma.Comment) {
			t.Fatalf("line %d lost multiline comment state: %+v", row+1, tokens)
		}
	}
	for _, dark := range []bool{false, true} {
		terminalIsDark = dark
		line := &parsed.lines[3]
		rendered := line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
		if rendered == line.source || ansi.Strip(rendered) != line.source {
			t.Fatalf("dark=%t: highlighting missing or changed source: %q", dark, rendered)
		}
		inline := []gitpkg.InlineSegment{{Content: "var count = "}, {Content: "42", Changed: true}}
		diff := line.render(inline, diffCommonTextBg, diffAddedTextBg, nil)
		if ansi.Strip(diff) != line.source || !strings.Contains(diff, bgToAnsi(diffAddedTextBg.GetBackground())) {
			t.Fatalf("word diff lost source or changed background: %q", diff)
		}
	}
}

func TestMarkdownCodeBlockPatchGaps(t *testing.T) {
	const source = "missing\n```go\nvar count = 42\n```\nmissing\n/* unknown context\nvar next = 1\n```"
	parsed := parseMarkdownSource(strings.Split(source, "\n"), map[int]bool{2: true, 3: true, 4: true, 6: true, 7: true, 8: true})
	for _, row := range []int{2, 6} {
		for _, token := range parsed.lines[row].tokens {
			if token.syntax != 0 {
				t.Fatalf("patch boundary invented a code context: %+v", token)
			}
		}
	}
}
