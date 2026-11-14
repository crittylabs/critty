package tui

func (m *AppModel) sourceGutterWidth() int {
	if m.hideLineNumbers {
		return 1
	}
	return gutterWidth
}

func (m *AppModel) toggleLineNumbers() {
	attention := m.captureDiffAttention()
	m.clearTextSelection()
	m.hideLineNumbers = !m.hideLineNumbers
	m.recalculateLayout()
	m.rebuildContent()
	m.restoreDiffAttention(attention)
}

func (m AppModel) lineNumbersButton() string {
	state := "on"
	if m.hideLineNumbers {
		state = "off"
	}
	return modalBtnFocusedLabel.Render("Lines: "+state) + modalBtnFocusedKey.Render("#")
}
