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

func documentDisplayLine(t *FileTab, index int, fallback string) string {
	if !t.isMarkdown && t.chromaLines != nil && index < len(t.chromaLines) {
		return expandDisplayTabs(t.chromaLines[index])
	}
	return expandDisplayTabs(fallback)
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	if len(m.tabs) == 0 {
		m.contentViewport.SetContent("")
		return
	}
	t := m.tab()
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
	tableBlocks := detectTableBlocks(sourceLines)
	tableBlockMap := make(map[int]*tableBlock)
	for i := range tableBlocks {
		tb := &tableBlocks[i]
		for l := tb.startLine; l <= tb.endLine; l++ {
			tableBlockMap[l] = tb
		}
	}

	var b strings.Builder
	b.Grow(len(sourceLines) * 200) // pre-allocate to reduce allocations
	layout := newRenderedContentLayout()
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
		if !m.hideComments || ann.drifted {
			focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == 0 && t.cursorAnnoIdx == idx
			appendAnnotation(ann, focused, contentMouseTarget{annotation: true, annotationIndex: idx})
		}
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
		m.contentViewport.SetContent(m.decorateDrifted(b.String(), &m.contentLayout, groups))
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
			for wi, content := range deletedDisplayLines(del.Content, cached, del.Inline, isMarkdown, textWidth, decorate) {
				if isSelected {
					content = inlineBackground(selectedLineBg, ansi.Strip(content))
				} else if isSidebarHighlight {
					content = inlineBackground(sidebarHighlightBg, ansi.Strip(content))
				}
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, num, content), target)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, content), target)
				}
			}
			if images := m.renderMarkdownImages(del.OldLineNum, true); images != "" {
				layout.appendBlock(&b, images, target)
			}
			for idx, ann := range oldAnnosByEndLine[del.OldLineNum] {
				if m.hideComments && !ann.drifted {
					continue
				}
				focused := m.focused == contentPane && t.cursorOnAnnotation && isCursor && t.cursorAnnoIdx == idx
				appendAnnotation(ann, focused, contentMouseTarget{
					line: del.OldLineNum, side: "old", annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	for i, line := range t.doc.Lines {
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
		var numStr string
		if isCursor {
			numStr = cursorLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else if isSelected {
			numStr = selectedLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else {
			numStr = lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		}

		// Check if this line is part of a table block
		if len(inlineChanges) > 0 && !isSelected && !isSidebarHighlight {
			for wi, styledLine := range inlineDiffDisplayLines(inlineChanges, isMarkdown, textWidth, diffCommonTextBg, diffAddedTextBg, decorate) {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, styledLine), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, styledLine), lineTarget)
				}
			}
		} else if tb, inTable := tableBlockMap[lineNum]; inTable {
			var styledLine string
			if reTableSep.MatchString(line) {
				styledLine = formatTableSep(tb.colWidths)
			} else {
				isHeader := lineNum == tb.startLine
				styledLine = formatTableRow(line, tb.colWidths, isHeader)
			}

			if isSelected {
				styledLine = inlineBackground(selectedLineBg, styledLine)
			} else if isSidebarHighlight {
				styledLine = inlineBackground(sidebarHighlightBg, styledLine)
			} else if isChanged {
				styledLine = inlineBackground(diffChangedLineBg, styledLine)
			}

			wrappedLines := strings.Split(lipgloss.Wrap(highlight(expandDisplayTabs(styledLine)), textWidth, ""), "\n")
			for wi, wrappedLine := range wrappedLines {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, wrappedLine), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, wrappedLine), lineTarget)
				}
			}
		} else if formLine, checkboxX, interactive := m.formDisplayLine(lineNum, line); interactive {
			if isSelected {
				formLine = inlineBackground(selectedLineBg, formLine)
			} else if isSidebarHighlight {
				formLine = inlineBackground(sidebarHighlightBg, formLine)
			}
			for wi, row := range strings.Split(lipgloss.Wrap(highlight(formLine), textWidth, ""), "\n") {
				if checkboxX >= 0 {
					plain := ansi.Strip(row)
					start := lipgloss.Width(plain[:len(plain)-len(strings.TrimLeft(plain, " \t"))])
					if wi == 0 {
						start = checkboxX
					}
					end := lipgloss.Width(strings.TrimRight(plain, " \t"))
					if start < end {
						layout.formControls = append(layout.formControls, formControl{line: lineNum,
							rect: mouseRect{left: gutterWidth + start, right: gutterWidth + end, top: len(layout.rows), bottom: len(layout.rows) + 1}})
					}
				}
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, row), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, row), lineTarget)
				}
			}
		} else {
			// Get the display content: Chroma-highlighted or raw
			displayLine := documentDisplayLine(t, i, line)

			styleFunc := func(s string) string { return s }
			if isMarkdown {
				styleFunc = func(s string) string { return highlightMarkdown(s) }
			}
			// Highlight search matches in the styled line before wrapping, so a
			// matched Markdown line is styled whole instead of row by row.
			if decorate != nil {
				if decorated, ok := decorate(styleFunc(displayLine)); ok {
					displayLine = decorated
					styleFunc = func(s string) string { return s }
				}
			}
			if isSelected {
				styleFunc = func(s string) string { return inlineBackground(selectedLineBg, s) }
			} else if isSidebarHighlight {
				styleFunc = func(s string) string { return inlineBackground(sidebarHighlightBg, s) }
			} else if isChanged {
				base := styleFunc
				styleFunc = func(s string) string { return inlineBackground(diffChangedLineBg, base(s)) }
			}

			wrapped := lipgloss.Wrap(displayLine, textWidth, "")
			wrappedLines := strings.Split(wrapped, "\n")
			for wi, wl := range wrappedLines {
				if wi == 0 {
					layout.appendBlock(&b, fmt.Sprintf("%s%s %s", marker, numStr, styleFunc(wl)), lineTarget)
				} else {
					layout.appendBlock(&b, fmt.Sprintf(" %s %s", continuationGutter, styleFunc(wl)), lineTarget)
				}
			}
		}

		// Render inline annotations after this line
		if images := m.renderMarkdownImages(lineNum, false); images != "" {
			layout.appendBlock(&b, images, lineTarget)
		}
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				if m.hideComments && !ann.drifted {
					continue
				}
				focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorSide == "" && t.cursorLine == lineNum && t.cursorAnnoIdx == idx
				appendAnnotation(ann, focused, contentMouseTarget{
					line: lineNum, annotation: true, annotationIndex: idx,
				})
			}
		}
	}
	renderDeleted(t.doc.LineCount())
	if t.doc.Known != nil {
		layout.appendBlock(&b, "       ⋯ remaining context not included in diff", contentMouseTarget{})
	}

	m.contentLayout = layout
	m.contentViewport.SetContent(m.decorateDrifted(b.String(), &m.contentLayout, groups))
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) (string, commentHeaderRegion) {
	collapsed := m.collapseResolved(ann.resolved, ann.resolvedRound) && !focused
	if ann.drifted && m.tab().expandedDrifted[ann.id] && !m.foldResolved {
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
	header, button := renderCommentHeader(header, ann.resolved, m.canDeleteComment(ann.id), max(1, maxWidth-4))
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

func (m *AppModel) collapseResolved(resolved bool, round int) bool {
	return resolved && (m.foldResolved || round == 0 || round < m.reviewRound())
}

func renderResolution(resolved bool, label string) string {
	checkbox := "☐"
	style := inlineLabelComment.Foreground(lipgloss.Yellow)
	if resolved {
		checkbox = "☑︎"
		style = resolvedBadge
	}
	if label != "" {
		checkbox += " " + label
	}
	return style.Background(buttonBackground).Render(checkbox)
}

// renderCommentHeader appends the right-aligned action group
// "☐ Resolve  ↑ / ↓  x" to label.  Both resolution states reserve the same
// width, and the x slot stays blank on comments that cannot be deleted, so
// the buttons line up across threads.  When the label row is too narrow,
// the arrows go first; the rest moves to its own row only when it does not
// fit either.
func renderCommentHeader(label string, resolved, deletable bool, width int) (string, commentHeaderRegion) {
	resolveLabel := "Resolve"
	if resolved {
		resolveLabel = "Resolved"
	}
	resolve := renderResolution(resolved, resolveLabel)
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
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reCode       = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
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
	trimmed := strings.TrimSpace(line)

	if reHr.MatchString(line) {
		return mdHrStyle.Render("─────────────────────────────────")
	}

	if strings.HasPrefix(trimmed, "#### ") {
		return mdH4Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") {
		return mdH3Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return mdH2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "# ") {
		return mdH1Style.Render(line)
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
				parts = append(parts, highlightInline(cell))
			}
		}
		return strings.Join(parts, mdTablePipe.Render("|"))
	}

	if loc := reBlockquote.FindStringSubmatchIndex(line); loc != nil {
		rest := line[loc[4]:loc[5]]
		return mdBlockquoteBar.Render("▎") + " " + mdBlockquoteStyle.Render(rest)
	}

	if loc := reCheckbox.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		checked := line[loc[4]:loc[5]]
		rest := line[loc[6]:loc[7]]
		if checked == "x" || checked == "X" {
			return indent + mdCheckboxDone.Render("✓") + " " + mdCheckboxDoneText.Render(rest)
		}
		return indent + mdCheckboxOpen.Render("☐") + " " + highlightInline(rest)
	}

	if loc := reListItem.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(indent) + highlightInline(rest)
	}
	if loc := reNumList.FindStringSubmatchIndex(line); loc != nil {
		marker := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	return highlightInline(line)
}

