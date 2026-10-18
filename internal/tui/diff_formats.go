package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

const diffColumnMinimum = gutterWidth + 12

type diffLine struct {
	ref     lineRef
	oldLine int // nonzero for unchanged lines shared by both sides
	text    string
	styled  string
	changed bool
	inline  []gitpkg.InlineSegment
	before  *diffLine // original text of a whitespace-equivalent replacement
}

type diffRow struct{ before, after *diffLine }
type diffHunk []diffRow

type diffMouseRow struct {
	left, right contentMouseTarget
	split       int
}

func (m *AppModel) diffVisualLines(t *FileTab) []lineRef {
	var lines []lineRef
	for _, hunk := range diffHunks(t) {
		for _, row := range hunk {
			if row.before != nil && row.before.ref.side == "old" {
				lines = append(lines, row.before.ref)
			}
			if row.after != nil {
				lines = append(lines, row.after.ref)
			}
		}
	}
	return lines
}

func adjacentDiffLine(t *FileTab, step int) (lineRef, bool) {
	var rows []diffRow
	for _, hunk := range diffHunks(t) {
		rows = append(rows, hunk...)
	}
	current := lineRef{line: t.cursorLine, side: t.cursorSide}
	index := -1
	for i, row := range rows {
		if row.before != nil && row.before.ref == current || row.after != nil && row.after.ref == current {
			index = i
			break
		}
	}
	if index < 0 && t.cursorLine != 0 {
		return lineRef{}, false
	}
	next := index + step
	if next < 0 || next >= len(rows) {
		return lineRef{}, false
	}
	row := rows[next]
	if current.side == "old" && row.before != nil {
		return row.before.ref, true
	}
	if row.after != nil {
		return row.after.ref, true
	}
	return row.before.ref, true
}

func (m *AppModel) moveDiffSide(before bool) {
	t := m.tab()
	if t.selecting {
		return
	}
	current := lineRef{line: t.cursorLine, side: t.cursorSide}
	for _, hunk := range diffHunks(t) {
		for _, row := range hunk {
			if row.before != nil && row.before.ref == current || row.after != nil && row.after.ref == current {
				line := row.after
				if before {
					line = row.before
				}
				if line != nil {
					t.cursorLine, t.cursorSide = line.ref.line, line.ref.side
					t.cursorOnAnnotation, t.cursorOnForm = false, false
					m.rebuildContent()
					m.scrollToCursor()
				}
				return
			}
		}
	}
}

func (m *AppModel) contentMouseTargetAt(x, y int) (contentMouseTarget, bool) {
	if y >= 0 && y < len(m.contentLayout.columns) {
		row := m.contentLayout.columns[y]
		if row.split > 0 {
			target := row.left
			if x >= row.split+3 {
				target = row.right
			} else if x >= row.split {
				return contentMouseTarget{}, false
			}
			return target, target.line > 0
		}
	}
	target, ok := m.contentMouseTarget(y)
	return target, ok && (target.line > 0 || target.annotation || target.drifted)
}

func (m *AppModel) isContentGutter(x int) bool {
	if x == 0 {
		return true
	}
	if m.alternateDiff(m.tab()) {
		for _, row := range m.contentLayout.columns {
			if row.split > 0 {
				return x == row.split+3
			}
		}
	}
	return false
}

func (m *AppModel) alternateDiff(t *FileTab) bool {
	return m.multiFile && (t.diff != nil || t.status == "untracked") && t.doc != nil && !t.isBinary &&
		m.split && m.contentViewport.Width() >= 2*diffColumnMinimum+3
}

func (m AppModel) splitButton() string {
	if !m.multiFile {
		return ""
	}
	state := "off"
	if m.split {
		state = "on "
	}
	return modalBtnFocusedKey.Render("S") + modalBtnFocusedLabel.Bold(true).Padding(0).Render("plit: "+state)
}

type diffAttention struct {
	target          contentMouseTarget
	screenRow       int
	valid           bool
	cursor          lineRef
	onAnnotation    bool
	annotationIndex int
	tab             int
	focus           pane
	offset          int
}

