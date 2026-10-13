package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/form"
)

type progressKind int

const (
	progressThreads progressKind = iota
	progressRequiredChecks
	progressRequiredChoices
	progressOptionalChecks
	progressOptionalChoices
)

type progressCount struct {
	kind            progressKind
	complete, total int
}

type progressRegion struct {
	rect mouseRect
	kind progressKind
}

func (p *progressCount) add(complete bool) {
	p.total++
	if complete {
		p.complete++
	}
}

func formProgressKind(item form.Item) progressKind {
	kind := progressOptionalChecks
	if item.Required {
		kind = progressRequiredChecks
	}
	if item.Choice {
		kind++
	}
	return kind
}

func (m *AppModel) progressCounts() []progressCount {
	var counts [progressOptionalChoices + 1]progressCount
	for i := range counts {
		counts[i].kind = progressKind(i)
	}
	for _, tab := range m.tabs {
		if tab.path == overallCommentsPath || tab.state == nil {
			continue
		}
		for _, comment := range tab.state.Comments {
			counts[progressThreads].add(comment.Resolved)
		}
	}
	if m.session != nil {
		for _, comment := range m.session.CJ.ReviewComments {
			counts[progressThreads].add(comment.Resolved)
		}
	}
	for _, field := range m.formReviewItems() {
		counts[formProgressKind(field.item)].add(field.answered)
	}
	var result []progressCount
	for _, count := range counts {
		if count.kind == progressThreads || count.total > 0 {
			result = append(result, count)
		}
	}
	return result
}

func (p progressCount) render(focused bool) string {
	foreground := color.Color(warning)
	if p.kind <= progressRequiredChoices {
		foreground = lipgloss.Red
		if p.complete == p.total {
			foreground = success
		}
	}
	symbol := "☐"
	switch p.kind {
	case progressThreads:
		symbol = "💬"
	case progressRequiredChoices, progressOptionalChoices:
		symbol = "○"
	}
	if p.total > 0 && p.complete == p.total {
		switch symbol {
		case "☐":
			symbol = "☑"
		case "○":
			symbol = "●"
		}
	}
	style := lipgloss.NewStyle().Foreground(foreground).Bold(true).Underline(focused)
	if focused {
		style = style.Background(buttonBackground)
	}
	return style.Render(fmt.Sprintf("%d/%d %s", p.complete, p.total, symbol))
}

func (m *AppModel) layoutProgressCounts(width, top int, focused progressKind) (string, []progressRegion) {
	var specs []modalButtonSpec
	counts := m.progressCounts()
	unwrappedWidth := 0
	for _, count := range counts {
		rendered := count.render(count.kind == focused)
		unwrappedWidth += lipgloss.Width(rendered) + 2
		specs = append(specs, modalButtonSpec{rendered: rendered, action: modalMouseAction{focus: int(count.kind)}})
	}
	if width <= 0 {
		width = unwrappedWidth
	}
	text, buttons := layoutModalButtonRow(specs, width, top)
	var regions []progressRegion
	for _, button := range buttons {
		for _, count := range counts {
			if int(count.kind) == button.action.focus && count.total > 0 {
				regions = append(regions, progressRegion{rect: button.rect, kind: count.kind})
				break
			}
		}
	}
	return text, regions
}

func (m *AppModel) jumpToProgress(kind progressKind) bool {
	hasTarget := false
	for _, count := range m.progressCounts() {
		if count.kind == kind && count.total > 0 {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		return false
	}
	if m.modal == finishModal || m.modal == finishFormsModal || m.modal == finishResolveModal {
		m.closeFinishModal()
		if m.modal != noModal {
			return false
		}
	}
	if kind == progressThreads {
		m.ensureOverallCommentsTab()
	}
	tab, target, ok := m.firstProgressTarget(kind)
	if !ok {
		return false
	}
	m.tabs[tab].selecting = false
	m.selectComment(tab, target)
	return true
}

// firstProgressTarget prefers unfinished items, falling back to the first
// completed item when a count is complete.
func (m *AppModel) firstProgressTarget(kind progressKind) (int, commentTarget, bool) {
	firstTab := -1
	var first commentTarget
	if kind == progressThreads {
		for tab := range m.tabs {
			for _, target := range m.commentTargets(tab) {
				if !target.resolved {
					return tab, target, true
				}
				if firstTab < 0 {
					firstTab, first = tab, target
				}
			}
		}
	} else {
		for _, field := range m.formReviewItems() {
			if formProgressKind(field.item) != kind {
				continue
			}
			target := commentTarget{formID: field.item.ID, line: field.item.Line}
			if !field.answered {
				return field.tab, target, true
			}
			if firstTab < 0 {
				firstTab, first = field.tab, target
			}
		}
	}
	return firstTab, first, firstTab >= 0
}
