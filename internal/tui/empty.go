package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m AppModel) renderEmptyReview() (string, []modalMouseRegion) {
	body := m.renderReviewBar() + "\n\nNo changes remain in this review.\n\n" + m.renderReloadStatus()
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
	if m.modal == noModal {
		bar := m.renderReviewBar()
		if indicator := m.watchIndicator(); indicator != "" {
			if rect, ok := buttonRect(bar, 0, indicator); ok && rect.contains(msg.Mouse()) {
				return m, m.toggleWatch()
			}
		}
		if rect, ok := buttonRect(bar, 0, m.submitButton()); ok && rect.contains(msg.Mouse()) {
			if m.readOnly {
				return m, tea.Quit
			}
			m.openFinishModal()
		}
	}
	return m, nil
}