func inlineBackground(style lipgloss.Style, content string) string {
	bgAnsi := bgToAnsi(style.GetBackground())
	if bgAnsi == "" {
		return style.Render(content)
	}
	var patched strings.Builder
	patched.Grow(len(content) + len(bgAnsi))
	reapply := false
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	parser.SetHandler(ansi.Handler{HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
		if cmd != 'm' {
			return
		}
		reapply = len(params) == 0
		params.ForEach(0, func(_ int, param int, _ bool) {
			switch {
			case param == 0 || param == 49:
				reapply = true
			case param == 48, param >= 40 && param <= 47, param >= 100 && param <= 107:
				reapply = false
			}
		})
	}})
	for i := range len(content) {
		parser.Advance(content[i])
		patched.WriteByte(content[i])
		if reapply {
			patched.WriteString(bgAnsi)
			reapply = false
		}
	}
	return bgAnsi + patched.String() + "\033[0m"
}

// inlineDiffDisplayLines renders an inline diff wrapped to width.  decorate,
// when not nil, highlights search matches before wrapping.
func inlineDiffDisplayLines(segments []gitpkg.InlineSegment, isMarkdown bool, width int, base, changed lipgloss.Style, decorate func(string) (string, bool)) []string {
	var b strings.Builder
	for _, segment := range segments {
		content := segment.Content
		if isMarkdown {
			content = highlightMarkdown(content)
		}
		style := base
		if segment.Changed {
			style = changed
		}
		b.WriteString(inlineBackground(style, content))
	}
	line := expandDisplayTabs(b.String())
	if decorate != nil {
		line, _ = decorate(line)
	}
	return strings.Split(lipgloss.Wrap(line, width, ""), "\n")
}

