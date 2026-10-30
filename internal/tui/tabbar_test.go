package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestAttachmentTabColorMatchesFilesAcrossFocusAndClipping(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, active := range []bool{false, true} {
			for _, focus := range []pane{contentPane, commentPane} {
				for _, width := range []int{0, 6} {
					t.Run(fmt.Sprintf("dark=%t/active=%t/focus=%d/width=%d", dark, active, focus, width), func(t *testing.T) {
						app := newFileTreeTestApp()
						app.tabs[0].attachment = &review.AttachedDocument{Name: "note.md"}
						app.activeTab, app.focused = 1, focus
						want := warning
						if active {
							app.activeTab, want = 0, activeAttachmentColor
						}
						tab := app.renderTabWidth(app.tabLabels(), 0, width)
						runes, styles := renderedCells(tab)
						for i, r := range runes {
							if r != ' ' && !sameColor(styles[i].Fg, want) {
								t.Fatalf("attachment character %q color = %v, want %v: %q", r, styles[i].Fg, want, tab)
							}
						}
					})
				}
			}
		}
	}
}

func overflowTabTestApp() AppModel {
	app := newFileTreeTestApp()
	template := app.tabs[0]
	app.tabs = make([]FileTab, 7)
	for i := range app.tabs {
		app.tabs[i] = template
		app.tabs[i].path = strings.Repeat("long-", 12) + fmt.Sprintf("%d.go", i)
	}
	app.tabs[3].path = "current.go"
	app.activeTab = 3
	app.viewMode = true
	return app
}

func TestOverflowTabsAnchorAndPreviewNeighbors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		width   int
		preview bool
	}{
		{"partial neighbors", 64, true},
		{"space absorbed by overflow", 37, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := overflowTabTestApp()
			app.width = tc.width
			items := app.tabBarItems()
			left, right := items[0], items[len(items)-1]
			if !left.overflow || !right.overflow || left.left != 2 || right.left+right.width != tc.width-2 {
				t.Fatalf("overflow controls are not pinned to the pane edges: %+v", items)
			}
			if !strings.HasPrefix(ansi.Strip(left.text), fmt.Sprintf(" … %d more", left.target+1)) || !strings.Contains(ansi.Strip(right.text), fmt.Sprintf("%d more", len(app.tabs)-right.target)) || !strings.HasSuffix(ansi.Strip(right.text), "… ") {
				t.Fatalf("overflow text is not aligned to its outer edge: %q / %q", left.text, right.text)
			}
			if tc.preview {
				if len(items) < 5 || items[1].overflow || items[len(items)-2].overflow {
					t.Fatalf("missing adjacent previews: %+v", items)
				}
				for _, item := range []tabBarItem{items[1], items[len(items)-2]} {
					if !strings.Contains(ansi.Strip(item.text), "…") || item.width >= (tc.width-4)/4 {
						t.Fatalf("neighbor is not a clipped preview: %q", item.text)
					}
				}
			} else if len(items) != 3 {
				t.Fatalf("unused preview space was not absorbed by overflow controls: %+v", items)
			}
		})
	}
}

func TestMouseClickOverflowPreviewsAndExpandedControls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		width    int
		right    bool
		overflow bool
	}{
		{"left preview", 64, false, false},
		{"right preview", 64, true, false},
		{"left expanded control", 37, false, true},
		{"right expanded control", 37, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := overflowTabTestApp()
			app.width = tc.width
			items := app.tabBarItems()
			index := 0
			if !tc.overflow {
				index++
			}
			if tc.right {
				index = len(items) - 1 - index
			}
			item := items[index]
			if item.overflow != tc.overflow {
				t.Fatalf("unexpected tab kind: %+v", item)
			}
			app = clickMouse(app, item.left+item.width-1, app.headerHeight())
			if app.activeTab != item.target {
				t.Fatalf("active tab = %d, want %d", app.activeTab, item.target)
			}
		})
	}
}

func TestTabLayoutFitsUnicodeAcrossWidthsAndActiveTabs(t *testing.T) {
	app := overflowTabTestApp()
	app.tabs[0].path = strings.Repeat("界é", 12) + ".go"
	for _, width := range []int{8, 9, 18, 40, 41, 42, 63, 64, 65, 120} {
		for _, active := range []int{0, len(app.tabs) / 2, len(app.tabs) - 1} {
			t.Run(fmt.Sprintf("width=%d/active=%d", width, active), func(t *testing.T) {
				app.width, app.activeTab = width, active
				items := app.tabBarItems()
				foundActive := false
				for i, item := range items {
					if !utf8.ValidString(item.text) || lipgloss.Height(item.text) != 1 || item.width != lipgloss.Width(item.text) {
						t.Fatalf("invalid rendered tab: %+v", item)
					}
					if item.left < 2 || item.left+item.width > width-2 {
						t.Fatalf("tab exceeds the pane: %+v", item)
					}
					if i > 0 && item.left != items[i-1].left+items[i-1].width+1 {
						t.Fatalf("tab spacing is not one cell: %+v", items)
					}
					foundActive = foundActive || !item.overflow && item.target == active
				}
				if !foundActive {
					t.Fatalf("current tab is not visible: %+v", items)
				}
				if last := items[len(items)-1]; last.overflow && last.left+last.width != width-2 {
					t.Fatalf("Unicode clipping moved the overflow control: %+v", items)
				}
				if row := app.renderTabBar(); lipgloss.Height(row) != 1 || lipgloss.Width(row) != width {
					t.Fatalf("tab row does not fit the pane: %q", row)
				}
			})
		}
	}
}

