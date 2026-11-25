package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/v2/internal/form"
	"github.com/crittylabs/critty/v2/internal/icons"
	"github.com/crittylabs/critty/v2/internal/review"
)

// rankLayout maps a ranking in display order back to its source lines.
type rankLayout struct {
	nodes  []form.RankNode
	ranked bool
	items  map[string]form.Item
	// start is the first source line of the complete movable block, or zero.
	start int
	// tree records whether the source or answer uses nesting.
	tree        bool
	depthLimits bool
}

func (m *AppModel) rankLayout(d *review.FormDocument, id string) rankLayout {
	answer := d.Answers[id]
	if drag := m.rankDrag; drag.path == d.Path() && drag.id == id && drag.preview != nil {
		answer = *drag.preview
	}
	return m.rankLayoutForAnswer(d, id, answer)
}

func (m *AppModel) rankLayoutForAnswer(d *review.FormDocument, id string, answer form.Answer) rankLayout {
	nodes, ranked := form.RankTree(d.Items, id, answer)
	layout := rankLayout{nodes: nodes, ranked: ranked, items: map[string]form.Item{}, tree: len(answer.RankParents) != 0}
	var previous form.Item
	for _, item := range d.Items {
		if item.ID != id || !item.Rank {
			continue
		}
		if previous.Line == 0 {
			layout.start = item.Line
		} else if layout.start != 0 && item.Line != previous.EndLine+1 {
			rows := m.formRows[d.Path()]
			from, to := rows[previous.EndLine].offset, rows[item.Line].offset
			_, gap, _ := strings.Cut(d.Content[from:to], "\n")
			if strings.TrimSpace(gap) == "" {
				previous.EndLine = item.Line - 1
				layout.items[previous.Value] = previous
			} else {
				layout.start = 0
			}
		}
		previous = item
		layout.items[item.Value] = item
		layout.tree = layout.tree || item.RankParent != ""
		layout.depthLimits = layout.depthLimits || item.RankMaxDepth != nil || item.RankMinLevel != 0 || item.RankMaxLevel != 0
	}
	return layout
}

func (l rankLayout) position(line int) int {
	return slices.IndexFunc(l.nodes, func(node form.RankNode) bool {
		return l.items[node.Value].Line == line
	})
}

func (l rankLayout) end(index int) int {
	depth := l.nodes[index].Depth
	for index++; index < len(l.nodes) && l.nodes[index].Depth > depth; index++ {
	}
	return index
}

func (l rankLayout) previousSibling(index int) int {
	depth := l.nodes[index].Depth
	for i := index - 1; i >= 0 && l.nodes[i].Depth >= depth; i-- {
		if l.nodes[i].Depth == depth {
			return i
		}
	}
	return -1
}

func (l rankLayout) parent(index int) int {
	parent := l.nodes[index].Parent
	return slices.IndexFunc(l.nodes[:index], func(node form.RankNode) bool {
		return node.Value == parent
	})
}

// depthDestination reports the gap and parent for a one-level change.
func (l rankLayout) depthDestination(index int, indent bool) (int, string, bool) {
	if l.start != 0 {
		if indent {
			if sibling := l.previousSibling(index); sibling >= 0 {
				return index, l.nodes[sibling].Value, true
			}
		} else if parent := l.parent(index); parent >= 0 {
			return l.end(parent), l.nodes[parent].Parent, true
		}
	}
	return 0, "", false
}

// changeDepth indents under the previous sibling or outdents after the parent.
func (l rankLayout) changeDepth(index int, indent bool) []form.RankNode {
	gap, parent, ok := l.depthDestination(index, indent)
	if !ok {
		return nil
	}
	return l.relocate(index, gap, parent)
}

// nextAllowedParent finds room at the same depth when a vertical move cannot
// promote a branch. Move to the edge of the nearest parent that accepts it.
func (l rankLayout) nextAllowedParent(items []form.Item, id string, index int, down bool) []form.RankNode {
	parent := l.parent(index)
	if parent < 0 || l.start == 0 {
		return nil
	}
	position, step := parent-1, -1
	if down {
		position, step = l.end(parent), 1
	}
	for ; position >= 0 && position < len(l.nodes); position += step {
		next := l.nodes[position]
		if next.Depth != l.nodes[parent].Depth {
			continue
		}
		gap := l.end(position)
		if down {
			gap = position + 1
		}
		nodes := l.relocate(index, gap, next.Value)
		if form.CheckRank(items, id, rankAnswer(nodes)) == nil {
			return nodes
		}
	}
	return nil
}

