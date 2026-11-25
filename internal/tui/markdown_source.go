package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/v2/internal/icons"
)

func (m *AppModel) toggleMarkdownSource() tea.Cmd {
	t := m.tab()
	if t.doc == nil || !t.isMarkdown {
		return nil
	}
	if m.textSelection.active {
		m.clearTextSelection()
	}
	attention := m.captureDiffAttention()
	t.markdownSourceView = !t.markdownSourceView
	m.rebuildContent()
	m.restoreDiffAttention(attention)
	return m.prepareImage()
}

func (m AppModel) headerSyntax(style lipgloss.Style) string {
	t := m.tab()
	if t.doc == nil || t.lexer == nil {
		return ""
	}
	label := t.lexer.Config().Name
	if label == "" || label == "plaintext" {
		return ""
	}
	if label == "markdown" {
		label = "Markdown"
	}
	if t.markdownSourceView {
		label += ":raw"
	}
	return style.Render(m.icons.Glyph(icons.DividerLeft, "·") + " " + headerLabel(label) + " ")
}

// sourceTokens restores the raw bytes covered by each parsed token, filling
// in the markup that the formatted renderer omits.
func (line *markdownLine) sourceTokens() []markdownToken {
	if line.thematicBreak || line.tableSep {
		return []markdownToken{{end: len(line.source), text: line.source, style: markdownDelimiter}}
	}
	tokens := make([]markdownToken, 0, len(line.tokens))
	pos := 0
	for _, token := range line.tokens {
		if token.decoration != noDecoration {
			continue
		}
		start := max(pos, token.start)
		end := min(len(line.source), max(token.end, token.sourceEnd))
		if end <= start {
			continue
		}
		if start > pos {
			tokens = append(tokens, markdownToken{start: pos, end: start, text: line.source[pos:start], style: markdownDelimiter})
		}
		token.start, token.end, token.text = start, end, line.source[start:end]
		token.style &^= markdownRendered | markdownAtomic
		tokens = append(tokens, token)
		pos = end
	}
	if pos < len(line.source) {
		tokens = append(tokens, markdownToken{start: pos, end: len(line.source), text: line.source[pos:], style: markdownDelimiter})
	}
	if line.tableRow {
		return line.styleTablePipes(tokens)
	}
	return tokens
}

func (line *markdownLine) styleTablePipes(tokens []markdownToken) []markdownToken {
	result := make([]markdownToken, 0, len(tokens))
	cell := 0
	for _, token := range tokens {
		for column := token.start; column < token.end; column++ {
			if line.source[column] != '|' {
				continue
			}
			for cell < len(line.cells) && line.cells[cell][1] <= column {
				cell++
			}
			if cell < len(line.cells) && line.cells[cell][0] <= column || column > 0 && line.source[column-1] == '\\' {
				continue
			}
			if token.start < column {
				before := token
				before.end, before.text = column, line.source[token.start:column]
				result = append(result, before)
			}
			result = append(result, markdownToken{start: column, end: column + 1, text: "|", style: markdownDelimiter})
			token.start = column + 1
		}
		if token.start < token.end {
			token.text = line.source[token.start:token.end]
			result = append(result, token)
		}
	}
	return result
}

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
