package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestMouseClickSelectsFileTab(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.width = 120
	if app = clickMouse(app, 20, app.headerHeight()+app.tabBarHeight()); app.activeTab != 0 {
		t.Fatalf("click below the tab bar selected tab %d", app.activeTab)
	}
	labels := app.tabLabels()
	firstWidth := lipgloss.Width(app.renderTab(labels, 0, true))
	if app = clickMouse(app, firstWidth+1, app.headerHeight()); app.activeTab != 1 {
		t.Fatalf("active tab = %d, want 1", app.activeTab)
	}
}

func TestMouseClickSelectsVisibleOverflowTab(t *testing.T) {
	app := newCommentNavigationTestApp()
	fourth := app.tabs[1]
	fourth.path = "fourth.go"
	fifth := app.tabs[1]
	fifth.path = "fifth.go"
	app.tabs = append(app.tabs, fourth, fifth)
	app.width = 82 // a 56-column content pane beside the 26-column sidebar
	app.activeTab = 2

	labels := app.tabLabels()
	for i := range labels {
		labels[i].width = lipgloss.Width(app.renderTab(labels, i, i == 0))
	}
	start, end := app.visibleTabWindow(labels)
	target := start
	if target == app.activeTab {
		target = end - 1
	}
	if target == app.activeTab {
		t.Fatal("test setup did not expose another tab")
	}
	x := 0
	if start > 0 {
		x = lipgloss.Width(inactiveTabStyle.Render("↤ 1 more"))
	}
	for i := start; i < target; i++ {
		x += labels[i].width
	}

	app = clickMouse(app, x+1, app.headerHeight())

	if app.activeTab != target {
		t.Fatalf("active tab = %d, want %d", app.activeTab, target)
	}
}

func TestMouseClickSelectsTabBehindOverflowIndicator(t *testing.T) {
	newApp := func() AppModel {
		app := newCommentNavigationTestApp()
		template := app.tabs[1]
		paths := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go"}
		app.tabs = make([]FileTab, len(paths))
		for i, path := range paths {
			app.tabs[i] = template
			app.tabs[i].path = path
		}
		app.width = 50 // a 24-column content pane beside the 26-column sidebar
		app.activeTab = 4
		return app
	}

	tests := []struct {
		name  string
		right bool
	}{
		{name: "left"},
		{name: "right", right: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newApp()
			labels := app.tabLabels()
			for i := range labels {
				labels[i].rendered = app.renderTab(labels, i, i == 0)
				labels[i].width = lipgloss.Width(labels[i].rendered)
			}
			start, end := app.visibleTabWindow(labels)
			if start == 0 || end == len(labels) {
				t.Fatalf("visible window = [%d,%d), want overflow on both sides", start, end)
			}

			x := 1
			want := start - 1
			if tt.right {
				x = lipgloss.Width(app.renderTabOverflowIndicator(app.tabOverflowText(start, true), true))
				for i := start; i < end; i++ {
					x += labels[i].width
				}
				x++
				want = end
			}

			app = clickMouse(app, x, app.headerHeight())
			if app.activeTab != want {
				t.Fatalf("active tab = %d, want adjacent hidden tab %d", app.activeTab, want)
			}
		})
	}
}

func TestMouseWheelScrollsCodePane(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width = 100
	app.contentViewport.SetWidth(75)
	_, top, right, _ := app.contentBounds()
	if app = wheelMouse(app, right+1, top+1, tea.MouseWheelDown); app.contentViewport.YOffset() != 0 {
		t.Fatal("wheel outside the code pane scrolled it")
	}
	x, y := contentScreenPoint(app, 10, 1)
	app = wheelMouse(app, x, y, tea.MouseWheelDown)
	if got := app.contentViewport.YOffset(); got != app.contentViewport.MouseWheelDelta {
		t.Fatalf("viewport offset = %d, want %d", got, app.contentViewport.MouseWheelDelta)
	}
}

func TestMouseClickFocusesCodeLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].cursorLine = 1

	x, y := contentScreenPoint(app, 10, 1)
	app = clickMouse(app, x, y)

	if app.focused != contentPane || app.tab().cursorLine != 2 || app.tab().cursorOnAnnotation {
		t.Fatalf("focus = %v, line = %d, annotation = %t; want content line 2",
			app.focused, app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}
}

func TestMouseClickCodeGutterOpensLineComment(t *testing.T) {
	for _, gutter := range []bool{true, false} {
		app := setupAppWithDoc(t, "first\nsecond\nthird\n")
		app.width = 100
		app.contentViewport.SetWidth(75)
		x := gutterWidth
		if gutter {
			x = 0
		}
		x, y := contentScreenPoint(app, x, 1)
		app = clickMouse(app, x, y)
		if (app.modal == commentModal) != gutter || app.tab().cursorLine != 2 || app.tab().selecting {
			t.Fatalf("gutter %t: modal = %v, line = %d, selecting = %t",
				gutter, app.modal, app.tab().cursorLine, app.tab().selecting)
		}
	}
}

func TestMouseHoverCodeGutterShowsCommentMarker(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].changedLines = map[int]bool{2: true}
	app.rebuildContent()

	x, y := contentScreenPoint(app, 0, 1)
	app = hoverMouse(app, x, y)

	lines := strings.Split(app.contentViewport.View(), "\n")
	if app.hoveredGutterLine != 2 || !strings.HasPrefix(lines[1], commentGutterMarker.Render(">")) {
		t.Fatalf("hovered line = %d, rendered line = %q; want comment marker on line 2",
			app.hoveredGutterLine, ansi.Strip(lines[1]))
	}

	app = hoverMouse(app, x+gutterWidth, y)
	lines = strings.Split(app.contentViewport.View(), "\n")
	if app.hoveredGutterLine != 0 || !strings.HasPrefix(lines[1], diffAddedGutter.Render("+")) {
		t.Fatalf("hovered line = %d, rendered line = %q; want diff marker restored",
			app.hoveredGutterLine, ansi.Strip(lines[1]))
	}
}

func TestMouseClickSelectionEndGutterPreservesRangeAndOpensComment(t *testing.T) {
	tests := []struct {
		name   string
		anchor int
		cursor int
	}{
		{name: "selected downward", anchor: 2, cursor: 4},
		{name: "selected upward", anchor: 4, cursor: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\nthird\nfourth\nfifth\n")
			app.width = 100
			app.contentViewport.SetWidth(75)
			app.tabs[0].selecting = true
			app.tabs[0].selectAnchor = tt.anchor
			app.tabs[0].cursorLine = tt.cursor
			app.rebuildContent()

			x, y := contentScreenPoint(app, 0, 3)
			app = clickMouse(app, x, y)

			start, end := app.selectionRange()
			if app.modal != commentModal || !app.tab().selecting || start != 2 || end != 4 {
				t.Fatalf("modal = %v, selecting = %t, range = %d-%d; want selection comment for 2-4",
					app.modal, app.tab().selecting, start, end)
			}
		})
	}
}

func TestMouseClickRenderedCodeLineUsesItsDisplayedPosition(t *testing.T) {
	app := setupAppWithDoc(t, "first unique\nsecond unique\nthird unique\n")
	app.multiFile = true
	app.width = 60
	app.height = 20
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	y := renderedLineY(t, app, "second unique")

	app = clickMouse(app, left, y)

	if app.tab().cursorLine != 2 {
		t.Fatalf("line = %d, want displayed line 2 at screen row %d", app.tab().cursorLine, y)
	}
}