// relocate moves an entire subtree to the gap before index at the new parent.
func (l rankLayout) relocate(from, index int, parent string) []form.RankNode {
	end := l.end(from)
	if index > from && index < end {
		return l.nodes
	}
	branch := slices.Clone(l.nodes[from:end])
	branch[0].Parent = parent
	nodes := slices.Delete(slices.Clone(l.nodes), from, end)
	if index >= end {
		index -= end - from
	}
	return slices.Insert(nodes, index, branch...)
}

func rankAnswer(nodes []form.RankNode) form.Answer {
	order := make([]string, len(nodes))
	var parents map[string]string
	for i, node := range nodes {
		order[i] = node.Value
		if node.Parent != "" {
			if parents == nil {
				parents = map[string]string{}
			}
			parents[node.Value] = node.Parent
		}
	}
	return form.Answer{Value: order, RankParents: parents}
}

// lineOrder lists source line indices in display order.
func (m *AppModel) lineOrder(t *FileTab) []int {
	order := make([]int, len(t.doc.Lines))
	for i := range order {
		order[i] = i
	}
	if m.forms == nil || t.markdownSourceView {
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
		if layout.start == 0 {
			continue
		}
		slot := layout.start - 1
		for _, node := range layout.nodes {
			item := layout.items[node.Value]
			for line := item.Line; line <= item.EndLine; line++ {
				order[slot] = line - 1
				slot++
			}
		}
	}
	return order
}

// rankMove reports whether an item is displayed above or below its source line.
func (m *AppModel) rankMove(t *FileTab, line int) int {
	if t.markdownSourceView {
		return 0
	}
	row := m.formRows[t.path][line]
	if row.item == nil || !row.item.Rank || row.item.Line != line {
		return 0
	}
	layout := m.rankLayout(m.forms.Document(t.path), row.item.ID)
	if !layout.ranked || layout.start == 0 {
		return 0
	}
	slot := layout.start
	for _, node := range layout.nodes {
		item := layout.items[node.Value]
		if item.Line == line {
			return min(1, max(-1, slot-line))
		}
		slot += item.EndLine - item.Line + 1
	}
	return 0
}

// moveRank applies a keyboard operation to a ranked subtree.
func (m *AppModel) moveRank(line int, key string) bool {
	t := m.tab()
	if m.readOnly || m.forms == nil || t.markdownSourceView {
		return false
	}
	d := m.forms.Document(t.path)
	row := m.formRows[t.path][line]
	if d == nil || row.item == nil || !row.item.Rank {
		return false
	}
	if row.item.RankFixed {
		m.locationError = fmt.Sprintf("%q is fixed", row.item.Label)
		return true
	}
	layout := m.rankLayout(d, row.item.ID)
	position := layout.position(row.item.Line)
	nodes := layout.nodes
	node := nodes[position]
	switch key {
	case "alt+up":
		if sibling := layout.previousSibling(position); sibling >= 0 {
			nodes = layout.relocate(position, sibling, node.Parent)
		} else if parent := layout.parent(position); layout.start != 0 && parent >= 0 {
			nodes = layout.relocate(position, parent, nodes[parent].Parent)
		}
	case "alt+down":
		if sibling := layout.end(position); sibling < len(nodes) && nodes[sibling].Parent == node.Parent {
			nodes = layout.relocate(position, layout.end(sibling), node.Parent)
		} else if parent := layout.parent(position); layout.start != 0 && parent >= 0 {
			nodes = layout.relocate(position, layout.end(parent), nodes[parent].Parent)
		}
	case "alt+left", "alt+b", "alt+right", "alt+f":
		nodes = layout.changeDepth(position, key == "alt+right" || key == "alt+f")
		if nodes == nil {
			return true
		}
	default:
		return false
	}
	answer := rankAnswer(nodes)
	if (key == "alt+up" || key == "alt+down") && form.CheckRank(d.Items, row.item.ID, answer) != nil {
		if next := layout.nextAllowedParent(d.Items, row.item.ID, position, key == "alt+down"); next != nil {
			answer = rankAnswer(next)
		}
	}
	m.saveRank(d, row.item.ID, answer)
	t.cursorLine, t.cursorOnForm = row.item.Line, true
	m.rebuildContent()
	m.scrollToCursor()
	return true
}

