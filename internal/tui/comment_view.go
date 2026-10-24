package tui

// View temporarily overrides expansion without changing the saved preference.
func (m AppModel) commentCollapsed(resolved, focused bool) bool {
	return m.viewMode || resolved && !m.openAll && !focused
}

func (m *AppModel) toggleOpenAll() {
	m.locationError = ""
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
