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
)

// sidebarView selects what the sidebar pane shows, in tab order.  The file
// tree is the default.
type sidebarView int

const (
	filesView sidebarView = iota
	commentsView
)

// treeIcons holds the markers drawn before tree entries.  They occupy the
// same width so names align; Nerd Font glyphs can replace them later.
type treeIcons struct {
	dirOpen, dirClosed, file string
}

var defaultTreeIcons = treeIcons{dirOpen: "▾ ", dirClosed: "▸ ", file: "  "}

const attachmentHistoryPath = "\x00attachment-history"

// treeIndent is the number of columns each nesting level adds.
const treeIndent = 1

// treeRow is a visible row of the file tree.
type treeRow struct {
	name      string
	path      string // slash-separated path of the entry
	depth     int
	isDir     bool
	collapsed bool
	tabIndex  int // index into AppModel.tabs for files
}

// fileTree is the sidebar's file tree state.
type fileTree struct {
	root      *treeNode
	rows      []treeRow
	cursor    int
	hscroll   int             // columns the cursor row is scrolled to the right
	collapsed map[string]bool // directory path -> folded
	syncedTab int             // tab the cursor last followed; -1 before the first sync
	viewport  viewport.Model
}

func newFileTree() fileTree {
	return fileTree{collapsed: map[string]bool{attachmentHistoryPath: true}, syncedTab: -1, viewport: viewport.New()}
}

type treeNode struct {
	name     string
	path     string
	isDir    bool
	tabIndex int
	children []*treeNode
}

// buildFileTree arranges tab paths into a directory tree.  Children are
// sorted with directories first, then by name.
func buildFileTree(paths []string) *treeNode {
	root := &treeNode{isDir: true}
	for i, p := range paths {
		parent := root
		parts := strings.Split(strings.Trim(filepath.ToSlash(p), "/"), "/")
		for j, part := range parts {
			if part == "" {
				continue
			}
			if j == len(parts)-1 {
				parent.children = append(parent.children, &treeNode{name: part, path: strings.Join(parts[:j+1], "/"), tabIndex: i})
				break
			}
			var dir *treeNode
			for _, child := range parent.children {
				if child.isDir && child.name == part {
					dir = child
					break
				}
			}
			if dir == nil {
				dir = &treeNode{name: part, path: strings.Join(parts[:j+1], "/"), isDir: true}
				parent.children = append(parent.children, dir)
			}
			parent = dir
		}
	}
	sortTree(root)
	return root
}