// saveRank records an order, restoring the previous answers if saving fails.
func (m *AppModel) saveRank(d *review.FormDocument, id string, answer form.Answer) {
	if err := form.CheckRank(d.Items, id, answer); err != nil {
		m.locationError = err.Error()
		return
	}
	previous := d.Answers[id]
	answer.Human, answer.Author = true, m.author
	answer.Revision, answer.Round = m.forms.Revision, m.session.CJ.ReviewRound
	d.Answers[id] = answer
	if err := m.session.SaveForms(*m.forms); err != nil {
		d.Answers[id] = previous
		m.err = err
		return
	}
	m.err = nil
}

const rankIndentWidth = 2

func rankSourceIndent(raw string) int {
	return lipgloss.Width(expandDisplayTabs(raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]))
}

func (m *AppModel) rankIndent(d *review.FormDocument, item *form.Item, indent int) int {
	layout := m.rankLayout(d, item.ID)
	position := layout.position(item.Line)
	if layout.start == 0 || !layout.tree {
		return indent
	}
	start := m.formRows[d.Path()][layout.start].offset
	return rankSourceIndent(d.Content[start:]) + layout.nodes[position].Depth*rankIndentWidth
}

func (m *AppModel) rankHandle(d *review.FormDocument, item *form.Item, layout rankLayout, position int) string {
	style := lipgloss.NewStyle().Foreground(formChip.rankHandleForeground)
	if item.RankFixed {
		return style.Render(m.icons.Glyph(icons.Lock, "▣"))
	}
	symbol := m.icons.Glyph(icons.MoveVertical, "↕")
	for _, indent := range []bool{false, true} {
		gap, parent, ok := layout.depthDestination(position, indent)
		if ok && (!layout.depthLimits || form.CheckRank(d.Items, item.ID, rankAnswer(layout.relocate(position, gap, parent))) == nil) {
			symbol = m.icons.Glyph(icons.MoveFourDirections, "✥")
			break
		}
	}
	return style.Render(symbol)
}

// rankDisplay uses saved numbers even while a drag previews new positions.
func (m *AppModel) rankDisplay(d *review.FormDocument, item *form.Item, label string, background lipgloss.Style, focused bool) string {
	layout := m.rankLayoutForAnswer(d, item.ID, d.Answers[item.ID])
	position := layout.position(item.Line)
	nodes := layout.nodes
	depth, number := nodes[position].Depth, 1
	for i := position - 1; i >= 0 && nodes[i].Depth >= depth; i-- {
		if nodes[i].Depth == depth {
			number++
		}
	}
	tint, tagColor := formChip.rankPendingBadge, formChip.cyan
	if layout.ranked {
		tint, tagColor = formChip.rankSavedBadge, formChip.green
	} else if item.Required {
		tint, tagColor = formChip.rankRequiredBadge, formChip.red
	}
	tag := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
	if item.Required {
		label = lipgloss.NewStyle().Bold(true).Render(label) + " " + tag.Render("Required")
	}
	if item.RankFixed {
		tint = formChip.fixedRankBadge
	}
	badge := lipgloss.NewStyle().Foreground(formChip.rankBadgeSymbol).Background(tint).Bold(true).Render(fmt.Sprintf(" %d ", number))
	content := inlineColors(background, " "+label, true)
	if focused {
		content = sourceLineHighlight(background, " "+label)
	}
	return m.rankHandle(d, item, layout, position) + " " + badge + content
}

type rankGroupRow struct {
	row, offset int
	group       int
}

// addRankGroupBrackets fills the reserved cell on each rendered rank row.
// Interrupted runs close and reopen with the same color, including when an
// inline comment separates two items of the group.
func addRankGroupBrackets(content string, rows []rankGroupRow, groups int) string {
	if len(rows) == 0 {
		return content
	}
	colors := rankGroupColors(groups, lipgloss.LightDark(terminalIsDark))
	var result strings.Builder
	result.Grow(len(content))
	from := 0
	for i, mark := range rows {
		continues := i > 0 && rows[i-1].group == mark.group && rows[i-1].row+1 == mark.row
		continuesAfter := i+1 < len(rows) && rows[i+1].group == mark.group && mark.row+1 == rows[i+1].row
		bracket := "⎜"
		switch {
		case !continues && continuesAfter:
			bracket = "⎛"
		case continues && !continuesAfter:
			bracket = "⎝"
		}
		result.WriteString(content[from:mark.offset])
		result.WriteString(lipgloss.NewStyle().Foreground(colors[mark.group]).Render(bracket))
		from = mark.offset + 1
	}
	result.WriteString(content[from:])
	return result.String()
}

