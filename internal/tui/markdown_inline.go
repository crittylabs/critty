package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/util"

	"github.com/crittylabs/critty/v2/internal/icons"
)

var inlineHighlightParser = parser.New(parser.WithExtensions(extension.LinkifyParser), parser.WithInlineParsers(util.Prioritized[parser.InlineParser](mathParser{}, 50)))

func highlightInlineReferences(line string, linkStyle, codeStyle lipgloss.Style, symbols icons.Set, references func(string) string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	// Keep list markers and other block syntax in this inline-only input.
	const prefix = "x "
	// Markdown discards trailing spaces, but diff fragments must join without
	// changing the source text between highlighted words.
	content := strings.TrimRight(line, " \t")
	suffix := line[len(content):]
	source := []byte(prefix + content)
	root := inlineHighlightParser.Parse(source)
	groupKeyboardNodes(root, source)
	var render func(ast.Node, lipgloss.Style) string
	linkDepth := 0
	keyboardDepth := 0
	decorate := func(text string) string {
		if references != nil && linkDepth == 0 {
			return references(text)
		}
		return text
	}
	children := func(n ast.Node, style lipgloss.Style) string {
		var b strings.Builder
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			b.WriteString(render(child, style))
		}
		return b.String()
	}
	render = func(n ast.Node, style lipgloss.Style) string {
		switch n := n.(type) {
		case *keyboardNode:
			if containsKeyboard(n) {
				return children(n, style)
			}
			if n.FirstChild() == nil {
				return ""
			}
			keyStyle := keyboardStyle().Inherit(style)
			keyboardDepth++
			label := children(n, keyStyle)
			keyboardDepth--
			return keyStyle.Render("⟦") + label + keyStyle.Render("⟧")
		case *mathNode:
			return style.Render(string(source[n.start:n.end]))
		case *ast.Text:
			text := n.Value.Str(source)
			if keyboardDepth > 0 {
				text = n.Value.Value(source)
			}
			if n.SoftLineBreak() || n.HardLineBreak() {
				text += "\n"
			}
			return style.Render(decorate(text))
		case *ast.CodeSpan:
			return codeStyle.Inherit(style).Render(" " + decorate(n.Value.Value(source)) + " ")
		case *ast.Strong:
			return children(n, mdBoldStyle.Inherit(style))
		case *ast.Emphasis:
			return children(n, mdItalicStyle.Inherit(style))
		case *ast.Link:
			linkDepth++
			label := children(n, linkStyle.Inherit(style))
			linkDepth--
			if !markdownHasLabel(n, source) {
				label = linkStyle.Inherit(style).Render(n.Destination.Str(source))
			}
			uri := n.Destination.Str(source)
			return linkedText(uri, label+linkStyle.Inherit(style).Render(externalLinkSuffix(uri, symbols)))
		case *ast.AutoLink:
			uri := n.Destination.Str(source)
			return linkedText(uri, linkStyle.Inherit(style).Render(n.Label.Str(source)+externalLinkSuffix(uri, symbols)))
		case *ast.Image:
			// Previewable images get lightbox links from the image renderer.
			if !markdownHasLabel(n, source) {
				return linkedText(n.Destination.Str(source), "!"+linkStyle.Inherit(style).Render(n.Destination.Str(source)))
			}
			return linkedText(n.Destination.Str(source), "!"+children(n, linkStyle.Inherit(style)))
		case *ast.RawHTML:
			return style.Render(n.Value.Str(source))
		}
		return children(n, style)
	}
	return strings.TrimPrefix(render(root, lipgloss.NewStyle()), prefix) + suffix
}

func externalLinkSuffix(uri string, symbols icons.Set) string {
	if _, ok := webImageURL(uri); ok {
		return " " + symbols.Glyph(icons.LinkExternal, "↗")
	}
	return ""
}

func markdownHasLabel(n ast.Node, source []byte) bool {
	switch n := n.(type) {
	case *mathNode:
		return strings.TrimSpace(n.formula) != ""
	case *ast.Text:
		return strings.TrimSpace(n.Value.Str(source)) != ""
	case *ast.CodeSpan:
		return strings.TrimSpace(n.Value.Str(source)) != ""
	case *ast.RawHTML:
		return strings.TrimSpace(n.Value.Str(source)) != ""
	}
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if markdownHasLabel(child, source) {
			return true
		}
	}
	return false
}
