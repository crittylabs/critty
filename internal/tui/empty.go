package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m AppModel) renderEmptyReview() (string, []modalMouseRegion) {
	body := "Critty: [" + m.reviewScopeLabel() + "]\n\nNo changes remain in this review.\n\nq: finish review"
	body = lipgloss.NewStyle().Width(m.width).Height(m.height).Render(body)
	if m.modal != noModal {
		return m.renderWithModalLayout(body)
	}
	return body, nil
}

func (m *AppModel) handleEmptyReviewClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.modal != finishModal || msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	_, regions := m.renderEmptyReview()
	mouse := msg.Mouse()
	if dismissesModal(regions, mouse) {
		return m.handleFinishModal(tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	for _, region := range regions {
		if region.action.dialog || !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		return m.handleFinishModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	return m, nil
}
