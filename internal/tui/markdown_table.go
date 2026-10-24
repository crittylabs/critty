package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Cap long columns equally while retaining the natural width of short ones.
func fitTableWidths(natural []int, width int) []int {
	budget := width - 3*len(natural) - 1
	if budget < 2*len(natural) {
		return nil
	}
	used := func(cap int) int {
		total := 0
		for _, w := range natural {
			total += max(2, min(w, cap))
		}
		return total
	}
	low, high := 2, 2
	for _, w := range natural {
		high = max(high, w)
	}
	for low < high {
		mid := low + (high-low+1)/2
		if used(mid) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	widths := slices.Clone(natural)
	remaining := budget - used(low)
	for i, w := range widths {
		widths[i] = max(2, min(w, low))
		if remaining > 0 && w > widths[i] {
			widths[i]++
			remaining--
		}
	}
	return widths
}

type tableCellFragment struct {
	text   string
	offset int
}

func wrapTableCells(cells []string, widths []int) [][]tableCellFragment {
	columns := make([][]tableCellFragment, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		plain := ansi.Strip(cell)
		offset := 0
		for _, row := range strings.Split(lipgloss.Wrap(cell, width, ""), "\n") {
			text := ansi.Strip(row)
			if start := strings.Index(plain[offset:], text); start >= 0 {
				offset += start
			}
			columns[i] = append(columns[i], tableCellFragment{row, offset})
			offset += len(text)
		}
	}
	return columns
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
	for _, token := range line.tokens {
		if token.start >= sourceOffset {
			break
		}
		if token.end <= sourceOffset {
			offset += len(displaySourceText(token.text))
		} else if len(token.text) == token.end-token.start {
			offset += len(displaySourceText(token.text[:sourceOffset-token.start]))
		}
	}
	return offset
}

// Keep spans in source order even though wrapped cells share display rows.
func (l *renderedContentLayout) addTableSpans(firstRow int, columns [][]tableCellFragment, widths []int, line *markdownLine, target contentMouseTarget) {
	source := ansi.Strip(line.render(nil, diffCommonTextBg, diffAddedTextBg, nil))
	left := gutterWidth
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
				ref: lineRef{line: target.line, side: target.side}, text: text,
				offset: offset, source: line.source, tableSource: source, literal: true, mapped: true,
			})
		}
		left += widths[i] + 3
	}
}
