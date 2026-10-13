package tui

import (
	"sort"

	"github.com/crittylabs/critty/internal/form"
)

// selectionRange returns the ordered start/end of the current selection.
// If not selecting, returns cursorLine, cursorLine.
func (m *AppModel) selectionRange() (int, int) {
	t := m.tab()
	if !t.selecting {
		return t.cursorLine, t.cursorLine
	}
	start, end := t.selectAnchor, t.cursorLine
	if start > end {
		start, end = end, start
	}
	return start, end
}

func (m *AppModel) selectionSide() string {
	t := m.tab()
	if t.selecting {
		return t.selectSide
	}
	return t.cursorSide
}

type lineRef struct {
	side string
	line int
}

func (m *AppModel) visualLines(t *FileTab) []lineRef {
	if t.doc == nil {
		return nil
	}
	lines := make([]lineRef, 0, t.doc.LineCount()+len(t.deletedAfter))
	for line := 1; line <= t.doc.LineCount(); line++ {
		for _, del := range t.deletedAfter[line-1] {
			lines = append(lines, lineRef{side: "old", line: del.OldLineNum})
		}
		if t.doc.HasLine(line) {
			lines = append(lines, lineRef{line: line})
		}
	}
	for _, del := range t.deletedAfter[t.doc.LineCount()] {
		lines = append(lines, lineRef{side: "old", line: del.OldLineNum})
	}
	return lines
}

func (m *AppModel) adjacentLine(t *FileTab, step int) (lineRef, bool) {
	lines := m.visualLines(t)
	if t.cursorLine == 0 && step > 0 && len(lines) > 0 {
		return lines[0], true
	}
	for i, ref := range lines {
		if ref.line != t.cursorLine || ref.side != t.cursorSide {
			continue
		}
		next := i + step
		if next >= 0 && next < len(lines) {
			return lines[next], true
		}
		break
	}
	return lineRef{}, false
}

func (m *AppModel) visualLineIndex(t *FileTab, target lineRef) int {
	for i, ref := range m.visualLines(t) {
		if ref == target {
			return i
		}
	}
	return -1
}

func (m *AppModel) moveCursorBy(t *FileTab, step, count int) {
	for range count {
		next, ok := m.adjacentLine(t, step)
		if !ok || (t.selecting && next.side != t.selectSide) {
			return
		}
		t.cursorLine, t.cursorSide = next.line, next.side
	}
}

// commentTargets lists a tab's comments in navigation order: file comments
// first in insertion order, then line
// comments in visual order.
func (m *AppModel) commentTargets(tabIndex int) []commentTarget {
	t := &m.tabs[tabIndex]
	if t.state == nil {
		return nil
	}

	indices := make(map[lineRef]int)
	var fileTargets, lineTargets []commentTarget
	for _, c := range t.state.Comments {
		ref := commentLocation(t, c)
		if ref.line == 0 {
			fileTargets = append(fileTargets, commentTarget{id: c.ID, scope: c.Scope, resolved: c.Resolved, annoIdx: len(fileTargets)})
			continue
		}
		line := ref.line
		lineTargets = append(lineTargets, commentTarget{id: c.ID, resolved: c.Resolved, line: line, side: c.Side, annoIdx: indices[ref]})
		indices[ref]++
	}
	if len(lineTargets) < 2 {
		return append(fileTargets, lineTargets...)
	}
	positions := make(map[lineRef]int)
	for i, ref := range m.visualLines(t) {
		positions[ref] = i
	}
	position := func(target commentTarget) int {
		if index, ok := positions[lineRef{side: target.side, line: target.line}]; ok {
			return index
		}
		return -1
	}
	sort.SliceStable(lineTargets, func(i, j int) bool {
		return position(lineTargets[i]) < position(lineTargets[j])
	})
	return append(fileTargets, lineTargets...)
}

