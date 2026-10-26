package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/review"
)

func overflowTabTestApp() AppModel {
	app := newFileTreeTestApp()
	app.tabs = app.tabs[:3]
	app.tabs[0].path = strings.Repeat("left-", 12) + "file.go"
	app.tabs[1].path = "current.go"
	app.tabs[2].path = strings.Repeat("right-", 12) + "file.go"
	app.activeTab = 1
	app.viewMode = true
	return app
}

func TestOverflowTabsAnchorAndPreviewNeighbors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		width   int
		preview bool
	}{
		{"partial neighbors", 66, true},
		{"space absorbed by overflow", 41, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := overflowTabTestApp()
			app.width = tc.width
			items := app.tabBarItems()
			left, right := items[0], items[len(items)-1]
			if !left.overflow || !right.overflow || left.left != 0 || right.left+right.width != tc.width {
				t.Fatalf("overflow controls are not pinned to the pane edges: %+v", items)
			}
			if !strings.HasPrefix(tabCaption(left.text), "│ ↤ 1 more") || !strings.HasSuffix(tabCaption(right.text), "1 more ↦ │") {
				t.Fatalf("overflow text is not aligned to its outer edge: %q / %q", left.text, right.text)
			}
			if tc.preview {
				if len(items) != 5 || items[1].target != 0 || items[3].target != 2 {
					t.Fatalf("missing adjacent previews: %+v", items)
				}
				for _, item := range []tabBarItem{items[1], items[3]} {
					if !strings.HasSuffix(tabCaption(item.text), "… │") {
						t.Fatalf("neighbor does not end in an ellipsis: %q", item.text)
					}
				}
			} else if len(items) != 3 || left.width != 13 || right.width != 14 {
				t.Fatalf("unused preview space was not absorbed by overflow controls: %+v", items)
			}
		})
	}
}

func TestMouseClickOverflowPreviewsAndExpandedControls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		width    int
		item     int
		target   int
		overflow bool
	}{
		{"left preview", 66, 1, 0, false},
		{"right preview", 66, 3, 2, false},
		{"left expanded control", 41, 0, 0, true},
		{"right expanded control", 41, 2, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := overflowTabTestApp()
			app.width = tc.width
			item := app.tabBarItems()[tc.item]
			if item.overflow != tc.overflow {
				t.Fatalf("unexpected tab kind: %+v", item)
			}
			app = clickMouse(app, item.left+item.width-1, app.headerHeight())
			if app.activeTab != tc.target {
				t.Fatalf("active tab = %d, want %d", app.activeTab, tc.target)
			}
		})
	}
}

func TestTabTitlesOmitUnreadMarkers(t *testing.T) {
	for _, unread := range []bool{false, true} {
		app := newFileTreeTestApp()
		app.activeTab = 1
		app.tabs[0].attachment = &review.AttachedDocument{Name: "note.md"}
		app.tabs[0].unread = unread
		tab := app.renderTab(app.tabLabels(), 0, true)
		want := "│ note.md │"
		if caption := tabCaption(tab); caption != want {
			t.Fatalf("unread=%t: caption = %q, want %q", unread, caption, want)
		}
		runes, styles := renderedCells(tab)
		for i, r := range runes {
			if r == 'n' && !sameColor(styles[i].Fg, warning) {
				t.Fatalf("unread=%t: attachment lost its yellow: %+v", unread, styles[i])
			}
		}
	}
}

func TestOverflowTabColorMatchesInactiveFileNames(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := overflowTabTestApp()
		app.width = 66
		for _, item := range app.tabBarItems() {
			if !item.overflow {
				continue
			}
			runes, styles := renderedCells(item.text)
			for i, r := range runes {
				if strings.ContainsRune("more0123456789↤↦", r) && !sameColor(styles[i].Fg, inactiveTabStyle.GetForeground()) {
					t.Fatalf("dark=%t: overflow character %q has a different color from filenames: %+v", dark, r, styles[i])
				}
			}
		}
	}
}

func TestTabLayoutFitsUnicodeAcrossWidthsAndActiveTabs(t *testing.T) {
	app := overflowTabTestApp()
	app.tabs[0].path = strings.Repeat("界é", 12) + ".go"
	for width := 14; width <= 120; width++ {
		for active := range app.tabs {
			t.Run(fmt.Sprintf("width=%d/active=%d", width, active), func(t *testing.T) {
				app.width, app.activeTab = width, active
				items := app.tabBarItems()
				foundActive := false
				for i, item := range items {
					if !utf8.ValidString(item.text) || lipgloss.Height(item.text) != 3 || item.width != lipgloss.Width(item.text) {
						t.Fatalf("invalid rendered tab: %+v", item)
					}
					if item.left < 0 || item.left+item.width > width {
						t.Fatalf("tab exceeds the pane: %+v", item)
					}
					if i > 0 && item.left != items[i-1].left+items[i-1].width {
						t.Fatalf("bordered tabs are not adjacent: %+v", items)
					}
					foundActive = foundActive || !item.overflow && item.target == active
				}
				if !foundActive {
					t.Fatalf("current tab is not visible: %+v", items)
				}
				if row := app.renderTabBar(); lipgloss.Height(row) != 3 || lipgloss.Width(row) != width {
					t.Fatalf("tab row does not fit the pane: %q", row)
				}
			})
		}
	}
}

func tabCaption(rendered string) string {
	return strings.Split(ansi.Strip(rendered), "\n")[1]
}
