package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

func layoutModalButtonRow(specs []modalButtonSpec, width, top int) (string, []modalMouseRegion) {
	var row strings.Builder
	regions := make([]modalMouseRegion, 0, len(specs))
	x, y := 0, top
	for _, spec := range specs {
		buttonWidth := lipgloss.Width(spec.rendered)
		gap := 0
		if x > 0 {
			gap = 2
		}
		if x > 0 && x+gap+buttonWidth > width {
			row.WriteByte('\n')
			x = 0
			y++
			gap = 0
		}
		if gap > 0 {
			row.WriteString("  ")
			x += gap
		}
		row.WriteString(spec.rendered)
		regions = append(regions, modalMouseRegion{
			rect: mouseRect{
				left: x, top: y,
				right: x + buttonWidth, bottom: y + lipgloss.Height(spec.rendered),
			},
			action: spec.action,
		})
		x += buttonWidth
	}
	return row.String(), regions
}

// layoutModalActions places secondary actions before the first (primary)
// action and right-aligns each wrapped row, including its mouse targets.
func layoutModalActions(specs []modalButtonSpec, width, top int) (string, []modalMouseRegion) {
	if len(specs) == 0 {
		return "", nil
	}
	ordered := append(slices.Clone(specs[1:]), specs[0])
	content, regions := layoutModalButtonRow(ordered, width, top)
	rows := strings.Split(content, "\n")
	for i := range regions {
		row := regions[i].rect.top - top
		offset := max(0, width-lipgloss.Width(rows[row]))
		regions[i].rect.left += offset
		regions[i].rect.right += offset
	}
	for i := range rows {
		rows[i] = strings.Repeat(" ", max(0, width-lipgloss.Width(rows[i]))) + rows[i]
	}
	return strings.Join(rows, "\n"), regions
}

func (m *AppModel) cycleModalFocus(order []int, backward bool) {
	step := 1
	if backward {
		step = -1
	}
	index := max(0, slices.Index(order, m.modalFocus))
	m.modalFocus = order[(index+step+len(order))%len(order)]
}
