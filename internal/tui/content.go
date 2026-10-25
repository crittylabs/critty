package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

func expandDisplayTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", displayTabWidth))
}

// displaySourceText makes a source line safe to render as fixed-width rows:
// tabs become spaces, and other C0 control characters and DEL become caret
// notation such as ^M so a stray CR cannot overwrite the row.  ESC passes
// through so syntax highlighting survives; apply this before adding
// hyperlinks, whose OSC terminator is BEL.
func displaySourceText(s string) string {
	if strings.IndexFunc(s, needsDisplayEscape) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString(strings.Repeat(" ", displayTabWidth))
		case r == 0x7f:
			b.WriteString("^?")
		case needsDisplayEscape(r):
			b.WriteByte('^')
			b.WriteByte(byte(r) + 0x40)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func needsDisplayEscape(r rune) bool {
	return r == 0x7f || r < 0x20 && r != '\n' && r != 0x1b
}

func documentDisplayLine(t *FileTab, index int, fallback string) string {
	if !t.isMarkdown && t.chromaLines != nil && index < len(t.chromaLines) {
		return displaySourceText(t.chromaLines[index])
	}
	return displaySourceText(fallback)
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	if len(m.tabs) == 0 {
		m.textSelection = contentTextSelection{}
		m.contentViewport.SetContent("")
		return
	}
	t := m.tab()
	t.unread = false
	if m.alternateDiff(t) {

		m.rebuildDiffContent()
		return
	}
	m.contentLayout = newRenderedContentLayout()

	// Collect annotations keyed by the line they appear AFTER
	annosByEndLine := make(map[int][]annotation)
	oldAnnosByEndLine := make(map[int][]annotation)
	if t.state != nil {
		for _, c := range t.state.Comments {
			if isGeneralCommentScope(c.Scope) {
				continue
			}
			ref := commentLocation(t, c)
			if ref.line == 0 {
				continue
			}
			endAt := ref.line
			if ref.side == "old" {
				oldAnnosByEndLine[endAt] = append(oldAnnosByEndLine[endAt], tabAnnotation(t, c))
			} else {
				annosByEndLine[endAt] = append(annosByEndLine[endAt], tabAnnotation(t, c))
			}
		}
	}

	// Count how many comments cover each line (for overlap detection)
	annotatedLines := make(map[int]int)
	oldAnnotatedLines := make(map[int]int)
	if t.state != nil {
		for _, c := range t.state.Comments {
			if isGeneralCommentScope(c.Scope) || commentDrifted(t, c) {
				continue
			}
			lines := annotatedLines
			if c.Side == "old" {
				lines = oldAnnotatedLines
			}
			for l := c.StartLine; l <= c.EndAt(); l++ {
				lines[l]++
			}
		}
	}

	selStart, selEnd := m.selectionRange()
	selSide := m.selectionSide()

	// Determine which lines to highlight from the selected annotation.
	sidebarHighlightStart, sidebarHighlightEnd, sidebarHighlightSide := m.highlightedCommentLines()

	groups := m.driftedGroups()
	contentWidth := m.contentViewport.Width()
	for _, ids := range groups {
		width := lipgloss.Width(fmt.Sprintf("> ☑︎ 💬 %d", len(ids))) + 1
		contentWidth = min(contentWidth, m.contentViewport.Width()-width)
	}
	boxWidth := contentWidth - gutterWidth
	if boxWidth < 20 {
		boxWidth = 20
	}

	textWidth := contentWidth - gutterWidth - 1
	if textWidth < 10 {
		textWidth = 10
	}

	// Use cached syntax highlighting
	isMarkdown := t.isMarkdown
	// Detect table blocks so we can align columns across rows
	var sourceLines []string
	if t.doc != nil {
		sourceLines = t.doc.Lines
	}
	var tableBlocks []tableBlock
	if isMarkdown {
		t.ensureMarkdownCache()
		if t.markdown != nil {
			tableBlocks = detectTableBlocks(t.markdown.after.lines, func(line int) []string { return t.markdownTableCells(line, nil) })
		}
	}
	tableBlockMap := make(map[int]*tableBlock)
	for i := range tableBlocks {
		tb := &tableBlocks[i]
		totalWidth := 1
		for i, width := range tb.colWidths {
			tb.colWidths[i] = max(2, min(width, tableColumnMaxWidth))
			totalWidth += tb.colWidths[i] + 3
		}
		width, left := textWidth, gutterWidth
		if totalWidth > width {
			width -= 2
			left++
		}
		tb.viewport = tableViewport{startLine: tb.startLine, endLine: tb.endLine, left: left,
			window: horizontalViewport{width: width, contentWidth: totalWidth}}
		tb.viewport.window.offset = tb.viewport.window.clamp(t.tableOffsets[tb.startLine])
		if t.tableOffsets != nil {
			t.tableOffsets[tb.startLine] = tb.viewport.window.offset
		}
		for l := tb.startLine; l <= tb.endLine; l++ {
			tableBlockMap[l] = tb
		}
	}

	var b strings.Builder
	b.Grow(len(sourceLines) * 200) // pre-allocate to reduce allocations
	layout := newRenderedContentLayout()
	if t.outsideChanges && t.doc != nil {
		layout.appendBlock(&b, ansi.Wrap("Saved snapshot: outside current changes.\n", max(1, contentWidth), ""), contentMouseTarget{})
	} else if m.documentSnapshot != nil && t.path == m.filePath {
		layout.appendBlock(&b, ansi.Wrap("Saved snapshot: source file unavailable in this workspace.\n", max(1, contentWidth), ""), contentMouseTarget{})
	}
	if t.attachment != nil && t.attachment.Removed {
		notice := "History (read-only): this attachment is not part of this review.\n" +
			"Checkboxes are inactive because this attachment was removed or replaced.\n" +
			"Answer forms in the current attachments section.  This snapshot is kept for comment history."
		layout.appendBlock(&b, ansi.Wrap(notice, max(1, contentWidth), ""), contentMouseTarget{})
		layout.appendBlock(&b, "", contentMouseTarget{})
	}
	appendAnnotation := func(ann annotation, focused bool, target contentMouseTarget) {
		if ann.drifted && !m.driftedExpanded(ann.id, ann.resolved) && (!focused || !ann.resolved) {
			return
		}
		box, button := m.renderAnnotationBox(ann, boxWidth, focused)
		button.translate(0, len(layout.rows))
		button.id = ann.id
		layout.actions = append(layout.actions, button)
		layout.appendBlock(&b, box, target)
	}
	if len(groups[lineRef{}]) > 0 {
		layout.appendBlock(&b, "", contentMouseTarget{drifted: true})
	}
	for idx, ann := range m.annotationsAfterLine(0, "") {
		focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == 0 && t.cursorAnnoIdx == idx
		appendAnnotation(ann, focused, contentMouseTarget{annotation: true, annotationIndex: idx})
	}
	if t.isBinary || t.doc == nil {
		if t.isBinary {
			if preview := m.renderImagePreview(); preview != "" {
				layout.appendBlock(&b, "\n  Image "+t.changeSummary(), contentMouseTarget{})
				layout.appendBlock(&b, preview, contentMouseTarget{})
			} else {
				layout.appendBlock(&b, "\n  Binary file "+t.changeSummary()+" — cannot display content.", contentMouseTarget{})
				if t.preview.before.failed || t.preview.after.failed {
					layout.appendBlock(&b, "  Image preview unavailable (unsupported, damaged, or too large).", contentMouseTarget{})
				}
			}
		} else if t.outsideChanges {
			layout.appendBlock(&b, "\n  Added file removed — no longer part of the changes.", contentMouseTarget{})
		}
		m.contentLayout = layout
		m.contentViewport.SetContent(m.decorateTextSelection(m.decorateDrifted(b.String(), &m.contentLayout, groups)))
		return
	}
	renderDeleted := func(afterLine int) {
		dels := t.deletedAfter[afterLine]
		cachedHL := t.deletedLineCache[afterLine]
		for di, del := range dels {
			target := contentMouseTarget{line: del.OldLineNum, side: "old"}
			isCursor := t.cursorSide == "old" && del.OldLineNum == t.cursorLine
			isSelected := t.selecting && selSide == "old" && del.OldLineNum >= selStart && del.OldLineNum <= selEnd
			isSidebarHighlight := sidebarHighlightSide == "old" && del.OldLineNum >= sidebarHighlightStart && del.OldLineNum <= sidebarHighlightEnd

			marker := diffDeletedGutter.Render("-")
			switch {
			case del.OldLineNum == m.hoveredGutterLine && m.hoveredGutterSide == "old":
				marker = commentGutterMarker.Render(">")
			case isCursor && !t.cursorOnAnnotation:
				marker = cursorMarker.Render(">")
			case isSelected:
				marker = selectedMarker.Render("|")
			case isSidebarHighlight:
				marker = cursorMarker.Render(">")
			case oldAnnotatedLines[del.OldLineNum] > 1:
				marker = gutterOverlap.Render("◆")
			case oldAnnotatedLines[del.OldLineNum] == 1:
				marker = annotationGutter.Render("■")
			}

			numStyle := diffDeletedLineNum
			if isCursor {
				numStyle = cursorLineNumStyle
			} else if isSelected {
				numStyle = selectedLineNumStyle
			}
			num := numStyle.Render(fmt.Sprintf("%d", del.OldLineNum))
			var cached string
			if di < len(cachedHL) {
				cached = cachedHL[di]
			}
			decorate := m.isearchDecorator(lineRef{side: "old", line: del.OldLineNum})
			var rows []string
			if isMarkdown {
				content := t.markdownDisplayLine(del.OldLineNum, true, del.Content, del.Inline, m.imageSpans(del.OldLineNum, true))
				if len(del.Inline) == 0 {
					content = inlineBackground(diffDeletedLineBg, content)
				}
				rows = markdownDisplayRows(content, textWidth, decorate)
			} else {
				rows = deletedDisplayLines(del.Content, cached, del.Inline, false, textWidth, decorate)
			}
			for wi, content := range rows {
				if isSelected {
					content = inlineBackground(selectedLineBg, ansi.Strip(content))
				} else if isSidebarHighlight {
					content = inlineBackground(sidebarHighlightBg, ansi.Strip(content))
				}
				if wi == 0 {
					layout.appendSourceBlock(&b, fmt.Sprintf("%s%s %s", marker, num, content), target)
				} else {
					layout.appendSourceBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, content), target)
				}
			}
			if images := m.renderMarkdownImages(del.OldLineNum, true); images != "" {
				layout.appendBlock(&b, images, target)
			}
			for idx, ann := range oldAnnosByEndLine[del.OldLineNum] {
				focused := m.focused == contentPane && t.cursorOnAnnotation && isCursor && t.cursorAnnoIdx == idx
				appendAnnotation(ann, focused, contentMouseTarget{
					line: del.OldLineNum, side: "old", annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	for _, i := range m.lineOrder(t) {
		line := t.doc.Lines[i]
		lineNum := i + 1
		lineTarget := contentMouseTarget{line: lineNum}

		// Render deleted lines that appear before this line
		renderDeleted(lineNum - 1)
		if !t.doc.HasLine(lineNum) {
			if lineNum == 1 || t.doc.HasLine(lineNum-1) {
				layout.appendBlock(&b, "       ⋯ context not included in diff", contentMouseTarget{})
			}
			continue
		}

		isCursor := t.cursorSide == "" && lineNum == t.cursorLine
		isSelected := t.selecting && selSide == "" && lineNum >= selStart && lineNum <= selEnd
		isSidebarHighlight := sidebarHighlightSide == "" && sidebarHighlightStart > 0 && lineNum >= sidebarHighlightStart && lineNum <= sidebarHighlightEnd
		isChanged := t.changedLines != nil && t.changedLines[lineNum]
		inlineChanges := t.inlineChanges[lineNum]
		formBackground := formLineBackground(isSelected, isSidebarHighlight)
		decorate := m.isearchDecorator(lineRef{line: lineNum})
		highlight := func(s string) string {
			if decorate != nil {
				s, _ = decorate(s)
			}
			return s
		}

		// Marker column
		var marker string
		if lineNum == m.hoveredGutterLine && m.hoveredGutterSide == "" {
			marker = commentGutterMarker.Render(">")
		} else if isCursor && !t.cursorOnAnnotation {
			marker = cursorMarker.Render(">")
		} else if isSelected {
			marker = selectedMarker.Render("|")
		} else if isSidebarHighlight {
			marker = cursorMarker.Render(">")
		} else if count, ok := annotatedLines[lineNum]; ok && count > 0 {
			if count > 1 {
				marker = gutterOverlap.Render("◆")
			} else {
				marker = annotationGutter.Render("■")
			}
		} else if isChanged {
			marker = diffAddedGutter.Render("+")
		} else {
			marker = " "
		}

		// Line number
		numStyle := lineNumStyle
		if isCursor {
			numStyle = cursorLineNumStyle
		} else if isSelected {
			numStyle = selectedLineNumStyle
		}
		switch m.rankMove(t, lineNum) {
		case -1:
			numStyle = numStyle.Background(rankMovedUp.GetBackground())
		case 1:
			numStyle = numStyle.Background(rankMovedDown.GetBackground())
		}
		numStr := numStyle.Render(fmt.Sprintf("%d", lineNum))

		// Check if this line is part of a table block
		if _, inTable := tableBlockMap[lineNum]; len(inlineChanges) > 0 && !isSelected && !isSidebarHighlight && !inTable {
			var rows []string
			if isMarkdown {
				rows = markdownDisplayRows(t.markdownDisplayLine(lineNum, false, line, inlineChanges, m.documentLinkSpans(lineNum, false)), textWidth, decorate)
			} else {
				rows = inlineDiffDisplayLines(inlineChanges, false, textWidth, diffCommonTextBg, diffAddedTextBg, decorate)
			}
			for wi, styledLine := range rows {
				if wi == 0 {
					layout.appendSourceBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, styledLine), lineTarget)
				} else {
					layout.appendSourceBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, styledLine), lineTarget)
				}
			}
		} else if tb, inTable := tableBlockMap[lineNum]; inTable {
			var rows []string
			var columns [][]tableCellFragment
			if t.markdown.after.lines[lineNum-1].tableSep {
				rows = []string{formatTableSep(tb.colWidths)}
			} else {
				isHeader := lineNum == tb.startLine
				cells := t.markdownTableCellsWithInline(lineNum, m.documentLinkSpans(lineNum, false), inlineChanges)
				m.highlightTableCells(lineRef{line: lineNum}, cells)
				columns = wrapTableCells(cells, tb.colWidths, &t.markdown.after.lines[lineNum-1])
				for row := range tableRowHeight(columns) {
					rows = append(rows, formatTableRow(tableRowCells(columns, row), tb.colWidths, isHeader))
				}
			}

			firstRow := len(layout.rows)
			var wrappedLines []string
			for _, row := range rows {
				if isSelected {
					row = inlineBackground(selectedLineBg, row)
				} else if isSidebarHighlight {
					row = inlineBackground(sidebarHighlightBg, row)
				} else if isChanged && len(inlineChanges) == 0 {
					row = inlineBackground(diffChangedLineBg, row)
				}
				wrappedLines = append(wrappedLines, row)
			}
			for wi, row := range wrappedLines {
				prefix := fmt.Sprintf("%s%s ", marker, numStr)
				if wi > 0 {
					prefix = fmt.Sprintf(" %s ", continuationGutter)
				}
				if columns == nil {
					layout.addTextSpan(len(layout.rows), 0, 0, prefix+row, lineTarget)
					span := &layout.textSpans[len(layout.textSpans)-1]
					span.literal = true
					span.left = tb.viewport.left - tb.viewport.window.offset
					span.clipLeft, span.clipRight = tb.viewport.left, tb.viewport.left+tb.viewport.window.width
				}
				layout.appendBlock(&b, prefix+tb.viewport.render(row), lineTarget)
			}
			if columns != nil {
				layout.addTableSpans(firstRow, columns, tb.colWidths, &t.markdown.after.lines[lineNum-1], lineTarget, tb.viewport)
				if match := m.tableSearchMatch(lineNum, firstRow, columns, tb.colWidths); match != nil {
					tb.viewport.match = match
				}
			}
		} else if formLine, checkboxX, interactive := m.formDisplayLine(lineNum, line, formBackground); interactive {
			if isSelected || isSidebarHighlight {
				formLine = inlineBackground(formBackground, formLine)
			}
			for wi, row := range strings.Split(lipgloss.Wrap(highlight(formLine), textWidth, ""), "\n") {
				if checkboxX >= 0 {
					plain := ansi.Strip(row)
					start := lipgloss.Width(plain[:len(plain)-len(strings.TrimLeft(plain, " \t"))])
					if wi == 0 {
						start = checkboxX
					}
					end := contentWidth - gutterWidth
					if start < end {
						layout.formControls = append(layout.formControls, formControl{line: lineNum,
							rect: mouseRect{left: gutterWidth + start, right: gutterWidth + end, top: len(layout.rows), bottom: len(layout.rows) + 1}})
					}
					row += formBackground.Render(strings.Repeat(" ", max(0, textWidth-lipgloss.Width(row))))
				}
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, row), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, row), lineTarget)
				}
			}
			for _, row := range m.formTextRows(lineNum, textWidth) {
				if item := m.formRows[t.path][lineNum].item; item != nil {
					layout.formControls = append(layout.formControls, formControl{line: item.Line,
						rect: mouseRect{left: gutterWidth, right: contentWidth, top: len(layout.rows), bottom: len(layout.rows) + 1}})
				}
				layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, row), lineTarget)
			}
		} else {
			// Get the display content: Chroma-highlighted or raw
			displayLine := documentDisplayLine(t, i, line)

			if isMarkdown {
				displayLine = t.markdownDisplayLine(lineNum, false, line, nil, m.documentLinkSpans(lineNum, false))
			} else if t.attachment != nil {
				displayLine = linkedDisplayLine(line, m.documentLinkSpans(lineNum, false), displaySourceText)
			}
			displayLine = highlight(displayLine)
			if isSelected {
				displayLine = inlineBackground(selectedLineBg, displayLine)
			} else if isSidebarHighlight {
				displayLine = inlineBackground(sidebarHighlightBg, displayLine)
			} else if isChanged {
				displayLine = inlineBackground(diffChangedLineBg, displayLine)
			}

			wrapped := lipgloss.Wrap(displayLine, textWidth, "")
			wrappedLines := strings.Split(wrapped, "\n")
			for wi, wl := range wrappedLines {
				if wi == 0 {
					layout.appendSourceBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, wl), lineTarget)
				} else {
					layout.appendSourceBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, wl), lineTarget)
				}
			}
		}

		// Render inline annotations after this line
		if images := m.renderMarkdownImages(lineNum, false); images != "" {
			layout.appendBlock(&b, images, lineTarget)
		}
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorSide == "" && t.cursorLine == lineNum && t.cursorAnnoIdx == idx
				appendAnnotation(ann, focused, contentMouseTarget{
					line: lineNum, annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	renderDeleted(t.doc.LineCount())
	for _, table := range tableBlocks {
		layout.tables = append(layout.tables, table.viewport)
	}
	if t.doc.Known != nil {
		layout.appendBlock(&b, "       ⋯ remaining context not included in diff", contentMouseTarget{})
	}

	m.contentLayout = layout
	m.contentViewport.SetContent(m.decorateTextSelection(m.decorateDrifted(b.String(), &m.contentLayout, groups)))
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) (string, commentHeaderRegion) {
	collapsed := m.commentCollapsed(ann.resolved, focused)
	if ann.drifted && m.driftedExpanded(ann.id, ann.resolved) && !m.viewMode {
		collapsed = false
	}
	var lineLabel string
	if isGeneralCommentScope(ann.scope) {
		lineLabel = "File"
		if ann.scope == "review" {
			lineLabel = "Review"
		}
	} else if ann.endLine > ann.line {
		lineLabel = fmt.Sprintf("L%d-%d", ann.line, ann.endLine)
	} else {
		lineLabel = fmt.Sprintf("L%d", ann.line)
	}
	if ann.side == "old" {
		lineLabel += " (deleted)"
	}
	if ann.drifted {
		lineLabel += " (drifted)"
	}

	var boxContent strings.Builder
	label := inlineLabelComment.Render("comment")
	lineRef := commentLineStyle.Foreground(lipgloss.Cyan).Render(lineLabel)
	header := fmt.Sprintf("%s %s", label, lineRef)
	if len(ann.replies) > 0 {
		header += commentLineStyle.Render(" · ") + inlineLabelComment.Bold(false).Render(countNoun(len(ann.replies), "reply", "replies"))
	}
	header, button := renderCommentHeader(header, ann.resolved, m.canDeleteComment(ann.id), focused && !m.readOnly, max(1, maxWidth-4))
	boxContent.WriteString(header)
	if !collapsed {
		boxContent.WriteString("\n")
		boxContent.WriteString(m.renderThread(threadViewKey{id: ann.id}, ann.author, ann.body, ann.replies, max(1, maxWidth-4), focused))
	}
	boxStyle := inlineCommentBox

	if focused {
		boxStyle = boxStyle.BorderForeground(commentFocusedBorderColor)
	}
	box := boxStyle.Width(maxWidth).Render(boxContent.String())

	var prefix string
	if focused {
		cursor := lipgloss.NewStyle().Width(2).Render(cursorMarker.Render(">"))
		prefix = cursor + strings.Repeat(" ", gutterWidth-2)
	} else {
		prefix = strings.Repeat(" ", gutterWidth)
	}

	var b strings.Builder
	for _, line := range strings.Split(box, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	button.translate(gutterWidth+2, 1)
	return b.String(), button
}

// formLineBackground returns the background of form labels, which yields to
// selection and sidebar highlights.
func formLineBackground(selected, highlighted bool) lipgloss.Style {
	switch {
	case selected:
		return selectedLineBg
	case highlighted:
		return sidebarHighlightBg
	}
	return formChip.label
}

// renderResolution renders the resolution checkbox followed by label.  With
// accessKey, the first letter of label is underlined as the toggle key.
func renderResolution(resolved bool, label string, accessKey, focused bool) string {
	checkbox := "☐"
	style := inlineLabelComment.Foreground(lipgloss.Yellow)
	if resolved {
		checkbox = "☑︎"
		style = resolvedBadge
	}
	style = style.Background(buttonBackground)
	if accessKey {
		style = style.Bold(focused)
	}
	switch {
	case label == "":
		return style.Render(checkbox)
	case accessKey:
		return style.Render(checkbox+" ") + renderAccessKey(label, style)
	default:
		return style.Render(checkbox + " " + label)
	}
}

// renderCommentHeader appends the right-aligned action group
// "☐ Resolve  ↑ / ↓  x" to label.  Both resolution states reserve the same
// width, and the x slot stays blank on comments that cannot be deleted, so
// the buttons line up across threads.  When the label row is too narrow,
// the arrows go first; the rest moves to its own row only when it does not
// fit either.
func renderCommentHeader(label string, resolved, deletable, focused bool, width int) (string, commentHeaderRegion) {
	resolveLabel := "Resolve"
	if resolved {
		resolveLabel = "Resolved"
	}
	resolve := renderResolution(resolved, resolveLabel, true, focused)
	resolveWidth := lipgloss.Width("☑︎ Resolved")
	group := func(arrows bool) (string, commentHeaderRegion) {
		actions := lipgloss.NewStyle().Width(resolveWidth).Render(resolve)
		region := commentHeaderRegion{resolve: mouseRect{right: resolveWidth, bottom: 1}}
		end := resolveWidth
		if arrows {
			actions += "  " + commentNavButtonStyle.Render("↑") + commentLineStyle.Render(" / ") + commentNavButtonStyle.Render("↓")
			region.up = mouseRect{left: end + 2, right: end + 4, bottom: 1}
			region.down = mouseRect{left: end + 5, right: end + 7, bottom: 1}
			end += 7
		}
		deleteButton := closeButtonStyle.Render("x")
		if deletable {
			actions += "  " + deleteButton
			region.delete = mouseRect{left: end + 2, right: end + 2 + lipgloss.Width(deleteButton), bottom: 1}
		} else {
			actions += strings.Repeat(" ", 2+lipgloss.Width(deleteButton))
		}
		return actions, region
	}

	header := lipgloss.Wrap(expandDisplayTabs(label), width, "")
	rows := strings.Split(header, "\n")
	last, y := lipgloss.Width(rows[len(rows)-1]), len(rows)-1
	actions, region := group(true)
	if last+1+lipgloss.Width(actions) > width {
		actions, region = group(false)
	}
	if last+1+lipgloss.Width(actions) > width {
		header += "\n"
		last, y = 0, y+1
		if actions, region = group(true); lipgloss.Width(actions) > width {
			actions, region = group(false)
		}
	}
	x := max(0, width-lipgloss.Width(actions))
	header += strings.Repeat(" ", x-last) + actions
	region.translate(x, y)
	return header, region
}

var (
	reListItem   = regexp.MustCompile(`^(\s*[-*+]\s)(.*)$`)
	reCheckbox   = regexp.MustCompile(`^(\s*[-*+]\s)\[([ xX])\]\s(.*)$`)
	reNumList    = regexp.MustCompile(`^(\s*\d+\.\s)(.*)$`)
	reBlockquote = regexp.MustCompile(`^(\s*>\s?)(.*)$`)
	reHr         = regexp.MustCompile(`^(\s*)([-*_]{3,})\s*$`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	reTableSep   = regexp.MustCompile(`^\s*\|[\s:]*[-]+[\s:|-]*\|\s*$`)
)

// highlightMarkdown applies markdown syntax highlighting to a single line.
func highlightMarkdown(line string) string {
	return highlightMarkdownWith(line, highlightInline)
}

func highlightMarkdownBlock(line string) string {
	return highlightMarkdownWith(line, func(s string) string { return s })
}

func highlightMarkdownWith(line string, inline func(string) string) string {
	trimmed := strings.TrimSpace(line)

	if reHr.MatchString(line) {
		return mdHrStyle.Render("─────────────────────────────────")
	}

	if strings.HasPrefix(trimmed, "#### ") {
		return mdH4Style.Render(inline(line))
	}
	if strings.HasPrefix(trimmed, "### ") {
		return mdH3Style.Render(inline(line))
	}
	if strings.HasPrefix(trimmed, "## ") {
		return mdH2Style.Render(inline(line))
	}
	if strings.HasPrefix(trimmed, "# ") {
		return mdH1Style.Render(inline(line))
	}

	if reTableSep.MatchString(line) {
		return mdTableSepStyle.Render(line)
	}
	if reTableRow.MatchString(line) {
		cells := strings.Split(line, "|")
		var parts []string
		for i, cell := range cells {
			if i == 0 || i == len(cells)-1 {
				parts = append(parts, cell)
			} else {
				parts = append(parts, inline(cell))
			}
		}
		return strings.Join(parts, mdTablePipe.Render("|"))
	}

	if loc := reBlockquote.FindStringSubmatchIndex(line); loc != nil {
		rest := line[loc[4]:loc[5]]
		return mdBlockquoteBar.Render("▎") + " " + mdBlockquoteStyle.Render(inline(rest))
	}

	if loc := reCheckbox.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		checked := line[loc[4]:loc[5]]
		rest := line[loc[6]:loc[7]]
		if checked == "x" || checked == "X" {
			return indent + mdCheckboxDone.Render("✓") + " " + mdCheckboxDoneText.Render(inline(rest))
		}
		return indent + mdCheckboxOpen.Render("☐") + " " + inline(rest)
	}

	if loc := reListItem.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(indent) + inline(rest)
	}
	if loc := reNumList.FindStringSubmatchIndex(line); loc != nil {
		marker := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(marker) + inline(rest)
	}

	return inline(line)
}

func inlineBackground(style lipgloss.Style, content string) string {
	return inlineColors(style, content, false)
}

// inlineColors applies style's background, and its foreground when
// foreground is true, across content, restoring them after inner resets.
func inlineColors(style lipgloss.Style, content string, foreground bool) string {
	bgAnsi := bgToAnsi(style.GetBackground())
	fgAnsi := ""
	if foreground {
		fgAnsi = fgToAnsi(style.GetForeground())
	}
	if bgAnsi == "" && fgAnsi == "" {
		return style.Render(content)
	}
	var patched strings.Builder
	patched.Grow(len(content) + len(bgAnsi) + len(fgAnsi))
	reapplyBg, reapplyFg := false, false
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	parser.SetHandler(ansi.Handler{HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
		if cmd != 'm' {
			return
		}
		if len(params) == 0 {
			reapplyBg, reapplyFg = true, true
		}
		params.ForEach(0, func(_ int, param int, _ bool) {
			switch {
			case param == 0:
				reapplyBg, reapplyFg = true, true
			case param == 49:
				reapplyBg = true
			case param == 39:
				reapplyFg = true
			case param == 48, param >= 40 && param <= 47, param >= 100 && param <= 107:
				reapplyBg = false
			case param == 38, param >= 30 && param <= 37, param >= 90 && param <= 97:
				reapplyFg = false
			}
		})
	}})
	for i := range len(content) {
		parser.Advance(content[i])
		patched.WriteByte(content[i])
		if reapplyBg {
			patched.WriteString(bgAnsi)
			reapplyBg = false
		}
		if reapplyFg {
			patched.WriteString(fgAnsi)
			reapplyFg = false
		}
	}
	return bgAnsi + fgAnsi + patched.String() + "\033[0m"
}

