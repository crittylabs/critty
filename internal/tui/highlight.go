package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/crittylabs/critty/internal/syntax"
)

var patchLightStyle = chroma.MustNewStyle("critty-diff-light", chroma.StyleEntries{
	chroma.Text:              "#272822",
	chroma.GenericDeleted:    "#B42318",
	chroma.GenericInserted:   "#1A7F37",
	chroma.GenericHeading:    "#8250DF",
	chroma.GenericSubheading: "#0969DA",
	chroma.GenericStrong:     "bold",
})

var patchDarkStyle = chroma.MustNewStyle("critty-diff-dark", chroma.StyleEntries{
	chroma.Text:              "#F8F8F2",
	chroma.GenericDeleted:    "#F92672",
	chroma.GenericInserted:   "#A6E22E",
	chroma.GenericHeading:    "#AE81FF",
	chroma.GenericSubheading: "#66D9EF",
	chroma.GenericStrong:     "bold",
})

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

// newCodeHighlighter captures the current terminal palette for this pass factory.
func newCodeHighlighter(lexer chroma.Lexer, observers ...func(chroma.Token)) func(string) []string {
	style := codeHighlightStyle()
	if lexer != nil && strings.EqualFold(lexer.Config().Name, "diff") {
		style = patchLightStyle
		if terminalIsDark {
			style = patchDarkStyle
		}
	}
	return syntax.NewHighlighter(lexer, style, observers...)
}

func codeHighlightStyle() *chroma.Style {
	// Monokai's light foregrounds are unreadable on a light background.
	if !terminalIsDark {
		return styles.Get("monokailight")
	}
	return styles.Get("monokai")
}
