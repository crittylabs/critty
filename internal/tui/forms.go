package tui

import (
	"crypto/sha256"
	"fmt"
	"image/color"
	"maps"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/form"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

type formControl struct {
	rect mouseRect
	line int
}

type formRow struct {
	offset int
	item   *form.Item
	hidden []form.Span
}

type formReviewItem struct {
	tab      int
	item     form.Item
	answered bool
	// viewed reports whether every option of the item has been shown in the
	// content viewport.
	viewed bool
}

// formReviewItems represents each choice group once, at its first option.
func (m *AppModel) formReviewItems() []formReviewItem {
	if m.forms == nil {
		return nil
	}
	var items []formReviewItem
	for tabIndex, tab := range m.tabs {
		d := m.forms.Document(tab.path)
		if d == nil {
			continue
		}
		viewed := m.formViewed[d.Path()]
		unviewed := map[string]bool{}
		for _, item := range d.Items {
			if !viewed[item.Line] {
				unviewed[item.ID] = true
			}
		}
		seen := map[string]bool{}
		for _, item := range d.Items {
			if seen[item.ID] || item.SelectAll != nil {
				continue
			}
			seen[item.ID] = true
			answer := d.Answers[item.ID]
			answered := form.Selected(item, answer)
			if item.Choice {
				value, ok := answer.Value.(string)
				answered = ok && value != ""
			}
			if item.Required || item.Consent {
				answered = form.Satisfied(item, answer)
			}
			items = append(items, formReviewItem{tab: tabIndex, item: item, answered: answered, viewed: !unviewed[item.ID]})
		}
	}
	return items
}

func (m *AppModel) pendingForms() []formReviewItem {
	var pending []formReviewItem
	for _, item := range m.formReviewItems() {
		if !item.answered {
			pending = append(pending, item)
		}
	}
	return pending
}

// unviewedForms returns unanswered items that have never been shown.
func (m *AppModel) unviewedForms() []formReviewItem {
	var unviewed []formReviewItem
	for _, item := range m.pendingForms() {
		if !item.viewed {
			unviewed = append(unviewed, item)
		}
	}
	return unviewed
}

// markVisibleForms records the form lines shown in the content viewport.
// Content behind a modal is not counted because the modal may cover it.
func (m *AppModel) markVisibleForms() {
	if m.forms == nil || len(m.tabs) == 0 || m.modal != noModal {
		return
	}
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return
	}
	top := m.contentViewport.YOffset()
	bottom := top + m.contentViewport.Height()
	for _, control := range m.contentLayout.formControls {
		if control.rect.top < top || control.rect.top >= bottom {
			continue
		}
		if m.formViewed == nil {
			m.formViewed = map[string]map[int]bool{}
		}
		if m.formViewed[d.Path()] == nil {
			m.formViewed[d.Path()] = map[int]bool{}
		}
		m.formViewed[d.Path()][control.line] = true
	}
}

func (m *AppModel) indexForms() error {
	m.formRows = map[string]map[int]formRow{}
	m.formTargets = map[string]map[string][]int{}
	m.formViewed = nil
	for i := range m.forms.Documents {
		d := &m.forms.Documents[i]
		targets, err := form.Targets(d.Items)
		if err != nil {
			return fmt.Errorf("attachment %s: %w", d.Name, err)
		}
		m.formTargets[d.Path()] = map[string][]int{}
		for index, leaves := range targets {
			m.formTargets[d.Path()][d.Items[index].ID] = leaves
		}
		starts := []int{0}
		for j, char := range d.Content {
			if char == '\n' {
				starts = append(starts, j+1)
			}
		}
		rows := map[int]formRow{}
		for j := range d.Items {
			item := &d.Items[j]
			for line := item.Line; line <= item.EndLine; line++ {
				row := rows[line]
				row.item, row.offset = item, starts[line-1]
				rows[line] = row
			}
			for _, span := range item.Metadata {
				line := sort.Search(len(starts), func(k int) bool { return starts[k] > span.Start }) - 1
				for ; line < len(starts) && starts[line] < span.End; line++ {
					end := len(d.Content)
					if line+1 < len(starts) {
						end = starts[line+1] - 1
					}
					row := rows[line+1]
					row.offset = starts[line]
					row.hidden = append(row.hidden, form.Span{Start: max(span.Start, starts[line]) - starts[line], End: min(span.End, end) - starts[line]})
					rows[line+1] = row
				}
			}
		}
		m.formRows[d.Path()] = rows
	}
	return nil
}

