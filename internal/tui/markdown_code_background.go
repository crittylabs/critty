package tui

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *markdownSource) markCodeRows(start, end int) {
	m.hasCodeBlocks = true
	first := max(0, sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > start })-1)
	last := min(len(m.lines), sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] >= end }))
	for row := first; row < last; row++ {
		m.lines[row].codeBlock = true
	}
}

func codeBlockBackground() color.Color {
	foreground := lipgloss.Color("#000000")
	if terminalIsDark {
		foreground = lipgloss.Color("#FFFFFF")
	}
	return tintedBackground(foreground, 5)
}

// Paint after wrapping and selection mapping, so decorative padding never
// becomes selectable text. Existing diff and search backgrounds take priority.
func shadeCodeRow(content string, left, right int) string {
	if left >= right {
		return content
	}
	content += strings.Repeat(" ", max(0, right-ansi.StringWidth(content)))
	style := lipgloss.NewStyle().Background(codeBlockBackground())
	return ansi.Cut(content, 0, left) + inlineBackground(style, ansi.Cut(content, left, right)) + ansi.Cut(content, right, ansi.StringWidth(content))
}

func (m *AppModel) shadeMarkdownCodeBlocks(content string, layout *renderedContentLayout, width int) string {
	tab := m.tab()
	if !tab.isMarkdown || tab.markdown == nil {
		return content
	}
	hasAfter := tab.markdown.after != nil && tab.markdown.after.hasCodeBlocks
	hasBefore := tab.markdown.before != nil && tab.markdown.before.hasCodeBlocks
	if !hasAfter && !hasBefore {
		return content
	}
	rows := strings.Split(content, "\n")
	for _, span := range layout.textSpans {
		source := tab.markdown.after
		if span.ref.side == "old" {
			source = tab.markdown.before
		}
		if source == nil || span.ref.line <= 0 || span.ref.line > len(source.lines) {
			continue
		}
		line := &source.lines[span.ref.line-1]
		if !line.codeBlock || line.previewTokens != nil || span.row >= len(rows) {
			continue
		}
		right := width
		if span.column == 0 && span.row < len(layout.columns) && layout.columns[span.row].split > 0 {
			right = layout.columns[span.row].split
		}
		rows[span.row] = shadeCodeRow(rows[span.row], span.left, right)
	}
	return strings.Join(rows, "\n")
}
