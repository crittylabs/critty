package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/review"
)

func TestFileTabNavigationKeepsPaneFocus(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		for _, comments := range []bool{false, true} {
			t.Run(fmt.Sprintf("sidebar=%t/comments=%t", sidebar, comments), func(t *testing.T) {
				app := setupAppWithDoc(t, "first file\n")
				second := setupAppWithDoc(t, "second file\n")
				app.tabs = append(app.tabs, second.tabs[0])
				app.multiFile = true
				if sidebar {
					app.focused = commentPane
				}
				if comments {
					for i := range app.tabs {
						app.tabs[i].state.Comments = []review.Comment{{
							ID: fmt.Sprintf("comment-%d", i), Scope: "file", Body: fmt.Sprintf("file %d", i),
						}}
					}
				}
				app.updateCommentSidebar()
				focus := app.focused
				for _, step := range []struct {
					mod  tea.KeyMod
					want int
				}{
					{tea.ModShift, 0},
					{0, 1},
					{0, 1},
					{tea.ModShift, 0},
				} {
					updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: step.mod})
					app = *updated.(*AppModel)
					if app.activeTab != step.want || app.focused != focus {
						t.Fatalf("tab/focus = %d/%v, want %d/%v", app.activeTab, app.focused, step.want, focus)
					}
					if comments && app.tab().sidebarItems[0].id != fmt.Sprintf("comment-%d", step.want) {
						t.Fatal("sidebar did not switch to the active file's comments")
					}
				}
			})
		}
	}
}

func TestLineNavigation(t *testing.T) {
	for _, tt := range []struct {
		name        string
		key         rune
		start, want int
	}{
		{"home", tea.KeyHome, 8, 1},
		{"end", tea.KeyEnd, 1, 10},
		{"page down", tea.KeyPgDown, 1, 3},
		{"page up", tea.KeyPgUp, 8, 6},
		{"page up at start", tea.KeyPgUp, 2, 1},
		{"page down at end", tea.KeyPgDown, 9, 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, strings.Repeat("line\n", 9)+"line")
			app.contentViewport.SetHeight(5)
			app.tab().cursorLine = tt.start
			app = pressKey(app, tt.key)
			if got := app.tab().cursorLine; got != tt.want {
				t.Errorf("cursor line = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCommentNavigationCrossesFiles(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.tabs[0].cursorLine = 4
	app.tabs[0].cursorOnAnnotation = true

	app = pressKey(app, ']')

	if app.activeTab != 2 || app.tab().cursorLine != 1 || !app.tab().cursorOnAnnotation {
		t.Fatalf("next comment = tab %d, line %d; want tab 2, line 1", app.activeTab, app.tab().cursorLine)
	}

	app = pressKey(app, '[')

	if app.activeTab != 0 || app.tab().cursorLine != 4 || !app.tab().cursorOnAnnotation {
		t.Fatalf("previous comment = tab %d, line %d; want tab 0, line 4", app.activeTab, app.tab().cursorLine)
	}
}

func TestCommentNavigationFinishesAfterLastAndWrapsBackward(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.tabs[0].cursorLine = 2

	app = pressKey(app, ']')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("first comment = tab %d, annotation %d; want tab 0, annotation 0", app.activeTab, app.tab().cursorAnnoIdx)
	}

	app = pressKey(app, ']')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 1 {
		t.Fatalf("second comment = tab %d, annotation %d; want tab 0, annotation 1", app.activeTab, app.tab().cursorAnnoIdx)
	}

	// ] on the last thread of the review opens the finish dialog and stays put.
	app.activeTab = 2
	app.tabs[2].cursorLine = 3
	app.tabs[2].cursorOnAnnotation = true
	app.tabs[2].cursorAnnoIdx = 0
	app = pressKey(app, ']')
	if app.modal != finishModal {
		t.Fatalf("] on the last thread opened modal %v, want the finish dialog", app.modal)
	}
	if app.activeTab != 2 || app.tab().cursorLine != 3 || !app.tab().cursorOnAnnotation {
		t.Fatalf("] on the last thread moved to tab %d, line %d", app.activeTab, app.tab().cursorLine)
	}
	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatal("esc did not close the finish dialog")
	}

	// From a source line past every thread, ] still wraps to the first thread.
	app.tab().cursorOnAnnotation = false
	app.tab().cursorLine = 4
	app = pressKey(app, ']')
	if app.modal != noModal || app.activeTab != 0 || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("wrapped next from source = modal %v, tab %d, line %d, annotation %d", app.modal, app.activeTab, app.tab().cursorLine, app.tab().cursorAnnoIdx)
	}

	app = pressKey(app, '[')
	if app.activeTab != 2 || app.tab().cursorLine != 3 {
		t.Fatalf("wrapped previous = tab %d, line %d; want tab 2, line 3", app.activeTab, app.tab().cursorLine)
	}
}

