package tui

import (
	"crypto/sha256"
	"fmt"
	"image/color"
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
		seen := map[string]bool{}
		for _, item := range d.Items {
			if seen[item.ID] {
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
			items = append(items, formReviewItem{tab: tabIndex, item: item, answered: answered})
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

func (m *AppModel) indexForms() {
	m.formRows = map[string]map[int]formRow{}
	for i := range m.forms.Documents {
		d := &m.forms.Documents[i]
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
		if tab.isBinary && !tab.isDeleted && m.patch == nil {
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
	m.indexForms()
	return nil
}

func (m *AppModel) missingForms() []string {
	if m.forms == nil {
		return nil
	}
	return m.forms.Missing()
}

func (m *AppModel) toggleForm(line int) bool {
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return false
	}
	for _, item := range d.Items {
		if line < item.Line || line > item.EndLine {
			continue
		}
		m.tab().cursorOnForm = true
		var value any = !form.Selected(item, d.Answers[item.ID])
		if item.Choice {
			value = item.Value
		}
		previous := d.Answers[item.ID]
		d.Answers[item.ID] = form.Answer{Value: value, Human: true, Author: m.author,
			Revision: m.forms.Revision, Round: m.session.CJ.ReviewRound}
		if err := m.session.SaveForms(*m.forms); err != nil {
			d.Answers[item.ID] = previous
			m.err = err
		} else {
			m.err = nil
		}
		m.rebuildContent()
		return true
	}
	return false
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
		return strings.TrimRight(raw, " \t\r"), -1, true
	}
	if checkbox.Line != line {
		label := highlightInlineWith(strings.TrimSpace(raw), formChip.link, formChip.code)
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
	label := highlightInlineWith(strings.TrimSpace(raw[column+3:]), formChip.link, formChip.code)
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
	if checkbox.Choice {
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
	if checkbox.Consent {
		label += " " + tag.Render("Consent")
	}
	badge := lipgloss.NewStyle().Foreground(formChip.badgeSymbol).Background(tint).Bold(true).Render(" " + symbol + " ")
	return indent + badge + inlineColors(background, " "+label, true), lipgloss.Width(indent), true
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
