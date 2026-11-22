package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/v2/internal/git"
)

const tableColumnMaxWidth = 40

func commentTableLayout(source *markdownSource, width int) map[int]*tableBlock {
	tables := detectTableBlocks(source.lines, func(line int) []string {
		return source.lines[line-1].renderTableCells(nil, lipgloss.Style{}, lipgloss.Style{}, nil)
	})
	if len(tables) == 0 {
		return nil
	}
	rows := make(map[int]*tableBlock)
	for i := range tables {
		table := &tables[i]
		// The regular comment renderer keeps multi-row previews intact.
		if slices.ContainsFunc(source.lines[table.startLine-1:table.endLine], func(line markdownLine) bool {
			return line.previewTokens != nil || line.previewHidden
		}) {
			continue
		}
		const minCellWidth = 2 // Leave room for wide characters.
		budget := width - (1 + 3*len(table.colWidths))
		if budget < minCellWidth*len(table.colWidths) {
			continue
		}
		for _, desired := range table.colWidths {
			budget -= max(minCellWidth, min(desired, tableColumnMaxWidth))
		}
		// Cap the widest columns first, leaving short columns intact.
		limit := tableColumnMaxWidth
		for budget < 0 && limit > minCellWidth {
			for _, desired := range table.colWidths {
				if desired >= limit {
					budget++
				}
			}
			limit--
		}
		for column, desired := range table.colWidths {
			table.colWidths[column] = max(minCellWidth, min(desired, limit))
		}
		for line := table.startLine; line <= table.endLine; line++ {
			rows[line] = table
		}
	}
	return rows
}

func commentTableRows(line *markdownLine, widths []int, header bool, style lipgloss.Style, links []linkSpan) []string {
	if line.tableSep {
		return []string{formatTableSep(widths)}
	}
	cells := line.renderTableCells(nil, style, style, links)
	columns := wrapTableCells(cells, widths, line)
	rows := make([]string, tableRowHeight(columns))
	for row := range rows {
		rows[row] = formatTableRow(tableRowCells(columns, row), widths, header)
	}
	return rows
}

type tableCellFragment struct {
	text   string
	offset int
}

func wrapTableCells(cells []string, widths []int, line *markdownLine) [][]tableCellFragment {
	columns := make([][]tableCellFragment, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		plain := ansi.Strip(cell)
		wrapText := plain
		if line != nil && i < len(line.cells) {
			wrapText = line.tableCellWrapText(i, plain)
		}
		offset := 0
		for _, text := range strings.Split(lipgloss.Wrap(wrapText, width, ""), "\n") {
			if start := strings.Index(wrapText[offset:], text); start >= 0 {
				offset += start
			}
			start := ansi.StringWidth(wrapText[:offset])
			row := ansi.Cut(cell, start, start+ansi.StringWidth(text))
			columns[i] = append(columns[i], tableCellFragment{row, len(ansi.Truncate(plain, start, ""))})
			offset += len(text)
		}
	}
	return columns
}

// Keep synthetic code padding attached while wrapping, then cut the original
// styled cell so text, hyperlinks, and source offsets stay unchanged.
func (line *markdownLine) tableCellWrapText(column int, plain string) string {
	base := line.displayOffset(line.cells[column][0])
	end := line.cells[column][1]
	var b strings.Builder
	start := 0
	for _, token := range line.displayTokens() {
		if token.start < line.cells[column][0] || token.end > end || token.style&markdownCodePadding == 0 {
			continue
		}
		offset := line.displayOffset(token.start) - base
		b.WriteString(plain[start:offset])
		b.WriteRune('\u00a0')
		start = offset + 1
	}
	b.WriteString(plain[start:])
	return b.String()
}

func tableRowHeight(columns [][]tableCellFragment) int {
	height := 1
	for _, column := range columns {
		height = max(height, len(column))
	}
	return height
}

func tableRowCells(columns [][]tableCellFragment, row int) []string {
	cells := make([]string, len(columns))
	for i, column := range columns {
		if row < len(column) {
			cells[i] = column[row].text
		}
	}
	return cells
}

