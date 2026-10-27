package tui

import (
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/v2/ast"
)

type outlineHeading struct {
	line, level int
	label       string
}

type documentOutline struct {
	rows       []outlineHeading
	source     any
	active     int
	focused    bool
	dragging   bool
	proportion int // requested percentage, zero means automatic
	viewport   viewport.Model
}

func newDocumentOutline() documentOutline {
	return documentOutline{active: -1, viewport: viewport.New()}
}

func outlineHeadingLabel(n ast.Node, source []byte) string {
	var b strings.Builder
	var visit func(ast.Node)
	visit = func(n ast.Node) {
		switch n := n.(type) {
		case *ast.Text:
			b.WriteString(n.Value.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
			return
		case *ast.CodeSpan:
			b.WriteString(n.Value.Value(source))
			return
		case *ast.AutoLink:
			b.WriteString(n.Label.Value(source))
			return
		case *ast.RawHTML:
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(n)
	label := strings.Join(strings.Fields(ansi.Strip(b.String())), " ")
	if label == "" {
		return "(untitled)"
	}
	return label
}

// The title and heading rows share at most two thirds of the Files body.
// Short outlines give unused space back to the file tree.
func (m *AppModel) layoutFilesSidebar() {
	height := m.commentViewport.Height()
	size := 0
	if len(m.outline.rows) > 0 && height >= 6 {
		requested := height / 2
		if m.outline.proportion > 0 {
			requested = height * m.outline.proportion / 100
		}
		size = min(len(m.outline.rows)+1, max(2, min(requested, height*2/3)))
	}
	m.fileTree.viewport.SetWidth(m.commentViewport.Width())
	m.fileTree.viewport.SetHeight(height - size)
	m.outline.viewport.SetWidth(m.commentViewport.Width())
	m.outline.viewport.SetHeight(max(0, size-1))
	if size == 0 {
		m.outline.focused = false
		m.outline.dragging = false
	}
}

func (m *AppModel) syncOutline() {
	o := &m.outline
	var source any
	var rows []outlineHeading
	if len(m.tabs) > 0 {
		t := m.tab()
		if t.isMarkdown && t.doc != nil {
			t.ensureMarkdownCache()
			source, rows = t.markdown.after, t.markdown.after.headings
		} else if t.doc != nil && strings.EqualFold(filepath.Ext(t.path), ".go") {
			cache := t.ensureGoOutline()
			source, rows = cache, cache.rows
		} else if t.doc != nil && !t.isBinary {
			if cache := t.ensureCodeOutline(); cache != nil {
				source, rows = cache, cache.rows
			}
		}
	}
	changed := source != o.source
	previous := o.active
	previousHeight := o.viewport.Height()
	o.source, o.rows, o.active = source, rows, -1
	if source != nil {
		line := m.tab().cursorLine
		if m.tab().cursorSide == "old" {
			line = m.outlineNewLine(line)
		}
		o.active = sort.Search(len(o.rows), func(i int) bool { return o.rows[i].line > line }) - 1
	}
	if changed {
		o.viewport.SetYOffset(0)
	}
	m.layoutFilesSidebar()
	m.renderOutline()
	if changed || previous != o.active || previousHeight != o.viewport.Height() {
		m.revealOutlineHeading()
		m.scrollToTreeCursor()
	}
}

// Deleted lines belong to the section at their insertion point on the new side.
func (m *AppModel) outlineNewLine(old int) int {
	for after, lines := range m.tab().deletedAfter {
		for _, line := range lines {
			if line.OldLineNum == old {
				return after
			}
		}
	}
	return 0
}

func (m *AppModel) renderOutline() {
	o := &m.outline
	var levels []int
	lines := make([]string, len(o.rows))
	for i, heading := range o.rows {
		for len(levels) > 0 && levels[len(levels)-1] >= heading.level {
			levels = levels[:len(levels)-1]
		}
		depth := len(levels)
		levels = append(levels, heading.level)
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(textSecondary)
		if i == o.active {
			style = style.Foreground(textPrimary).Bold(true)
			if o.focused && m.focused == commentPane {
				prefix = cursorMarker.Render(">") + " "
			}
		}
		line := prefix + style.Render(strings.Repeat(" ", depth*treeIndent)+heading.label)
		line = ansi.Truncate(line, o.viewport.Width(), "…")
		if i == o.active {
			padding := strings.Repeat(" ", max(0, o.viewport.Width()-ansi.StringWidth(line)))
			line += lipgloss.NewStyle().Foreground(buttonBackground).Background(buttonBackground).Reverse(true).Render(padding)
			line = inlineBackground(lipgloss.NewStyle().Background(buttonBackground), line)
		}
		lines[i] = line
	}
	o.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *AppModel) revealOutlineHeading() {
	o := &m.outline
	if o.active < o.viewport.YOffset() {
		o.viewport.SetYOffset(max(0, o.active))
	} else if o.active >= o.viewport.YOffset()+o.viewport.Height() {
		o.viewport.SetYOffset(o.active - o.viewport.Height() + 1)
	}
}

func (m AppModel) renderFilesSidebar() string {
	view := m.fileTree.viewport.View()
	if m.outline.viewport.Height() == 0 {
		return view
	}
	title := "─ Outline "
	title += strings.Repeat("─", max(0, m.outline.viewport.Width()-ansi.StringWidth(title)))
	title = ansi.Truncate(title, m.outline.viewport.Width(), "")
	return view + "\n" + lipgloss.NewStyle().Foreground(m.sidebarBorderColor()).Render(title) + "\n" + m.outline.viewport.View()
}

func (m *AppModel) jumpOutline(i int) {
	o := &m.outline
	if i < 0 || i >= len(o.rows) {
		return
	}
	t := m.tab()
	t.cursorLine, t.cursorSide = o.rows[i].line, ""
	t.cursorOnAnnotation, t.cursorOnForm, t.selecting = false, false, false
	m.clearTextSelection()
	m.updateCommentSidebar()
	m.rebuildContent()
	m.revealOutlineHeading()
	m.scrollToCursor()
}

func (m AppModel) outlineHint() string {
	if len(m.outline.rows) == 0 {
		return ""
	}
	if m.focused == contentPane {
		return footerKeyStyle.Render("M-G") + " " + footerDescStyle.Render("outline")
	}
	if m.sidebarView == filesView {
		return footerKeyStyle.Render("C-↑/↓") + " " + footerDescStyle.Render("⇅ tree/outline")
	}
	return ""
}

func (m *AppModel) focusOutline() {
	m.syncOutline()
	if len(m.outline.rows) == 0 {
		return
	}
	m.viewMode = false
	m.sidebarView = filesView
	m.recalculateLayout()
	m.updateCommentSidebar()
	if m.outline.viewport.Height() > 0 {
		m.focused = commentPane
		m.outline.focused = true
	}
	m.rebuildContent()
	m.renderFileTree()
	m.renderOutline()
	m.revealOutlineHeading()
}

func (m *AppModel) handleFilesSidebarKey(msg tea.KeyPressMsg) bool {
	o := &m.outline
	if o.viewport.Height() > 0 {
		switch msg.String() {
		case "ctrl+down", "ctrl+up":
			o.focused = msg.String() == "ctrl+down"
			m.renderOutline()
			m.renderFileTree()
			m.revealOutlineHeading()
			return true
		case "ctrl+shift+up", "ctrl+shift+down":
			delta := 1
			if msg.String() == "ctrl+shift+down" {
				delta = -1
			}
			m.setOutlineHeight(o.viewport.Height() + 1 + delta)
			return true
		}
	}
	if !o.focused {
		return m.handleFileTreeKey(msg)
	}
	next := o.active
	switch {
	case key.Matches(msg, keys.Down):
		next++
	case key.Matches(msg, keys.Up):
		next--
	case key.Matches(msg, keys.Top):
		next = 0
	case key.Matches(msg, keys.Bottom):
		next = len(o.rows) - 1
	case key.Matches(msg, keys.HalfPageDown):
		next += max(1, o.viewport.Height()/2)
	case key.Matches(msg, keys.HalfPageUp):
		next -= max(1, o.viewport.Height()/2)
	case key.Matches(msg, keys.Confirm, keys.Cancel):
		m.focused = contentPane
		m.updateCommentSidebar()
		m.rebuildContent()
		return true
	case msg.String() == "h" || msg.String() == "left":
		if next >= 0 {
			for i := next - 1; i >= 0; i-- {
				if o.rows[i].level < o.rows[next].level {
					next = i
					break
				}
			}
		}
	case msg.String() == "l" || msg.String() == "right":
		if next >= 0 && next+1 < len(o.rows) && o.rows[next+1].level > o.rows[next].level {
			next++
		}
	default:
		return false
	}
	m.jumpOutline(max(0, min(next, len(o.rows)-1)))
	return true
}

func (m *AppModel) handleFilesSidebarClick(row int) {
	o := &m.outline
	treeHeight := m.fileTree.viewport.Height()
	if o.viewport.Height() > 0 && row >= treeHeight {
		m.focused, o.focused = commentPane, true
		if row == treeHeight {
			o.dragging = true
			m.renderOutline()
			m.renderFileTree()
			return
		}
		m.jumpOutline(row - treeHeight - 1 + o.viewport.YOffset())
		return
	}
	o.focused = false
	m.handleFileTreeClick(row + m.fileTree.viewport.YOffset())
}

func (m *AppModel) scrollFilesSidebar(row, delta int) {
	vp := &m.fileTree.viewport
	if m.outline.viewport.Height() > 0 && row > m.fileTree.viewport.Height() {
		vp = &m.outline.viewport
	}
	if delta < 0 {
		vp.ScrollUp(-delta)
	} else {
		vp.ScrollDown(delta)
	}
}

func (m *AppModel) setOutlineHeight(height int) {
	total := m.commentViewport.Height()
	if total < 6 {
		return
	}
	height = max(2, min(height, total*2/3, len(m.outline.rows)+1))
	// Round up so converting the percentage back to rows preserves height.
	m.outline.proportion = (height*100 + total - 1) / total
	m.layoutFilesSidebar()
	m.scrollToTreeCursor()
	m.revealOutlineHeading()
}

func (m *AppModel) resizeOutlineAt(y int) {
	_, _, _, bottom := m.commentBounds()
	m.setOutlineHeight(bottom - y)
}