// captureDiffAttention prefers the focused source or thread when visible,
// otherwise the first visible source.  It also works across a resize.
func (m *AppModel) captureDiffAttention() diffAttention {
	if !m.multiFile || len(m.tabs) == 0 || m.tab().state == nil {
		return diffAttention{}
	}
	t := m.tab()
	// Compression can bring an offscreen cursor into view without a user
	// move.  Continue anchoring the same visible source across consecutive
	// format changes until the cursor, focus, or scroll position changes.
	if previous := m.viewAttention; previous != nil && previous.valid && previous.tab == m.activeTab &&
		previous.cursor == (lineRef{side: t.cursorSide, line: t.cursorLine}) && previous.onAnnotation == t.cursorOnAnnotation &&
		previous.annotationIndex == t.cursorAnnoIdx && previous.focus == m.focused && previous.offset == m.contentViewport.YOffset() {
		if row, found := m.diffAnchorRow(previous.target); found && row-previous.offset == previous.screenRow {
			return *previous
		}
	}
	anchor := contentMouseTarget{line: t.cursorLine, side: t.cursorSide,
		annotation: t.cursorOnAnnotation, annotationIndex: t.cursorAnnoIdx}
	row, ok := m.diffAnchorRow(anchor)
	offset := m.contentViewport.YOffset()
	if !ok || row < offset || row >= offset+m.contentViewport.Height() {
		ok = false
		for y := offset; y < min(len(m.contentLayout.rows), offset+m.contentViewport.Height()); y++ {
			candidate := m.contentLayout.rows[y]
			if candidate.line > 0 || candidate.annotation {
				anchor, row, ok = candidate, y, true
				break
			}
		}
	}
	return diffAttention{target: anchor, screenRow: row - offset, valid: ok}
}

func (m *AppModel) restoreDiffAttention(attention diffAttention) {
	if !attention.valid {
		return
	}
	next, found := m.diffAnchorRow(attention.target)

	if found {
		m.contentViewport.SetYOffset(max(0, next-attention.screenRow))
		t := m.tab()
		attention.screenRow = next - m.contentViewport.YOffset()
		attention.cursor = lineRef{side: t.cursorSide, line: t.cursorLine}
		attention.onAnnotation, attention.annotationIndex = t.cursorOnAnnotation, t.cursorAnnoIdx
		attention.tab, attention.focus, attention.offset = m.activeTab, m.focused, m.contentViewport.YOffset()
		m.viewAttention = &attention
	}
}

func (m *AppModel) toggleSplit() {
	if !m.multiFile || len(m.tabs) == 0 {
		return
	}
	attention := m.captureDiffAttention()
	m.split = !m.split
	m.rebuildContent()
	m.updateCommentSidebar()
	m.restoreDiffAttention(attention)
}

func (m *AppModel) diffAnchorRow(anchor contentMouseTarget) (int, bool) {
	if anchor.annotation {
		for i, row := range m.contentLayout.rows {
			if row.annotation && row.side == anchor.side && row.line == anchor.line && row.annotationIndex == anchor.annotationIndex {
				return i, true
			}
		}
	}
	r, ok := m.contentRenderedRange(anchor.side, anchor.line, anchor.line)
	return r.start, ok
}

// diffHunks pairs before/after rows within each change hunk using only known
// source lines.  Unavailable patch context separates the hunks.
func diffHunks(t *FileTab) []diffHunk {
	if t.doc == nil {
		return nil
	}
	var groups [][]*diffLine
	var lines []*diffLine
	flush := func() {
		if len(lines) > 0 {
			groups = append(groups, lines)
			lines = nil
		}
	}
	old := 1
	originalDeletions := make(map[int]gitpkg.DeletedLine)
	if t.diff != nil {
		for _, deleted := range t.diff.DeletedAfter {
			for _, line := range deleted {
				originalDeletions[line.OldLineNum] = line
			}
		}
	}
	for n := 0; n <= t.doc.LineCount(); n++ {
		for i, del := range t.deletedAfter[n] {
			line := &diffLine{ref: lineRef{side: "old", line: del.OldLineNum},
				text: del.Content, changed: true, inline: del.Inline}
			if cached := t.deletedLineCache[n]; i < len(cached) {
				line.styled = cached[i]
			}
			lines = append(lines, line)
		}
		// Use the original diff for coordinate accounting even when whitespace
		// filtering hides some deletions and additions.
		if t.diff != nil {
			for _, del := range t.diff.DeletedAfter[n] {
				old = del.OldLineNum + 1
			}
		}
		next := n + 1
		if !t.doc.HasLine(next) {
			flush()
			continue
		}
		if next == t.doc.LineCount() && t.doc.Lines[n] == "" &&
			(t.doc.Content == "" || strings.HasSuffix(t.doc.Content, "\n")) && t.doc.Known == nil {
			continue // strings.Split's trailing sentinel is not a source line
		}
		line := &diffLine{ref: lineRef{line: next}, text: t.doc.Lines[n],
			styled:  documentDisplayLine(t, n, t.doc.Lines[n]),
			changed: t.changedLines[next] || t.status == "untracked", inline: t.inlineChanges[next]}
		if mapped := t.commonLines[next]; mapped > 0 {
			line.oldLine, old = mapped, mapped+1
		} else if !line.changed && (t.diff == nil || !t.diff.ChangedLines[next]) {
			line.oldLine = old
			old++
		}
		if t.diff != nil && t.diff.ChangedLines[next] && !line.changed {
			if original, ok := originalDeletions[line.oldLine]; ok {
				before := *line
				before.text, before.styled = original.Content, displaySourceText(original.Content)
				line.before = &before
			}
		}

		lines = append(lines, line)
	}
	flush()
	hunks := make([]diffHunk, 0, len(groups))
	for _, group := range groups {
		var hunk diffHunk
		for i := 0; i < len(group); {
			line := group[i]
			if line.ref.side == "old" {
				var before, after []*diffLine
				for i < len(group) && group[i].ref.side == "old" {
					before = append(before, group[i])
					i++
				}
				for i < len(group) && group[i].ref.side == "" && group[i].changed {
					after = append(after, group[i])
					i++
				}

				for j := 0; j < max(len(before), len(after)); j++ {
					var row diffRow
					if j < len(before) {
						row.before = before[j]
					}
					if j < len(after) {
						row.after = after[j]
					}
					hunk = append(hunk, row)
				}
			} else {
				row := diffRow{after: line}
				if !line.changed {
					row.before = line
					if line.before != nil {
						row.before = line.before
					}
				}
				hunk = append(hunk, row)
				i++
			}
		}
		hunks = append(hunks, hunk)
	}
	return hunks
}

