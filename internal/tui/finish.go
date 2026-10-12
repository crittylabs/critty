package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/review"
)

const (
	finishText = iota
	finishApprove
	finishReject
	finishComment
	finishSubmit
	finishBack
	finishAbort
)

func (m *AppModel) defaultFinishVerdict() review.Verdict {
	if len(m.missingForms()) > 0 {
		return review.VerdictComment
	}
	for _, tab := range m.tabs {
		if tab.state == nil {
			continue
		}
		for _, c := range tab.state.Comments {
			if m.hasFinishFeedback(tab.path, c) {
				return review.VerdictComment
			}
		}
	}
	if m.session != nil {
		for _, c := range m.session.CJ.ReviewComments {
			if m.hasFinishFeedback("", c) {
				return review.VerdictComment
			}
		}
	}
	if m.newFeedback && (m.session == nil || m.session.CJ.RoundState.FeedbackComments == nil) && m.unresolvedTotal() > 0 {
		return review.VerdictComment
	}
	return review.VerdictApprove
}

func (m *AppModel) noteFinishFeedback(path, id string) {
	if path == overallCommentsPath {
		path = ""
	} else if path != "" {
		path = review.NormalizePath(path)
	}
	if m.session == nil {
		return
	}
	state := &m.session.CJ.RoundState
	if state.FeedbackComments == nil {
		state.FeedbackComments = make(map[string]map[string]bool)
	}
	if state.FeedbackComments[path] == nil {
		state.FeedbackComments[path] = make(map[string]bool)
	}
	state.FeedbackComments[path][id] = true
}

func (m *AppModel) hasFinishFeedback(path string, c review.Comment) bool {
	if path == overallCommentsPath {
		path = ""
	} else if path != "" {
		path = review.NormalizePath(path)
	}
	if c.Resolved {
		return false
	}
	if m.session != nil && m.session.CJ.RoundState.FeedbackComments[path][c.ID] {
		return true
	}
	if m.reviewRound() > 0 && c.Author == m.author && c.ReviewRound == m.reviewRound() {
		return true
	}
	for _, reply := range c.Replies {
		if m.reviewRound() > 0 && reply.Author == m.author && reply.ReviewRound == m.reviewRound() {
			return true
		}
	}
	return false
}

func (m *AppModel) openFinishModal() {
	m.persist()
	m.modal = finishModal
	m.finishVerdict = m.defaultFinishVerdict()
	if m.session != nil {
		m.finishDraft = m.session.CJ.RoundState.ReviewCommentDraft
	}
	m.modalTextarea.Reset()
	m.modalTextarea.SetValue(m.finishDraft)
	m.modalTextarea.Placeholder = "Overall comment (optional)..."
	m.modalTextarea.Blur()
	m.modalFocus = finishSubmit
	m.modalReferenceOffset = -1
	m.clipboardStatus = ""
	m.recalculateLayout()
}

func (m *AppModel) saveFinishDraft() {
	m.finishDraft = m.modalTextarea.Value()
	if m.session != nil {
		m.session.CJ.RoundState.ReviewCommentDraft = m.finishDraft
		m.persist()
	}
}

// SaveUnsubmittedDraft preserves the editor when the program is stopped
// externally rather than through the modal's own controls.
func (m *AppModel) SaveUnsubmittedDraft() error {
	if (m.modal != finishModal && m.modal != finishResolveModal && m.modal != finishAbortModal) || m.session == nil || m.session.CJ.RoundState.Finished {
		return nil
	}
	m.err = nil
	m.saveFinishDraft()
	return m.err
}

func (m *AppModel) closeFinishModal() {
	m.saveFinishDraft()
	if m.err != nil {
		return
	}
	m.modalTextarea.Blur()
	m.clipboardPending = false
	m.modal = noModal
	m.recalculateLayout()
}

