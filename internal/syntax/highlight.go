package syntax

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
)

// NewHighlighter reuses the selected lexer.  Each call independently
// tokenises content and returns lines with ANSI styling.  Observers receive
// every token of the pass, including errors before restyling and the final EOF.
// The caller must supply a non-nil style.
//
// On tokenisation/formatting failure or a line-count mismatch, the result is
// unstyled strings.Split(content, "\n"), including any trailing empty element.
// Observers may already have received some or all tokens when this happens.
// Successful output omits the trailing empty element.
func NewHighlighter(lexer chroma.Lexer, style *chroma.Style, observers ...func(chroma.Token)) func(string) []string {
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

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

		// Lexer failures can flag valid constructs, such as TOML multiline
		// strings.  Render them as text rather than syntax diagnostics.
		readable := func() chroma.Token {
			token := iterator()
			for _, observe := range observers {
				observe(token)
			}
			if token.Type == chroma.Error {
				token.Type = chroma.Text
			}
			return token
		}
		var buf bytes.Buffer
		if err := formatter.Format(&buf, style, readable); err != nil {
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