func (m *AppModel) beginForms(files map[string]review.SnapshotFile, rawDiff string) error {
	var docs []review.AttachedDocument
	hasForms := false
	for _, tab := range m.tabs {
		if tab.attachment != nil && !tab.attachment.Removed {
			docs = append(docs, *tab.attachment)
			items, err := review.ParseDocumentForm(*tab.attachment)
			if err != nil {
				return err
			}
			hasForms = hasForms || len(items) > 0
		}
	}
	// Also update an existing form state when the final form was removed.
	old, err := m.session.LoadForms()
	if err != nil {
		return err
	}
	if !hasForms && old.Version == 0 {
		return nil
	}
	input := review.ReviewInput{Scope: "document", Files: map[string]review.ReviewedFile{}, Diff: rawDiff}
	if m.patch != nil {
		input.Scope = "diff"
	} else if m.filePath == "" {
		source := gitpkg.ReviewSource{Scope: "all", Base: m.baseRef}
		if m.source != nil {
			source = *m.source
		} else if m.staged {
			source.Scope = "staged"
		}
		resolved, diff, err := source.Snapshot()
		if err != nil {
			return err
		}
		input.Scope, input.Source, input.Diff = resolved.Scope, &resolved, diff
	}
	for _, tab := range m.tabs {
		if tab.outsideChanges || tab.attachment != nil || tab.path == overallCommentsPath {
			continue
		}
		file := review.ReviewedFile{SnapshotFile: files[tab.path], Status: tab.status, Binary: tab.isBinary, Diff: tab.diff}
		if (tab.isBinary || tab.textconv.Command != "") && !tab.isDeleted && m.patch == nil {
			source := gitpkg.ReviewSource{}
			if m.source != nil {
				source = *m.source
			} else if m.staged {
				source.Scope = "staged"
			}
			data, err := source.Content(tab.path)
			if err != nil {
				return err
			}
			file.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		}
		input.Files[tab.path] = file
	}
	state, err := m.session.BeginForms(input, docs)
	if err != nil {
		return err
	}
	m.forms = &state
	return m.indexForms()
}

func (m *AppModel) missingForms() []string {
	if m.forms == nil {
		return nil
	}
	return m.forms.Missing()
}

func (m *AppModel) toggleForm(line int) bool {
	if m.readOnly {
		return false
	}
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return false
	}
	for _, item := range d.Items {
		if line < item.Line || line > item.EndLine {
			continue
		}
		m.tab().cursorOnForm = true
		if item.Rank {
			m.saveRank(d, item.ID, m.rankLayout(d, item.ID).order)
			m.rebuildContent()
			return true
		}
		if form.IsTextField(item) {
			m.openFormText(item)
			return true
		}
		previous := maps.Clone(d.Answers)
		answer := form.Answer{Human: true, Author: m.author, Revision: m.forms.Revision, Round: m.session.CJ.ReviewRound}
		if item.SelectAll != nil {
			answer.Via = item.ID
			form.ApplyBulk(d.Items, m.formTargets[d.Path()][item.ID], d.Answers, answer)
		} else {
			selected := item.Choice || !form.Selected(item, d.Answers[item.ID])
			d.Answers[item.ID] = form.SelectAnswer(d.Items, item, d.Answers[item.ID], selected, answer)
		}
		var targets []form.Item
		if item.SelectAll != nil {
			for _, i := range m.formTargets[d.Path()][item.ID] {
				targets = append(targets, d.Items[i])
			}
		} else {
			targets = []form.Item{item}
		}
		for _, target := range targets {
			proposed := d.Answers[target.ID]
			if target.Type == "" || !form.Selected(target, proposed) {
				continue
			}
			if err := m.validateFormPath(target, proposed.Text); err != nil {
				d.Answers = previous
				m.openFormText(target)
				m.pendingFormAnswer = &proposed
				m.modalTextarea.SetValue(proposed.Text)
				m.modalInitial = proposed.Text
				m.resizePathInput()
				m.refreshPathInfo()
				m.clipboardStatus = err.Error()
				m.rebuildContent()
				return true
			}
		}
		if err := m.session.SaveForms(*m.forms); err != nil {
			d.Answers = previous
			m.err = err
		} else {
			m.err = nil
			if item.Text && form.Selected(item, d.Answers[item.ID]) {
				m.openFormText(item)
			}
		}
		m.rebuildContent()
		return true
	}
	return false
}

// openFormText edits standalone input or input on a selected control.
func (m *AppModel) openFormText(item form.Item) {
	m.clipboardStatus = ""
	m.pendingFormAnswer = nil
	m.pathInfo = formPathState{}
	text := m.formText(item)
	m.modal = formTextModal
	m.editingFormLine = item.Line
	m.modalFocus = 0
	m.modalTextarea.Placeholder = "Type your answer..."
	m.modalTextarea.Reset()
	m.modalTextarea.SetValue(text)
	m.completion = completionState{}
	if item.Type != "" {
		m.modalTextarea.Placeholder = "Type a path..."
		m.resizePathInput()
	}
	m.modalInitial = m.modalTextarea.Value()
	m.modalTextarea.Focus()
	m.refreshPathInfo()
}

