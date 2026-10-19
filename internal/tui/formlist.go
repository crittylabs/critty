package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/form"
)

type formListState struct {
	selected       int
	unansweredOnly bool
}

// formListRow is one field of the answer list with the section asking it.
type formListRow struct {
	formReviewItem
	document, section string
	missing           bool
	answer            string
}

// formListRows lists each field once, under the nearest preceding heading.
func (m *AppModel) formListRows() []formListRow {
	var rows []formListRow
	headings := map[int][]form.Heading{}
	for _, field := range m.formReviewItems() {
		if m.formList.unansweredOnly && field.answered {
			continue
		}
		d := m.forms.Document(m.tabs[field.tab].path)
		if _, ok := headings[field.tab]; !ok {
			found, err := form.Headings(d.Content)
			if err != nil {
				// Headings only label rows, so list the fields without them.
				found = []form.Heading{}
			}
			headings[field.tab] = found
		}
		section := ""
		for _, heading := range headings[field.tab] {
			if heading.Line < field.item.Line {
				section = heading.Text
			}
		}
		answer := d.Answers[field.item.ID]
		row := formListRow{formReviewItem: field, document: d.Name, section: section,
			missing: field.item.Required && !form.Satisfied(field.item, answer)}
		if field.answered {
			row.answer = m.formAnswerSummary(d.Items, field.item, answer)
		}
		rows = append(rows, row)
	}
	return rows
}

// formAnswerSummary describes an answer on one line.
func (m *AppModel) formAnswerSummary(items []form.Item, field form.Item, answer form.Answer) string {
	firstLine := func(text string) string {
		line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
		return line
	}
	switch {
	case field.Choice:
		for _, item := range items {
			if item.ID == field.ID && form.Selected(item, answer) {
				if item.Text && answer.Text != "" {
					return item.Label + " — " + firstLine(answer.Text)
				}
				return item.Label
			}
		}
		return ""
	case field.Text:
		text, _ := answer.Value.(string)
		return firstLine(text)
	case field.Consent:
		return "Allowed on approval"
	case field.Required:
		return "Confirmed"
	}
	return "Checked"
}

func (m *AppModel) openFormList() {
	if m.forms == nil || !m.forms.HasForms() {
		return
	}
	m.formList.selected = 0
	m.modal = formListModal
}

func (m *AppModel) pickFormListRow(index int) {
	rows := m.formListRows()
	m.modal = noModal
	if index < 0 || index >= len(rows) {
		return
	}
	row := rows[index]
	m.tabs[row.tab].selecting = false
	m.selectComment(row.tab, commentTarget{formID: row.item.ID, line: row.item.Line})
}

func (m *AppModel) toggleFormListFilter() {
	m.formList.unansweredOnly = !m.formList.unansweredOnly
	m.formList.selected = min(m.formList.selected, max(0, len(m.formListRows())-1))
}

func (m *AppModel) handleFormListKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	count := len(m.formListRows())
	switch msg.String() {
	case "esc", "q", "F", "ctrl+c":
		m.modal = noModal
	case "enter":
		m.pickFormListRow(m.formList.selected)
	case "down", "j", "ctrl+n":
		if count > 0 {
			m.formList.selected = (m.formList.selected + 1) % count
		}
	case "up", "k", "ctrl+p":
		if count > 0 {
			m.formList.selected = (m.formList.selected + count - 1) % count
		}
	case "u":
		m.toggleFormListFilter()
	}
	return m, nil
}

func (m *AppModel) handleFormListMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		switch {
		case region.action.close:
			m.modal = noModal
		case region.action.pick:
			m.pickFormListRow(region.action.pickIndex)
		case region.action.focus == 1:
			m.toggleFormListFilter()
		}
		return m, nil
	}
	return m, nil
}

// formListModalContent shows headings between rows, keeping the selected
// row visible within maxLines.
func (m AppModel) formListModalContent(width, maxLines int) (string, []modalMouseRegion) {
	title, closeRegion := modalTitle("Answers", width)
	content := title + "\n"
	filter, filterRegions := layoutModalButtonRow([]modalButtonSpec{{
		rendered: renderResolution(m.formList.unansweredOnly, "Unanswered only", false) + " " + modalBtnNormalKey.Render("u"),
		action:   modalMouseAction{focus: 1},
	}}, width, strings.Count(content, "\n"))
	content += filter + "\n\n"
	regions := append([]modalMouseRegion{closeRegion}, filterRegions...)

	rows := m.formListRows()
	type line struct {
		text string
		row  int
	}
	var lines []line
	document, section := "", ""
	multiple := len(m.forms.Documents) > 1
	selectedLine := 0
	for i, row := range rows {
		if multiple && row.document != document {
			document, section = row.document, ""
			lines = append(lines, line{lipgloss.NewStyle().Bold(true).Render(ansi.Truncate(row.document, width, "…")), -1})
		}
		if row.section != section {
			section = row.section
			lines = append(lines, line{helpHeadingStyle.Render(ansi.Truncate(section, width, "…")), -1})
		}
		status, statusColor := "○", formChip.cyan
		switch {
		case row.missing:
			status, statusColor = "!", formChip.red
		case row.answered:
			status, statusColor = "✓", formChip.green
		}
		label := row.item.Label
		if row.item.Choice {
			label = row.item.ID
		}
		text := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(status) + " " + label
		if row.answer != "" {
			text += footerStyle.Render(" · " + row.answer)
		}
		text = ansi.Truncate(text, width, "…")
		if i == m.formList.selected {
			selectedLine = len(lines)
			text = lipgloss.NewStyle().Reverse(true).Render(ansi.Strip(text))
		}
		lines = append(lines, line{text, i})
	}
	available := max(1, maxLines)
	start, end := fileSelectWindow(len(lines), selectedLine, available)
	for _, l := range lines[start:end] {
		if l.row >= 0 {
			top := strings.Count(content, "\n")
			regions = append(regions, modalMouseRegion{
				rect:   mouseRect{top: top, bottom: top + 1, right: width},
				action: modalMouseAction{pick: true, pickIndex: l.row},
			})
		}
		content += l.text + "\n"
	}
	if len(rows) == 0 {
		content += footerStyle.Render("No unanswered items") + "\n"
	}
	answered := 0
	for _, field := range m.formReviewItems() {
		if field.answered {
			answered++
		}
	}
	status := fmt.Sprintf("%d of %d answered", answered, len(m.formReviewItems()))
	if len(lines) > available {
		status += fmt.Sprintf(" · %d-%d of %d lines", start+1, end, len(lines))
	}
	return content + "\n" + footerStyle.Render(status) + "\n" + footerStyle.Render("↑/↓ choose · RET go to · u filter · ESC close"), regions
}