func deletedDisplayLines(content, cached string, inline []gitpkg.InlineSegment, isMarkdown bool, width int, decorate func(string) (string, bool)) []string {
	if len(inline) > 0 {
		return inlineDiffDisplayLines(inline, isMarkdown, width, diffCommonTextBg, diffDeletedTextBg, decorate)
	}
	display := expandDisplayTabs(content)
	if cached != "" {
		display = expandDisplayTabs(cached)
	}
	if decorate != nil {
		styled := display
		if isMarkdown {
			styled = highlightMarkdown(styled)
		}
		if decorated, ok := decorate(styled); ok {
			lines := strings.Split(lipgloss.Wrap(decorated, width, ""), "\n")
			for i := range lines {
				lines[i] = inlineBackground(diffDeletedLineBg, lines[i])
			}
			return lines
		}
	}
	lines := strings.Split(lipgloss.Wrap(display, width, ""), "\n")
	for i := range lines {
		if isMarkdown {
			lines[i] = highlightMarkdown(lines[i])
		}
		lines[i] = inlineBackground(diffDeletedLineBg, lines[i])
	}
	return lines
}

// tableBlock represents a contiguous range of markdown table lines.
type tableBlock struct {
	startLine int
	endLine   int
	colWidths []int
}

func detectTableBlocks(lines []string) []tableBlock {
	var blocks []tableBlock
	inTable := false
	var current tableBlock

	for i, line := range lines {
		isTable := reTableRow.MatchString(line) || reTableSep.MatchString(line)
		if isTable {
			if !inTable {
				inTable = true
				current = tableBlock{startLine: i + 1}
			}
			current.endLine = i + 1

			if !reTableSep.MatchString(line) {
				cells := parseTableCells(line)
				for len(current.colWidths) < len(cells) {
					current.colWidths = append(current.colWidths, 0)
				}
				for ci, cell := range cells {
					if len(cell) > current.colWidths[ci] {
						current.colWidths[ci] = len(cell)
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

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func formatTableRow(line string, colWidths []int, isHeader bool) string {
	cells := parseTableCells(line)
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
	// Render links before injecting any ANSI styles. CSI sequences also start
	// with "[", so running the Markdown link regexp afterward can mistake an
	// escape sequence followed by a link for part of the link label.
	line = reLink.ReplaceAllStringFunc(line, func(match string) string {
		idx := strings.Index(match, "](")
		if idx < 0 {
			return match
		}
		text := match[1:idx]
		return mdLinkStyle.Render(text)
	})

	line = reCode.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[1 : len(match)-1]
		return mdCodeStyle.Render(" " + inner + " ")
	})

	line = reBold.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[2 : len(match)-2]
		return mdBoldStyle.Render(inner)
	})

	line = reItalic.ReplaceAllStringFunc(line, func(match string) string {
		start := 0
		end := len(match)
		if match[0] != '*' {
			start = 1
		}
		if match[end-1] != '*' {
			end--
		}
		inner := match[start+1 : end-1]
		prefix := match[:start]
		suffix := match[end:]
		return prefix + mdItalicStyle.Render(inner) + suffix
	})

	return line
}
