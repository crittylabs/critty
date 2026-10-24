package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestClickRenderedDriftedBubblePreservesResolution(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{60, 24}, {80, 60}, {100, 40}, {160, 24}} {
		width, height := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
			app := setupParallelAppWithDoc(t, strings.Repeat("source line\n", 20))
			app.width, app.height = width, height
			app.recalculateLayout()
			app.tab().state.Comments = []review.Comment{
				{ID: "file", Scope: "file", Author: "Other", Body: "file feedback"},
				{ID: "a", StartLine: 3, EndLine: 3, Drifted: true, Resolved: true, Author: "Other", Body: "resolved"},
				{ID: "b", StartLine: 6, EndLine: 6, Drifted: true, Author: "Other", Body: "unresolved"},
				{ID: "d1", StartLine: 12, EndLine: 12, Drifted: true, Resolved: true, Author: "Other", Body: "group resolved"},
				{ID: "d2", StartLine: 12, EndLine: 12, Drifted: true, Author: "Other", Body: "group unresolved"},
			}
			app.rebuildContent()
			app.updateCommentSidebar()
			for step := range 30 {
				rows := strings.Split(ansi.Strip(app.View().Content), "\n")
				var bubbles []tea.Mouse
				for y, row := range rows {
					if x := strings.Index(row, "💬"); x >= 0 {
						bubbles = append(bubbles, tea.Mouse{X: lipgloss.Width(row[:x]), Y: y})
					}
				}
				if len(bubbles) == 0 {
					t.Fatal("no visible bubble")
				}
				point := bubbles[step%len(bubbles)]
				app = clickMouse(app, point.X, point.Y)
				openedModal := app.modal != noModal
				app = clickMouse(app, point.X, point.Y)
				for _, c := range app.tab().state.Comments {
					want := c.ID == "a" || c.ID == "d1"
					if c.Resolved != want {
						t.Fatalf("step %d: bubble at %d,%d changed %s resolution", step, point.X, point.Y, c.ID)
					}
				}
				if openedModal {
					if app.modal == noModal {
						t.Fatal("second click dismissed the newly opened dialog")
					}
					app.modalOpenedAt = time.Now().Add(-time.Second)
					for _, region := range app.modalMouseRegions() {
						if region.action.resolve {
							c, _ := app.driftedComment(app.editingID)
							app = clickMouse(app, region.rect.left, region.rect.top)
							got, _ := app.driftedComment(c.ID)
							if got.Resolved == c.Resolved {
								t.Fatal("intentional checkbox click was suppressed")
							}
							app.toggleResolve(c.ID)
							break
						}
					}
				}
				if app.modal != noModal {
					app = pressKey(app, tea.KeyEscape)
				}
			}
		})
	}
}

func TestDriftedUnresolvedVisibleByDefault(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 100, 40
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{
		{ID: "drifted", StartLine: 1, EndLine: 1, Body: "drifted body", Drifted: true},
		{ID: "lost", StartLine: 200, EndLine: 200, Body: "lost body", Anchor: "old source"},
	}
	app.rebuildContent()
	app.updateCommentSidebar()
	for _, body := range []string{"drifted body", "lost body", "old source"} {
		if !strings.Contains(ansi.Strip(app.contentViewport.View()), body) {
			t.Fatalf("unresolved thread hidden: %q", body)
		}
	}
	if !strings.Contains(ansi.Strip(app.commentViewport.View()), "drifted body") {
		t.Fatal("unresolved drifted sidebar thread hidden")
	}
}

func TestResolvedThreadsExpandOnKeyboardFocus(t *testing.T) {
	for _, drifted := range []bool{false, true} {
		for _, fold := range []bool{false, true} {
			for _, key := range []rune{']', '[', tea.KeyDown, tea.KeyUp} {
				t.Run(fmt.Sprintf("drifted=%t/fold=%t/key=%d", drifted, fold, key), func(t *testing.T) {
					app := setupAppWithDoc(t, "one\ntwo\nthree\n")
					app.width, app.height = 120, 40
					app.openAll = fold
					app.recalculateLayout()
					app.tab().state.Comments = []review.Comment{{ID: "done", StartLine: 2, EndLine: 2, Resolved: true, Drifted: drifted, Body: "resolved body"}}
					app.tab().cursorLine = 2
					if key == '[' || key == tea.KeyUp {
						app.tab().cursorLine = 3
					}
					app.rebuildContent()
					if strings.Contains(ansi.Strip(app.contentViewport.View()), "resolved body") != fold {
						t.Fatal("resolved thread does not match the Open all setting")
					}
					app = pressKey(app, key)
					if app.selectedCommentID() != "done" || app.modal != noModal || !strings.Contains(ansi.Strip(app.contentViewport.View()), "resolved body") {
						t.Fatal("keyboard focus did not expand resolved thread inline")
					}
				})
			}
		}
	}
}