func (m *AppModel) finishFocusedAction() (tea.Model, tea.Cmd) {
	switch m.modalFocus {
	case finishApprove, finishReject, finishComment, finishSubmit:
		return m.doFinish()
	case finishBack:
		m.closeFinishModal()
	case finishAbort:
		return m.requestFinishAbort()
	}
	return m, nil
}

func (m *AppModel) requestFinishAbort() (tea.Model, tea.Cmd) {
	m.saveFinishDraft()
	if m.err != nil {
		return m, nil
	}
	m.modal, m.modalFocus = finishAbortModal, 1
	m.modalTextarea.Blur()
	return m, nil
}

func (m *AppModel) handleFinishModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeFinishModal()
		return m, nil
	case "ctrl+c":
		return m.requestFinishAbort()
	case "tab", "shift+tab":
		delta := 1
		if msg.String() == "shift+tab" {
			delta = -1
		}
		m.modalFocus = (m.modalFocus + delta + finishAbort + 1) % (finishAbort + 1)
		if m.modalFocus == finishText {
			return m, m.modalTextarea.Focus()
		}
		m.modalTextarea.Blur()
		return m, nil
	case "ctrl+s":
		if m.modalFocus == finishText {
			m.modalFocus = finishSubmit
			m.modalTextarea.Blur()
		}
		return m, nil
	case "ctrl+pgup":
		m.scrollModalReference(-1)
		return m, nil
	case "ctrl+pgdown":
		m.scrollModalReference(1)
		return m, nil
	}
	if m.modalFocus == finishText {
		return m.handleTextModal(msg)
	}
	switch msg.String() {
	case "a", "A", "r", "R", "c", "C":
		focus := finishApprove
		if strings.ToLower(msg.String()) == "r" {
			focus = finishReject
		}
		if strings.ToLower(msg.String()) == "c" {
			focus = finishComment
		}
		m.selectFinishVerdict(focus)
		m.modalFocus = finishSubmit
	case "space", " ":
		m.selectFinishVerdict(m.modalFocus)
	case "y", "Y":
		return m.doFinish()
	case "n", "N":
		m.closeFinishModal()
	case "q":
		return m.requestFinishAbort()
	case "left", "h":
		m.modalFocus = max(finishApprove, m.modalFocus-1)
		m.selectFinishVerdict(m.modalFocus)
	case "right", "l":
		m.modalFocus = min(finishAbort, m.modalFocus+1)
		m.selectFinishVerdict(m.modalFocus)
	case "up", "k":
		m.scrollFinishReference(-1)
	case "down", "j":
		m.scrollFinishReference(1)
	case "enter":
		return m.finishFocusedAction()
	}
	return m, nil
}

func (m *AppModel) handleFinishModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		switch {
		case region.action.textarea:
			m.focusTextareaAt(mouse, region.rect)
		case region.action.close:
			m.closeFinishModal()
		case region.action.pick:
			m.modalFocus = region.action.focus
			m.modalTextarea.Blur()
			m.selectFinishVerdict(region.action.focus)
		case !region.action.dialog && !region.action.scrollable:
			m.modalFocus = region.action.focus
			m.modalTextarea.Blur()
			return m.finishFocusedAction()
		}
		return m, nil
	}
	return m, nil
}

func (m AppModel) finishTextareaHeight() int {
	return max(1, min(5, m.height-22))
}