// rankDragState follows a ranked item dragged with the mouse.  The display
// reorders as it moves; the answer is saved once on release.
type rankDragState struct {
	active  bool
	moved   bool
	fixed   bool
	origin  tea.Mouse
	line    int
	path    string
	id      string
	preview *form.Answer
}

func (m *AppModel) startRankDrag(line int, mouse tea.Mouse) bool {
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
	m.rankDrag = rankDragState{active: true, fixed: row.item.RankFixed, origin: mouse, line: row.item.Line, path: d.Path(), id: row.item.ID}
	return true
}

// dragRank moves the dragged item to the position of the ranked item under
// the pointer, without saving.
func (m *AppModel) dragRank(mouse tea.Mouse) {
	drag := &m.rankDrag
	if mouse.X != drag.origin.X || mouse.Y != drag.origin.Y {
		drag.moved = true
	}
	if drag.fixed {
		return
	}
	d := m.forms.Document(m.tab().path)
	if d == nil || d.Path() != drag.path || m.tab().markdownSourceView {
		return
	}
	if mouse.Y == drag.origin.Y {
		// Use the original order so another motion on the press row does not
		// change depth again or pick up the row that moved into this spot.
		layout := m.rankLayoutForAnswer(d, drag.id, d.Answers[drag.id])
		delta := mouse.X - drag.origin.X
		var nodes []form.RankNode
		if delta <= -rankIndentWidth || delta >= rankIndentWidth {
			nodes = layout.changeDepth(layout.position(drag.line), delta > 0)
		}
		m.previewRank(d, nodes)
		return
	}
	left, top, right, bottom := m.contentBounds()
	if mouse.X < left || mouse.X >= right || mouse.Y < top || mouse.Y >= bottom {
		return
	}
	target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset())
	if !ok || target.annotation {
		return
	}
	row, ok := m.formRows[d.Path()][target.line]
	if !ok || row.item == nil || row.item.ID != drag.id || row.item.Line == drag.line {
		return
	}
	layout := m.rankLayout(d, drag.id)
	from, to := layout.position(drag.line), layout.position(row.item.Line)
	if to > from && to < layout.end(from) {
		return
	}
	parent, index := layout.nodes[to].Parent, to
	if layout.start == 0 && parent != layout.nodes[from].Parent {
		return
	}
	if layout.start != 0 && mouse.X >= drag.origin.X+rankIndentWidth {
		parent, index = layout.nodes[to].Value, layout.end(to)
	} else if from < to {
		index = layout.end(to)
	}
	m.previewRank(d, layout.relocate(from, index, parent))
}

func (m *AppModel) previewRank(d *review.FormDocument, nodes []form.RankNode) {
	drag := &m.rankDrag
	previous := drag.preview
	drag.preview, m.locationError = nil, ""
	if nodes != nil {
		answer := rankAnswer(nodes)
		if err := form.CheckRank(d.Items, drag.id, answer); err != nil {
			m.locationError = err.Error()
		} else {
			drag.preview = &answer
		}
	}
	if previous != nil || drag.preview != nil {
		m.rebuildContent()
	}
}

// finishRankDrag saves a dragged order, or treats a press without movement
// as a click that confirms the current order.
func (m *AppModel) finishRankDrag() {
	drag := m.rankDrag
	m.rankDrag = rankDragState{}
	d := m.forms.Document(m.tab().path)
	if d == nil || d.Path() != drag.path || m.tab().markdownSourceView {
		return
	}
	if drag.preview == nil {
		if !drag.moved {
			m.toggleForm(drag.line)
		}
		return
	}
	m.saveRank(d, drag.id, *drag.preview)
	m.tab().cursorLine, m.tab().cursorOnForm = drag.line, true
	m.rebuildContent()
}
