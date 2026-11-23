package tui

import (
	"image/color"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"golang.org/x/net/html"
)

// HTML token boundaries keep the lexer out of prose and raw-text elements
// such as scripts.  Keep the original bytes, including quotes and entities.
func markdownHTMLTokens(source string) []chroma.Token {
	tokenizer := html.NewTokenizer(strings.NewReader(source))
	var tokens []chroma.Token
	for offset := 0; offset < len(source); {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			return append(tokens, chroma.Token{Value: source[offset:]})
		}
		end := offset + len(tokenizer.Raw())
		raw := source[offset:end]
		offset = end
		switch kind {
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tokens = append(tokens, markdownTagTokens(raw)...)
		case html.CommentToken, html.DoctypeToken:
			tokens = append(tokens, chroma.Token{Type: chroma.Comment, Value: raw})
		default:
			tokens = append(tokens, chroma.Token{Value: raw})
		}
	}
	return tokens
}

func markdownTagTokens(raw string) []chroma.Token {
	iterator, err := lexers.Get("html").Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, raw)
	if err == nil {
		tokens := slices.DeleteFunc(iterator.Tokens(), func(token chroma.Token) bool { return token.Value == "" })
		rest := raw
		for i := range tokens {
			token := &tokens[i]
			var ok bool
			rest, ok = strings.CutPrefix(rest, token.Value)
			if !ok {
				return []chroma.Token{{Value: raw}}
			}
			switch token.Type {
			case chroma.NameTag, chroma.NameAttribute, chroma.LiteralString, chroma.Punctuation, chroma.Operator:
			default:
				token.Type = 0
			}
		}
		if rest == "" {
			return tokens
		}
	}
	return []chroma.Token{{Value: raw}}
}

func markdownHTMLStyle(style lipgloss.Style, kind chroma.TokenType) lipgloss.Style {
	ld := lipgloss.LightDark(terminalIsDark)
	var foreground color.Color
	switch kind {
	case chroma.NameTag:
		foreground = ld(lipgloss.Color("#5C476D"), lipgloss.Color("#B7A6CB"))
	case chroma.NameAttribute:
		foreground = ld(lipgloss.Color("#405D70"), lipgloss.Color("#91ADBD"))
	case chroma.LiteralString:
		foreground = ld(lipgloss.Color("#495D37"), lipgloss.Color("#A9B894"))
	case chroma.Punctuation, chroma.Operator, chroma.Comment:
		foreground = ld(lipgloss.Color("#565A62"), lipgloss.Color("#99A4B0"))
	default:
		return style
	}
	return style.Foreground(tintedBackground(foreground, 85))
}
