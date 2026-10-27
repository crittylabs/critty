package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/review"
)

// commentLocation is a display location only; saved coordinates and scope stay intact.
func commentLocation(t *FileTab, c review.Comment) lineRef {
	if isGeneralCommentScope(c.Scope) || t.doc == nil || t.isBinary {
		return lineRef{}
	}
	if c.Side == "old" {
		for _, lines := range t.deletedAfter {
			for _, line := range lines {
				if line.OldLineNum == c.EndAt() {
					return lineRef{side: c.Side, line: c.EndAt()}
				}
			}
		}
	} else if t.doc.HasLine(c.EndAt()) {
		return lineRef{line: c.EndAt()}
	}
	return lineRef{}
}

func commentDrifted(t *FileTab, c review.Comment) bool {
	return !isGeneralCommentScope(c.Scope) && (c.Drifted || commentLocation(t, c).line == 0)
}

func tabAnnotation(t *FileTab, c review.Comment) annotation {
	ann := newAnnotation(c)
	ann.drifted = commentDrifted(t, c)
	return ann
}

func driftedContext(c review.Comment) string {
	quote := c.Quote
	if quote == "" {
		quote = c.Anchor
	}
	context := "Position could not be tracked."
	if quote != "" {
		context += "\nOriginal text:\n> " + strings.ReplaceAll(quote, "\n", "\n> ")
	}
	return context
}

func (m *AppModel) driftedComment(id string) (review.Comment, bool) {
	if m.tab().state != nil {
		for _, c := range m.tab().state.Comments {
			if c.ID == id {
				return c, commentDrifted(m.tab(), c)
			}
		}
	}
	return review.Comment{}, false
}

type commentMarker struct {
	rect mouseRect
	ids  []string
}

func (m *AppModel) driftedGroups() map[lineRef][]string {
	groups := make(map[lineRef][]string)
	if m.tab().state != nil {
		for _, c := range m.tab().state.Comments {
			if commentDrifted(m.tab(), c) {
				ref := commentLocation(m.tab(), c)
				groups[ref] = append(groups[ref], c.ID)
			}
		}
	}
	return groups
}

// decorateDrifted puts one marker on the last source row at each display location.
func (m *AppModel) decorateDrifted(content string, layout *renderedContentLayout, groups map[lineRef][]string) string {
	if len(groups) == 0 {
		return content
	}
	rows := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	last := make(map[lineRef]int)
	for i, target := range layout.rows {
		if !target.annotation && (target.line > 0 || target.drifted) {
			last[lineRef{side: target.side, line: target.line}] = i
		}
	}
	for i, row := range layout.columns {
		if row.left.line > 0 && !row.left.passive {
			last[lineRef{side: row.left.side, line: row.left.line}] = i
		}
	}
	for ref, ids := range groups {
		y, ok := last[ref]
		if !ok {
			continue
		}
		label := commentLineStyle.Faint(true).Render("💬")
		resolved := true
		for _, id := range ids {
			c, _ := m.driftedComment(id)
			resolved = resolved && c.Resolved
		}
		for _, id := range ids {
			if id == m.selectedCommentID() {
				c, _ := m.driftedComment(id)
				resolved = c.Resolved
				break
			}
		}
		if len(ids) > 1 {
			label += commentLineStyle.Foreground(lipgloss.Blue).Render(fmt.Sprintf(" %d", len(ids)))
		}
		label = renderResolution(resolved, "", false, false) + " " + label
		prefix := "  "
		for _, id := range ids {
			if id == m.selectedCommentID() {
				prefix = cursorMarker.Render(">") + " "
				break
			}
		}
		x := max(0, m.contentViewport.Width()-lipgloss.Width(label)-2)
		rows[y] = lipgloss.NewStyle().Width(x).Render(rows[y]) + prefix + label
		layout.markers = append(layout.markers, commentMarker{
			rect: mouseRect{left: x, right: m.contentViewport.Width(), top: y, bottom: y + 1}, ids: ids,
		})
	}
	return strings.Join(rows, "\n") + "\n"
}

func (m *AppModel) selectMarker(ids []string) {
	id := ids[0]
	selected := m.selectedCommentID()
	for _, candidate := range ids {
		if candidate == selected {
			id = selected
			break
		}
	}
	if m.viewMode || m.openAll {
		m.openCommentThread(id)
		return
	}
	if len(ids) > 1 {
		t := m.tab()
		if t.expandedDrifted == nil {
			t.expandedDrifted = make(map[string]bool)
		}
		expand := false
		for _, id := range ids {
			c, _ := m.driftedComment(id)
			expand = expand || m.threadCollapsed(id, c.Resolved, true, id == selected)
		}
		for _, id := range ids {
			t.expandedDrifted[id] = expand
			delete(t.foldedComments, id)
		}
	}
	for _, target := range m.commentTargets(m.activeTab) {
		if target.id == id {
			m.selectComment(m.activeTab, target)
			if len(ids) == 1 {
				m.toggleDrifted(id)
			}
			return
		}
	}
}

func (m *AppModel) driftedExpanded(id string, resolved bool) bool {
	if m.viewMode {
		return false
	}
	if m.openAll {
		return true
	}
	if expanded, ok := m.tab().expandedDrifted[id]; ok {
		return expanded
	}
	return !resolved
}

func (m *AppModel) toggleDrifted(id string) {
	if m.viewMode || m.openAll {
		m.openCommentThread(id)
		return
	}
	if c, _ := m.driftedComment(id); c.Resolved {
		m.openCommentThread(id)
		return
	}
	t := m.tab()
	if t.expandedDrifted == nil {
		t.expandedDrifted = make(map[string]bool)
	}
	t.expandedDrifted[id] = m.threadCollapsed(id, false, true, m.selectedCommentID() == id)
	delete(t.foldedComments, id)
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}
