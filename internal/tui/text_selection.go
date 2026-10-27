package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Text spans exclude gutters, annotations, images and split-diff padding.
type contentTextSpan struct {
	row, left, column   int
	ref                 lineRef
	text                string
	offset              int
	source              string
	tableSource         string
	literal             bool
	mapped              bool
	right               int
	clipLeft, clipRight int
}

type textPoint struct{ span, offset int }

type contentTextSelection struct {
	active, dragging bool
	tab              int
	anchor, cursor   textPoint
	spans            []contentTextSpan
	link             string
	target           contentMouseTarget
}

func (l *renderedContentLayout) appendSourceBlock(b *strings.Builder, block string, target contentMouseTarget) {
	l.addTextSpan(len(l.rows), 0, 0, block, target)
	l.appendBlock(b, block, target)
}

func (l *renderedContentLayout) addTextSpan(row, left, column int, text string, target contentMouseTarget) {
	if target.line <= 0 || target.annotation || text == "" {
		return
	}
	l.textSpans = append(l.textSpans, contentTextSpan{
		row: row, left: left + gutterWidth, column: column,
		ref:  lineRef{line: target.line, side: target.side},
		text: ansi.Strip(ansi.Cut(text, gutterWidth, ansi.StringWidth(text))),
	})
}

// Snap a terminal column to a complete grapheme, including wide characters.
func textColumnOffset(text string, column int) int {
	return len(ansi.Truncate(text, max(0, column), ""))
}

func (m *AppModel) startTextSelection(mouse tea.Mouse) {
	left, top, _, _ := m.contentBounds()
	x, row := mouse.X-left, mouse.Y-top+m.contentViewport.YOffset()
	for i, span := range m.contentLayout.textSpans {
		right := m.contentViewport.Width()
		if row >= 0 && row < len(m.contentLayout.columns) && span.column == 0 && m.contentLayout.columns[row].split > 0 {
			right = m.contentLayout.columns[row].split
		}
		if span.right > 0 {
			right = span.right
		}
		left := span.left
		if span.clipRight > 0 {
			right = span.left + ansi.StringWidth(span.text)
			left, right = max(left, span.clipLeft), min(right, span.clipRight)
		}
		if span.row == row && x >= left && x < right {
			point := textPoint{i, textColumnOffset(span.text, x-span.left)}
			m.textSelection = contentTextSelection{active: true, dragging: true, tab: m.activeTab,
				anchor: point, cursor: point, spans: slices.Clone(m.contentLayout.textSpans)}
			m.tab().selecting = false
			return
		}
	}
}

func (m *AppModel) updateTextSelection(mouse tea.Mouse, autoScroll bool) {
	hadText := m.hasTextSelection()
	s := &m.textSelection
	left, top, _, bottom := m.contentBounds()
	if autoScroll && mouse.Y <= top {
		m.contentViewport.ScrollUp(1)
	} else if autoScroll && mouse.Y >= bottom-1 {
		m.contentViewport.ScrollDown(1)
	}
	row := min(max(mouse.Y, top), bottom-1) - top + m.contentViewport.YOffset()
	column := s.spans[s.anchor.span].column
	best := -1
	bestRow, bestColumn := int(^uint(0)>>1), int(^uint(0)>>1)
	for i, span := range s.spans {
		if span.column != column {
			continue
		}
		distance := span.row - row
		if distance < 0 {
			distance = -distance
		}
		x := mouse.X - left
		right := span.left + ansi.StringWidth(span.text)
		if span.right > 0 {
			right = span.right
		}
		left := span.left
		if span.clipRight > 0 {
			left, right = max(left, span.clipLeft), min(right, span.clipRight)
			if left >= right {
				continue
			}
		}
		horizontal := max(left-x, x-right, 0)
		if distance < bestRow || distance == bestRow && horizontal < bestColumn {
			best, bestRow, bestColumn = i, distance, horizontal
		}
	}
	if best >= 0 {
		span := s.spans[best]
		x := mouse.X - left - span.left
		if span.row < row {
			x = ansi.StringWidth(span.text)
		} else if span.row > row {
			x = 0
		}
		s.cursor = textPoint{best, textColumnOffset(span.text, x)}
	}
	if hadText != (m.hasTextSelection()) {
		m.recalculateLayout()
	}
	m.rebuildContent()
}

func (s contentTextSelection) bounds() (textPoint, textPoint) {
	a, b := s.anchor, s.cursor
	if a.span > b.span || a.span == b.span && a.offset > b.offset {
		a, b = b, a
	}
	return a, b
}

func (m *AppModel) hasTextSelection() bool {
	s := m.textSelection
	return s.active && s.tab == m.activeTab && s.anchor != s.cursor
}