func TestDriftedMarkersOpenAndCollapseAllThreads(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("long source ", 30)+"\nsecond\n")
	app.width, app.height = 100, 80
	app.recalculateLayout()
	comments := []review.Comment{
		{ID: "normal", StartLine: 1, EndLine: 1, Body: "normal body"},
		{ID: "first", StartLine: 1, EndLine: 1, Body: "first body", Drifted: true, Quote: "original quote", Anchor: "whole anchor"},
		{ID: "second", StartLine: 1, EndLine: 1, Body: "second body", Drifted: true, Resolved: true, Anchor: "original anchor"},
	}
	app.tab().state.Comments = append([]review.Comment(nil), comments...)
	app.rebuildContent()
	app.updateCommentSidebar()
	view := ansi.Strip(app.contentViewport.View())
	if !strings.Contains(view, "normal body") || !strings.Contains(view, "💬 2") || !strings.Contains(view, "first body") || strings.Contains(view, "second body") {
		t.Fatalf("initial view = %q", view)
	}
	if len(app.contentLayout.markers) != 1 {
		t.Fatalf("markers = %+v", app.contentLayout.markers)
	}
	marker := app.contentLayout.markers[0]
	if marker.rect.top == 0 || app.contentLayout.rows[marker.rect.top].annotation {
		t.Fatal("marker must be on the last wrapped source row, not an annotation")
	}
	for _, row := range strings.Split(app.contentViewport.View(), "\n") {
		if lipgloss.Width(row) > app.contentViewport.Width() {
			t.Fatalf("row exceeds viewport: %q", row)
		}
	}
	clickMarker := func() {
		marker := app.contentLayout.markers[0]
		app.contentViewport.SetYOffset(0)
		left, top, _, _ := app.contentBounds()
		app = clickMouse(app, left+marker.rect.left, top+marker.rect.top)
	}
	clickMarker()
	if app.selectedCommentID() != "first" {
		t.Fatalf("selected = %q", app.selectedCommentID())
	}
	view = ansi.Strip(app.contentViewport.View())
	if !strings.Contains(view, "Position could not be tracked.") || !strings.Contains(view, "original quote") || strings.Contains(view, "whole anchor") {
		t.Fatalf("opened thread = %q", view)
	}
	for _, body := range []string{"first body", "second body", "original anchor"} {
		if !strings.Contains(view, body) {
			t.Fatalf("one click did not open every grouped thread: missing %q", body)
		}
	}
	clickMarker()
	view = ansi.Strip(app.contentViewport.View())
	if app.selectedCommentID() != "first" || strings.Contains(view, "first body") || strings.Contains(view, "second body") {
		t.Fatal("second click did not collapse the whole group while keeping selection")
	}
	app = pressKey(app, ']')
	if app.selectedCommentID() != "second" {
		t.Fatal("next-thread navigation skipped a folded comment")
	}
	clickMarker()
	if app.modal != noModal || app.selectedCommentID() != "second" || !app.tab().expandedDrifted["first"] || !app.tab().expandedDrifted["second"] {
		t.Fatal("clicking a group with a resolved selection must expand all threads inline")
	}
	app = pressKey(app, '[')
	if app.selectedCommentID() != "first" {
		t.Fatal("previous-thread navigation skipped a comment")
	}
	if !reflect.DeepEqual(comments, app.tab().state.Comments) {
		t.Fatal("display changed persisted comment data")
	}
}

