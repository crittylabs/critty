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
	capacity := max(0, m.contentPaneWidth())
	if len(labels) == 0 || capacity == 0 {
		return nil
	}
	prefix := make([]int, len(labels)+1)
	for i := range labels {
		labels[i].rendered = m.renderTab(labels, i, i == 0)
		labels[i].width = lipgloss.Width(labels[i].rendered)
		prefix[i+1] = prefix[i] + labels[i].width
	}
	overflowWidth := func(count int, backwards bool) int {
		if count == 0 {
			return 0
		}
		return lipgloss.Width(m.renderTabOverflowIndicator(m.tabOverflowText(count, backwards), false))
	}
	rangeWidth := func(start, end int) int {
		width := prefix[end] - prefix[start]
		if start > 0 {
			width += overflowWidth(start, true)
		}
		if end < len(labels) {
			width += overflowWidth(len(labels)-end, false)
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
	x := 0
	appendItem := func(text string, target int, overflow bool) {
		width := lipgloss.Width(text)
		items = append(items, tabBarItem{text: text, left: x, width: width, target: target, overflow: overflow})
		x += width
	}
	if capacity < 3*overflows+2 {
		appendItem(m.renderTabWidth(labels, m.activeTab, true, capacity), m.activeTab, false)
		return items
	}
	coreWidth := prefix[end] - prefix[start]
	frame := inactiveTabStyle.GetHorizontalFrameSize()
	minimumActive := min(labels[m.activeTab].width, max(frame+1, lipgloss.Width(labels[m.activeTab].indicators)+frame))
	budget := max(3*overflows, capacity-coreWidth+labels[m.activeTab].width-minimumActive)
	leftWidth, rightWidth = fitTabOverflowWidths(leftWidth, rightWidth, budget)
	activeWidth := min(labels[m.activeTab].width, capacity-coreWidth+labels[m.activeTab].width-leftWidth-rightWidth)
	coreWidth += activeWidth - labels[m.activeTab].width
	extra := capacity - coreWidth - leftWidth - rightWidth
	leftExtra, rightExtra := 0, 0
	switch {
	case leftWidth > 0 && rightWidth > 0:
		leftExtra, rightExtra = extra/2, extra-extra/2
		if extra >= 6 {
			leftPreview := labels[start-1].width > 6
			rightPreview := labels[end].width > 6
			if leftPreview && !rightPreview {
				leftExtra, rightExtra = extra, 0
			} else if rightPreview && (!leftPreview || extra < 12) {
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
			leftWidth -= preview
		}
		appendItem(m.renderTabOverflowWidth(m.tabOverflowText(start, true), true, leftWidth, true), start-1, true)
		if preview > 0 {
			appendItem(m.renderTabWidth(labels, start-1, false, preview), start-1, false)
		}
	}
	for i := start; i < end; i++ {
		text := labels[i].rendered
		if i == m.activeTab && activeWidth < labels[i].width {
			text = m.renderTabWidth(labels, i, i == 0, activeWidth)
		}
		appendItem(text, i, false)
	}
	if rightWidth > 0 {
		preview := partialTabWidth(rightExtra, labels[end].width)
		rightWidth += rightExtra
		if preview > 0 {
			rightWidth -= preview
			appendItem(m.renderTabWidth(labels, end, false, preview), end, false)
		}
		appendItem(m.renderTabOverflowWidth(m.tabOverflowText(len(labels)-end, false), false, rightWidth, false), end, true)
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
	left = min(left, max(3, (budget+1)/2))
	right = min(right, budget-left)
	return budget - right, right
}

// A preview needs borders, padding, a filename cell, and an ellipsis.
func partialTabWidth(extra, natural int) int {
	if extra < 6 || natural <= 6 {
		return 0
	}
	return min(extra, natural-1)
}