func (m *AppModel) selectedText() string {
	s := m.textSelection
	if !s.active || s.tab != m.activeTab {
		return ""
	}
	a, b := s.bounds()
	if a == b {
		return ""
	}
	column := s.spans[a.span].column
	type sourceRange struct {
		first, last contentTextSpan
		start, end  int
	}
	var ranges []sourceRange
	rawLines := make(map[lineRef]string)
	for i := a.span; i <= b.span; i++ {
		span := s.spans[i]
		if span.column != column {
			continue
		}
		rawLines[span.ref] = span.source
		start, end := span.offset, span.offset+len(span.text)
		if i == a.span {
			start += a.offset
		}
		if i == b.span {
			end = span.offset + b.offset
		}
		if span.literal {
			start = span.offset + tableSourceOffset(span.tableSource[span.offset:], span.text, start-span.offset, false)
			end = span.offset + tableSourceOffset(span.tableSource[span.offset:], span.text, end-span.offset, true)
		}
		if len(ranges) > 0 {
			last := &ranges[len(ranges)-1]
			if last.last.ref == span.ref || last.last.ref.side == span.ref.side && last.last.ref.line+1 == span.ref.line {
				last.last, last.end = span, end
				continue
			}
		}
		ranges = append(ranges, sourceRange{span, span, start, end})
	}
	var result strings.Builder
	for i, r := range ranges {
		if i > 0 {
			result.WriteByte('\n')
		}
		if m.tab().isMarkdown {
			m.tab().ensureMarkdownCache()
			source := m.tab().markdown.after
			if r.first.ref.side == "old" {
				source = m.tab().markdown.before
			}
			if source != nil && r.first.ref.line > 0 && r.last.ref.line <= len(source.lines) {
				start, end := r.start, r.end
				start = source.lines[r.first.ref.line-1].sourceOffset(start, false)
				end = source.lines[r.last.ref.line-1].sourceOffset(end, true)
				result.WriteString(source.copySourceRange(source.offsets[r.first.ref.line-1]+start, source.offsets[r.last.ref.line-1]+end))
				continue
			}
		}
		for line := r.first.ref.line; line <= r.last.ref.line; line++ {
			raw := rawLines[lineRef{line: line, side: r.first.ref.side}]
			start, end := 0, len(raw)
			if line == r.first.ref.line {
				start = rawTextOffset(raw, raw, r.start, false)
			}
			if line == r.last.ref.line {
				end = rawTextOffset(raw, raw, r.end, true)
			}
			if line > r.first.ref.line {
				result.WriteByte('\n')
			}
			result.WriteString(raw[start:end])
		}
	}
	return result.String()
}

func (m *AppModel) clearTextSelection() {
	hadText := m.hasTextSelection()
	m.textSelection = contentTextSelection{}
	if hadText {
		m.recalculateLayout()
	}
	m.rebuildContent()
}

func (m *AppModel) copySelectedText() tea.Cmd {
	text := m.selectedText()
	if text == "" {
		return nil
	}
	m.killRing.interrupt()
	m.killRing.add(text, false)
	m.killRing.interrupt()
	m.locationError = "Copied text to kill ring and system clipboard"
	m.textSelection = contentTextSelection{}
	m.recalculateLayout()
	m.rebuildContent()
	return tea.SetClipboard(text)
}

func (m AppModel) copyTextButton() string {
	return m.renderModalButton("Copy text", "M-w", false)
}

// Wrapping can omit spaces between words. Keep those spaces when copying
// across two displayed rows of the same source line.
func (m *AppModel) prepareTextSpans() {
	type flow struct {
		text   string
		source string
		offset int
	}
	type flowKey struct {
		ref    lineRef
		column int
	}
	flows := make(map[flowKey]*flow)
	oldLines := make(map[int]string)
	for _, lines := range m.tab().deletedAfter {
		for _, line := range lines {
			oldLines[line.OldLineNum] = line.Content
		}
	}
	for i := range m.contentLayout.textSpans {
		span := &m.contentLayout.textSpans[i]
		if span.mapped {
			continue
		}
		key := flowKey{span.ref, span.column}
		f := flows[key]
		if f == nil {
			value := span.source
			t := m.tab()
			if value == "" {
				if span.ref.side == "old" {
					value = oldLines[span.ref.line]
				} else if t.doc != nil && t.doc.HasLine(span.ref.line) {
					value = t.doc.Lines[span.ref.line-1]
				}
			}
			raw := value
			if t.isMarkdown {
				value = ansi.Strip(t.markdownDisplayLine(span.ref.line, span.ref.side == "old", value, nil, nil))
			} else {
				value = displaySourceText(value)
			}
			f = &flow{text: value, source: raw}
			flows[key] = f
		}
		if span.literal {
			span.offset = f.offset
			span.tableSource = f.text
			f.offset += tableSourceOffset(f.text[f.offset:], span.text, len(span.text), true)
		} else if offset := strings.Index(f.text[f.offset:], span.text); offset >= 0 {
			span.offset = f.offset + offset
			f.offset += offset + len(span.text)
		}
		span.source = f.source
	}
}

func (m *AppModel) decorateTextSelection(content string) string {
	m.prepareTextSpans()
	s := m.textSelection
	if !s.active {
		return content
	}
	if s.tab != m.activeTab || !slices.EqualFunc(s.spans, m.contentLayout.textSpans, func(a, b contentTextSpan) bool {
		a.row, a.left, a.right = b.row, b.left, b.right
		a.clipLeft, a.clipRight = b.clipLeft, b.clipRight
		return a == b
	}) {
		m.textSelection = contentTextSelection{}
		m.recalculateLayout()
		return content
	}
	s.spans = slices.Clone(m.contentLayout.textSpans)
	m.textSelection.spans = s.spans
	a, b := s.bounds()
	column := s.spans[a.span].column
	rows := strings.Split(content, "\n")
	for i := a.span; i <= b.span; i++ {
		span := s.spans[i]
		if span.column != column {
			continue
		}
		start, end := 0, len(span.text)
		if i == a.span {
			start = a.offset
		}
		if i == b.span {
			end = b.offset
		}
		x, y := span.left+ansi.StringWidth(span.text[:start]), span.left+ansi.StringWidth(span.text[:end])
		if span.clipRight > 0 {
			x, y = max(x, span.clipLeft), min(y, span.clipRight)
		}
		if x >= y {
			continue
		}
		row := rows[span.row]
		rows[span.row] = ansi.Cut(row, 0, x) + inlineBackground(textSelectionBg, ansi.Cut(row, x, y)) + ansi.Cut(row, y, lipgloss.Width(row))
	}
	return strings.Join(rows, "\n")
}