func TestChangeNavigationCrossesFilesWithoutWrapping(t *testing.T) {
	app := newChangeNavigationTestApp()
	for i := range app.tabs {
		app.tabs[i].state.Comments = nil
	}
	app.tabs[0].cursorLine = 4

	app = pressKey(app, 'n')
	if app.activeTab != 2 || app.tab().cursorLine != 1 {
		t.Fatalf("next change = tab %d, line %d; want tab 2, line 1", app.activeTab, app.tab().cursorLine)
	}

	app = pressKey(app, 'N')
	if app.activeTab != 0 || app.tab().cursorLine != 4 {
		t.Fatalf("previous change = tab %d, line %d; want tab 0, line 4", app.activeTab, app.tab().cursorLine)
	}

	app.activeTab = 2
	app.tabs[2].cursorLine = 3
	app = pressKey(app, 'n')
	if app.activeTab != 2 || app.tab().cursorLine != 3 {
		t.Fatalf("next change wrapped to tab %d, line %d", app.activeTab, app.tab().cursorLine)
	}

	app.activeTab = 0
	app.tabs[0].cursorLine = 2
	app = pressKey(app, 'N')
	if app.activeTab != 0 || app.tab().cursorLine != 2 {
		t.Fatalf("previous change wrapped to tab %d, line %d", app.activeTab, app.tab().cursorLine)
	}
}

func TestChangeNavigationIncludesUnresolvedComments(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs[0].state.Comments[1].Resolved = true
	app.tabs[0].state.Comments = append(app.tabs[0].state.Comments,
		review.Comment{ID: "same-line", StartLine: 2, EndLine: 2},
		review.Comment{ID: "inside-hunk", StartLine: 3, EndLine: 3},
		review.Comment{ID: "file", Scope: "file"})
	app.tabs[0].changeChunks[0].endLine = 3
	app.tabs[1].state.Comments = []review.Comment{{ID: "comments-only", Scope: "file"}}
	app = pressKey(app, 'N')
	if app.selectedCommentID() != "file" || app.focused != contentPane {
		t.Fatal("previous target should be the file comment")
	}
	type stop struct {
		tab  int
		line int
		id   string
	}
	stops := []stop{
		{0, 0, "file"}, {0, 2, ""}, {0, 2, "first-a"}, {0, 2, "same-line"},
		{0, 3, "inside-hunk"}, {0, 4, ""}, {0, 4, "first-c"},
		{1, 0, "comments-only"}, {2, 1, ""}, {2, 1, "last-a"},
		{2, 3, ""}, {2, 3, "last-b"},
	}
	check := func(want stop) {
		t.Helper()
		if app.activeTab != want.tab || app.selectedCommentID() != want.id ||
			(want.line != 0 && app.tab().cursorLine != want.line) {
			t.Fatalf("got tab=%d line=%d comment=%q, want %+v", app.activeTab, app.tab().cursorLine, app.selectedCommentID(), want)
		}
		if want.id == "" && app.focused != contentPane {
			t.Fatal("change should focus content")
		}
	}
	for _, want := range stops[1:] {
		app = pressKey(app, 'n')
		check(want)
	}
	app = pressKey(app, 'n')
	check(stops[len(stops)-1])
	for i := len(stops) - 2; i >= 0; i-- {
		app = pressKey(app, 'N')
		check(stops[i])
	}
	app = pressKey(app, 'N')
	check(stops[0])
}

