package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestModalActionsWrapWholeButtons(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{24, 44} {
			t.Run(fmt.Sprintf("dark=%t/width=%d", dark, width), func(t *testing.T) {
				initAdaptiveStyles(dark)
				m := AppModel{}
				specs := []modalButtonSpec{
					{rendered: m.renderModalButton("保存", "C-s", true), action: modalMouseAction{focus: 1}},
					{rendered: m.renderModalButton("Keep Editing", "n / esc", false), action: modalMouseAction{focus: 2}},
					{rendered: m.renderModalButton("Close", "esc", false), action: modalMouseAction{focus: 3}},
				}
				content, regions := layoutModalActions(specs, width, 7)
				rows := strings.Split(content, "\n")
				for _, row := range rows {
					if lipgloss.Width(row) != width {
						t.Fatalf("row width=%d, want %d: %q", lipgloss.Width(row), width, ansi.Strip(row))
					}
				}
				if len(regions) != len(specs) || regions[len(regions)-1].action.focus != 1 {
					t.Fatal("primary action is not last")
				}
				for _, region := range regions {
					if region.rect.left < 0 || region.rect.right > width || region.rect.top < 7 || region.rect.bottom != region.rect.top+1 {
						t.Fatalf("invalid target: %+v", region)
					}
					label := ansi.Strip(ansi.Cut(rows[region.rect.top-7], region.rect.left, region.rect.right))
					want := ansi.Strip(specs[region.action.focus-1].rendered)
					if label != want {
						t.Fatalf("button or shortcut wrapped: got %q, want %q", label, want)
					}
				}
				if specs[0].action.focus != 1 {
					t.Fatal("layout reordered caller's actions")
				}
			})
		}
	}
}
