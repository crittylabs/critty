package tui

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

func isCodeLexer(lexer chroma.Lexer) bool {
	if lexer == nil {
		return false
	}
	switch lexer.Config().Name {
	case "plaintext", "markdown", "reStructuredText", "Org Mode":
		return false
	}
	return true
}

// newCodeHighlighter reuses the selected lexer.  Each call independently
// tokenises content and returns lines with ANSI styling.
func newCodeHighlighter(lexer chroma.Lexer) func(string) []string {
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Monokai's light foregrounds are unreadable on a light background.
	style := styles.Get("monokai")
	if !terminalIsDark {
		style = styles.Get("monokailight")
	}
	formatter := formatters.Get("terminal256")

	// Chroma's default options turn a lone CR into a newline, which would
	// shift every later line against the document's line numbers.
	options := &chroma.TokeniseOptions{State: "root", EnsureLF: false}

	return func(content string) []string {
		raw := strings.Split(content, "\n")
		iterator, err := lexer.Tokenise(options, content)
		if err != nil {
			return raw
		}

		var buf bytes.Buffer
		if err := formatter.Format(&buf, style, iterator); err != nil {
			return raw
		}

		// Split the ANSI output by newlines
		lines := strings.Split(buf.String(), "\n")

		// Chroma sometimes adds a trailing empty line from the final newline
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		// A lexer that merges or splits lines would misplace the rest of the
		// file, so prefer unstyled lines that stay aligned.
		if len(lines) != len(trimTrailingEmpty(raw)) {
			return raw
		}

		return lines
	}
}

func trimTrailingEmpty(lines []string) []string {
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}