func TestChangeNavigationFromResolvedAndHiddenComments(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs[0].state.Comments[0].Resolved = true
	app.tabs[0].cursorLine = 2
	app.tabs[0].cursorOnAnnotation = true
	app = pressKey(app, 'n')
	if app.selectedCommentID() != "first-b" {
		t.Fatal("should advance from a resolved comment to the next unresolved thread")
	}
	app.tabs[0].cursorOnAnnotation = false
	app.hideComments = true
	app = pressKey(app, 'n')
	if app.hideComments || app.selectedCommentID() != "first-b" {
		t.Fatal("should reveal the unresolved navigation target")
	}
}

func TestChangeNavigationStopsAtTrailingDeletion(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs = app.tabs[:1]
	app.tab().state.Comments = nil
	app.tab().changeChunks = []changeChunk{{startLine: 2, endLine: 2}, {startLine: 5, endLine: 5}}
	app.tab().cursorLine = 4
	app = pressKey(app, 'n')
	app = pressKey(app, 'n')
	if app.tab().cursorLine != 5 {
		t.Fatal("navigation should stop at the trailing deletion")
	}
	app = pressKey(app, 'N')
	if app.tab().cursorLine != 2 {
		t.Fatal("previous should return from the trailing deletion to the earlier hunk")
	}
}

func TestAngleBracketsMoveToFileBoundaries(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.tabs[0].cursorLine = 2
	app.tabs[0].cursorOnAnnotation = true

	app = pressKey(app, '>')
	if app.tab().cursorLine != 4 || app.tab().cursorOnAnnotation {
		t.Fatalf("> moved to line %d, annotation=%t; want line 4", app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}

	app = pressKey(app, '<')
	if app.tab().cursorLine != 1 || app.tab().cursorOnAnnotation {
		t.Fatalf("< moved to line %d, annotation=%t; want line 1", app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}

}

func TestFooterKeepsOnlyNonstandardNavigationHints(t *testing.T) {
	app := newChangeNavigationTestApp()
	app.height = 24
	footer := app.renderFooter()

	for _, omitted := range []string{"j/k", "shift+↑↓", "</>"} {
		if strings.Contains(footer, omitted) {
			t.Errorf("footer contains fallback navigation hint %q: %q", omitted, footer)
		}
	}
	for _, retained := range []string{"[/]", "?"} {
		if !strings.Contains(footer, retained) {
			t.Errorf("footer does not contain %q: %q", retained, footer)
		}
	}

	app.tabs[0].selecting = true
	if footer := app.renderFooter(); strings.Contains(footer, "j/k") || !strings.Contains(footer, "?") {
		t.Errorf("selection footer = %q, want help without j/k", footer)
	}
}

func TestScrollToChunk_SourceWithLongLines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 500) // longer than the viewport width
	}
	app := newScrollTestApp("test.go", lines, false, 80, 10)
	app.tabs[0].changedLines = map[int]bool{30: true}
	app.rebuildContent()

	app.scrollToChunk(changeChunk{startLine: 30, endLine: 30})

	// Chunk start minus padding should use the row recorded by rendering.
	want := app.contentLayout.lineRanges[30-chunkScrollPadding].start
	if got := app.contentViewport.YOffset(); got != want {
		t.Errorf("expected YOffset %d, got %d", want, got)
	}
}