func TestTabPathsExpandWithinTheirPaneShare(t *testing.T) {
	for _, tc := range []struct {
		width, tabs int
		path, want  string
	}{
		{64, 4, "internal/server/file.go", "i/se/file.go"},
		{88, 4, "internal/server/file.go", "int/server/file.go"},
		{120, 4, "internal/server/file.go", "internal/server/file.go"},
		{80, 4, "éclair/界面/file.go", "écl/界面/file.go"},
		{44, 4, "internal/server/file.go", "file.go"},
		{40, 1, "internal/server/file.go", "internal/server/file.go"},
		{60, 2, "internal/server/file.go", "internal/server/file.go"},
		{84, 3, "internal/server/file.go", "internal/server/file.go"},
	} {
		t.Run(fmt.Sprint(tc.width, "/", tc.tabs, "/", tc.path), func(t *testing.T) {
			app := newFileTreeTestApp()
			app.tabs = app.tabs[:tc.tabs]
			app.tabs[0].path = tc.path
			app.viewMode, app.width = true, tc.width
			labels := app.tabLabels()
			limit := (tc.width - 4 - tc.tabs + 1) / tc.tabs
			item := app.renderTabWithin(labels, 0, limit, false)
			if lipgloss.Width(item) > limit {
				t.Fatalf("tab exceeds its pane share: %q", item)
			}
			if got := strings.TrimSpace(ansi.Strip(item)); got != tc.want {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTabWidthSharesFitWithoutUnnecessaryOverflow(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			app := newFileTreeTestApp()
			template := app.tabs[0]
			app.tabs = make([]FileTab, count)
			for i := range app.tabs {
				app.tabs[i] = template
				app.tabs[i].path = strings.Repeat("long/", 40) + fmt.Sprintf("%d.go", i)
			}
			app.viewMode, app.width = true, 120
			limit := (app.width - 4 - min(count, 4) + 1) / min(count, 4)
			items := app.tabBarItems()
			for _, item := range items {
				if item.overflow && count <= 4 {
					t.Fatal("tab spacing caused unnecessary overflow")
				}
				if !item.overflow && item.width > limit {
					t.Fatalf("tab exceeds its pane share: %+v", item)
				}
			}
			if count <= 4 && len(items) != count {
				t.Fatalf("only %d of %d tabs fit", len(items), count)
			}
		})
	}
}

func TestExpandedTabReservesUnreadCountsAndComments(t *testing.T) {
	app := newFileTreeTestApp()
	app.tabs = app.tabs[:1]
	app.tabs[0].path = "internal/server/file.go"
	app.tabs[0].unread = true
	app.tabs[0].changedLines = map[int]bool{1: true}
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "old"}}}
	app.tabs[0].state.Comments = []review.Comment{{Scope: "file"}}
	app.viewMode, app.width = true, 116
	item := app.renderTabWithin(app.tabLabels(), 0, (app.width-4)/4, false)
	plain := ansi.Strip(item)
	if lipgloss.Width(item) > (app.width-4)/4 || !strings.Contains(plain, "●") || !strings.Contains(plain, "file.go (+1 -1) 💬") {
		t.Fatalf("expanded tab lost its filename or indicators: %q", plain)
	}
}

func TestActiveTabKeepsFilenameBeforeNeighbors(t *testing.T) {
	for _, width := range []int{80, 99, 100, 120} {
		for _, active := range []int{0, 3, 6} {
			t.Run(fmt.Sprint(width, "/", active), func(t *testing.T) {
				app := overflowTabTestApp()
				app.width, app.activeTab = width, active
				app.viewMode = false
				for i := range app.tabs {
					app.tabs[i].path = fmt.Sprintf("internal/server/file_%d_test.go", i)
					app.tabs[i].changedLines = map[int]bool{1: true}
					app.tabs[i].deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "old"}}}
				}
				filename := fmt.Sprintf("file_%d_test.go", active)
				if active == 3 {
					filename = "界é_test.go"
					app.tabs[active].path = "internal/server/" + filename
					app.tabs[active].unread = true
					app.tabs[active].state = &fileReview{Comments: []review.Comment{{Scope: "file"}}}
				}
				wantSummary := "(+1 -1)"
				if width < 100 {
					wantSummary = "+-"
				}
				app.syncFileTree()
				for i, row := range app.fileTree.rows {
					if !row.isDir && row.tabIndex == active && !strings.Contains(ansi.Strip(app.treeRowText(i, app.tabIndicators())), wantSummary) {
						t.Fatalf("tree summary did not follow screen width %d", width)
					}
				}
				found := false
				for _, item := range app.tabBarItems() {
					if item.target == active && !item.overflow {
						found = true
						runes, styles := renderedCells(item.text)
						for cell, r := range runes {
							if r == '+' && !sameColor(styles[cell].Fg, tabAddedCount.GetForeground()) || r == '-' && !sameColor(styles[cell].Fg, tabDeletedCount.GetForeground()) {
								t.Fatal("change markers lost their colors")
							}
						}
						if !strings.Contains(ansi.Strip(item.text), wantSummary) {
							t.Fatalf("wrong summary at width %d: %q", width, ansi.Strip(item.text))
						}
						if !strings.Contains(ansi.Strip(item.text), filename) {
							t.Fatalf("active tab lost its filename: %q", ansi.Strip(item.text))
						}
					}
				}
				if !found {
					t.Fatal("active tab is missing")
				}
			})
		}
	}
}
