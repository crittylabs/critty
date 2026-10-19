package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/review"
)

const (
	finishText = iota
	finishApprove
	finishComment
	finishReject
	finishSubmit
	finishBack
	finishAbort
	finishProgressStart
)

const (
	finishReviewForms = iota
	finishSubmitForms
	finishCancelForms
)

func (m *AppModel) defaultFinishVerdict() review.Verdict {
	if m.answerMode || len(m.missingForms()) > 0 {
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
	if m.readOnly {
		return
	}
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
	m.modalFocus = m.finishVerdictFocus()
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
	if (m.modal != finishModal && m.modal != finishResolveModal && m.modal != finishAbortModal && m.modal != finishFormsModal) || m.session == nil || m.session.CJ.RoundState.Finished {
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
	if m.modalFocus >= finishProgressStart {
		m.jumpToProgress(progressKind(m.modalFocus - finishProgressStart))
		return m, nil
	}
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

func (m *AppModel) finishFocusOrder() []int {
	order := []int{finishText, m.finishVerdictFocus()}
	for _, count := range m.progressCounts() {
		if count.total > 0 {
			order = append(order, finishProgressStart+int(count.kind))
		}
	}
	return append(order, finishBack, finishAbort, finishSubmit)
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
		order := m.finishFocusOrder()
		index := slices.Index(order, m.modalFocus)
		m.modalFocus = order[(index+delta+len(order))%len(order)]
		if m.modalFocus == finishText {
			return m, m.modalTextarea.Focus()
		}
		m.modalTextarea.Blur()
		return m, nil
	case "ctrl+s":
		if m.modalFocus == finishText {
			m.saveFinishDraft()
			if m.err != nil {
				return m, nil
			}
			m.modalFocus = m.finishVerdictFocus()
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
		m.modalFocus = focus
	case "space", " ":
		m.selectFinishVerdict(m.modalFocus)
	case "y", "Y":
		return m.doFinish()
	case "n", "N":
		m.closeFinishModal()
	case "q":
		return m.requestFinishAbort()
	case "]", "}":
		m.jumpFromFinish(1, msg.String() == "]")
	case "[", "{":
		m.jumpFromFinish(-1, msg.String() == "[")
	case "left", "h":
		if m.modalFocus >= finishApprove && m.modalFocus <= finishReject {
			m.modalFocus = finishApprove + (m.modalFocus-finishApprove+2)%3
			m.selectFinishVerdict(m.modalFocus)
			break
		}
		order := m.finishFocusOrder()
		m.modalFocus = order[max(0, slices.Index(order, m.modalFocus)-1)]
		m.selectFinishVerdict(m.modalFocus)
	case "right", "l":
		if m.modalFocus >= finishApprove && m.modalFocus <= finishReject {
			m.modalFocus = finishApprove + (m.modalFocus-finishApprove+1)%3
			m.selectFinishVerdict(m.modalFocus)
			break
		}
		order := m.finishFocusOrder()
		m.modalFocus = order[min(len(order)-1, slices.Index(order, m.modalFocus)+1)]
		m.selectFinishVerdict(m.modalFocus)
	case "up":
		if m.modalFocus >= finishApprove && m.modalFocus <= finishReject {
			m.modalFocus = finishText
			return m, m.modalTextarea.Focus()
		}
		if m.modalFocus >= finishSubmit {
			m.modalFocus = m.finishVerdictFocus()
			break
		}
		m.scrollFinishReference(-1)
	case "k":
		m.scrollFinishReference(-1)
	case "down":
		if m.modalFocus >= finishApprove && m.modalFocus <= finishReject {
			order := m.finishFocusOrder()
			m.modalFocus = order[2]
			break
		}
		m.scrollFinishReference(1)
	case "j":
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

func (m AppModel) optionalFormsNotice(width int) string {
	hasOptional := false
	for _, pending := range m.pendingForms() {
		if !pending.item.Required {
			hasOptional = true
			break
		}
	}
	if !hasOptional {
		return ""
	}
	text := "Optional items remain unanswered."
	if len(m.missingForms()) == 0 {
		text += " You can still approve."
	}
	return lipgloss.Wrap(lipgloss.NewStyle().Foreground(warning).Render(text), width, "")
}

func (m AppModel) finishAttentionColor() color.Color {
	if m.finishVerdict == review.VerdictReject || m.finishVerdict == review.VerdictComment {
		return finishColor(m.finishVerdict)
	}
	if len(m.missingForms()) > 0 {
		return lipgloss.Red
	}
	if len(m.pendingForms()) > 0 {
		return warning
	}
	if m.unresolvedTotal() > 0 {
		return lipgloss.Cyan
	}
	return success
}

func (m AppModel) finishNotices(width int) string {
	var notices []string
	if m.unresolvedTotal() > 0 {
		notices = append(notices, "Unresolved threads remain.")
	}
	if len(m.missingForms()) > 0 {
		notices = append(notices, "Required items remain unchecked or unselected.")
	}
	text := ""
	if len(notices) > 0 {
		text = lipgloss.Wrap(lipgloss.NewStyle().Foreground(lipgloss.Red).Render(strings.Join(notices, " ")), width, "")
	}
	if optional := m.optionalFormsNotice(width); optional != "" {
		if text != "" {
			text += "\n"
		}
		text += optional
	}
	return text
}

func (m AppModel) finishModalContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(success).Render("Submit review"), width)
	content := title + "\n"
	choices, _ := m.finishChoices(width, 0)
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
	regions := []modalMouseRegion{closeRegion}
	buttons, _ := m.finishButtons(width, 0)
	progress, _ := m.layoutProgressCounts(width, 0, progressKind(m.modalFocus-finishProgressStart))
	notice := m.finishNotices(width)
	available := m.height - modalStyle.GetVerticalFrameSize() - lipgloss.Height(content) - lipgloss.Height(choices) - m.finishTextareaHeight() - lipgloss.Height(buttons) - lipgloss.Height(progress) - lipgloss.Height(notice) - 4
	// Keep actions reachable; counters still show pending items in small terminals.
	if available < 0 {
		available += lipgloss.Height(notice)
		notice = ""
	}
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
	choices, choiceRegions := m.finishChoices(width, strings.Count(content, "\n"))
	content += choices + "\n"
	regions = append(regions, choiceRegions...)
	progress, progressRegions := m.layoutProgressCounts(width, strings.Count(content, "\n"), progressKind(m.modalFocus-finishProgressStart))
	content += progress + "\n"
	for _, region := range progressRegions {
		regions = append(regions, modalMouseRegion{rect: region.rect, action: modalMouseAction{focus: finishProgressStart + int(region.kind)}})
	}
	if notice != "" {
		content += notice + "\n"
	}
	buttons, buttonRegions := m.finishButtons(width, strings.Count(content, "\n"))
	content += buttons
	regions = append(regions, buttonRegions...)
	return content, regions
}

func (m AppModel) finishButtons(width, top int) (string, []modalMouseRegion) {
	return layoutModalActions([]modalButtonSpec{
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

func (m AppModel) finishVerdictFocus() int {
	switch m.finishVerdict {
	case review.VerdictComment:
		return finishComment
	case review.VerdictReject:
		return finishReject
	default:
		return finishApprove
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

// commentLabel names the Comment verdict; answer rounds confirm the
// submitted answers without approving the reviewed design.  Both start
// with the same access key.
func (m AppModel) commentLabel() string {
	if m.answerMode {
		return "Confirm answers"
	}
	return "Comment"
}

func (m AppModel) finishChoices(width, top int) (string, []modalMouseRegion) {
	var choices []modalButtonSpec
	for i, option := range []struct {
		verdict review.Verdict
		label   string
	}{
		{review.VerdictApprove, "Approve"},
		{review.VerdictComment, m.commentLabel()},
		{review.VerdictReject, "Reject"},
	} {
		selected := m.finishVerdict == option.verdict
		symbol := "○"
		if selected {
			symbol = "●"
		}
		style := lipgloss.NewStyle().Foreground(finishColor(option.verdict)).Background(buttonBackground).Bold(selected).Underline(m.modalFocus == finishApprove+i)
		choices = append(choices, modalButtonSpec{
			rendered: style.Render(symbol+" ") + renderAccessKey(option.label, style),
			action:   modalMouseAction{focus: finishApprove + i, pick: true},
		})
	}
	return layoutModalButtonRow(choices, width, top)
}

func (m *AppModel) backFromFinishConfirmation() {
	m.modal, m.modalFocus = finishModal, finishSubmit
	m.recalculateLayout()
}

// confirmFormApproval preserves the required-answer gate and the separate
// confirmation before resolving outstanding threads.
func (m *AppModel) confirmFormApproval() (tea.Model, tea.Cmd) {
	if len(m.missingForms()) > 0 {
		m.clipboardStatus = "Complete required items before approving."
		return m, nil
	}
	if m.unresolvedTotal() > 0 {
		m.modal, m.modalFocus = finishResolveModal, 0
		m.modalTextarea.Blur()
		return m, nil
	}
	if m.modal == finishFormsModal {
		m.modal = finishModal
	}
	return m.submitReview(review.VerdictApprove)
}

func (m *AppModel) finishFormsAction() (tea.Model, tea.Cmd) {
	switch m.modalFocus {
	case finishReviewForms:
		pending := m.pendingForms()
		if m.canSubmitWithForms() {
			pending = m.unviewedForms()
		}
		m.closeFinishModal()
		if m.modal != noModal {
			return m, nil
		}
		if len(pending) > 0 {
			first := pending[0]
			m.selectComment(first.tab, commentTarget{formID: first.item.ID, line: first.item.Line})
		}
	case finishSubmitForms:
		if m.canSubmitWithForms() {
			return m.continueFinish()
		}
	case finishCancelForms:
		m.backFromFinishConfirmation()
	default:
		if m.modalFocus >= finishProgressStart {
			m.jumpToProgress(progressKind(m.modalFocus - finishProgressStart))
		}
	}
	return m, nil
}

func (m *AppModel) handleFinishFormsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "r", "R":
		m.modalFocus = finishReviewForms
		return m.finishFormsAction()
	case "s", "S":
		if m.canSubmitWithForms() {
			m.modalFocus = finishSubmitForms
			return m.finishFormsAction()
		}
	case "c", "C", "n", "N", "esc":
		m.backFromFinishConfirmation()
	case "tab", "shift+tab", "left", "right", "up", "down", "h", "j", "k", "l":
		order := []int{finishCancelForms, finishReviewForms}
		if m.canSubmitWithForms() {
			order = []int{finishReviewForms, finishCancelForms, finishSubmitForms}
		}
		for _, count := range m.progressCounts() {
			if count.total > 0 {
				order = append(order, finishProgressStart+int(count.kind))
			}
		}
		step := 1
		if msg.String() == "shift+tab" || msg.String() == "left" || msg.String() == "up" || msg.String() == "h" || msg.String() == "k" {
			step = -1
		}
		index := max(0, slices.Index(order, m.modalFocus))
		m.modalFocus = order[(index+step+len(order))%len(order)]
	case "enter", "space", " ":
		return m.finishFormsAction()
	case "q", "ctrl+c":
		return m.requestFinishAbort()
	}
	return m, nil
}

func (m *AppModel) handleFinishFormsMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.close {
			m.backFromFinishConfirmation()
			return m, nil
		}
		if !region.action.dialog {
			m.modalFocus = region.action.focus
			return m.finishFormsAction()
		}
		return m, nil
	}
	return m, nil
}

// canSubmitWithForms reports whether the forms dialog only asks for
// confirmation, rather than blocking approval on required items.
func (m AppModel) canSubmitWithForms() bool {
	return m.finishVerdict != review.VerdictApprove || len(m.missingForms()) == 0
}

func (m AppModel) finishFormsContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(warning).Render("Unanswered form items"), width)
	prefix := title + "\n"
	if m.canSubmitWithForms() {
		count := len(m.unviewedForms())
		text := fmt.Sprintf("%d unanswered items have not been viewed yet.", count)
		if count == 1 {
			text = "1 unanswered item has not been viewed yet."
		}
		prefix += lipgloss.Wrap(text+" Review them before submitting?", width, "") + "\n"
	} else {
		prefix += lipgloss.Wrap(lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true).Render("Required items must be completed."), width, "") + "\n"
	}
	prefix += "\n"
	specs := []modalButtonSpec{
		{rendered: m.renderModalButton("Review", "r", m.modalFocus == finishReviewForms), action: modalMouseAction{focus: finishReviewForms}},
	}
	if m.canSubmitWithForms() {
		specs = append([]modalButtonSpec{{rendered: m.renderModalButton("Submit", "s", m.modalFocus == finishSubmitForms), action: modalMouseAction{focus: finishSubmitForms}}}, specs...)
	}
	specs = append(specs, modalButtonSpec{rendered: m.renderModalButton("Cancel", "esc", m.modalFocus == finishCancelForms), action: modalMouseAction{focus: finishCancelForms}})
	buttons, regions := layoutModalActions(specs, width, strings.Count(prefix, "\n"))
	content := prefix + buttons + "\n"
	progress, progressRegions := m.layoutProgressCounts(width, strings.Count(content, "\n"), progressKind(m.modalFocus-finishProgressStart))
	for _, region := range progressRegions {
		regions = append(regions, modalMouseRegion{rect: region.rect, action: modalMouseAction{focus: finishProgressStart + int(region.kind)}})
	}
	return content + progress, append(regions, closeRegion)
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
		order := []int{1, 0}
		if m.modal == finishResolveModal {
			for _, count := range m.progressCounts() {
				if count.total > 0 {
					order = append(order, finishProgressStart+int(count.kind))
				}
			}
		}
		step := 1
		if msg.String() == "shift+tab" || msg.String() == "left" || msg.String() == "h" {
			step = -1
		}
		index := max(0, slices.Index(order, m.modalFocus))
		m.modalFocus = order[(index+step+len(order))%len(order)]
	case "enter":
		if m.modal == finishResolveModal && m.modalFocus >= finishProgressStart {
			m.jumpToProgress(progressKind(m.modalFocus - finishProgressStart))
			return m, nil
		}
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
		if m.modal == finishResolveModal && region.action.focus >= finishProgressStart {
			m.jumpToProgress(progressKind(region.action.focus - finishProgressStart))
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
	count := lipgloss.NewStyle().Foreground(lipgloss.Red).Render(fmt.Sprintf("%d unresolved thread(s)", m.unresolvedTotal()))
	info := "Resolve all " + count + " before approving?"
	prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", width, "")
	buttons, regions := layoutModalActions([]modalButtonSpec{
		{rendered: m.renderModalButton("Resolve All", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
		{rendered: m.renderModalButton("Back", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
	}, width, strings.Count(prefix, "\n"))
	content := prefix + buttons + "\n"
	progress, progressRegions := m.layoutProgressCounts(width, strings.Count(content, "\n"), progressKind(m.modalFocus-finishProgressStart))
	for _, region := range progressRegions {
		regions = append(regions, modalMouseRegion{rect: region.rect, action: modalMouseAction{focus: finishProgressStart + int(region.kind)}})
	}
	return content + progress, append(regions, closeRegion)
}

func (m AppModel) finishAbortContent(width int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Foreground(finishColor(review.VerdictReject)).Render("Abort review?"), width)
	prefix := lipgloss.Wrap(title+"\nExit without submitting?  Your comments and draft will be kept.\n\n", width, "")
	buttons, regions := layoutModalActions([]modalButtonSpec{
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