func sortTree(node *treeNode) {
	sort.SliceStable(node.children, func(i, j int) bool {
		a, b := node.children[i], node.children[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		return a.name < b.name
	})
	for _, child := range node.children {
		if child.isDir {
			sortTree(child)
		}
	}
}

// visibleTreeRows flattens the tree, skipping the children of folded
// directories.
func visibleTreeRows(root *treeNode, collapsed map[string]bool) []treeRow {
	var rows []treeRow
	var walk func(node *treeNode, depth int)
	walk = func(node *treeNode, depth int) {
		for _, child := range node.children {
			row := treeRow{name: child.name, path: child.path, depth: depth, isDir: child.isDir, tabIndex: child.tabIndex}
			if child.isDir {
				row.collapsed = collapsed[child.path]
			}
			rows = append(rows, row)
			if child.isDir && !row.collapsed {
				walk(child, depth+1)
			}
		}
	}
	walk(root, 0)
	return rows
}

// reviewFileTree keeps attachment order separate from directory-first paths.
func (m *AppModel) reviewFileTree() *treeNode {
	var paths []string
	var indices []int
	var overall *treeNode
	attachments := &treeNode{name: "attachments", path: "\x00attachments", isDir: true}
	history := &treeNode{name: "attachment history (read-only)", path: attachmentHistoryPath, isDir: true}
	for i, t := range m.tabs {
		if t.path == overallCommentsPath {
			overall = &treeNode{name: t.displayPath(), path: t.path, tabIndex: i}
		} else if t.attachment != nil {
			name := t.attachment.Name
			if t.attachment.Removed {
				history.children = append(history.children, &treeNode{name: name, path: t.path, tabIndex: i})
				continue
			}
			attachments.children = append(attachments.children, &treeNode{name: name, path: t.path, tabIndex: i})
		} else {
			paths = append(paths, t.path)
			indices = append(indices, i)
		}
	}
	root := buildFileTree(paths)
	var remap func(*treeNode)
	remap = func(node *treeNode) {
		if !node.isDir {
			node.tabIndex = indices[node.tabIndex]
		}
		for _, child := range node.children {
			remap(child)
		}
	}
	remap(root)
	root.name = m.project
	if root.name == "" {
		root.name = "Files"
	}
	forest := &treeNode{isDir: true, path: "\x00root"}
	if overall != nil {
		forest.children = append(forest.children, overall)
	}
	if len(attachments.children) > 0 {
		forest.children = append(forest.children, attachments)
	}
	if len(paths) > 0 {
		forest.children = append(forest.children, root)
	}
	if len(history.children) > 0 {
		forest.children = append(forest.children, history)
	}
	return forest
}

func (m *AppModel) orderTabsByTree() {
	rows := visibleTreeRows(m.reviewFileTree(), nil)
	tabs := make([]FileTab, 0, len(m.tabs))
	for _, row := range rows {
		if !row.isDir {
			tabs = append(tabs, m.tabs[row.tabIndex])
		}
	}
	m.tabs = tabs
}

// syncFileTree rebuilds the visible rows and, when the active tab changed
// elsewhere, moves the cursor to it, unfolding its directories.
func (m *AppModel) syncFileTree() {
	tree := &m.fileTree
	if len(m.tabs) == 0 {
		tree.rows = nil
		tree.viewport.SetContent("")
		return
	}
	tree.root = m.reviewFileTree()
	if tree.syncedTab != m.activeTab {
		delete(tree.collapsed, "")
		if m.tabs[m.activeTab].attachment != nil {
			if m.tabs[m.activeTab].attachment.Removed {
				delete(tree.collapsed, attachmentHistoryPath)
			} else {
				delete(tree.collapsed, "\x00attachments")
			}
		}
		parts := strings.Split(strings.Trim(filepath.ToSlash(m.tabs[m.activeTab].path), "/"), "/")
		for j := 1; j < len(parts); j++ {
			delete(tree.collapsed, strings.Join(parts[:j], "/"))
		}
	}
	tree.rows = visibleTreeRows(tree.root, tree.collapsed)
	if tree.syncedTab != m.activeTab {
		for i, row := range tree.rows {
			if !row.isDir && row.tabIndex == m.activeTab {
				tree.cursor = i
				tree.hscroll = 0
				break
			}
		}
		tree.syncedTab = m.activeTab
	}
	tree.cursor = max(0, min(tree.cursor, len(tree.rows)-1))
	m.renderFileTree()
	m.scrollToTreeCursor()
}

// treeCursorWidth is the width of the cursor column before each row.
const treeCursorWidth = 2

// treeRowText renders row i without the cursor column or truncation.
func (m *AppModel) treeRowText(i int, indicators []string) string {
	row := m.fileTree.rows[i]
	icon := defaultTreeIcons.file
	label := row.name
	if row.isDir {
		icon = defaultTreeIcons.dirOpen
		if row.collapsed {
			icon = defaultTreeIcons.dirClosed
		}
		label += "/"
	}
	style := lipgloss.NewStyle().Foreground(textSecondary)
	if row.isDir {
		style = style.Foreground(lipgloss.Cyan)
		if row.path == "\x00attachments" {
			style = style.Foreground(lipgloss.Magenta)
		}
	} else if m.tabs[row.tabIndex].attachment != nil {
		style = style.Foreground(warning)
	}
	if !row.isDir && row.tabIndex == m.activeTab {
		style = style.Bold(true).Foreground(textPrimary)
		if m.tabs[row.tabIndex].attachment != nil {
			style = style.Foreground(activeAttachmentColor)
		}
	}
	if !row.isDir {
		icon = m.tabs[row.tabIndex].unreadMarker()
	}
	text := strings.Repeat(" ", row.depth*treeIndent) + style.Render(icon) + style.Render(label)
	if !row.isDir {
		if counts := m.tabs[row.tabIndex].changeSummary(); counts != "" {
			text += " " + counts
		}
		if indicator := indicators[row.tabIndex]; indicator != "" {
			text += " " + indicator
		}
	}
	return text
}

// treeTextWidth is the number of columns available for a row's text.
func (m *AppModel) treeTextWidth() int {
	return max(1, m.fileTree.viewport.Width()-treeCursorWidth)
}

// maxTreeScroll returns how far the cursor row can scroll to the right
// before its end is visible.
func (m *AppModel) maxTreeScroll() int {
	tree := &m.fileTree
	if len(tree.rows) == 0 {
		return 0
	}
	return max(0, lipgloss.Width(m.treeRowText(tree.cursor, m.tabIndicators()))-m.treeTextWidth())
}

// renderFileTree draws the visible rows into the tree viewport.  Only the
// cursor row honors the horizontal scroll.
func (m *AppModel) renderFileTree() {
	tree := &m.fileTree
	tree.hscroll = max(0, min(tree.hscroll, m.maxTreeScroll()))
	width := m.treeTextWidth()
	cursorCol := lipgloss.NewStyle().Width(treeCursorWidth)
	lines := make([]string, len(tree.rows))
	indicators := m.tabIndicators()
	for i := range tree.rows {
		prefix := cursorCol.Render("")
		text := m.treeRowText(i, indicators)
		if i == tree.cursor {
			if m.focused == commentPane && !m.outline.focused {
				prefix = cursorCol.Render(cursorMarker.Render(">"))
			}
			if tree.hscroll > 0 {
				text = "…" + ansi.Cut(text, tree.hscroll+1, lipgloss.Width(text))
			}
		}
		line := prefix + ansi.Truncate(text, width, "…")
		row := tree.rows[i]
		if !row.isDir && row.tabIndex == m.activeTab {
			padding := strings.Repeat(" ", max(0, tree.viewport.Width()-ansi.StringWidth(line)))
			// Reversing equal colors keeps the gray fill while preventing
			// background-color erase optimizations from dropping the padding.
			line += lipgloss.NewStyle().Foreground(buttonBackground).Background(buttonBackground).Reverse(true).Render(padding)
			line = inlineBackground(lipgloss.NewStyle().Background(buttonBackground), line)
		}
		lines[i] = line
	}
	tree.viewport.SetContent(strings.Join(lines, "\n"))
}

// scrollTreeRow scrolls the cursor row horizontally by delta columns and
// reports whether the view moved.
func (m *AppModel) scrollTreeRow(delta int) bool {
	tree := &m.fileTree
	next := max(0, min(tree.hscroll+delta, m.maxTreeScroll()))
	if next == tree.hscroll {
		return false
	}
	tree.hscroll = next
	m.renderFileTree()
	return true
}

func (m *AppModel) scrollToTreeCursor() {
	tree := &m.fileTree
	height, offset := tree.viewport.Height(), tree.viewport.YOffset()
	if height <= 0 {
		return
	}
	if tree.cursor < offset {
		tree.viewport.SetYOffset(tree.cursor)
	} else if tree.cursor >= offset+height {
		tree.viewport.SetYOffset(tree.cursor - height + 1)
	}
}

// setTreeCursor moves the cursor to row i.  Landing on a file opens its tab.
func (m *AppModel) setTreeCursor(i int) {
	tree := &m.fileTree
	if len(tree.rows) == 0 {
		return
	}
	if next := max(0, min(i, len(tree.rows)-1)); next != tree.cursor {
		tree.cursor = next
		tree.hscroll = 0
	}
	row := tree.rows[tree.cursor]
	if !row.isDir && row.tabIndex != m.activeTab {
		m.activeTab = row.tabIndex
		tree.syncedTab = row.tabIndex
		m.rebuildContent()
	}
	m.updateCommentSidebar()
}

// toggleTreeDir folds or unfolds the directory at row i.
func (m *AppModel) toggleTreeDir(i int) {
	tree := &m.fileTree
	if i < 0 || i >= len(tree.rows) || !tree.rows[i].isDir {
		return
	}
	row := tree.rows[i]
	var find func(*treeNode) *treeNode
	find = func(node *treeNode) *treeNode {
		if node.isDir && node.path == row.path {
			return node
		}
		for _, child := range node.children {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	for node := find(tree.root); node != nil; {
		if row.collapsed {
			delete(tree.collapsed, node.path)
		} else {
			tree.collapsed[node.path] = true
		}
		if len(node.children) != 1 || !node.children[0].isDir {
			break
		}
		node = node.children[0]
	}
	tree.cursor = i
	m.updateCommentSidebar()
}

// treeParentRow returns the row of the directory containing row i.
func (m *AppModel) treeParentRow(i int) (int, bool) {
	rows := m.fileTree.rows
	for j := i - 1; j >= 0; j-- {
		if rows[j].depth < rows[i].depth {
			return j, true
		}
	}
	return 0, false
}

// handleFileTreeKey handles navigation while the file tree is focused.  It
// reports whether the key was consumed.
func (m *AppModel) handleFileTreeKey(msg tea.KeyPressMsg) bool {
	tree := &m.fileTree
	if len(tree.rows) == 0 {
		return false
	}
	row := tree.rows[tree.cursor]
	switch {
	case key.Matches(msg, keys.Down):
		m.setTreeCursor(tree.cursor + 1)
	case key.Matches(msg, keys.Up):
		m.setTreeCursor(tree.cursor - 1)
	case key.Matches(msg, keys.HalfPageDown):
		m.setTreeCursor(tree.cursor + max(1, tree.viewport.Height()/2))
	case key.Matches(msg, keys.HalfPageUp):
		m.setTreeCursor(tree.cursor - max(1, tree.viewport.Height()/2))
	case key.Matches(msg, keys.Top):
		m.setTreeCursor(0)
	case key.Matches(msg, keys.Bottom):
		m.setTreeCursor(len(tree.rows) - 1)
	case key.Matches(msg, keys.Confirm):
		if row.isDir {
			m.toggleTreeDir(tree.cursor)
		} else {
			m.focused = contentPane
			m.updateCommentSidebar()
			m.rebuildContent()
		}
	case msg.String() == "h" || msg.String() == "left":
		// Scroll a shifted row back first; at the left edge, fold or climb.
		if m.scrollTreeRow(-m.treeScrollStep()) {
			break
		}
		if row.isDir && !row.collapsed {
			m.toggleTreeDir(tree.cursor)
		} else if parent, ok := m.treeParentRow(tree.cursor); ok {
			m.setTreeCursor(parent)
		}
	case msg.String() == "l" || msg.String() == "right":
		// Unfold a folded directory first; then reveal a truncated row's
		// tail; then enter an open directory.
		if row.isDir && row.collapsed {
			m.toggleTreeDir(tree.cursor)
		} else if m.scrollTreeRow(m.treeScrollStep()) {
			break
		} else if row.isDir {
			m.setTreeCursor(tree.cursor + 1)
		}
	default:
		return false
	}
	return true
}

// treeScrollStep is how many columns one horizontal scroll moves.
func (m *AppModel) treeScrollStep() int {
	return max(1, m.treeTextWidth()/2)
}

// handleFileTreeClick selects the clicked row; a directory row also folds
// or unfolds.
func (m *AppModel) handleFileTreeClick(row int) {
	tree := &m.fileTree
	if row < 0 || row >= len(tree.rows) {
		return
	}
	m.focused = commentPane
	if tree.rows[row].isDir {
		m.toggleTreeDir(row)
		return
	}
	m.setTreeCursor(row)
}

// toggleSidebarView switches the sidebar between the file tree and the
// comments and focuses it.
func (m *AppModel) toggleSidebarView() {
	if m.viewMode {
		m.viewMode = false
		m.recalculateLayout()
	}
	if m.sidebarView == filesView {
		m.sidebarView = commentsView
	} else {
		m.sidebarView = filesView
	}
	m.focused = commentPane
	m.updateCommentSidebar()
	m.rebuildContent()
}