func TestFileCommentArrowNavigation(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.contentViewport.SetWidth(80)
	app.contentViewport.SetHeight(8)
	app.tab().state.Comments = []review.Comment{
		{ID: "first", Scope: "file", Body: "first thread"},
		{ID: "second", Scope: "file", Body: "second thread"},
	}
	app.rebuildContent()
	for _, id := range []string{"second", "first", "first"} {
		app = pressKey(app, 'k')
		if app.selectedCommentID() != id || !strings.Contains(app.contentViewport.View(), id+" thread") {
			t.Fatalf("up selected %q, want visible %q", app.selectedCommentID(), id)
		}
	}
	app = pressKey(app, 'v')
	if app.tab().selecting {
		t.Fatal("file comment should not start a line selection")
	}
	app = pressKey(app, 'j')
	if app.selectedCommentID() != "second" {
		t.Fatal("down should select the next file comment")
	}
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 1 || app.tab().cursorOnAnnotation {
		t.Fatal("down should return to the first source line")
	}
}

func TestSidebarNavigationToFileCommentKeepsLineCursor(t *testing.T) {
	line := testComment()
	line.ID = "c_line"
	line.StartLine = 3
	line.EndLine = 3
	file := review.Comment{ID: "c_file", Scope: "file", Body: "whole file"}
	app := setupAppWithDoc(t, "one\ntwo\nthree\n")
	app.tabs[0].state.Comments = []review.Comment{file, line}
	app.focused = commentPane
	app.tabs[0].cursorLine = 3
	app.tabs[0].sidebarCursor = 1
	app.updateCommentSidebar()

	app = pressKey(app, 'k')

	if app.tabs[0].sidebarCursor != 0 {
		t.Fatalf("sidebar cursor = %d, want file comment", app.tabs[0].sidebarCursor)
	}
	if app.tabs[0].cursorLine != 3 {
		t.Fatalf("line cursor = %d, want it unchanged", app.tabs[0].cursorLine)
	}
}

func TestCommentNavigationSkipsResolvedComments(t *testing.T) {
	app := newCommentNavigationTestApp()
	for i := range app.tabs {
		for j := range app.tabs[i].state.Comments {
			app.tabs[i].state.Comments[j].Resolved = true
		}
		app.tabs[i].state.Comments = append(app.tabs[i].state.Comments,
			review.Comment{ID: "resolved-file", Scope: "file", Resolved: true})
	}
	app.tabs[0].state.Comments[1].Resolved = false
	app.tabs[2].state.Comments[1].Resolved = false
	for _, step := range []struct {
		key rune
		tab int
	}{{'}', 0}, {'}', 2}, {'}', -1}, {'{', 0}, {'{', 2}, {'{', 0}} {
		app = pressKey(app, step.key)
		if step.tab < 0 {
			if app.modal != finishModal {
				t.Fatalf("key %c on the last unresolved thread opened modal %v, want the finish dialog", step.key, app.modal)
			}
			app = pressKey(app, tea.KeyEscape)
			continue
		}
		anns := app.annotationsAfterLine(app.tab().cursorLine, app.tab().cursorSide)
		if app.modal != noModal || app.activeTab != step.tab || app.focused != contentPane || !app.tab().cursorOnAnnotation {
			t.Fatalf("key %c: tab %d, want %d with inline focus", step.key, app.activeTab, step.tab)
		}
		if anns[app.tab().cursorAnnoIdx].resolved {
			t.Fatalf("key %c focused a resolved comment", step.key)
		}
	}
	// Navigation from a resolved annotation preserves the order of same-line threads.
	app.activeTab = 0
	app.tab().cursorLine, app.tab().cursorAnnoIdx = 2, 0
	app = pressKey(app, '}')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 1 {
		t.Fatal("next from resolved annotation skipped its unresolved neighbor")
	}
	app.tabs[0].state.Comments[1].Resolved = true
	app.tabs[2].state.Comments[1].Resolved = true
	if app.jumpToComment(1, false) || app.jumpToComment(-1, false) {
		t.Fatal("navigation found a target in an entirely resolved review")
	}
}

