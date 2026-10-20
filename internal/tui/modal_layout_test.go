package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
					{rendered: m.renderPrimaryModalButton("保存", "C-s", true), action: modalMouseAction{focus: 1}},
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

func TestTextModalTabOrderMatchesActions(t *testing.T) {
	app, _ := formTestAppWithContent(t, "- [ ] Answer <!-- form: id=answer text -->\n")
	app.toggleForm(1)
	for _, focus := range []int{2, 1, 0} {
		app = pressKey(app, tea.KeyTab)
		if app.modalFocus != focus || app.modalTextarea.Focused() != (focus == 0) {
			t.Fatalf("focus=%d, want %d", app.modalFocus, focus)
		}
	}
	var closeX, saveX int
	for _, region := range app.modalMouseRegions() {
		if region.action.close || region.action.textarea || region.action.scrollable {
			continue
		}
		switch region.action.focus {
		case 1:
			saveX = region.rect.left
		case 2:
			closeX = region.rect.left
		}
	}
	if closeX >= saveX {
		t.Fatalf("Close at %d, Save at %d", closeX, saveX)
	}
}

func TestModalActionContrastAndRole(t *testing.T) {
	luminance := func(c color.Color) float64 {
		r, g, b, _ := c.RGBA()
		linear := func(v uint32) float64 {
			x := float64(v) / 65535
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
	}
	defer initAdaptiveStyles(true)
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, style := range []lipgloss.Style{modalBtnPrimaryLabel, modalBtnPrimaryFocusedLabel, modalDangerBtnLabel, modalDangerBtnFocused} {
			fg, bg := luminance(style.GetForeground()), luminance(style.GetBackground())
			if ratio := (max(fg, bg) + 0.05) / (min(fg, bg) + 0.05); ratio < 4.5 {
				t.Fatalf("dark=%t: button contrast %.2f is below 4.5", dark, ratio)
			}
		}
		m := AppModel{}
		for _, focus := range []int{finishText, finishBack, finishSubmit, finishAbort} {
			m.modalFocus = focus
			content, _ := m.finishButtons(70, 0)
			primary, danger := modalBtnPrimaryLabel, modalDangerBtnLabel
			if focus == finishSubmit {
				primary = modalBtnPrimaryFocusedLabel
			}
			if focus == finishAbort {
				danger = modalDangerBtnFocused
			}
			for _, c := range []color.Color{primary.GetForeground(), danger.GetForeground()} {
				parameter := strings.TrimSuffix(strings.TrimPrefix(fgToAnsi(c), "\x1b["), "m")
				if !strings.Contains(content, parameter) {
					t.Fatalf("dark=%t/focus=%d: action color disappeared", dark, focus)
				}
			}
			if lipgloss.Width(m.renderPrimaryModalButton("保存", "C-s", focus == finishSubmit)) != lipgloss.Width(m.renderModalButton("保存", "C-s", focus == finishSubmit)) {
				t.Fatal("primary emphasis changed the button width")
			}
		}
	}
}
