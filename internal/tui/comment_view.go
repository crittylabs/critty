package tui

import tea "charm.land/bubbletea/v2"

func (m AppModel) threadCollapsed(id string, resolved, drifted, focused bool) bool {
	if m.viewMode {
		return true
	}
	if folded, ok := m.tab().foldedComments[id]; ok {
		return folded
	}
	if drifted {
		return !m.driftedExpanded(id, resolved) && (!focused || !resolved)
	}
	return m.commentCollapsed(resolved, focused)
}

func (m *AppModel) setCommentFolded(id string, folded bool) {
	if !folded && m.viewMode {
		m.viewMode = false
		m.recalculateLayout()
	}
	t := m.tab()
	if t.foldedComments == nil {
		t.foldedComments = make(map[string]bool)
	}
	t.foldedComments[id] = folded
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

func (m *AppModel) handleCommentKey(msg tea.KeyPressMsg) bool {
	id := m.selectedCommentID()
	if id == "" || m.tab().selecting {
		return false
	}
	c, drifted := m.driftedComment(id)
	folded := m.threadCollapsed(id, c.Resolved, drifted, true)
	switch msg.String() {
	case "enter":
		if folded {
			m.setCommentFolded(id, false)
		} else {
			m.openCommentThread(id)
		}
	case "right", "l":
		if !folded {
			return false
		}
		m.setCommentFolded(id, false)
	case "left", "h":
		if folded {
			return false
		}
		m.setCommentFolded(id, true)
	case "e":
		if !drifted {
			return false
		}
		m.openCommentThread(id)
	default:
		return false
	}
	return true
}

// View temporarily overrides expansion without changing the saved preference.
func (m AppModel) commentCollapsed(resolved, focused bool) bool {
	return m.viewMode || resolved && !m.openAll && !focused
}

func (m *AppModel) toggleOpenAll() {
	m.locationError = ""
	for i := range m.tabs {
		clear(m.tabs[i].foldedComments)
	}
	if m.viewMode {
		m.viewMode = false
		m.openAll = true
		m.recalculateLayout()
	} else {
		m.openAll = !m.openAll
	}
	m.updateCommentSidebar()
	m.rebuildContent()
	m.scrollToCursor()
}

func (m *AppModel) toggleCommentView() {
	m.locationError = ""
	m.viewMode = !m.viewMode
	if m.viewMode {
		m.focused = contentPane
		m.tab().cursorOnAnnotation = false
		if m.tab().cursorLine == 0 {
			m.moveCursorBy(m.tab(), 1, 1)
		}
	}
	m.recalculateLayout()
	m.updateCommentSidebar()
	m.rebuildContent()
	m.scrollToCursor()
}