func TestCommentNavigationVisitsResolvedComments(t *testing.T) {
	app := newCommentNavigationTestApp()
	for i := range app.tabs {
		for j := range app.tabs[i].state.Comments {
			app.tabs[i].state.Comments[j].Resolved = true
		}
	}
	app.tabs[1].state.Comments = []review.Comment{{ID: "file", Scope: "file", Body: "file comment", Resolved: true}}
	for _, step := range []struct {
		key rune
		id  string
	}{
		{']', "first-a"}, {']', "first-b"}, {']', "first-c"},
		{']', "file"}, {']', "last-a"}, {']', "last-b"}, {']', ""},
		{'[', "last-a"}, {'[', "file"},
		{'[', "first-c"}, {'[', "first-b"}, {'[', "first-a"},
	} {
		app = pressKey(app, step.key)
		if step.id == "" {
			if app.modal != finishModal {
				t.Fatalf("key %c on the last resolved thread opened modal %v, want the finish dialog", step.key, app.modal)
			}
			app = pressKey(app, tea.KeyEscape)
			continue
		}
		targets := app.commentTargets(app.activeTab)
		current := app.currentCommentTarget(targets)
		if app.modal != noModal || current < 0 || targets[current].id != step.id {
			t.Fatalf("key %c: selected target %d in %+v, want %s", step.key, current, targets, step.id)
		}
	}
	// Backward navigation from a source line also includes resolved threads.
	app.tab().cursorOnAnnotation = false
	app.tab().cursorLine = 3
	app = pressKey(app, '[')
	if app.activeTab != 0 || app.tab().cursorAnnoIdx != 1 || !app.tab().cursorOnAnnotation {
		t.Fatal("previous from source skipped a resolved thread")
	}
	if app.jumpToComment(1, false) || app.jumpToComment(-1, false) {
		t.Fatal("unresolved navigation included resolved threads")
	}
}

func TestCommentNavigationVisitsFileComments(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.commentViewport.SetWidth(40)
	app.commentViewport.SetHeight(20)
	fileComment := review.Comment{ID: "first-file", Scope: "file", Body: "file-wide note"}
	app.tabs[0].state.Comments = append(app.tabs[0].state.Comments, fileComment)
	app.updateCommentSidebar()

	// Backward from the top of the first file lands on its file comment.
	app = pressKey(app, '[')
	if app.activeTab != 0 || app.focused != contentPane || app.selectedCommentID() != fileComment.ID {
		t.Fatalf("previous from top = tab %d, focus %v, sidebar %d; want the inline file comment",
			app.activeTab, app.focused, app.tab().sidebarCursor)
	}

	// Forward from the file comment continues to the first line comment.
	app = pressKey(app, ']')
	if app.focused != contentPane || app.tab().cursorLine != 2 || !app.tab().cursorOnAnnotation || app.tab().cursorAnnoIdx != 0 {
		t.Fatalf("next from file comment = focus %v, line %d, annotation %t/%d; want line 2 annotation 0",
			app.focused, app.tab().cursorLine, app.tab().cursorOnAnnotation, app.tab().cursorAnnoIdx)
	}

	// Backward from the first line comment returns to the file comment.
	app = pressKey(app, '[')
	if app.focused != contentPane || app.selectedCommentID() != fileComment.ID {
		t.Fatalf("previous from line comment did not return to the file comment (focus %v)", app.focused)
	}

	// The last comment of the review has no next: ] opens the finish dialog.
	app.focused = contentPane
	app.activeTab = 2
	app.tabs[2].cursorLine = 3
	app.tabs[2].cursorOnAnnotation = true
	app = pressKey(app, ']')
	if app.modal != finishModal || app.activeTab != 2 {
		t.Fatalf("] on the last thread = modal %v, tab %d; want the finish dialog on tab 2", app.modal, app.activeTab)
	}
	app = pressKey(app, tea.KeyEscape)

	// Wrapping backward from the file comment reaches the last comment of the review.
	app.activeTab = 0
	app.tabs[0].cursorLine = 0
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0
	if app.selectedCommentID() != fileComment.ID {
		t.Fatalf("selected %q, want the file comment", app.selectedCommentID())
	}
	app = pressKey(app, '[')
	if app.activeTab != 2 || app.focused != contentPane || app.tab().cursorLine != 3 {
		t.Fatalf("wrapped previous = tab %d, line %d; want tab 2, line 3", app.activeTab, app.tab().cursorLine)
	}
}
