package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Patch documents share Markdown's styles while retaining every source byte.
// Display decorations and layout changes belong to the document renderer.
func (t *FileTab) highlightMarkdownSource(lines []string) []string {
	parsed := parseMarkdownWithSyntax(lines, nil, t.syntax, t.symbols)
	result := make([]string, len(lines))
	foreground := lipgloss.NewStyle().GetForeground()
	for i, line := range parsed.lines {
		var b strings.Builder
		pos := 0
		for _, token := range line.tokens {
			start, end := max(pos, token.start), min(len(line.source), token.end)
			if end <= start {
				continue
			}
			b.WriteString(line.source[pos:start])
			style := markdownTokenStyle(token, line.quoted && !line.calloutBody && !line.codeBlock, foreground)
			b.WriteString(style.UnsetBackground().Render(line.source[start:end]))
			pos = end
		}
		b.WriteString(line.source[pos:])
		result[i] = b.String()
		if level := line.headingLevel; level > 0 {
			result[i] = markdownHeadingStyle(level).Render(result[i])
		}
	}
	return result
}