// inlineDiffDisplayLines renders an inline diff wrapped to width.  decorate,
// when not nil, highlights search matches before wrapping.
func inlineDiffDisplayLines(segments []gitpkg.InlineSegment, isMarkdown bool, width int, base, changed lipgloss.Style, decorate func(string) (string, bool)) []string {
	if isMarkdown {
		var source strings.Builder
		for _, segment := range segments {
			source.WriteString(segment.Content)
		}
		parsed := parseMarkdownSource([]string{source.String()}, nil)
		return markdownDisplayRows(parsed.lines[0].render(segments, base, changed, nil), width, decorate)
	}
	var b strings.Builder
	for _, segment := range segments {
		content := displaySourceText(segment.Content)
		style := base
		if segment.Changed {
			style = changed
		}
		b.WriteString(inlineBackground(style, content))
	}
	line := b.String()
	if decorate != nil {
		line, _ = decorate(line)
	}
	return strings.Split(lipgloss.Wrap(line, width, ""), "\n")
}

func deletedDisplayLines(content, cached string, inline []gitpkg.InlineSegment, isMarkdown bool, width int, decorate func(string) (string, bool)) []string {
	if len(inline) > 0 {
		return inlineDiffDisplayLines(inline, isMarkdown, width, diffCommonTextBg, diffDeletedTextBg, decorate)
	}
	display := displaySourceText(content)
	if cached != "" {
		display = displaySourceText(cached)
	}
	if isMarkdown {
		display = highlightMarkdown(display)
	}
	if decorate != nil {
		display, _ = decorate(display)
	}
	lines := strings.Split(lipgloss.Wrap(inlineBackground(diffDeletedLineBg, display), width, ""), "\n")
	return lines
}

