package tui

import "charm.land/lipgloss/v2"

type tabBarItem struct {
	text        string
	left, width int
	target      int
	overflow    bool
}

// tabBarItems is shared by rendering and mouse targeting.  Overflow controls
// stay at the pane edges; spare space previews the next hidden tab or extends
// the control when a meaningful preview cannot fit.
func (m *AppModel) tabBarItems() []tabBarItem {
	labels := m.tabLabels()
	capacity := max(0, m.contentPaneWidth()-4)
	if len(labels) == 0 || capacity == 0 {
		return nil
	}
	prefix := make([]int, len(labels)+1)
	for i := range labels {
		labels[i].rendered = m.renderTab(labels, i)
		labels[i].width = lipgloss.Width(labels[i].rendered)
		prefix[i+1] = prefix[i] + labels[i].width
	}
	overflowWidth := func(count int, backwards bool) int {
		if count == 0 {
			return 0
		}
		return lipgloss.Width(m.renderTabOverflowIndicator(m.tabOverflowText(count, backwards)))
	}
	rangeWidth := func(start, end int) int {
		width := prefix[end] - prefix[start] + end - start - 1
		if start > 0 {
			width += overflowWidth(start, true) + 1
		}
		if end < len(labels) {
			width += overflowWidth(len(labels)-end, false) + 1
		}
		return width
	}
	start, end := 0, len(labels)
	if rangeWidth(start, end) > capacity {
		start, end = m.activeTab, m.activeTab+1
		for {
			expanded := false
			if start > 0 && rangeWidth(start-1, end) <= capacity {
				start--
				expanded = true
			}
			if end < len(labels) && rangeWidth(start, end+1) <= capacity {
				end++
				expanded = true
			}
			if !expanded {
				break
			}
		}
	}
	leftWidth, rightWidth := overflowWidth(start, true), overflowWidth(len(labels)-end, false)
	overflows := 0
	if leftWidth > 0 {
		overflows++
	}
	if rightWidth > 0 {
		overflows++
	}
	var items []tabBarItem
	x := 2
	appendItem := func(text string, target int, overflow bool) {
		width := lipgloss.Width(text)
		items = append(items, tabBarItem{text: text, left: x, width: width, target: target, overflow: overflow})
		x += width + 1
	}
	if capacity < 2*overflows+1 {
		appendItem(m.renderTabWidth(labels, m.activeTab, capacity), m.activeTab, false)
		return items
	}
	coreWidth := prefix[end] - prefix[start] + end - start - 1
	minimumActive := min(labels[m.activeTab].width, max(3, lipgloss.Width(labels[m.activeTab].indicators)+2))
	budget := max(overflows, capacity-coreWidth+labels[m.activeTab].width-minimumActive-overflows)
	leftWidth, rightWidth = fitTabOverflowWidths(leftWidth, rightWidth, budget)
	activeWidth := min(labels[m.activeTab].width, capacity-coreWidth+labels[m.activeTab].width-leftWidth-rightWidth-overflows)
	coreWidth += activeWidth - labels[m.activeTab].width
	extra := capacity - coreWidth - leftWidth - rightWidth - overflows
	leftExtra, rightExtra := 0, 0
	switch {
	case leftWidth > 0 && rightWidth > 0:
		leftExtra, rightExtra = extra/2, extra-extra/2
		if extra >= 5 {
			leftPreview := labels[start-1].width > 4
			rightPreview := labels[end].width > 4
			if leftPreview && !rightPreview {
				leftExtra, rightExtra = extra, 0
			} else if rightPreview && (!leftPreview || extra < 10) {
				leftExtra, rightExtra = 0, extra
			}
		}
	case leftWidth > 0:
		leftExtra = extra
	case rightWidth > 0:
		rightExtra = extra
	}
	if leftWidth > 0 {
		preview := partialTabWidth(leftExtra, labels[start-1].width)
		leftWidth += leftExtra
		if preview > 0 {
			leftWidth -= preview + 1
		}
		appendItem(m.renderTabOverflowWidth(m.tabOverflowText(start, true), leftWidth, true), start-1, true)
		if preview > 0 {
			appendItem(m.renderTabWidth(labels, start-1, preview), start-1, false)
		}
	}
	for i := start; i < end; i++ {
		text := labels[i].rendered
		if i == m.activeTab && activeWidth < labels[i].width {
			text = m.renderTabWidth(labels, i, activeWidth)
		}
		appendItem(text, i, false)
	}
	if rightWidth > 0 {
		preview := partialTabWidth(rightExtra, labels[end].width)
		rightWidth += rightExtra
		if preview > 0 {
			rightWidth -= preview + 1
			appendItem(m.renderTabWidth(labels, end, preview), end, false)
		}
		appendItem(m.renderTabOverflowWidth(m.tabOverflowText(len(labels)-end, false), rightWidth, false), end, true)
	}
	return items
}

func fitTabOverflowWidths(left, right, budget int) (int, int) {
	if left+right <= budget {
		return left, right
	}
	if left == 0 {
		return 0, budget
	}
	if right == 0 {
		return budget, 0
	}
	left = min(left, (budget+1)/2)
	right = min(right, budget-left)
	return budget - right, right
}

// A preview needs padding, at least one filename cell, an ellipsis, and a gap.
func partialTabWidth(extra, natural int) int {
	if extra < 5 || natural <= 4 {
		return 0
	}
	return min(extra-1, natural-1)
}
