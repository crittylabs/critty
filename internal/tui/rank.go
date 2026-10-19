package tui

import (
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/review"
)

// rankLayout describes how the items of one ranking are displayed.
type rankLayout struct {
	order  []string // values from first to last
	ranked bool
	// lines maps each value to its source line.
	lines map[string]int
	// slots lists the source lines the field occupies, in source order, when
	// the field is a contiguous block of single-line items that can be
	// displayed in ranked order.
	slots []int
}

func (m *AppModel) rankLayout(d *review.FormDocument, id string) rankLayout {
	order, ranked := form.RankOrder(d.Items, id, d.Answers[id])
	layout := rankLayout{order: order, ranked: ranked, lines: map[string]int{}}
	contiguous := true
	for _, item := range d.Items {
		if item.ID != id || !item.Rank {
			continue
		}
		layout.lines[item.Value] = item.Line
		layout.slots = append(layout.slots, item.Line)
		contiguous = contiguous && item.EndLine == item.Line
	}
	slices.Sort(layout.slots)
	if !contiguous || layout.slots[len(layout.slots)-1]-layout.slots[0] != len(layout.slots)-1 {
		layout.slots = nil
	}
	return layout
}

// position returns the zero-based rank of the item on line.
func (l rankLayout) position(line int) int {
	for i, value := range l.order {
		if l.lines[value] == line {
			return i
		}
	}
	return -1
}

// lineOrder lists the document's line indices in display order, showing each
// contiguous ranking in its ranked order.
func (m *AppModel) lineOrder(t *FileTab) []int {
	order := make([]int, len(t.doc.Lines))
	for i := range order {
		order[i] = i
	}
	if m.forms == nil {
		return order
	}
	d := m.forms.Document(t.path)
	if d == nil {
		return order
	}
	seen := map[string]bool{}
	for _, item := range d.Items {
		if !item.Rank || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		layout := m.rankLayout(d, item.ID)
		if layout.slots == nil {
			continue
		}
		for i, value := range layout.order {
			order[layout.slots[i]-1] = layout.lines[value] - 1
		}
	}
	return order
}

// rankMove reports whether the ranked item on line is displayed above (-1)
// or below (1) its source position, or 0.
func (m *AppModel) rankMove(t *FileTab, line int) int {
	row, ok := m.formRows[t.path][line]
	if !ok || row.item == nil || !row.item.Rank || row.item.Line != line {
		return 0
	}
	layout := m.rankLayout(m.forms.Document(t.path), row.item.ID)
	if !layout.ranked || layout.slots == nil {
		return 0
	}
	position := layout.position(line)
	switch {
	case layout.slots[position] < line:
		return -1
	case layout.slots[position] > line:
		return 1
	}
	return 0
}

// moveRank swaps the ranked item on line with its neighbor in direction and
// records the resulting order as a human answer.
func (m *AppModel) moveRank(line, direction int) bool {
	if m.readOnly || m.forms == nil {
		return false
	}
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return false
	}
	row, ok := m.formRows[d.Path()][line]
	if !ok || row.item == nil || !row.item.Rank {
		return false
	}
	layout := m.rankLayout(d, row.item.ID)
	position := layout.position(row.item.Line)
	order := slices.Clone(layout.order)
	// Moving past either end confirms the order without changing it.
	if next := position + direction; next >= 0 && next < len(order) {
		order[position], order[next] = order[next], order[position]
	}
	m.saveRank(d, row.item.ID, order)
	m.tab().cursorLine, m.tab().cursorOnForm = row.item.Line, true
	m.rebuildContent()
	m.scrollToCursor()
	return true
}

// saveRank records order, restoring the previous answers if saving fails.
func (m *AppModel) saveRank(d *review.FormDocument, id string, order []string) {
	previous := maps.Clone(d.Answers)
	d.Answers[id] = form.Answer{Value: order, Human: true, Author: m.author, Revision: m.forms.Revision, Round: m.session.CJ.ReviewRound}
	if err := m.session.SaveForms(*m.forms); err != nil {
		d.Answers = previous
		m.err = err
		return
	}
	m.err = nil
}

// rankDisplay numbers a ranked item by its position, marking the field as
// unranked until the reviewer confirms or changes the order.
func (m *AppModel) rankDisplay(d *review.FormDocument, item *form.Item, label string, background lipgloss.Style) string {
	layout := m.rankLayout(d, item.ID)
	tint, tagColor, symbol := color.Color(lipgloss.Cyan), formChip.cyan, strconv.Itoa(layout.position(item.Line)+1)
	if layout.ranked {
		tint, tagColor = lipgloss.Green, formChip.green
	} else if item.Required {
		tint, tagColor = formChip.requiredBadge, formChip.red
	}
	tag := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
	if item.Required {
		label = lipgloss.NewStyle().Bold(true).Render(label) + " " + tag.Render("Required")
	}
	if !layout.ranked && layout.position(item.Line) == 0 {
		label += " " + tag.Render("Unranked")
	}
	badge := lipgloss.NewStyle().Foreground(formChip.badgeSymbol).Background(tint).Bold(true).Render(fmt.Sprintf("%2s ", symbol))
	return badge + inlineColors(background, " "+label, true)
}

// rankDragState follows a ranked item dragged with the mouse.  The display
// reorders as it moves; the answer is saved once on release.
type rankDragState struct {
	active bool
	moved  bool
	line   int
	id     string
	// before restores the answers if saving the dragged order fails.
	before map[string]form.Answer
}

func (m *AppModel) startRankDrag(line int) bool {
	if m.readOnly || m.forms == nil {
		return false
	}
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return false
	}
	row, ok := m.formRows[d.Path()][line]
	if !ok || row.item == nil || !row.item.Rank {
		return false
	}
	m.rankDrag = rankDragState{active: true, line: row.item.Line, id: row.item.ID, before: maps.Clone(d.Answers)}
	return true
}

// dragRank moves the dragged item to the position of the ranked item under
// the pointer, without saving.
func (m *AppModel) dragRank(mouse tea.Mouse) {
	left, top, _, bottom := m.contentBounds()
	if mouse.Y < top || mouse.Y >= bottom {
		return
	}
	target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset())
	d := m.forms.Document(m.tab().path)
	if !ok || target.annotation || d == nil {
		return
	}
	row, ok := m.formRows[d.Path()][target.line]
	if !ok || row.item == nil || row.item.ID != m.rankDrag.id || row.item.Line == m.rankDrag.line {
		return
	}
	layout := m.rankLayout(d, m.rankDrag.id)
	from, to := layout.position(m.rankDrag.line), layout.position(row.item.Line)
	order := slices.Delete(slices.Clone(layout.order), from, from+1)
	order = slices.Insert(order, to, layout.order[from])
	d.Answers[m.rankDrag.id] = form.Answer{Value: order}
	m.rankDrag.moved = true
	m.rebuildContent()
}

// finishRankDrag saves a dragged order, or treats a press without movement
// as a click that confirms the current order.
func (m *AppModel) finishRankDrag() {
	drag := m.rankDrag
	m.rankDrag = rankDragState{}
	d := m.forms.Document(m.tab().path)
	if d == nil {
		return
	}
	if !drag.moved {
		m.toggleForm(drag.line)
		return
	}
	order := m.rankLayout(d, drag.id).order
	d.Answers = drag.before
	m.saveRank(d, drag.id, order)
	m.tab().cursorLine, m.tab().cursorOnForm = drag.line, true
	m.rebuildContent()
}