// tableBlock represents a contiguous range of markdown table lines.
type tableBlock struct {
	startLine int
	endLine   int
	colWidths []int
	viewport  tableViewport
}

func detectTableBlocks(lines []markdownLine, renderedCells func(int) []string) []tableBlock {
	var blocks []tableBlock
	inTable := false
	var current tableBlock

	for i, line := range lines {
		cells := renderedCells(i + 1)
		isTable := len(cells) > 0 || line.tableSep
		if isTable {
			if !inTable {
				inTable = true
				current = tableBlock{startLine: i + 1}
			}
			current.endLine = i + 1

			if !line.tableSep {
				for len(current.colWidths) < len(cells) {
					current.colWidths = append(current.colWidths, 0)
				}
				for ci, cell := range cells {
					if width := lipgloss.Width(cell); width > current.colWidths[ci] {
						current.colWidths[ci] = width
					}
				}
			}
		} else {
			if inTable {
				blocks = append(blocks, current)
				inTable = false
			}
		}
	}
	if inTable {
		blocks = append(blocks, current)
	}
	return blocks
}

func formatTableRow(cells []string, colWidths []int, isHeader bool) string {
	pipe := mdTablePipe.Render("│")

	var parts []string
	for ci := 0; ci < len(colWidths); ci++ {
		w := colWidths[ci]
		cell := ""
		if ci < len(cells) {
			cell = cells[ci]
		}
		padded := lipgloss.NewStyle().Width(w).Render(cell)
		if isHeader {
			parts = append(parts, mdTableHeaderStyle.Render(" "+padded+" "))
		} else {
			parts = append(parts, mdTableCellStyle.Render(" "+padded+" "))
		}
	}

	return pipe + strings.Join(parts, pipe) + pipe
}

func formatTableSep(colWidths []int) string {
	pipe := mdTablePipe.Render("│")
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, mdTableSepStyle.Render(strings.Repeat("─", w+2)))
	}
	return pipe + strings.Join(parts, mdTablePipe.Render("┼")) + pipe
}

func highlightInline(line string) string {
	return highlightInlineWith(line, mdLinkStyle, mdCodeStyle)
}