func TestMouseClickWrappedRowsUseOriginalLine(t *testing.T) {
	words := func(n int) string { return strings.Repeat("word ", n) }
	for _, tt := range []struct {
		name, content, first, next string
		deleted                    bool
		minRows                    int
	}{
		{name: "plain", content: "wrapped-start " + words(20) + "continuation-tail\nnext unique\n", first: "wrapped-start", next: "next unique", minRows: 2},
		{name: "tab indented two rows", content: "\t\ttab-start " + words(10) + "two-row-tail\nnext unique\n", first: "tab-start", next: "next unique", minRows: 2},
		{name: "tab indented three rows", content: "\t\ttab-start " + words(20) + "three-row-tail\nnext unique\n", first: "tab-start", next: "next unique", minRows: 3},
		{name: "markdown table", content: "| very-long-header-cell " + words(12) + "| value |\n| --- | --- |\n| body | value |\n", first: "very-long-header-cell", next: "2 │", minRows: 2},
		{name: "deleted before line", content: "current unique\nnext unique\n", first: "deleted-start", next: "current unique", deleted: true, minRows: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, tt.content)
			app.multiFile = true
			app.width, app.height = 60, 24
			if tt.deleted {
				app.tabs[0].isMarkdown = false
				app.tabs[0].chromaLines = []string{"current unique", "next unique"}
				app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
					0: {{OldLineNum: 1, Content: "deleted-start " + words(20)}},
				}
			}
			app.recalculateLayout()
			app.rebuildContent()
			left, _, _, _ := app.contentBounds()
			firstY := renderedLineY(t, app, tt.first)
			nextY := renderedLineY(t, app, tt.next)
			if rows := nextY - firstY; rows < tt.minRows {
				t.Fatalf("rendered rows = %d, want at least %d", rows, tt.minRows)
			}
			for y := firstY; y < nextY; y++ {
				for _, x := range []int{left, left + gutterWidth} {
					if clicked := clickMouse(app, x, y); clicked.tab().cursorLine != 1 {
						t.Fatalf("screen %d,%d selected line %d, want source line 1", x, y, clicked.tab().cursorLine)
					}
				}
			}
		})
	}
}

func TestMouseDragRenderedCodeLinesUsesDisplayedPositions(t *testing.T) {
	app := setupAppWithDoc(t, "first unique\nsecond unique\nthird unique\nfourth unique\n")
	app.multiFile = true
	app.width = 60
	app.height = 22
	app.recalculateLayout()
	app.rebuildContent()
	left, _, _, _ := app.contentBounds()
	startY := renderedLineY(t, app, "second unique")
	endY := renderedLineY(t, app, "third unique")

	app = pressMouse(app, left, startY)
	app = moveMouse(app, left, endY)
	app = releaseMouse(app, left, endY)

	start, end := app.selectionRange()
	if start != 2 || end != 3 {
		t.Fatalf("selection = %d-%d, want displayed lines 2-3", start, end)
	}
}

func TestMouseDragCodeGutterSelectsLinesAndOpensComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\nfourth\nfifth\n")
	app.width = 100
	app.filePath = strings.Repeat("x", 76)
	app.contentViewport.SetWidth(75)

	startX, startY := contentScreenPoint(app, 0, 1)
	endX, endY := contentScreenPoint(app, 0, 3)
	app = pressMouse(app, startX, startY)
	app = moveMouse(app, endX, endY)
	if !app.mouseSelecting || !app.tab().selecting || app.tab().selectAnchor != 2 || app.tab().cursorLine != 4 {
		t.Fatalf("drag = %t, selecting = %t, range = %d-%d; want 2-4",
			app.mouseSelecting, app.tab().selecting, app.tab().selectAnchor, app.tab().cursorLine)
	}

	app = releaseMouse(app, endX, endY)
	if app.mouseSelecting || app.modal != commentModal || !app.tab().selecting {
		t.Fatalf("drag = %t, modal = %v, selecting = %t; want selection comment",
			app.mouseSelecting, app.modal, app.tab().selecting)
	}
}

func TestMouseDragCodeGutterScrollsAtBottomEdge(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 10))
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(3)
	app.contentViewport.SetYOffset(1)

	x, y := contentScreenPoint(app, 0, 1)
	_, _, _, bottom := app.contentBounds()
	app = pressMouse(app, x, y)
	app = moveMouse(app, x, bottom-1)

	if got := app.contentViewport.YOffset(); got != 2 {
		t.Fatalf("viewport offset = %d, want 2", got)
	}
}

func TestMouseClickFocusesScrolledCodeLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\nfourth\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.contentViewport.SetHeight(2)
	app.contentViewport.SetYOffset(1)

	x, y := contentScreenPoint(app, 10, app.contentViewport.YOffset())
	app = clickMouse(app, x, y)

	if app.tab().cursorLine != 2 {
		t.Fatalf("line = %d, want first visible line 2", app.tab().cursorLine)
	}
}

func TestMouseClickDeletedLineFocusesDeletedLine(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {{OldLineNum: 2, Content: "deleted"}},
	}
	app.tabs[0].cursorLine = 1
	app.rebuildContent()

	x, y := contentScreenPoint(app, 10, 1)
	app = clickMouse(app, x, y)

	if app.tab().cursorLine != 2 || app.tab().cursorSide != "old" || app.tab().cursorOnAnnotation {
		t.Fatalf("line = %d/%s, annotation = %t; want deleted line 2",
			app.tab().cursorLine, app.tab().cursorSide, app.tab().cursorOnAnnotation)
	}
}

func TestMouseDragSelectsDeletedLineRange(t *testing.T) {
	app := setupAppWithDoc(t, "first\nfourth\n")
	app.width = 100
	app.height = 20
	app.recalculateLayout()
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		1: {
			{OldLineNum: 2, Content: "second"},
			{OldLineNum: 3, Content: "third"},
		},
	}
	app.tabs[0].cursorLine = 1
	app.rebuildContent()
	left, top, _, _ := app.contentBounds()
	startY := top + app.contentLayout.oldRanges[2].start
	endY := top + app.contentLayout.oldRanges[3].start

	app = pressMouse(app, left, startY)
	app = moveMouse(app, left, endY)
	app = releaseMouse(app, left, endY)

	start, end := app.selectionRange()
	if app.modal != commentModal || app.selectionSide() != "old" || start != 2 || end != 3 {
		t.Fatalf("modal = %v, selection = %s:%d-%d; want old:2-3 comment",
			app.modal, app.selectionSide(), start, end)
	}
}

func TestMouseClickOpensFocusedInlineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_inline", StartLine: 1, EndLine: 1, Body: "comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	target := app.contentLayout.lineRanges[1]
	x, y := contentScreenPoint(app, 10, target.start+1)
	app = clickMouse(app, x, y)
	if app.modal != noModal || app.focused != contentPane || !app.tab().cursorOnAnnotation || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("first click: modal = %v, focus = %v, annotation = %t; want focus only",
			app.modal, app.focused, app.tab().cursorOnAnnotation)
	}
	app = clickMouse(app, x, y)

	if app.modal != replyModal || app.editingID != "c_inline" {
		t.Fatalf("second click opened modal %v for %q, want reply modal for c_inline", app.modal, app.editingID)
	}
}

