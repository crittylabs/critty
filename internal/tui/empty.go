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
	if m.modal == finishResolveModal || m.modal == finishAbortModal {
		return m.handleFinishConfirmationMouse(msg.Mouse())
	}
	if m.modal == finishModal {
		return m.handleFinishModalMouse(msg.Mouse())
	}
	return m, nil
}
