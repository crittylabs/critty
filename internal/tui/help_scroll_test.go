package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHelpScrollControlsOnlyOnOverflow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		readOnly      bool
		width, height int
		scrollable    bool
	}{
		{"view/fits", true, 120, 38, false},
		{"view/overflow", true, 40, 12, true},
		{"review/fits", false, 120, 80, false},
		{"review/overflow", false, 80, 24, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "text\n")
			app.readOnly = tc.readOnly
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			app = pressAltKey(app, '?')
			app = pressKey(app, tea.KeyEnd)
			controls := 0
			for _, region := range app.modalMouseRegions() {
				if region.action.scrollable {
					controls++
				}
			}
			want := 0
			if tc.scrollable {
				want = 2
			}
			if controls != want {
				t.Fatalf("scroll controls=%d, want %d", controls, want)
			}
			if (app.helpOffset > 0) != tc.scrollable {
				t.Fatalf("End offset=%d, scrollable=%t", app.helpOffset, tc.scrollable)
			}
			if tc.scrollable {
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: 120, Height: 80})
				for _, region := range app.modalMouseRegions() {
					if region.action.scrollable {
						t.Fatal("scroll controls remain after resizing to fit all shortcuts")
					}
				}
			}
		})
	}
}