func TestMouseClickTogglesCommentResolution(t *testing.T) {
	for _, location := range []string{"inline", "sidebar", "file"} {
		for _, width := range []int{60, 120} {
			for _, multiFile := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/width=%d/multiFile=%t", location, width, multiFile), func(t *testing.T) {
					app := setupAppWithDoc(t, strings.Repeat("source line\n", 30))
					app.width, app.height, app.multiFile = width, 30, multiFile
					comment := review.Comment{
						ID: "thread", StartLine: 1, EndLine: 1, Body: "original",
						Author: "日本語の長い名前", Replies: []review.Reply{{Body: "reply", Author: "AI"}},
					}
					if location == "file" {
						comment.Scope, comment.StartLine, comment.EndLine = "file", 0, 0
					}
					app.tab().state.Comments = []review.Comment{comment}
					app.recalculateLayout()
					app.updateCommentSidebar()
					app.rebuildContent()
					for _, label := range []string{"☐ Resolve", "☑︎ Resolved"} {
						left, top, right, bottom := app.contentBounds()
						if location == "inline" {
							app.contentViewport.SetYOffset(max(0, app.contentLayout.actions[0].resolve.top-1))
						} else {
							left, top, right, bottom = app.commentBounds()
							app.commentViewport.SetYOffset(max(0, app.sidebarActions[0].resolve.top-1))
						}
						clicked := false
						for y, line := range strings.Split(ansi.Strip(app.View().Content), "\n") {
							if y < top || y >= bottom {
								continue
							}
							pane := ansi.Cut(line, left, right)
							if index := strings.Index(pane, label); index >= 0 {
								app = clickMouse(app, left+lipgloss.Width(pane[:index])+2, y)
								clicked = true
								break
							}
						}
						if !clicked {
							t.Fatalf("button %q not visible:\n%s", label, ansi.Strip(app.View().Content))
						}
						wantResolved := label == "☐ Resolve"
						for _, comments := range [][]review.Comment{app.tab().state.Comments, app.session.FileComments(app.tab().path)} {
							if len(comments) != 1 || comments[0].Resolved != wantResolved {
								t.Fatalf("comments = %+v, want resolved=%t", comments, wantResolved)
							}
						}
						if app.modal != noModal {
							t.Fatalf("resolve click opened modal %v", app.modal)
						}
						// Resolving folds the thread by releasing focus; reopening
						// keeps it focused so the history is visible.
						view := ansi.Strip(app.commentViewport.View())
						onThread := app.focused == commentPane && app.tab().sidebarCursor == 0
						if location == "inline" {
							view = ansi.Strip(app.contentViewport.View())
							onThread = app.focused == contentPane && app.tab().cursorOnAnnotation
						}
						if onThread == wantResolved {
							t.Fatalf("after %q: focused=%v onAnnotation=%t, want on thread=%t", label, app.focused, app.tab().cursorOnAnnotation, !wantResolved)
						}
						if strings.Contains(view, "original") == wantResolved {
							t.Fatalf("after %q: history visible=%t, want %t:\n%s", label, !wantResolved, !wantResolved, view)
						}
					}
				})
			}
		}
	}
}

func TestResolveButtonPositionSurvivesCollapse(t *testing.T) {
	for _, width := range []int{20, 24, 32, 40} {
		for _, replies := range []int{0, 2} {
			t.Run(fmt.Sprintf("width=%d/replies=%d", width, replies), func(t *testing.T) {
				app := setupAppWithDoc(t, "source\n")
				comment := review.Comment{ID: "thread", StartLine: 1, EndLine: 1, Body: "body", Author: "Long reviewer name"}
				for range replies {
					comment.Replies = append(comment.Replies, review.Reply{Body: "reply"})
				}
				var inline, sidebar mouseRect
				for _, resolved := range []bool{false, true} {
					comment.Resolved = resolved
					comment.Scope = "line"
					box, buttons := app.renderAnnotationBox(newAnnotation(comment), width, false)
					button := buttons.resolve
					if resolved {
						if button != inline || strings.Contains(ansi.Strip(box), comment.Author) {
							t.Fatalf("collapsed inline button = %+v, want %+v; box=%q", button, inline, ansi.Strip(box))
						}
					} else {
						inline = button
					}
					comment.Scope = "file"
					app.tab().state.Comments = []review.Comment{comment}
					app.commentViewport.SetWidth(width)
					app.commentViewport.SetHeight(20)
					app.updateCommentSidebar()
					button = app.sidebarActions[0].resolve
					if resolved {
						if button != sidebar || strings.Contains(ansi.Strip(app.commentViewport.View()), comment.Author) {
							t.Fatalf("collapsed sidebar button = %+v, want %+v", button, sidebar)
						}
					} else {
						sidebar = button
					}
				}
			})
		}
	}
}

