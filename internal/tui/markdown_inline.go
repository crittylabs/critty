package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
)

func highlightInlineWith(line string, linkStyle, codeStyle lipgloss.Style) string {
	return highlightInlineReferences(line, linkStyle, codeStyle, nil)
}

func highlightInlineReferences(line string, linkStyle, codeStyle lipgloss.Style, references func(string) string) string {
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
	root := parser.New(parser.WithExtensions(extension.LinkifyParser)).Parse(source)
	var render func(ast.Node, lipgloss.Style) string
	linkDepth := 0
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
		case *ast.Text:
			text := n.Value.Str(source)
			if n.SoftLineBreak() || n.HardLineBreak() {
				text += "\n"
			}
			return style.Render(decorate(text))
		case *ast.CodeSpan:
			return codeStyle.Inherit(style).Render(" " + decorate(n.Value.Str(source)) + " ")
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
			return linkedText(n.Destination.Str(source), label)
		case *ast.AutoLink:
			return linkedText(n.Destination.Str(source), linkStyle.Inherit(style).Render(n.Label.Str(source)))
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

func markdownHasLabel(n ast.Node, source []byte) bool {
	switch n := n.(type) {
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
