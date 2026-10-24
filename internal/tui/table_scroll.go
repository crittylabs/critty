package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type tableViewport struct {
	startLine, endLine int
	left               int
	window             horizontalViewport
	match              *mouseRect // unscrolled horizontal position and rendered row
}

func (t tableViewport) render(row string) string {
	if t.window.maxOffset() == 0 {
		return row
	}
	left, right := " ", " "
	if t.window.offset > 0 {
		left = "◀"
	}
	if t.window.offset < t.window.maxOffset() {
		right = "▶"
	}
	return mdTablePipe.Render(left) + t.window.render(row) + mdTablePipe.Render(right)
}

func (m *AppModel) tableAtLine(line int) *tableViewport {
	for i := range m.contentLayout.tables {
		t := &m.contentLayout.tables[i]
		if line >= t.startLine && line <= t.endLine {
			return t
		}
	}
	return nil
}

func (m *AppModel) tableAtMouse(mouse tea.Mouse) *tableViewport {
	left, top, right, bottom := m.contentBounds()
	if mouse.X < left+gutterWidth || mouse.X >= right || mouse.Y < top || mouse.Y >= bottom {
		return nil
	}
	target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset())
	if !ok || target.annotation || target.side != "" {
		return nil
	}
	return m.tableAtLine(target.line)
}

func (m *AppModel) handleTableScrollClick(mouse tea.Mouse) bool {
	table := m.tableAtMouse(mouse)
	if table == nil || table.window.maxOffset() == 0 {
		return false
	}
	left, _, _, _ := m.contentBounds()
	x := mouse.X - left
	direction := 0
	if x == gutterWidth {
		direction = -1
	} else if x == table.left+table.window.width {
		direction = 1
	}
	if direction == 0 {
		return false
	}
	m.focused = contentPane
	m.scrollTable(*table, direction*max(1, table.window.width/2))
	return true
}

func (m *AppModel) scrollTable(table tableViewport, delta int) {
	offset := table.window.clamp(table.window.offset + delta)
	if offset == table.window.offset {
		return
	}
	if m.tab().tableOffsets == nil {
		m.tab().tableOffsets = make(map[int]int)
	}
	m.tab().tableOffsets[table.startLine] = offset
	m.rebuildContent()
}

func (m *AppModel) revealTableMatch() {
	table := m.tableAtLine(m.isearch.match.ref.line)
	if table == nil || table.match == nil || m.isearch.match.ref.side != "" {
		return
	}
	r := *table.match
	m.scrollTable(*table, table.window.reveal(r.left, r.right)-table.window.offset)
	top := m.contentViewport.YOffset()
	if r.top < top {
		m.contentViewport.SetYOffset(r.top)
	} else if r.bottom > top+m.contentViewport.Height() {
		m.contentViewport.SetYOffset(r.bottom - m.contentViewport.Height())
	}
}

func (m *AppModel) tableSearchMatch(lineNum, firstRow int, columns [][]tableCellFragment, widths []int) *mouseRect {
	s := &m.isearch
	if !s.active || !s.found || s.match.tab != m.activeTab || s.match.ref != (lineRef{line: lineNum}) {
		return nil
	}
	source := m.tab().markdown.after
	line := &source.lines[lineNum-1]
	left := 0
	for i, column := range columns {
		if i >= len(line.cells) {
			break
		}
		start := line.displayOffset(line.cells[i][0])
		for row, fragment := range column {
			plain := ansi.Strip(fragment.text)
			offset := start + fragment.offset
			for _, h := range s.markdownSearchHighlights(source)[lineNum] {
				from, to := max(0, h.start-offset), min(len(plain), h.end-offset)
				if h.ordinal == s.match.ordinal && h.line == lineNum && from < to {
					x := left + 2 + ansi.StringWidth(plain[:from])
					return &mouseRect{left: x, right: x + ansi.StringWidth(plain[from:to]), top: firstRow + row, bottom: firstRow + row + 1}
				}
			}
		}
		left += widths[i] + 3
	}
	return nil
}