func TestCommentHeaderActionsAlignRight(t *testing.T) {
	const label = "comment L1 · 2 replies" // 22 columns
	for _, tc := range []struct {
		width     int
		deletable bool
		rows      int
		arrows    bool
	}{
		{width: 80, deletable: false, rows: 1, arrows: true},
		{width: 80, deletable: true, rows: 1, arrows: true},
		{width: 40, deletable: false, rows: 1, arrows: false}, // arrows go before wrapping
		{width: 40, deletable: true, rows: 1, arrows: false},
		{width: 24, deletable: false, rows: 2, arrows: true}, // a new row has room again
		{width: 24, deletable: true, rows: 2, arrows: true},
		{width: 16, deletable: false, rows: 3, arrows: false}, // the label itself wraps
		{width: 16, deletable: true, rows: 3, arrows: false},
	} {
		t.Run(fmt.Sprintf("width=%d/deletable=%t", tc.width, tc.deletable), func(t *testing.T) {
			var open, resolved commentHeaderRegion
			for _, state := range []bool{false, true} {
				header, region := renderCommentHeader(label, state, tc.deletable, tc.width)
				rows := strings.Split(ansi.Strip(header), "\n")
				if len(rows) != tc.rows || region.resolve.top != tc.rows-1 {
					t.Fatalf("header rows = %q, actions at row %d, want %d rows", rows, region.resolve.top, tc.rows)
				}
				row := rows[region.resolve.top]
				if lipgloss.Width(row) != tc.width {
					t.Fatalf("action row %q is %d wide, want %d", row, lipgloss.Width(row), tc.width)
				}
				last := region.resolve.right
				if tc.arrows {
					if got := ansi.Cut(row, region.up.left, region.up.right); got != "↑ " {
						t.Fatalf("up region contains %q, want ↑", got)
					}
					if got := ansi.Cut(row, region.down.left, region.down.right); got != " ↓" {
						t.Fatalf("down region contains %q, want ↓", got)
					}
					last = region.down.right
				} else if region.up.left != region.up.right || region.down.left != region.down.right || strings.ContainsAny(row, "↑↓") {
					t.Fatalf("arrows should be omitted: row %q, region %+v", row, region)
				}
				slot := ansi.Cut(row, last+2, last+3)
				if tc.deletable {
					if got := ansi.Cut(row, region.delete.left, region.delete.right); got != "x" || region.delete.left != last+2 {
						t.Fatalf("delete region %+v contains %q, want x after %d", region.delete, got, last)
					}
				} else if region.delete.left != region.delete.right || slot != " " {
					t.Fatalf("undeletable header should keep a blank x slot: region %+v, slot %q", region.delete, slot)
				}
				if last+3 != tc.width {
					t.Fatalf("actions end at %d, want right edge %d", last+3, tc.width)
				}
				if state {
					resolved = region
				} else {
					open = region
				}
			}
			if open != resolved {
				t.Fatalf("regions differ by resolution:\nopen     = %+v\nresolved = %+v", open, resolved)
			}
		})
	}
}

func TestMouseClickHeaderArrowsMoveBetweenComments(t *testing.T) {
	for _, location := range []string{"inline", "sidebar"} {
		t.Run(location, func(t *testing.T) {
			app := setupAppWithDoc(t, strings.Repeat("source\n", 30))
			app.width, app.height = 120, 40 // wide enough for the sidebar to show the arrows
			app.tab().state.Comments = []review.Comment{
				{ID: "first", StartLine: 1, EndLine: 1, Body: "one", Author: "AI"},
				{ID: "second", StartLine: 3, EndLine: 3, Body: "two", Author: "AI"},
			}
			app.recalculateLayout()
			app.updateCommentSidebar()
			app.rebuildContent()

			click := func(index int, rect func(commentHeaderRegion) mouseRect) {
				t.Helper()
				regions := app.contentLayout.actions
				left, top, _, _ := app.contentBounds()
				if location == "sidebar" {
					regions = app.sidebarActions
					left, top, _, _ = app.commentBounds()
					left += paneHorizontalPadding
				}
				r := rect(regions[index])
				if r.left == r.right {
					t.Fatalf("%s header %d has no such button: %+v", location, index, regions[index])
				}
				app = clickMouse(app, left+r.left, top+r.top)
			}
			current := func() string {
				t.Helper()
				tab := app.tab()
				if !tab.cursorOnAnnotation || app.focused != contentPane {
					t.Fatalf("cursor not on an annotation in the content pane: focused=%v onAnnotation=%t", app.focused, tab.cursorOnAnnotation)
				}
				return app.annotationsAfterLine(tab.cursorLine, tab.cursorSide)[tab.cursorAnnoIdx].id
			}

			click(0, func(r commentHeaderRegion) mouseRect { return r.down })
			if got := current(); got != "second" {
				t.Fatalf("after ↓ on first: cursor on %q, want second", got)
			}
			click(1, func(r commentHeaderRegion) mouseRect { return r.up })
			if got := current(); got != "first" {
				t.Fatalf("after ↑ on second: cursor on %q, want first", got)
			}
			click(0, func(r commentHeaderRegion) mouseRect { return r.up })
			if got := current(); got != "second" {
				t.Fatalf("↑ on the first comment should wrap like [, got %q", got)
			}
			if app.modal != noModal {
				t.Fatalf("arrow click opened modal %v", app.modal)
			}
			click(1, func(r commentHeaderRegion) mouseRect { return r.down })
			if app.modal != finishModal {
				t.Fatalf("↓ on the last comment opened modal %v, want the finish dialog", app.modal)
			}
			if got := app.selectedCommentID(); got != "second" {
				t.Fatalf("↓ on the last comment moved to %q", got)
			}
		})
	}
}