// editingFormItem returns the item edited by the form text modal.
func (m *AppModel) editingFormItem() (*review.FormDocument, *form.Item) {
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return nil, nil
	}
	row, ok := m.formRows[d.Path()][m.editingFormLine]
	if !ok || row.item == nil || row.item.Line != m.editingFormLine {
		return nil, nil
	}
	return d, row.item
}

// formText returns the text shown for item, which is empty for a choice
// that is not selected.
func (m *AppModel) formText(item form.Item) string {
	d := m.forms.Document(m.tab().path)
	if d == nil || !item.Text {
		return ""
	}
	answer := d.Answers[item.ID]
	if form.IsTextField(item) {
		text, _ := answer.Value.(string)
		return text
	}
	if !form.Selected(item, answer) {
		return ""
	}
	return answer.Text
}

func (m *AppModel) formTextLabel() string {
	if _, item := m.editingFormItem(); item != nil {
		return item.Label
	}
	return ""
}

// saveFormText keeps the editor open when saving fails so the text survives.
func (m *AppModel) saveFormText() {
	d, item := m.editingFormItem()
	if item == nil {
		m.discardTextModal()
		return
	}
	text := strings.TrimSpace(m.modalTextarea.Value())
	if item.Type != "" {
		text = m.modalTextarea.Value()
		if err := m.validateFormPath(*item, text); err != nil {
			m.clipboardStatus = err.Error()
			return
		}
	}
	previous := d.Answers[item.ID]
	selected := previous
	if m.pendingFormAnswer != nil {
		selected = *m.pendingFormAnswer
	}
	answer := form.Answer{Value: text, Human: true, Author: m.author, Revision: m.forms.Revision, Round: m.session.CJ.ReviewRound}
	if !form.IsTextField(*item) {
		answer.Value, answer.Text, answer.Drafts = selected.Value, text, maps.Clone(selected.Drafts)
		if item.Choice {
			if answer.Drafts == nil {
				answer.Drafts = map[string]string{}
			}
			answer.Drafts[item.Value] = text
		}
	}
	d.Answers[item.ID] = answer
	if err := m.session.SaveForms(*m.forms); err != nil {
		d.Answers[item.ID] = previous
		m.err = err
		return
	}
	m.err = nil
	m.discardTextModal()
	m.rebuildContent()
}

// formTextRows renders the text answer below the last line of its item,
// formatted like a comment body with links and image previews.
func (m *AppModel) formTextRows(line int, width int) []string {
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return nil
	}
	row, ok := m.formRows[d.Path()][line]
	if !ok || row.item == nil || row.item.EndLine != line {
		return nil
	}
	text := m.formText(*row.item)
	if text == "" {
		return nil
	}
	first := m.formRows[d.Path()][row.item.Line]
	prefix := d.Content[first.offset:row.item.Offset]
	indent := strings.Repeat(" ", lipgloss.Width(expandDisplayTabs(prefix[:len(prefix)-len(strings.TrimLeft(prefix, " \t"))]))+6)
	rows := strings.Split(m.renderCommentContent(d.Answers[row.item.ID].Author, text, "", max(1, width-len(indent))), "\n")
	for i := range rows {
		rows[i] = indent + rows[i]
	}
	return rows
}