func (m AppModel) finishModalContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(finishColor(m.finishVerdict)).Render("Submit review"), width)
	content := title + "\n"
	if m.height >= 18 && len(m.missingForms()) > 0 {
		content += lipgloss.Wrap("Approve requires all confirmations.", width, "") + "\n"
	}
	choices, choiceRegions := m.finishChoices(width, strings.Count(content, "\n"))
	content += choices + "\n"
	var reference strings.Builder
	reference.WriteString(strings.TrimSpace(m.formSummary(max(1, width-4))))
	var latestThread threadLayout
	latestThreadStart := 0
	if m.session != nil {
		for _, c := range m.session.CJ.ReviewComments {
			if c.ID == m.session.CJ.RoundState.ReviewCommentID {
				continue
			}
			if reference.Len() > 0 {
				reference.WriteString("\n\n")
			}
			reference.WriteString(lipgloss.Wrap("Overall comment · round "+strconv.Itoa(c.ReviewRound), max(1, width-4), "") + "\n")
			thread := m.layoutThread(c.Author, c.Body, c.Replies, "", max(1, width-4))
			latestThreadStart = strings.Count(reference.String(), "\n")
			latestThread = thread
			reference.WriteString(strings.Join(thread.lines, "\n"))
		}
	}
	regions := append([]modalMouseRegion{closeRegion}, choiceRegions...)
	buttons, _ := m.finishButtons(width, 0)
	available := m.height - modalStyle.GetVerticalFrameSize() - lipgloss.Height(content) - m.finishTextareaHeight() - lipgloss.Height(buttons) - 4
	if reference.Len() > 0 && available >= 3 {
		height := min(8, available)
		initialOffset := latestThreadStart + latestThread.initialOffset(height-2)
		box, offset, maximum := renderModalBoxAt(strings.TrimSpace(reference.String()), width-2, height, m.modalReferenceOffset, initialOffset)
		top := strings.Count(content, "\n")
		regions = append(regions, modalMouseRegion{
			rect:   mouseRect{left: 0, top: top, right: lipgloss.Width(box), bottom: top + lipgloss.Height(box)},
			action: modalMouseAction{scrollable: true, scrollOffset: offset, scrollMaxOffset: maximum},
		})
		content += box + "\n"
		if maximum > 0 {
			content += footerStyle.Render("C-PgUp/PgDn: details") + "\n"
		}
	}
	content, textareaRegion := layoutModalTextarea(content, m.clipboardTextareaView(), width)
	regions = append(regions, textareaRegion)
	buttons, buttonRegions := m.finishButtons(width, strings.Count(content, "\n"))
	content += buttons
	regions = append(regions, buttonRegions...)
	if m.height >= 18 {
		hint := "a/r/c: select · Tab: focus · q: abort"
		if m.modalFocus == finishText {
			hint = "Tab: focus · C-s: focus Submit"
		}
		content += "\n" + footerStyle.Render(hint)
	}
	return content, regions
}

func (m AppModel) finishButtons(width, top int) (string, []modalMouseRegion) {
	return layoutModalButtonRow([]modalButtonSpec{
		{rendered: m.renderModalButton("Submit", "enter", m.modalFocus == finishSubmit), action: modalMouseAction{focus: finishSubmit}},
		{rendered: m.renderModalButton("Back", "esc", m.modalFocus == finishBack), action: modalMouseAction{focus: finishBack}},
		{rendered: m.renderModalButton("Abort", "q", m.modalFocus == finishAbort), action: modalMouseAction{focus: finishAbort}},
	}, width, top)
}

func (m *AppModel) failSubmission(err error) {
	m.session.CJ.RoundState.Finished = false
	m.session.CJ.RoundState.SubmittedReplies = nil
	if rollbackErr := m.session.Save(); rollbackErr != nil {
		err = fmt.Errorf("%w; restoring unfinished round: %v", err, rollbackErr)
	}
	m.err, m.finishSaveFailed = err, true
}

func (m *AppModel) selectFinishVerdict(focus int) {
	switch focus {
	case finishApprove:
		m.finishVerdict = review.VerdictApprove
	case finishReject:
		m.finishVerdict = review.VerdictReject
	case finishComment:
		m.finishVerdict = review.VerdictComment
	}
}

func finishColor(verdict review.Verdict) color.Color {
	switch verdict {
	case review.VerdictApprove:
		return success
	case review.VerdictReject:
		return lipgloss.Red
	default:
		return accent
	}
}