func TestMouseClickHeaderDelete(t *testing.T) {
	for _, location := range []string{"inline", "sidebar", "file"} {
		for _, width := range []int{60, 120} {
			for _, resolved := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/width=%d/resolved=%t", location, width, resolved), func(t *testing.T) {
					app := setupAppWithDoc(t, strings.Repeat("source\n", 30))
					app.width, app.height = width, 30
					comment := review.Comment{ID: "delete", StartLine: 1, EndLine: 1, Body: "delete me",
						Author: app.author, ReviewRound: app.reviewRound(), Resolved: resolved}
					if location == "file" {
						comment.Scope, comment.StartLine, comment.EndLine = "file", 0, 0
					}
					app.tab().state.Comments = []review.Comment{comment}
					app.recalculateLayout()
					app.updateCommentSidebar()
					app.rebuildContent()
					for _, confirm := range []bool{false, true} {
						region := app.contentLayout.actions
						left, top, _, _ := app.contentBounds()
						right := app.contentViewport.Width() - 2
						if location != "inline" {
							region = app.sidebarActions
							left, top, _, _ = app.commentBounds()
							left += paneHorizontalPadding
							right = app.commentViewport.Width()
						}
						button := region[0].delete
						if button.right != right || button.left <= region[0].resolve.right {
							t.Fatalf("delete button = %+v, want right edge %d after resolve", button, right)
						}
						button.left, button.right = button.left+left, button.right+left
						button.top, button.bottom = button.top+top, button.bottom+top
						rows := strings.Split(ansi.Strip(app.View().Content), "\n")
						if got := ansi.Cut(rows[button.top], button.left, button.right); got != "x" {
							t.Fatalf("delete region contains %q, want x", got)
						}
						app = clickMouse(app, button.left, button.top)
						if app.modal != deleteConfirmModal || app.editingID != comment.ID || len(app.tab().state.Comments) != 1 {
							t.Fatal("delete click should ask for confirmation without deleting")
						}
						if confirm {
							app = pressKey(app, 'y')
							if len(app.tab().state.Comments) != 0 || len(app.session.FileComments(app.tab().path)) != 0 {
								t.Fatal("confirmed deletion should be persisted")
							}
						} else {
							app = pressKey(app, tea.KeyEscape)
							if app.modal != noModal || len(app.tab().state.Comments) != 1 {
								t.Fatal("cancel should keep the comment")
							}
						}
					}
				})
			}
		}
	}
}