// targetPosition orders a target against the content cursor.  File comments
// sit before the first line.
func (m *AppModel) targetPosition(t *FileTab, target commentTarget) int {
	if target.line == 0 {
		if isGeneralCommentScope(target.scope) && m.hideComments {
			m.hideComments = false
			m.recalculateLayout()
		}
		return -1
	}
	return m.visualLineIndex(t, lineRef{side: target.side, line: target.line})
}

// reviewTargets adds form controls to the comment stops in display order.
// Required choices share a resolution state and one unresolved stop per group.
func (m *AppModel) reviewTargets(tabIndex int) []commentTarget {
	targets := m.commentTargets(tabIndex)
	if m.forms == nil {
		return targets
	}
	t := &m.tabs[tabIndex]
	d := m.forms.Document(t.path)
	if d == nil || len(d.Items) == 0 {
		return targets
	}
	seen := map[string]bool{}
	for _, item := range d.Items {
		targets = append(targets, commentTarget{
			formID: item.ID, line: item.Line, required: item.Required,
			resolved: !item.Required || form.Satisfied(item, d.Answers[item.ID]),
			first:    !seen[item.ID],
		})
		seen[item.ID] = true
	}
	positions := map[lineRef]int{}
	for i, ref := range m.visualLines(t) {
		positions[ref] = i
	}
	position := func(target commentTarget) int {
		if target.line == 0 {
			return -1
		}
		if pos, ok := positions[lineRef{side: target.side, line: target.line}]; ok {
			return pos
		}
		return -1
	}
	sort.SliceStable(targets, func(i, j int) bool {
		left, right := targets[i], targets[j]
		if left.line == right.line && left.side == right.side {
			// Controls are in the source row; comments follow that row.
			return left.formID != "" && right.formID == ""
		}
		return position(left) < position(right)
	})
	return targets
}

func (target commentTarget) unresolved() bool {
	return !target.resolved && (target.formID == "" || target.first)
}

func (target commentTarget) changeStop() bool {
	return target.unresolved() || target.formID != "" && !target.required
}

// currentCommentTarget returns the index of the target the user is on: the
// selected sidebar item when the sidebar has focus, otherwise the focused
// inline annotation or form control.
func (m *AppModel) currentCommentTarget(targets []commentTarget) int {
	t := m.tab()
	if m.focused == commentPane {
		if t.sidebarCursor < len(t.sidebarItems) {
			id := t.sidebarItems[t.sidebarCursor].id
			for i, target := range targets {
				if target.id == id {
					return i
				}
			}
		}
		return -1
	}
	if t.cursorOnAnnotation {
		for i, target := range targets {
			if target.formID == "" && target.line == t.cursorLine && target.side == t.cursorSide && target.annoIdx == t.cursorAnnoIdx {
				return i
			}
		}
	}
	if m.focused == contentPane && t.cursorOnForm && !t.cursorOnAnnotation && t.cursorSide == "" {
		for i, target := range targets {
			if target.formID != "" && target.line == t.cursorLine {
				return i
			}
		}
	}
	return -1
}

// jumpToComment moves to the adjacent thread or form control in display
// order, wrapping across the entire review.
func (m *AppModel) jumpToComment(step int, includeResolved bool) bool {
	tabIndex, target, ok := m.adjacentComment(step, includeResolved)
	if ok {
		m.selectComment(tabIndex, target)
	}
	return ok
}

// nextCommentOrFinish moves to the next comment like jumpToComment, except
// that on the last matching target it opens the finish dialog instead of
// wrapping around to the first.
func (m *AppModel) nextCommentOrFinish(includeResolved bool) {
	if m.currentCommentTarget(m.reviewTargets(m.activeTab)) >= 0 {
		if _, _, ok := m.adjacentCommentNoWrap(1, includeResolved); !ok {
			m.openFinishModal()
			return
		}
	}
	m.jumpToComment(1, includeResolved)
}

