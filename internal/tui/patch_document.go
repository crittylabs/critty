package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/crittylabs/critty/v2/internal/document"
)

type patchDocumentRow struct {
	left, right int
	full        bool
}

type patchDocumentCache struct {
	doc     *document.Document
	content string
	rows    []patchDocumentRow
	hunks   []patchDocumentHunk
	before  map[int]string
	kinds   map[int]gitdiff.LineOp
}

type patchDocumentHunk struct {
	oldName, newName string
	lines            []patchDocumentSourceLine
}

type patchDocumentSourceLine struct {
	line int
	op   gitdiff.LineOp
}

// Split projections always refer to the patch document's own source lines.
// A Git change to a patch file keeps the existing outer diff renderer.
func (m *AppModel) documentPatch(t *FileTab) *patchDocumentCache {
	if m.multiFile && t.attachment == nil && !t.viewDocument {
		return nil
	}
	return t.documentPatch()
}

func (t *FileTab) documentPatch() *patchDocumentCache {
	if t.doc == nil || t.doc.Known != nil || t.diff != nil || t.isBinary ||
		t.lexer == nil || !strings.EqualFold(t.lexer.Config().Name, "diff") {
		return nil
	}
	if t.patchDocument == nil || t.patchDocument.doc != t.doc || t.patchDocument.content != t.doc.Content {
		t.patchDocument = &patchDocumentCache{}
		if len(t.doc.Content) <= 64<<20 {
			if parsed := parsePatchDocument(t.doc.Lines, t.doc.Content); parsed != nil {
				t.patchDocument = parsed
			}
		}
		t.patchDocument.doc, t.patchDocument.content = t.doc, t.doc.Content
	}
	if len(t.patchDocument.rows) == 0 {
		return nil
	}
	return t.patchDocument
}

func (m *AppModel) splitPatchDocument(t *FileTab) bool {
	return m.split && m.contentViewport.Width() >= 2*diffColumnMinimum+3 && m.documentPatch(t) != nil
}

// The parser validates the grammar. Match each validated fragment back to its
// literal source, refusing ambiguous extra headers rather than guessing anchors.
func parsePatchDocument(lines []string, source string) *patchDocumentCache {
	files, _, err := gitdiff.Parse(strings.NewReader(source))
	if err != nil {
		return nil
	}
	type fragment struct {
		file *gitdiff.File
		text *gitdiff.TextFragment
	}
	var fragments []fragment
	for _, file := range files {
		for _, text := range file.TextFragments {
			fragments = append(fragments, fragment{file, text})
		}
	}
	var headers []int
	for i, line := range lines {
		if strings.HasPrefix(line, "@@ -") {
			headers = append(headers, i)
		}
	}
	if len(fragments) == 0 || len(headers) != len(fragments) {
		return nil
	}
	kinds := make(map[int]gitdiff.LineOp)
	var hunks []patchDocumentHunk
	for i, fragment := range fragments {
		index := headers[i] + 1
		hunk := patchDocumentHunk{oldName: fragment.file.OldName, newName: fragment.file.NewName}
		for _, line := range fragment.text.Lines {
			if index >= len(lines) {
				return nil
			}
			expected := strings.TrimSuffix(line.String(), "\n")
			if lines[index] != expected && !(line.Op == gitdiff.OpContext && lines[index] == "" && expected == " ") {
				return nil
			}
			kinds[index+1] = line.Op
			hunk.lines = append(hunk.lines, patchDocumentSourceLine{index + 1, line.Op})
			index++
			if index < len(lines) && strings.HasPrefix(lines[index], "\\ ") {
				index++
			}
		}
		hunks = append(hunks, hunk)
	}
	var rows []patchDocumentRow
	for n := 1; n <= len(lines); {
		kind, body := kinds[n]
		if !body {
			rows = append(rows, patchDocumentRow{left: n, full: true})
			n++
			continue
		}
		if kind == gitdiff.OpContext {
			rows = append(rows, patchDocumentRow{left: n, right: n})
			n++
			continue
		}
		var before, after []*diffLine
		for n <= len(lines) {
			kind, body = kinds[n]
			if !body || kind == gitdiff.OpContext {
				break
			}
			line := &diffLine{ref: lineRef{line: n}, text: lines[n-1][1:]}
			if kind == gitdiff.OpDelete {
				before = append(before, line)
			} else {
				after = append(after, line)
			}
			n++
		}
		for _, row := range alignDiffRows(before, after, nil) {
			pair := patchDocumentRow{}
			if row.before != nil {
				pair.left = row.before.ref.line
			}
			if row.after != nil {
				pair.right = row.after.ref.line
			}
			rows = append(rows, pair)
		}
	}
	return &patchDocumentCache{rows: rows, hunks: hunks, kinds: kinds}
}