func (m AppModel) finishChoices(width, top int) (string, []modalMouseRegion) {
	var choices []modalButtonSpec
	for i, option := range []struct {
		verdict review.Verdict
		label   string
	}{
		{review.VerdictApprove, "Approve"},
		{review.VerdictReject, "Reject"},
		{review.VerdictComment, "Comment"},
	} {
		selected := m.finishVerdict == option.verdict
		symbol := "○"
		if selected {
			symbol = "●"
		}
		style := lipgloss.NewStyle().Foreground(finishColor(option.verdict)).Background(buttonBackground).Bold(selected).Underline(m.modalFocus == finishApprove+i)
		keyStyle := style.Underline(true)
		choices = append(choices, modalButtonSpec{
			rendered: style.Render(symbol+" ") + keyStyle.Render(option.label[:1]) + style.Render(option.label[1:]),
			action:   modalMouseAction{focus: finishApprove + i, pick: true},
		})
	}
	return layoutModalButtonRow(choices, width, top)
}

func (m *AppModel) backFromFinishConfirmation() {
	m.modal, m.modalFocus = finishModal, finishSubmit
	m.recalculateLayout()
}

func (m *AppModel) confirmFinishAction() (tea.Model, tea.Cmd) {
	if m.modal == finishAbortModal {
		return m, tea.Quit
	}
	m.modal = finishModal
	return m.submitReview(review.VerdictApprove)
}

func (m *AppModel) handleFinishConfirmationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m.confirmFinishAction()
	case "n", "N", "esc":
		m.backFromFinishConfirmation()
	case "tab", "shift+tab", "left", "right", "h", "l":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			return m.confirmFinishAction()
		}
		m.backFromFinishConfirmation()
	case "q", "ctrl+c":
		if m.modal == finishAbortModal {
			m.backFromFinishConfirmation()
		} else {
			return m.requestFinishAbort()
		}
	}
	return m, nil
}

func (m *AppModel) handleFinishConfirmationMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.close {
			m.backFromFinishConfirmation()
			return m, nil
		}
		if region.action.dialog {
			return m, nil
		}
		if region.action.focus == 0 {
			return m.confirmFinishAction()
		}
		m.backFromFinishConfirmation()
		return m, nil
	}
	return m, nil
}

func (m AppModel) finishResolveContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(success).Render("Resolve all & approve?"), width)
	info := fmt.Sprintf("Resolve all %d unresolved thread(s) before approving?", m.unresolvedTotal())
	prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", width, "")
	buttons, regions := layoutModalButtonRow([]modalButtonSpec{
		{rendered: m.renderModalButton("Resolve All", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
		{rendered: m.renderModalButton("Back", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
	}, width, strings.Count(prefix, "\n"))
	return prefix + buttons, append(regions, closeRegion)
}

func (m AppModel) finishAbortContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(finishColor(review.VerdictReject)).Render("Abort review?"), width)
	prefix := lipgloss.Wrap(title+"\nExit without submitting?  Your comments and draft will be kept.\n\n", width, "")
	buttons, regions := layoutModalButtonRow([]modalButtonSpec{
		{rendered: m.renderModalButton("Abort", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
		{rendered: m.renderModalButton("Back", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
	}, width, strings.Count(prefix, "\n"))
	return prefix + buttons, append(regions, closeRegion)
}

func (m *AppModel) scrollFinishReference(delta int) {
	for _, region := range m.modalMouseRegions() {
		if region.action.scrollable {
			m.modalReferenceOffset = max(0, min(region.action.scrollMaxOffset, region.action.scrollOffset+delta))
			return
		}
	}
}

func (m *AppModel) updateFinishVerdictOnInput(emptyBefore bool) {
	if emptyBefore && m.modal == finishModal && m.modalTextarea.Value() != "" && m.finishVerdict == review.VerdictApprove {
		m.finishVerdict = review.VerdictComment
	}
}