func (m *AppModel) highlightTableCells(ref lineRef, cells []string) {
	s := &m.isearch
	if !s.active || s.re == nil {
		return
	}
	source := m.tab().markdown.after
	line := &source.lines[ref.line-1]
	highlights := s.markdownSearchHighlights(source)[ref.line]
	for i, cell := range cells {
		offset := line.displayOffset(line.cells[i][0])
		length := len(ansi.Strip(cell))
		var spans []isearchHighlight
		for _, h := range highlights {
			start, end := max(0, h.start-offset), min(length, h.end-offset)
			if start < end {
				current := s.found && s.match.tab == m.activeTab && s.match.ref == ref && s.match.ordinal == h.ordinal
				spans = append(spans, isearchHighlight{start, end, current})
			}
		}
		cells[i], _ = highlightISearchSpans(cell, spans)
	}
}

func (line *markdownLine) displayOffset(sourceOffset int) int {
	offset := 0
	for i, token := range line.displayTokens() {
		if token.decoration != noDecoration {
			if token.start < sourceOffset || token.start == sourceOffset && token.decoration == openingDecoration {
				offset += len(line.displayTokenText(i))
			}
			continue
		}
		if token.start >= sourceOffset {
			break
		}
		if token.end <= sourceOffset {
			offset += len(line.displayTokenText(i))
		} else if len(token.text) == token.end-token.start {
			offset += len(displaySourceText(token.text[:sourceOffset-token.start]))
		}
	}
	return offset
}

// Keep spans in source order even though wrapped cells share display rows.
func (l *renderedContentLayout) addTableSpans(firstRow int, columns [][]tableCellFragment, widths []int, line *markdownLine, target contentMouseTarget, viewport horizontalBlockViewport) {
	source := ansi.Strip(line.render(nil, diffCommonTextBg, diffAddedTextBg, nil))
	left := viewport.left - viewport.window.offset
	for i, column := range columns {
		if i >= len(line.cells) {
			break
		}
		start := line.displayOffset(line.cells[i][0])
		end := line.displayOffset(line.cells[i][1])
		for row := range tableRowHeight(columns) {
			offset, text := end, ""
			if row < len(column) {
				text = ansi.Strip(column[row].text)
				offset = start + column[row].offset
			}
			if row == 0 {
				offset = 0
				if i > 0 {
					offset = line.displayOffset(line.cells[i-1][1])
				}
			}
			text = "│ " + text + strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(text))) + " "
			if i == len(columns)-1 {
				text += "│"
			}
			l.textSpans = append(l.textSpans, contentTextSpan{
				row: firstRow + row, left: left, right: left + ansi.StringWidth(text),
				clipLeft: viewport.left, clipRight: viewport.left + viewport.window.width,
				ref: lineRef{line: target.line, side: target.side}, text: text,
				offset: offset, source: line.source, tableSource: source, literal: true, mapped: true,
			})
		}
		left += widths[i] + 3
	}
}

func (parsed *markdownLine) renderTableCells(inline []gitpkg.InlineSegment, base, changed lipgloss.Style, links []linkSpan) []string {
	var cells []string
	for _, span := range parsed.cells {
		cell := markdownLine{literal: true, source: parsed.source}
		for _, token := range parsed.displayTokens() {
			if token.decoration != noDecoration && token.start >= span[0] && token.start <= span[1] {
				cell.tokens = append(cell.tokens, token)
				continue
			}
			from, to := max(span[0], token.start), min(span[1], token.end)
			if from >= to {
				continue
			}
			part := token
			if len(token.text) == token.end-token.start {
				part.text = token.text[from-token.start : to-token.start]
			}
			part.start, part.end = from, to
			cell.tokens = append(cell.tokens, part)
		}
		for _, token := range cell.tokens {
			if token.style&markdownRendered != 0 {
				cell.previewTokens = cell.tokens
				break
			}
		}
		cells = append(cells, cell.render(inline, base, changed, links))
	}
	return cells
}