// formDisplayLine hides only parsed metadata and retains each original line.
// Labels of form items are drawn on background, which the caller extends to
// the end of each row.
func (m *AppModel) formDisplayLine(line int, raw string, background lipgloss.Style) (string, int, bool) {
	d := m.forms.Document(m.tab().path)
	if d == nil || len(d.Items) == 0 {
		return "", 0, false
	}
	row, ok := m.formRows[d.Path()][line]
	if !ok {
		return "", 0, false
	}
	checkbox := row.item
	for i := len(row.hidden) - 1; i >= 0; i-- {
		span := row.hidden[i]
		raw = raw[:span.Start] + raw[span.End:]
	}
	if checkbox == nil {
		return m.linkFileReferences(strings.TrimRight(raw, " \t\r")), -1, true
	}
	if checkbox.Line != line {
		label := highlightInlineReferences(strings.TrimSpace(raw), formChip.link, formChip.code, m.linkFileReferences)
		if checkbox.Required {
			label = lipgloss.NewStyle().Bold(true).Render(label)
		}
		indent := expandDisplayTabs(raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))])
		return indent + inlineColors(background, label, true), lipgloss.Width(indent), true
	}
	column := checkbox.Offset - row.offset
	// Preserve nesting without treating an ordered-list marker as indentation.
	prefix := raw[:column]
	indent := expandDisplayTabs(prefix[:len(prefix)-len(strings.TrimLeft(prefix, " \t"))]) + "  "
	label := highlightInlineReferences(strings.TrimSpace(raw[column+3:]), formChip.link, formChip.code, m.linkFileReferences)
	if checkbox.SelectAll != nil {
		return indent + m.selectAllDisplay(d, checkbox, label, background), lipgloss.Width(indent), true
	}
	if checkbox.Rank {
		// The drag handle sits between the line number and the rank.
		handle := lipgloss.NewStyle().Foreground(formChip.cyan).Render("⇅") + " "
		indent = strings.TrimSuffix(indent, "  ")
		return indent + handle + m.rankDisplay(d, checkbox, label, background), lipgloss.Width(indent), true
	}
	answer := d.Answers[checkbox.ID]
	selected := form.Selected(*checkbox, answer)
	// The badge uses the terminal's ANSI color, except for required items;
	// tags on the chip use the chip's matching shade.
	tint, tagColor := color.Color(lipgloss.Cyan), formChip.cyan
	if selected {
		tint, tagColor = lipgloss.Green, formChip.green
	} else if checkbox.Required && !form.Satisfied(*checkbox, answer) {
		tint, tagColor = formChip.requiredBadge, formChip.red
	}
	symbol := "☐"
	if form.IsTextField(*checkbox) {
		symbol = "✎"
	} else if checkbox.Choice {
		symbol = "○"
		if selected {
			symbol = "●"
		}
	} else if selected {
		symbol = "✓"
	}
	tag := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
	if checkbox.Required {
		label = lipgloss.NewStyle().Bold(true).Render(label) + " " + tag.Render("Required")
	}
	if checkbox.Default != "" && !answer.Human {
		label += " " + tag.Render("Default")
	}
	if checkbox.Consent {
		label += " " + tag.Render("Consent")
	}
	badge := lipgloss.NewStyle().Foreground(formChip.badgeSymbol).Background(tint).Bold(true).Render(" " + symbol + " ")
	return indent + badge + inlineColors(background, " "+label, true), lipgloss.Width(indent), true
}

// selectAllState reports how many targets of a select-all item are selected.
func (m *AppModel) selectAllState(d *review.FormDocument, item *form.Item) (selected, total int) {
	targets := m.formTargets[d.Path()][item.ID]
	return form.CountSelected(d.Items, targets, d.Answers), len(targets)
}

// selectAllDisplay labels select-all items so that their state does not depend
// on color alone.
func (m *AppModel) selectAllDisplay(d *review.FormDocument, item *form.Item, label string, background lipgloss.Style) string {
	selected, total := m.selectAllState(d, item)
	tint, tagColor, symbol := color.Color(lipgloss.Cyan), formChip.cyan, "☐"
	if selected == total {
		tint, tagColor, symbol = lipgloss.Green, formChip.green, "✓"
	} else if selected > 0 {
		symbol = "⊟"
	}
	tag := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
	tags := tag.Render("All")
	if selected > 0 && selected < total {
		tags += " " + tag.Render("Mixed")
	}
	label += fmt.Sprintf(" (%d/%d)", selected, total)
	consents := 0
	for _, i := range m.formTargets[d.Path()][item.ID] {
		if d.Items[i].Consent {
			consents++
		}
	}
	if consents == 1 {
		label += " " + tag.Render("1 consent")
	} else if consents > 1 {
		label += " " + tag.Render(fmt.Sprintf("%d consents", consents))
	}
	badge := lipgloss.NewStyle().Foreground(formChip.badgeSymbol).Background(tint).Bold(true).Render(" " + symbol + " ")
	return badge + inlineColors(background, " "+tags+" "+label, true)
}

func (m *AppModel) formSummary(width int) string {
	if m.forms == nil || !m.forms.HasForms() {
		return ""
	}
	var lines []string
	for _, missing := range m.missingForms() {
		lines = append(lines, "Required: "+missing)
	}
	for _, d := range m.forms.Documents {
		for _, item := range d.Items {
			if item.Consent {
				status := "Not allowed"
				if form.Satisfied(item, d.Answers[item.ID]) && len(m.missingForms()) == 0 {
					status = "Allow on approval"
				}
				lines = append(lines, status+": "+d.Name+" / "+item.ID+" — "+item.Label)
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return lipgloss.Wrap(strings.Join(lines, "\n"), width, "")
}