func (m *AppModel) adjacentComment(step int, includeResolved bool) (int, commentTarget, bool) {
	return m.findAdjacentComment(step, includeResolved, true)
}

func (m *AppModel) adjacentCommentNoWrap(step int, includeResolved bool) (int, commentTarget, bool) {
	return m.findAdjacentComment(step, includeResolved, false)
}

// findAdjacentComment searches from the current position in the given
// direction, continuing into the other tabs.  With wrap, the search continues
// from the opposite end of the review until it returns to the current tab.
func (m *AppModel) findAdjacentComment(step int, includeResolved, wrap bool) (int, commentTarget, bool) {
	t := m.tab()
	targets := m.reviewTargets(m.activeTab)
	current := m.currentCommentTarget(targets)

	if current >= 0 {
		for adjacent := current + step; adjacent >= 0 && adjacent < len(targets); adjacent += step {
			if !includeResolved && !targets[adjacent].unresolved() {
				continue
			}
			return m.activeTab, targets[adjacent], true
		}
	} else {
		cursor := m.visualLineIndex(t, lineRef{side: t.cursorSide, line: t.cursorLine})
		if step > 0 {
			for _, target := range targets {
				if (includeResolved || target.unresolved()) && m.targetPosition(t, target) >= cursor {
					return m.activeTab, target, true
				}
			}
		} else {
			for i := len(targets) - 1; i >= 0; i-- {
				if (includeResolved || targets[i].unresolved()) && m.targetPosition(t, targets[i]) <= cursor {
					return m.activeTab, targets[i], true
				}
			}
		}
	}

	for offset := 1; offset <= len(m.tabs); offset++ {
		tabIndex := m.activeTab + step*offset
		if wrap {
			tabIndex = (tabIndex + len(m.tabs)) % len(m.tabs)
		} else if tabIndex < 0 || tabIndex >= len(m.tabs) {
			break
		}
		targets = m.reviewTargets(tabIndex)
		if len(targets) == 0 {
			continue
		}
		start := 0
		if step < 0 {
			start = len(targets) - 1
		}
		for i := start; i >= 0 && i < len(targets); i += step {
			if !includeResolved && !targets[i].unresolved() {
				continue
			}
			return tabIndex, targets[i], true
		}
	}
	return 0, commentTarget{}, false
}

func (m *AppModel) selectComment(tabIndex int, target commentTarget) {
	m.activeTab = tabIndex
	t := m.tab()
	if isGeneralCommentScope(target.scope) && m.hideComments {
		m.hideComments = false
		m.recalculateLayout()
	}
	m.focused = contentPane
	t.cursorLine, t.cursorSide = target.line, target.side
	t.cursorOnAnnotation = target.formID == ""
	t.cursorOnForm = target.formID != ""
	t.cursorAnnoIdx = target.annoIdx
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

type changeTarget struct {
	position int
	chunk    changeChunk
	comment  commentTarget
}

func (m *AppModel) changeTargets(tabIndex int) []changeTarget {
	t := &m.tabs[tabIndex]
	lines := m.visualLines(t)
	positions := make(map[lineRef]int, len(lines))
	for i, line := range lines {
		positions[line] = i
	}
	var targets []changeTarget
	hasStop := len(t.changeChunks) > 0
	if t.isBinary && !t.outsideChanges {
		// Binary changes have no source lines, but still need a navigation stop.
		targets = append(targets, changeTarget{position: 0})
		hasStop = true
	}
	for _, chunk := range t.changeChunks {
		position, ok := positions[m.changeCursor(t, chunk)]
		if !ok {
			position = len(lines) // A trailing deletion can anchor past EOF.
		}
		targets = append(targets, changeTarget{position: position, chunk: chunk})
	}
	for _, comment := range m.reviewTargets(tabIndex) {
		position := -1
		if comment.line != 0 {
			var ok bool
			position, ok = positions[lineRef{side: comment.side, line: comment.line}]
			if !ok {
				continue
			}
		}
		targets = append(targets, changeTarget{position: position, comment: comment})
		hasStop = hasStop || comment.changeStop()
	}
	if !hasStop {
		chunk := changeChunk{}
		if t.doc != nil && t.doc.HasLine(1) {
			chunk = changeChunk{startLine: 1, endLine: 1}
		}
		targets = append(targets, changeTarget{position: 0, chunk: chunk})
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].position < targets[j].position
	})
	return targets
}