func (m *AppModel) rebuildDiffContent() {
	t := m.tab()
	layout := newRenderedContentLayout()
	var b strings.Builder
	width := m.contentViewport.Width()
	groups := m.driftedGroups()
	for _, ids := range groups {
		width = min(width, m.contentViewport.Width()-lipgloss.Width(fmt.Sprintf("> ☑︎ 💬 %d", len(ids)))-1)
	}
	appendAnnotations := func(ref lineRef) {
		for idx, ann := range m.annotationsAfterLine(ref.line, ref.side) {
			focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == ref.line && t.cursorSide == ref.side && t.cursorAnnoIdx == idx
			if m.hideComments && !ann.drifted || ann.drifted && !m.driftedExpanded(ann.id, ann.resolved) && (!focused || !ann.resolved) {
				continue
			}
			box, button := m.renderAnnotationBox(ann, max(1, width-gutterWidth), focused)
			button.translate(0, len(layout.rows))
			button.id = ann.id
			layout.actions = append(layout.actions, button)
			layout.appendBlock(&b, box, contentMouseTarget{line: ref.line, side: ref.side, annotation: true, annotationIndex: idx})
		}
	}
	appendExtras := func(line *diffLine) {
		if line == nil {
			return
		}
		if images := m.renderMarkdownImages(line.ref.line, line.ref.side == "old"); images != "" {
			layout.appendBlock(&b, images, contentMouseTarget{line: line.ref.line, side: line.ref.side})
		}
		appendAnnotations(line.ref)
	}
	if len(groups[lineRef{}]) > 0 {
		layout.appendBlock(&b, "", contentMouseTarget{drifted: true})
	}
	appendAnnotations(lineRef{})
	hunks := diffHunks(t)
	if len(hunks) == 0 {
		layout.appendBlock(&b, "  No textual changes.", contentMouseTarget{})
	}
	for hi, hunk := range hunks {
		if hi > 0 || t.doc.Known != nil {
			layout.appendBlock(&b, "       ⋯ context not shown", contentMouseTarget{})
		}

		leftWidth := (width - 3) / 2
		rightWidth := width - 3 - leftWidth
		for _, row := range hunk {
			left, lt := m.diffCell(row.before, leftWidth, "-", true)
			right, rt := m.diffCell(row.after, rightWidth, "+", false)
			for i := 0; i < max(len(left), len(right)); i++ {
				l, r := "", ""
				if i < len(left) {
					l = left[i]
				}
				if i < len(right) {
					r = right[i]
				}
				start := len(layout.rows)
				target := rt
				if row.after == nil {
					target = lt
				}
				layout.appendBlock(&b, fitDiffCell(l, leftWidth)+" │ "+fitDiffCell(r, rightWidth), target)
				for len(layout.columns) <= start {
					layout.columns = append(layout.columns, diffMouseRow{})
				}
				layout.columns[start] = diffMouseRow{left: lt, right: rt, split: leftWidth}
				if row.before != nil && !lt.passive {
					updateDiffRange(layout.oldRanges, lt.line, start)
				}
			}
			if row.before != nil && row.before.ref.side == "old" {
				appendExtras(row.before)
			}
			appendExtras(row.after)
		}
	}
	if t.doc.Known != nil {
		layout.appendBlock(&b, "       ⋯ remaining context not included in diff", contentMouseTarget{})
	}
	m.contentLayout = layout
	m.contentViewport.SetContent(m.decorateDrifted(b.String(), &m.contentLayout, groups))
}

