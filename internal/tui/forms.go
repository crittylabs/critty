package tui

import (
	"crypto/sha256"
	"fmt"
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
func (m *AppModel) formDisplayLine(line int, raw string) (string, int, bool) {
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
		label := highlightInline(strings.TrimRight(raw, " \t\r"))
		if checkbox.Required {
			label = lipgloss.NewStyle().Bold(true).Render(label)
		}
		indent := lipgloss.Width(expandDisplayTabs(raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]))
		return label, indent, true
	}
	column := checkbox.Offset - row.offset
	// Preserve nesting without treating an ordered-list marker as indentation.
	prefix := raw[:column]
	indent := expandDisplayTabs(prefix[:len(prefix)-len(strings.TrimLeft(prefix, " \t"))]) + "  "
	label := highlightInline(strings.TrimSpace(raw[column+3:]))
	answer := d.Answers[checkbox.ID]
	selected := form.Selected(*checkbox, answer)
	style := lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	if checkbox.Required && !form.Satisfied(*checkbox, answer) {
		style = style.Foreground(lipgloss.Red)
	}
	symbol := "☐"
	if checkbox.Choice {
		symbol = "○"
		if selected {
			symbol = "●"
		}
	} else if selected {
		symbol = "☑"
	}
	if checkbox.Required {
		label = lipgloss.NewStyle().Bold(true).Render(label) + " " + style.Bold(true).Render("Required")
	}
	if checkbox.Consent {
		label += " " + style.Render("Consent")
	}
	return indent + style.Bold(true).Render(symbol) + " " + label, lipgloss.Width(indent), true
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