// jumpToChange visits changes, unresolved threads and required forms, and
// optional form controls, stopping at the beginning and end of the review.
func (m *AppModel) jumpToChange(step int) bool {
	currentID := m.selectedCommentID()
	t := m.tab()
	reviewTargets := m.reviewTargets(m.activeTab)
	currentReview := m.currentCommentTarget(reviewTargets)
	cursor := m.visualLineIndex(t, lineRef{side: t.cursorSide, line: t.cursorLine})
	if t.isBinary && !t.cursorOnAnnotation {
		cursor = 0
	}
	if cursor < 0 && !t.cursorOnAnnotation && t.cursorLine == 0 && len(t.changeChunks) == 0 {
		cursor = 0
	}
	if cursor < 0 && t.cursorSide == "" && t.doc != nil && t.cursorLine > t.doc.LineCount() {
		cursor = len(m.visualLines(t))
	}
	for tabIndex := m.activeTab; tabIndex >= 0 && tabIndex < len(m.tabs); tabIndex += step {
		targets := m.changeTargets(tabIndex)
		current := -1
		if tabIndex == m.activeTab && (currentID != "" || currentReview >= 0) {
			for i, target := range targets {
				if currentID != "" && target.comment.id == currentID ||
					currentReview >= 0 && target.comment.formID != "" && target.comment == reviewTargets[currentReview] {
					current = i
					break
				}
			}
		}
		start := 0
		if step < 0 {
			start = len(targets) - 1
		}
		for i := start; i >= 0 && i < len(targets); i += step {
			target := targets[i]
			if (target.comment.id != "" || target.comment.formID != "") && !target.comment.changeStop() {
				continue
			}
			if tabIndex == m.activeTab {
				if current >= 0 {
					if (i-current)*step <= 0 {
						continue
					}
				} else if step > 0 {
					if target.position < cursor || (target.position == cursor && target.comment.id == "" && target.comment.formID == "") {
						continue
					}
				} else if target.position >= cursor {
					continue
				}
			}
			if target.comment.id != "" || target.comment.formID != "" {
				if target.comment.formID == "" && m.hideComments {
					m.hideComments = false
					m.recalculateLayout()
				}
				m.selectComment(tabIndex, target.comment)
			} else {
				m.focused = contentPane
				m.selectChange(tabIndex, target.chunk)
			}
			return true
		}
	}
	return false
}

func (m *AppModel) changeCursor(t *FileTab, chunk changeChunk) lineRef {
	if m.patch != nil && t.doc != nil && !t.doc.HasLine(chunk.startLine) {
		if dels := t.deletedAfter[chunk.startLine-1]; len(dels) > 0 {
			return lineRef{line: dels[0].OldLineNum, side: "old"}
		}
	}
	return lineRef{line: chunk.startLine}
}

func (m *AppModel) selectChange(tabIndex int, chunk changeChunk) {
	m.activeTab = tabIndex
	t := m.tab()
	cursor := m.changeCursor(t, chunk)
	t.cursorLine, t.cursorSide = cursor.line, cursor.side
	t.cursorOnAnnotation = false
	t.cursorOnForm = false
	t.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToChunk(chunk)
	if t.isBinary {
		m.contentViewport.SetYOffset(0)
	}
	if t.cursorSide == "old" {
		m.scrollToCursor()
	}
}