func TestDriftedMarkerResolution(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{
		{ID: "done", StartLine: 1, EndLine: 1, Drifted: true, Resolved: true},
		{ID: "open", StartLine: 1, EndLine: 1, Drifted: true},
	}
	app.rebuildContent()
	if !strings.Contains(ansi.Strip(app.contentViewport.View()), "☐ 💬 2") {
		t.Fatal("mixed group should be unchecked when unfocused")
	}
	app = pressKey(app, ']')
	if app.selectedCommentID() != "done" || !strings.Contains(ansi.Strip(app.contentViewport.View()), "> ☑︎ 💬 2") {
		t.Fatal("focused marker should show the selected thread's resolution")
	}
	app = pressKey(app, ']')
	if app.selectedCommentID() != "open" || !strings.Contains(ansi.Strip(app.contentViewport.View()), "> ☐ 💬 2") {
		t.Fatal("focused unresolved thread should be unchecked")
	}
	app.toggleResolve("open")
	app.releaseThreadFocus()
	app.rebuildContent()
	if !strings.Contains(ansi.Strip(app.contentViewport.View()), "☑︎ 💬 2") {
		t.Fatal("fully resolved group should be checked when unfocused")
	}
}

func TestUnplacedCommentsStayReachable(t *testing.T) {
	for _, kind := range []string{"past end", "missing old side", "partial diff", "missing file", "binary"} {
		t.Run(kind, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width, app.height = 100, 35
			app.recalculateLayout()
			c := review.Comment{ID: "lost", StartLine: 2, EndLine: 2, Body: "lost body", Anchor: "original source"}
			switch kind {
			case "past end":
				c.StartLine, c.EndLine = 500, 500
			case "missing old side":
				c.Side = "old"
			case "partial diff":
				app.tab().doc.Known = map[int]bool{1: true}
			case "missing file":
				app.tab().doc = nil
				app.tab().outsideChanges = true
			case "binary":
				app.tab().isBinary = true
			}
			app.tab().state.Comments = []review.Comment{{ID: "file", Scope: "file", Body: "file body"}, c}
			app.rebuildContent()
			app.updateCommentSidebar()
			if len(app.contentLayout.markers) != 1 || app.contentLayout.markers[0].rect.top != 0 {
				t.Fatalf("missing top marker: %+v", app.contentLayout.markers)
			}
			app.tab().cursorLine = 0
			app = pressKey(app, ']')
			app = pressKey(app, ']')
			if app.selectedCommentID() != c.ID || !strings.Contains(ansi.Strip(app.contentViewport.View()), c.Anchor) {
				t.Fatalf("thread not reachable: selected %q, view %q", app.selectedCommentID(), ansi.Strip(app.contentViewport.View()))
			}
			if app.tab().state.Comments[1].Drifted || app.tab().state.Comments[1].Scope != "" {
				t.Fatal("fallback changed stored drift or scope")
			}
			app = pressKey(app, 'V')
			app.selectMarker([]string{c.ID})
			if !app.viewMode || app.modal == noModal || !strings.Contains(ansi.Strip(app.View().Content), c.Anchor) {
				t.Fatal("unplaced comment cannot be opened in View")
			}
		})
	}
}

func TestDriftedDeletedMarkerAndReadingView(t *testing.T) {
	app := setupAppWithDoc(t, "new\n")
	app.width, app.height = 100, 35
	app.recalculateLayout()
	app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "deleted"}}}
	app.tab().state.Comments = []review.Comment{{ID: "old", StartLine: 1, EndLine: 1, Side: "old", Drifted: true, Body: "body", Anchor: "old anchor"}}
	app = pressKey(app, 'V')
	marker := app.contentLayout.markers[0]
	if row := app.contentLayout.rows[marker.rect.top]; row.side != "old" || row.line != 1 {
		t.Fatalf("marker attached to wrong row: %+v", row)
	}
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+marker.rect.left, top+marker.rect.top)
	if !app.viewMode || app.modal == noModal || app.editingID != "old" || app.tab().expandedDrifted["old"] {
		t.Fatal("View marker did not open its dialog without changing expansion")
	}
	if view := ansi.Strip(app.View().Content); !strings.Contains(view, "old anchor") {
		t.Fatalf("modal lacks original context: %q", view)
	}
	app.releaseThreadFocus()
	app.tab().state.Comments[0].Resolved = true
	app.rebuildContent()
	if len(app.contentLayout.markers) != 1 {
		t.Fatal("resolved thread lost its marker")
	}
}