func (m *AppModel) rebuildPatchDocument(patch *patchDocumentCache) {
	t := m.tab()
	layout := newRenderedContentLayout()
	layout.gutter = m.sourceGutterWidth()
	var b strings.Builder
	width := m.contentViewport.Width()
	groups := m.commentMarkerGroups()
	for _, ids := range groups {
		width = min(width, m.contentViewport.Width()-lipgloss.Width(fmt.Sprintf("> ☑︎ 💬 %d", len(ids)))-1)
	}
	gutter := m.diffGutter(t)
	cell := func(n, width int, before bool) ([]diffCellRow, contentMouseTarget) {
		if n == 0 {
			return nil, contentMouseTarget{}
		}
		line := &diffLine{ref: lineRef{line: n}, text: t.doc.LineAt(n), styled: t.patchDisplayLine(n, documentDisplayLine(t, n-1, t.doc.LineAt(n)))}
		if styled, ok := patch.before[n]; before && ok {
			line.styled = t.patchDisplayLine(n, styled)
		}
		return m.diffCell(line, width, false, gutter)
	}
	annotations := func(n int) {
		for i, ann := range m.annotationsAfterLine(n, "") {
			focused := m.focused == contentPane && t.cursorOnAnnotation && t.cursorLine == n && t.cursorAnnoIdx == i
			if m.viewMode || ann.drifted && m.threadCollapsed(ann.id, ann.resolved, true, focused) {
				continue
			}
			box, button := m.renderAnnotationBox(ann, max(1, width-m.sourceGutterWidth()), focused)
			button.translate(0, len(layout.rows))
			button.id = ann.id
			layout.actions = append(layout.actions, button)
			layout.appendBlock(&b, box, contentMouseTarget{line: n, annotation: true, annotationIndex: i})
		}
	}
	annotations(0)
	sourceLines := t.doc.SourceLineCount()
	for _, row := range patch.rows {
		if row.full {
			if row.left > sourceLines {
				layout.appendBlock(&b, "", contentMouseTarget{line: row.left, endOfFile: true})
			} else {
				cells, target := cell(row.left, width, false)
				for _, c := range cells {
					layout.addCellSpan(len(layout.rows), 0, 0, c, target)
					layout.appendBlock(&b, c.text, target)
				}
			}
		} else {
			leftWidth := max(1, (width-3)/2)
			rightWidth := max(1, width-3-leftWidth)
			left, lt := cell(row.left, leftWidth, true)
			right, rt := cell(row.right, rightWidth, false)
			for i := 0; i < max(len(left), len(right)); i++ {
				var l, r diffCellRow
				if i < len(left) {
					l = left[i]
				}
				if i < len(right) {
					r = right[i]
				}
				index := len(layout.rows)
				layout.addCellSpan(index, 0, 0, l, lt)
				layout.addCellSpan(index, leftWidth+3, 1, r, rt)
				target := rt
				if target.line == 0 {
					target = lt
				}
				layout.appendBlock(&b, l.fit(leftWidth)+" │ "+r.fit(rightWidth), target)
				for len(layout.columns) <= index {
					layout.columns = append(layout.columns, diffMouseRow{})
				}
				layout.columns[index] = diffMouseRow{left: lt, right: rt, split: leftWidth}
				if row.left > 0 && row.left != row.right {
					updateDiffRange(layout.lineRanges, row.left, index)
				}
			}
		}
		if row.left > 0 {
			annotations(row.left)
		}
		if row.right > 0 && row.right != row.left {
			annotations(row.right)
		}
	}
	m.contentLayout = layout
	m.setRenderedSourceContent(b.String(), width, groups)
}

func (m *AppModel) movePatchDocumentSide(before bool) {
	t := m.tab()
	if t.selecting {
		return
	}
	for _, row := range m.documentPatch(t).rows {
		if row.full || row.left != t.cursorLine && row.right != t.cursorLine {
			continue
		}
		n := row.right
		if before {
			n = row.left
		}
		if n == 0 {
			return
		}
		t.cursorLine, t.cursorSide = n, ""
		t.cursorOnAnnotation, t.cursorOnForm = false, false
		m.rebuildContent()
		m.scrollToCursor()
		return
	}
}