func (m *AppModel) scrollToCursor() {
	t := m.tab()
	if t.cursorOnAnnotation {
		id := m.selectedCommentID()
		if c, drifted := m.driftedComment(id); drifted && !m.driftedExpanded(id, c.Resolved) && (m.focused != contentPane || !c.Resolved) {
			for _, marker := range m.contentLayout.markers {
				for _, candidate := range marker.ids {
					if candidate == id {
						top := m.contentViewport.YOffset()
						if marker.rect.top < top {
							m.contentViewport.SetYOffset(marker.rect.top)
						}
						if marker.rect.bottom > top+m.contentViewport.Height() {
							m.contentViewport.SetYOffset(marker.rect.bottom - m.contentViewport.Height())
						}
						return
					}
				}
			}
		}
	}
	if t.cursorLine == 0 && t.cursorOnAnnotation {
		start, end := -1, 0
		for row, target := range m.contentLayout.rows {
			if target.line == 0 && target.annotation && target.annotationIndex == t.cursorAnnoIdx {
				if start < 0 {
					start = row
				}
				end = row + 1
			}
		}
		if start >= 0 {
			m.contentViewport.SetYOffset(max(0, min(start, end-m.contentViewport.Height())))
		}
		return
	}
	if t.doc == nil {
		return
	}
	r, ok := m.contentRenderedRange(t.cursorSide, t.cursorLine, t.cursorLine)
	if !ok {
		return
	}

	vpHeight := m.contentViewport.Height()
	currentTop := m.contentViewport.YOffset()

	if r.start < currentTop {
		m.contentViewport.SetYOffset(r.start)
	}
	if r.end > currentTop+vpHeight {
		m.contentViewport.SetYOffset(r.end - vpHeight)
	}
}

const chunkScrollPadding = 4

// scrollToChunk scrolls the viewport to show the entire change chunk
// plus padding lines above and below for context.
func (m *AppModel) scrollToChunk(chunk changeChunk) {
	t := m.tab()
	if t.doc == nil {
		return
	}

	startLine := chunk.startLine - chunkScrollPadding
	if startLine < 1 {
		startLine = 1
	}
	endLine := chunk.endLine + chunkScrollPadding
	if endLine > t.doc.LineCount() {
		endLine = t.doc.LineCount()
	}
	r, ok := m.contentRenderedRange("", startLine, endLine)
	if !ok {
		return
	}

	m.contentViewport.SetYOffset(r.start)
}

func (m *AppModel) scrollToAnnotation(side string, startLine, endLine int) {
	t := m.tab()
	if t.doc == nil {
		return
	}
	if endLine == 0 {
		endLine = startLine
	}

	r, ok := m.contentRenderedRange(side, startLine, endLine)
	if !ok {
		return
	}

	vpHeight := m.contentViewport.Height()

	offset := r.end - vpHeight
	if offset < 0 {
		offset = 0
	}
	if offset > r.start {
		offset = r.start
	}

	m.contentViewport.SetYOffset(offset)
}

func (m *AppModel) contentRenderedRange(side string, startLine, endLine int) (renderedRange, bool) {
	ranges := m.contentLayout.lineRanges
	if side == "old" {
		ranges = m.contentLayout.oldRanges
	}
	start, startOK := ranges[startLine]
	end, endOK := ranges[endLine]
	if !startOK || !endOK {
		return renderedRange{}, false
	}
	return renderedRange{start: start.start, end: end.end}, true
}

func (m *AppModel) scrollToSidebarCursor() {
	start, end := -1, 0
	for row, index := range m.sidebarTargets {
		if index == m.tab().sidebarCursor {
			if start < 0 {
				start = row
			}
			end = row + 1
		}
	}
	if start < 0 {
		return
	}
	height, offset := m.commentViewport.Height(), m.commentViewport.YOffset()
	if start < offset {
		m.commentViewport.SetYOffset(start)
	} else if end > offset+height {
		m.commentViewport.SetYOffset(min(start, end-height))
	}
}