func updateDiffRange(ranges map[int]renderedRange, line, row int) {
	r, ok := ranges[line]
	if !ok {
		r.start = row
	}
	r.end = row + 1
	ranges[line] = r
}

func fitDiffCell(text string, width int) string {
	return ansi.Truncate(text, max(1, width), "") + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func (m *AppModel) diffCell(line *diffLine, width int, marker string, before bool) ([]string, contentMouseTarget) {
	if line == nil {
		return nil, contentMouseTarget{}
	}
	t := m.tab()
	ref := line.ref
	passive := before && ref.side != "old"
	target := contentMouseTarget{line: ref.line, side: ref.side, passive: passive}
	n := ref.line
	if passive {
		n = line.oldLine
	}
	selectedStart, selectedEnd := m.selectionRange()
	selected := t.selecting && m.selectionSide() == ref.side && ref.line >= selectedStart && ref.line <= selectedEnd
	hs, he, side := m.highlightedCommentLines()
	highlighted := side == ref.side && hs > 0 && ref.line >= hs && ref.line <= he
	cursor := !passive && t.cursorLine == ref.line && t.cursorSide == ref.side
	markStyle, numStyle := lipgloss.NewStyle(), lineNumStyle
	if line.changed {
		if ref.side == "old" {
			markStyle, numStyle = diffDeletedGutter, diffDeletedLineNum
		} else {
			markStyle = diffAddedGutter
		}
	} else {
		marker = " "
	}
	if !passive {
		switch {
		case cursor && !t.cursorOnAnnotation:
			marker, markStyle = ">", cursorMarker
		case selected:
			marker, markStyle = "|", selectedMarker
		case highlighted:
			marker, markStyle = ">", cursorMarker
		case m.hoveredGutterLine == ref.line && m.hoveredGutterSide == ref.side:
			marker, markStyle = ">", commentGutterMarker
		default:
			count := 0
			if t.state != nil {
				for _, c := range t.state.Comments {
					if !isGeneralCommentScope(c.Scope) && !commentDrifted(t, c) && c.Side == ref.side && c.StartLine <= ref.line && c.EndAt() >= ref.line {
						count++
					}
				}
			}
			if count > 1 {
				marker, markStyle = "◆", gutterOverlap
			} else if count == 1 {
				marker, markStyle = "■", annotationGutter
			}
		}
	}
	if cursor {
		numStyle = cursorLineNumStyle
	} else if selected {
		numStyle = selectedLineNumStyle
	}
	textWidth := max(1, width-gutterWidth-1)
	decorate := m.isearchDecorator(ref)
	var rows []string
	if ref.side == "old" {
		rows = deletedDisplayLines(line.text, line.styled, line.inline, t.isMarkdown, textWidth, decorate)
	} else if len(line.inline) > 0 {
		rows = inlineDiffDisplayLines(line.inline, t.isMarkdown, textWidth, diffCommonTextBg, diffAddedTextBg, decorate)
	} else {
		text := line.styled
		if t.isMarkdown {
			text = highlightMarkdown(linkedDisplayLine(line.text, m.imageSpans(ref.line, false), displaySourceText))
		}
		if decorate != nil {
			text, _ = decorate(text)
		}
		rows = strings.Split(lipgloss.Wrap(text, textWidth, ""), "\n")
		if line.changed {
			for i := range rows {
				rows[i] = inlineBackground(diffChangedLineBg, rows[i])
			}
		}
	}
	for i, text := range rows {
		if selected {
			text = inlineBackground(selectedLineBg, ansi.Strip(text))
		} else if highlighted {
			text = inlineBackground(sidebarHighlightBg, ansi.Strip(text))
		}
		if i == 0 {
			rows[i] = fmt.Sprintf("%s%s %s", markStyle.Render(marker), numStyle.Render(fmt.Sprint(n)), text)
		} else {
			rows[i] = fmt.Sprintf(" %s %s", continuationGutter, text)
		}
	}
	return rows, target
}
