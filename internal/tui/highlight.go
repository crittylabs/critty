package tui

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// newCodeHighlighter selects a file's lexer once.  Each call independently
// tokenises content and returns lines with ANSI styling.
func newCodeHighlighter(filename string) func(string) []string {
	lexer := lexers.Match(filename)
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

	return func(content string) []string {
		iterator, err := lexer.Tokenise(nil, content)
		if err != nil {
			// Fallback to raw lines
			return strings.Split(content, "\n")
		}

		var buf bytes.Buffer
		if err := formatter.Format(&buf, style, iterator); err != nil {
			return strings.Split(content, "\n")
		}

		// Split the ANSI output by newlines
		lines := strings.Split(buf.String(), "\n")

		// Chroma sometimes adds a trailing empty line from the final newline
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		return lines
	}
}