func TestHeaderDeleteHonorsExistingRestrictions(t *testing.T) {
	for _, reason := range []string{"other author", "previous round", "has replies"} {
		t.Run(reason, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			comment := review.Comment{ID: "keep", StartLine: 1, EndLine: 1, Body: "keep me", Author: app.author, ReviewRound: app.reviewRound()}
			switch reason {
			case "other author":
				comment.Author = "Someone else"
			case "previous round":
				comment.ReviewRound--
			case "has replies":
				comment.Replies = []review.Reply{{Body: "reply"}}
			}
			app.tab().state.Comments = []review.Comment{comment}
			_, buttons := app.renderAnnotationBox(newAnnotation(comment), 60, false)
			app.updateCommentSidebar()
			if buttons.delete.left != buttons.delete.right || app.sidebarActions[0].delete.left != app.sidebarActions[0].delete.right {
				t.Fatal("ineligible comment should not have a delete button")
			}
			app.openCommentDelete(comment.ID)
			if app.modal != noModal || len(app.tab().state.Comments) != 1 {
				t.Fatal("ineligible comment should not be deleted")
			}
		})
	}
}

func TestMouseClickOpensFocusedSidebarComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.tabs[0].state.Comments = []review.Comment{{
		ID: "c_sidebar", StartLine: 1, EndLine: 1, Body: "comment",
	}}
	app.updateCommentSidebar()
	app.rebuildContent()

	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+1, top+1)
	if app.modal != noModal {
		t.Fatalf("first click opened modal %v, want focus only", app.modal)
	}
	app = clickMouse(app, left+1, top+1)

	if app.modal != replyModal || app.editingID != "c_sidebar" {
		t.Fatalf("second click opened modal %v for %q, want reply modal for c_sidebar", app.modal, app.editingID)
	}
}

func TestMouseClickSelectsWrappedSidebarCommentFromRenderedRows(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width = 60
	app.height = 24
	app.tabs[0].state.Comments = []review.Comment{
		{ID: "c_first", StartLine: 1, EndLine: 1, Body: strings.Repeat("wrapped sidebar text ", 8)},
		{ID: "c_second", StartLine: 3, EndLine: 3, Body: "second comment"},
	}
	app.recalculateLayout()
	app.updateCommentSidebar()
	app.rebuildContent()
	firstSecondRow := -1
	for y, target := range app.sidebarTargets {
		if target == 1 {
			firstSecondRow = y
			break
		}
	}
	if firstSecondRow < 4 {
		t.Fatalf("second sidebar item starts at row %d, want first item to wrap", firstSecondRow)
	}
	left, top, _, _ := app.commentBounds()

	app = clickMouse(app, left+1, top+1+firstSecondRow)

	if app.focused != commentPane || app.tab().sidebarCursor != 1 || app.tab().cursorLine != 3 {
		t.Fatalf("focus = %v, sidebar = %d, line = %d; want wrapped-row map to select second comment",
			app.focused, app.tab().sidebarCursor, app.tab().cursorLine)
	}
}

func TestMouseClickCommentPaneTabFocusesSidebar(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width, app.height = 80, 24
	app.recalculateLayout()
	app.rebuildContent()

	tabLeft, y := app.contentPaneWidth(), app.headerHeight()+1
	tabs := app.sidebarTabs()
	tabsWidth := lipgloss.Width(tabs[0]) + lipgloss.Width(tabs[1])
	app = clickMouse(app, tabLeft+tabsWidth, y)
	if app.focused != contentPane {
		t.Fatalf("focus = %v, want content pane after clicking beside the sidebar tabs", app.focused)
	}

	app = clickMouse(app, tabLeft+2, y)
	if app.focused != commentPane || app.sidebarView != filesView {
		t.Fatalf("focus = %v, view = %v; want the Files tab focused", app.focused, app.sidebarView)
	}

	app = clickMouse(app, tabLeft+lipgloss.Width(tabs[0])+1, y)

	if app.focused != commentPane || app.sidebarView != commentsView {
		t.Fatalf("focus = %v, view = %v; want the Comments tab focused", app.focused, app.sidebarView)
	}
	rows := strings.Split(app.View().Content, "\n")
	if tabRow := ansi.Strip(rows[y]); !strings.Contains(tabRow, "│ Files ││ Comments (0) │") {
		t.Fatalf("tab row = %q, want the sidebar tabs", tabRow)
	}
	if got := ansi.Cut(rows[y], tabLeft, tabLeft+1); !strings.Contains(got, "\x1b[94m") {
		t.Fatalf("focused comment pane border = %q, want bright blue", got)
	}
}
