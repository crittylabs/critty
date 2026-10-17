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

func TestThreadInitialOffset(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		replies    []review.Reply
		width      int
		height     int
		wantOffset int
	}{
		{name: "single long comment", body: strings.Repeat("parent\n", 20), width: 40, height: 10},
		{name: "two fit exactly", body: "parent", replies: []review.Reply{{Body: "latest"}}, width: 40, height: 5},
		{name: "two overflow by one", body: "parent", replies: []review.Reply{{Body: "latest"}}, width: 40, height: 4, wantOffset: 1},
		{name: "all three fit", body: "parent", replies: []review.Reply{{Body: "previous"}, {Body: "latest"}}, width: 40, height: 10},
		{name: "long predecessor tail", body: strings.Repeat("parent\n", 20), replies: []review.Reply{{Body: "latest"}}, width: 40, height: 10, wantOffset: 15},
		{name: "long latest starts at header", body: "parent", replies: []review.Reply{{Body: strings.Repeat("latest\n", 20)}}, width: 40, height: 10, wantOffset: 3},
		{name: "wrapped Japanese", body: strings.Repeat("日本語", 20), replies: []review.Reply{{Body: "latest"}}, width: 20, height: 5, wantOffset: 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := AppModel{}
			layout := app.layoutThread("Reviewer", tt.body, tt.replies, "", tt.width)
			if got, want := layout.initialOffset(tt.height), tt.wantOffset; got != want {
				t.Fatalf("offset = %d, want %d; lines = %q", got, want, layout.lines)
			}
			for _, line := range layout.lines {
				if ansi.StringWidth(line) > tt.width {
					t.Errorf("line exceeds width %d: %q", tt.width, line)
				}
			}
		})
	}
}

func TestThreadAuthorsKeepColorsAcrossRoles(t *testing.T) {
	app := AppModel{author: "Reviewer"}
	parent := app.commentAuthorStyle("Reviewer").Render("body")
	agent := app.commentAuthorStyle("Codex").Render("body")
	if parent == agent {
		t.Fatal("reviewer and agent share a color")
	}
	app.commentAuthorStyle("Another author")
	if got := app.commentAuthorStyle("Reviewer").Render("body"); got != parent {
		t.Fatal("adding an author changed the reviewer's color")
	}
	layout := app.layoutThread("Reviewer", "body", []review.Reply{{Author: "Codex", Body: "body"}, {Author: "Reviewer", Body: "body"}}, "", 40)
	if layout.lines[1] != parent || layout.lines[layout.starts[1]+1] != agent || layout.lines[layout.starts[2]+1] != parent {
		t.Fatalf("parent/reply colors differ for the same author: %q", layout.lines)
	}
}

func TestThreadViewportKeepsHistoryAndResetsForNewReply(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 40
	app.recalculateLayout()
	key := threadViewKey{path: app.tab().path, id: "thread"}
	body := "old first\n" + strings.Repeat("past\n", 20) + "old last"
	replies := []review.Reply{{ID: "r1", Author: "Codex", Body: "latest first\n" + strings.Repeat("new\n", 20) + "latest last"}}
	render := func() string { return ansi.Strip(app.renderThread(key, "Reviewer", body, replies, 40, true)) }
	first := render()
	if !strings.Contains(first, "latest first") || strings.Contains(first, "latest last") || strings.Contains(first, "old first") {
		t.Fatalf("initial viewport = %q", first)
	}
	height := lipgloss.Height(first)
	scroll := app.threadScrolls[key]
	scroll.manual, scroll.offset = true, 0
	app.threadScrolls[key] = scroll
	if got := render(); !strings.Contains(got, "old first") || lipgloss.Height(got) != height {
		t.Fatalf("history viewport = %q", got)
	}
	scroll.offset = scroll.maxOffset
	app.threadScrolls[key] = scroll
	if got := render(); !strings.Contains(got, "latest last") || lipgloss.Height(got) != height {
		t.Fatalf("bottom viewport = %q", got)
	}
	replies = append(replies, review.Reply{ID: "r2", Author: "Reviewer", Body: "newest short reply"})
	if got := render(); !strings.Contains(got, "newest short reply") || !strings.Contains(got, "latest last") || lipgloss.Height(got) != height {
		t.Fatalf("new reply viewport = %q", got)
	}
	rows := strings.Split(render(), "\n")
	if rows[len(rows)-2] != "newest short reply" {
		t.Fatalf("blank padding below the latest reply: %q", rows)
	}
}

func TestUnfocusedThreadShrinksToLatestComment(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 40
	app.recalculateLayout()
	key := threadViewKey{id: "thread"}
	body := strings.Repeat("history\n", 30)
	replies := []review.Reply{{Author: "Bob", Body: "short reply"}}
	for _, focused := range []bool{false, true, false} {
		view := app.renderThread(key, "Reviewer", body, replies, 40, focused)
		if focused {
			if lipgloss.Height(view) != maxThreadLines+1 {
				t.Fatalf("focused thread did not expand: %q", view)
			}
		} else if got := ansi.Strip(view); got != "Bob: short reply" {
			t.Fatalf("unfocused thread = %q, want only the latest reply on one row", got)
		}
	}
	replies[0].Body = strings.Repeat("long reply\n", 30)
	if got := app.renderThread(key, "Reviewer", body, replies, 40, false); lipgloss.Height(got) > maxThreadLines+1 || !strings.Contains(got, "↓") {
		t.Fatalf("long unfocused reply = %q", got)
	}
}

func TestDriftedThreadFocusStartsAtLatestReply(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		app := setupAppWithDoc(t, "source\n")
		app.width, app.height = 120, 40
		app.recalculateLayout()
		c := review.Comment{ID: "drifted", StartLine: 1, EndLine: 1, Drifted: true,
			Quote: "original source", Body: strings.Repeat("history\n", 20),
			Replies: []review.Reply{{ID: "reply", Body: "latest reply"}}}
		app.tab().state.Comments = []review.Comment{c}
		key := threadViewKey{path: app.tab().path, id: c.ID, sidebar: sidebar}
		for _, focused := range []bool{false, true, false, true} {
			view := ansi.Strip(app.renderThread(key, c.Author, c.Body, c.Replies, 40, focused))
			if !strings.Contains(view, "latest reply") {
				t.Fatalf("sidebar=%v focused=%v: latest reply missing: %q", sidebar, focused, view)
			}
		}
		scroll := app.threadScrolls[key]
		scroll.manual, scroll.offset = true, 0
		app.threadScrolls[key] = scroll
		view := ansi.Strip(app.renderThread(key, c.Author, c.Body, c.Replies, 40, true))
		if !strings.Contains(view, "original source") {
			t.Fatalf("original quote is not accessible: %q", view)
		}
	}
}

func TestSidebarNavigationShowsExpandedThread(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.width, app.height = 120, 24
	app.recalculateLayout()
	for i := range 8 {
		app.tabs[0].state.Comments = append(app.tabs[0].state.Comments, review.Comment{
			ID: string(rune('a' + i)), Scope: "file", Body: strings.Repeat("history\n", 20),
			Replies: []review.Reply{{Author: "Bob", Body: "short reply"}},
		})
	}
	app.focused = commentPane
	app.updateCommentSidebar()
	app = pressKey(app, 'G')
	if app.commentViewport.YOffset() == 0 {
		t.Fatal("selecting the final thread did not scroll it into view")
	}
	if got := ansi.Strip(app.commentViewport.View()); !strings.Contains(got, "> File") || !strings.Contains(got, "short reply") {
		t.Fatalf("selected thread is not fully visible: %q", got)
	}
	app = pressKey(app, 'g')
	if app.commentViewport.YOffset() != 0 {
		t.Fatal("returning to the first thread did not restore the top of the sidebar")
	}
}

func TestThreadScrollingUsesSelectedSurface(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		t.Run(map[bool]string{false: "inline", true: "sidebar"}[sidebar], func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.width, app.height = 120, 40
			app.recalculateLayout()
			app.tabs[0].state.Comments = []review.Comment{{ID: "thread", StartLine: 1, Body: strings.Repeat("history\n", 20),
				Replies: []review.Reply{{ID: "reply", Body: "latest"}}}}
			app.tabs[0].cursorOnAnnotation = true
			app.tabs[0].cursorLine = 1
			if sidebar {
				app.focused = commentPane
			}
			app.rebuildContent()
			app.updateCommentSidebar()
			key := threadViewKey{path: app.tab().path, id: "thread", sidebar: sidebar}
			otherKey := threadViewKey{path: app.tab().path, id: "thread", sidebar: !sidebar}
			before, other := app.threadScrolls[key], app.threadScrolls[otherKey]
			updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModCtrl})
			app = *updated.(*AppModel)
			if got := app.threadScrolls[key].offset; got != max(0, before.offset-before.height) {
				t.Fatalf("page-up offset = %d, before = %+v", got, before)
			}
			if app.threadScrolls[otherKey].offset != other.offset {
				t.Fatal("scrolling changed the other surface")
			}
			left, top, _, _ := app.contentBounds()
			x, y := left+gutterWidth+2, top+2
			if sidebar {
				left, top, _, _ = app.commentBounds()
				x, y = left+2, top+2
			}
			before = app.threadScrolls[key]
			app = wheelMouse(app, x, y, tea.MouseWheelUp)
			if got := app.threadScrolls[key].offset; got != max(0, before.offset-3) {
				t.Fatalf("wheel offset = %d, before = %+v", got, before)
			}
		})
	}
}

func TestResolvedThreadsExpandWhileFocused(t *testing.T) {
	for _, surface := range []string{"inline", "sidebar"} {
		for _, action := range []string{"keyboard", "click", "wheel"} {
			t.Run(surface+"/"+action, func(t *testing.T) {
				app := setupAppWithDoc(t, "source\n")
				app.width, app.height = 120, 40
				app.recalculateLayout()
				comment := review.Comment{ID: "resolved", StartLine: 1, EndLine: 1, Resolved: true,
					Body: strings.Repeat("history\n", 20), Replies: []review.Reply{{ID: "reply", Author: "Codex", Body: "latest reply"}}}
				if surface == "sidebar" {
					comment.Scope = "file"
				}
				app.tabs[0].state.Comments = []review.Comment{comment}
				app.tabs[0].cursorLine = 1
				app.rebuildContent()
				app.updateCommentSidebar()
				view := func() string {
					if surface == "sidebar" {
						return ansi.Strip(app.commentViewport.View())
					}
					return ansi.Strip(app.contentViewport.View())
				}
				if got := view(); strings.Contains(got, "latest reply") || !strings.Contains(got, "Resolved") {
					t.Fatalf("unfocused resolved thread = %q", got)
				}
				left, top, _, _ := app.contentBounds()
				x, y, key := left+gutterWidth+2, top+2, 'j'
				if surface == "sidebar" {
					left, top, _, _ = app.commentBounds()
					x, y, key = left+2, top, 's'
				}
				switch action {
				case "keyboard":
					app = pressKey(app, key)
				case "click":
					app = clickMouse(app, x, y)
				case "wheel":
					app = wheelMouse(app, x, y, tea.MouseWheelUp)
				}
				if got := view(); (!strings.Contains(got, "history") && !strings.Contains(got, "latest reply")) || !strings.Contains(got, "Resolved") {
					t.Fatalf("focused resolved thread = %q", got)
				}
				if !app.tab().state.Comments[0].Resolved {
					t.Fatal("expanding the thread changed its resolution")
				}
				app = pressKey(app, 's')
				if got := view(); strings.Contains(got, "history") || strings.Contains(got, "latest reply") {
					t.Fatalf("resolved thread did not collapse after losing focus: %q", got)
				}
			})
		}
	}
}

func TestResolveThreadAdvancesToUnresolved(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		t.Run(fmt.Sprintf("sidebar=%t", sidebar), func(t *testing.T) {
			app := newCommentNavigationTestApp()
			app.tabs[0].state.Comments[2].Resolved = true
			app.tabs[2].state.Comments[1].Resolved = true
			app.tabs[0].state.Comments = append(app.tabs[0].state.Comments,
				review.Comment{ID: "first-file", Scope: "file"})
			app.tabs[2].state.Comments = append(app.tabs[2].state.Comments,
				review.Comment{ID: "last-file-a", Scope: "file"},
				review.Comment{ID: "last-file-b", Scope: "file"})
			app.selectComment(0, app.commentTargets(0)[1])
			if sidebar {
				app.focused = commentPane
				app.tab().sidebarCursor = 1
				app.updateCommentSidebar()
			}
			for _, want := range []string{"first-b", "last-file-a", "last-file-b", "last-a", "first-file", ""} {
				app = pressKey(app, 'R')
				targets := app.commentTargets(app.activeTab)
				current := app.currentCommentTarget(targets)
				if want == "" {
					if current >= 0 || app.focused != contentPane || app.unresolvedTotal() != 0 {
						t.Fatal("final resolution did not release focus with all threads resolved")
					}
					if app.modal != finishModal {
						t.Fatalf("final resolution opened modal %v, want the finish dialog", app.modal)
					}
				} else if app.modal != noModal || current < 0 || targets[current].id != want || targets[current].resolved {
					t.Fatalf("selected target %d in %+v (modal %v), want unresolved %s", current, targets, app.modal, want)
				}
			}
		})
	}
}

func TestResolveThreadReleasesFocus(t *testing.T) {
	for _, surface := range []string{"inline", "sidebar-line", "sidebar-file"} {
		t.Run(surface, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.width, app.height = 120, 40
			app.recalculateLayout()
			comment := review.Comment{ID: "thread", StartLine: 1, EndLine: 1, Body: "thread body"}
			if surface == "sidebar-file" {
				comment.Scope = "file"
			}
			app.tab().state.Comments = []review.Comment{comment}
			app.tab().cursorLine = 1
			app.tab().cursorOnAnnotation = true
			app.focused = contentPane
			if surface != "inline" {
				app.focused = commentPane
			}
			app.rebuildContent()
			app.updateCommentSidebar()

			app = pressKey(app, 'R')

			if !app.tab().state.Comments[0].Resolved {
				t.Fatal("R did not resolve the thread")
			}
			if app.focused != contentPane || app.tab().cursorOnAnnotation {
				t.Fatal("resolved thread retained focus")
			}
			if app.modal != finishModal {
				t.Fatalf("resolving the last thread opened modal %v, want the finish dialog", app.modal)
			}
			app = pressKey(app, tea.KeyEscape)
			view := app.contentViewport.View()
			if surface == "sidebar-file" {
				view = app.commentViewport.View()
			}
			if !strings.Contains(ansi.Strip(view), comment.Body) {
				t.Fatalf("thread resolved this round was folded in normal mode: %q", view)
			}
		})
	}
}

func TestToggleCommentVisibility(t *testing.T) {
	for _, focus := range []pane{contentPane, commentPane} {
		app := setupAppWithDoc(t, "source\n")
		app.width, app.height = 120, 50
		app.recalculateLayout()
		app.tab().state.Comments = []review.Comment{
			{ID: "line", StartLine: 1, EndLine: 1, Body: "line body"},
			{ID: "file", Scope: "file", Body: "file body", Resolved: true},
		}
		app.focused = focus
		app.tab().cursorOnAnnotation = true
		app.updateCommentSidebar()
		for _, folded := range []bool{true, false} {
			app = pressKey(app, 'H')
			if app.hideComments != folded {
				t.Fatal("folding changed the wrong preference")
			}
			screen, _ := app.renderReviewScreen()
			for _, view := range []string{app.contentViewport.View(), screen} {
				if strings.Contains(ansi.Strip(view), "body") == folded {
					t.Fatalf("folded=%t: unexpected body visibility in %q", folded, ansi.Strip(view))
				}
			}
		}
	}
	app := newCommentNavigationTestApp()
	app = pressKey(app, 'H')
	app = pressKey(app, '3')
	if app.activeTab != 2 || !app.hideComments {
		t.Fatal("folding mode did not survive tab switch")
	}
}

func TestCommentVisibilityThreeStates(t *testing.T) {
	for _, initial := range []rune{0, 'h', 'H'} {
		for _, pressed := range []rune{'h', 'H'} {
			t.Run(fmt.Sprintf("%d/%c", initial, pressed), func(t *testing.T) {
				comments := []review.Comment{
					{ID: "open", Scope: "file", Body: "open body"},
					{ID: "current", Scope: "file", Body: "current body", Resolved: true, ResolvedRound: 3},
					{ID: "past", Scope: "file", Body: "past body", Resolved: true, ResolvedRound: 2},
				}
				app, _ := newFinishTestApp(t, comments)
				app.session.CJ.ReviewRound = 3
				app.width, app.height = 160, 60
				app.recalculateLayout()
				if initial != 0 {
					app = pressKey(app, initial)
				}
				app = pressKey(app, pressed)
				want := pressed
				if initial == pressed {
					want = 0
				}
				if app.hideComments != (want == 'H') || app.foldResolved != (want == 'h') {
					t.Fatalf("state: hidden=%v folded=%v, want %d", app.hideComments, app.foldResolved, want)
				}
				for _, view := range []string{app.contentViewport.View(), app.commentViewport.View()} {
					plain := ansi.Strip(view)
					if strings.Contains(plain, "open body") != (want != 'H') ||
						strings.Contains(plain, "current body") != (want == 0) || strings.Contains(plain, "past body") {
						t.Fatalf("state %d: incorrect bodies: %q", want, plain)
					}
				}
			})
		}
	}
}

func TestHiddenCommentsUseClickableGutter(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	width := app.contentViewport.Width()
	app.tab().state.Comments = []review.Comment{{ID: "c", StartLine: 1, EndLine: 2, Body: "body"}}
	app.tab().cursorLine = 1
	app = pressKey(app, 'H')
	if app.contentViewport.Width() <= width {
		t.Fatal("hiding comments did not widen the source")
	}
	if len(app.contentLayout.rows) != app.tab().doc.LineCount() {
		t.Fatal("hidden comments still occupy rows")
	}
	if app.gutterComment(0) != "c" || app.gutterComment(1) != "c" || app.gutterComment(2) != "" {
		t.Fatal("gutter markers do not match the commented range")
	}
	for _, row := range strings.Split(app.renderCommentGutter(), "\n") {
		if got := lipgloss.Width(row); got != app.commentPanelWidth() {
			t.Fatalf("gutter row width = %d, want %d: %q", got, app.commentPanelWidth(), row)
		}
	}
	app = pressKey(app, 'j')
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 3 || app.tab().cursorOnAnnotation {
		t.Fatal("movement stopped on a hidden comment")
	}
	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+1, top)
	if app.editingID != "c" || app.modal == noModal || !app.hideComments {
		t.Fatal("gutter click did not open the thread while keeping reading mode")
	}
	app = pressKey(app, tea.KeyEscape)
	app = pressKey(app, 'H')
	if app.contentViewport.Width() != width {
		t.Fatal("restoring comments did not restore source width")
	}
}

func TestHiddenCommentGutterTracksDeletedWrappedLines(t *testing.T) {
	app := setupAppWithDoc(t, "new line\n")
	app.width, app.height = 60, 30
	app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {{OldLineNum: 1, Content: strings.Repeat("deleted ", 30)}},
	}
	app.tab().state.Comments = []review.Comment{{ID: "old", StartLine: 1, EndLine: 1, Side: "old", Body: "hidden"}}
	app = pressKey(app, 'H')
	oldRows := 0
	for row, target := range app.contentLayout.rows {
		if target.side == "old" {
			oldRows++
		} else if app.gutterComment(row) != "" {
			t.Fatal("old-side marker leaked onto new-side line")
		}
	}
	if oldRows < 2 {
		t.Fatal("fixture did not wrap the deleted line")
	}
	for row := range oldRows {
		want := ""
		if row == oldRows-1 {
			want = "old"
		}
		if got := app.gutterComment(row); got != want {
			t.Fatalf("row %d marker = %q, want %q", row, got, want)
		}
	}
	app.contentViewport.SetHeight(1)
	app.contentViewport.SetYOffset(oldRows - 1)
	if !strings.Contains(ansi.Strip(app.renderCommentGutter()), "💬") {
		t.Fatal("gutter did not follow scrolling")
	}
	app.contentViewport.SetYOffset(0)
	if strings.Contains(ansi.Strip(app.renderCommentGutter()), "💬") {
		t.Fatal("marker appeared before the last wrapped row became visible")
	}
}

func TestFoldShortcutsPreserveInput(t *testing.T) {
	for _, key := range []rune{'h', 'H'} {
		for _, modal := range []modalType{commentModal, fileCommentModal, replyModal, editModal, finishModal, discardChangesModal, deleteConfirmModal, helpModal} {
			app := setupAppWithDoc(t, "source\n")
			app.modal = modal
			app.modalTextarea.Focus()
			app, _ = updateApp(app, tea.KeyPressMsg{Code: key, Text: string(key)})
			if app.hideComments || app.foldResolved {
				t.Fatalf("modal %v: %c changed comment visibility", modal, key)
			}
			switch modal {
			case commentModal, fileCommentModal, replyModal, editModal, finishModal:
				if app.modalTextarea.Value() != string(key) {
					t.Fatalf("modal %v swallowed %c", modal, key)
				}
			case discardChangesModal, deleteConfirmModal:
				if key == 'h' && app.modalFocus != 1 {
					t.Fatalf("modal %v did not move button focus", modal)
				}
			}
		}
		app := newCommentNavigationTestApp()
		app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
		app, _ = updateApp(app, tea.KeyPressMsg{Code: key, Text: string(key)})
		if app.fileSelect.input.Value() != string(key) || app.hideComments || app.foldResolved {
			t.Fatalf("%c was intercepted in the file selector", key)
		}
	}
}

func TestResolvedThreadFocusWithVerticalMovement(t *testing.T) {
	for _, direction := range []rune{'j', 'k'} {
		for _, selecting := range []bool{false, true} {
			app := setupAppWithDoc(t, "one\ntwo\nthree\n")
			app.tabs[0].state.Comments = []review.Comment{{ID: "resolved", StartLine: 2, EndLine: 2, Body: "resolved body", Resolved: true}}
			app.tab().cursorLine = 2
			if direction == 'k' {
				app.tab().cursorLine = 3
			}
			if selecting {
				app = pressKey(app, 'v')
			}
			app = pressKey(app, direction)
			if app.tab().cursorOnAnnotation == selecting {
				t.Fatalf("direction %c, selecting %t: annotation focus = %t", direction, selecting, app.tab().cursorOnAnnotation)
			}
			if !selecting {
				app = pressKey(app, 'v')
				if app.tab().cursorOnAnnotation {
					t.Fatal("entering visual mode retained annotation focus")
				}
			}
		}
	}
}

func TestReplyModalStartsAtLatestHeaderAndKeepsFullLines(t *testing.T) {
	app := setupAppWithDoc(t, "context\n")
	app.width, app.height = 120, 50
	app.recalculateLayout()
	app.tabs[0].state.Comments = []review.Comment{{ID: "thread", Author: "Reviewer", StartLine: 1,
		Body: strings.Repeat("past\n", 30), Replies: []review.Reply{{ID: "reply", Author: "Codex", Body: "latest first\n" + strings.Repeat("latest\n", 30) + "latest last"}}}}
	app.openCommentThread("thread")
	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered, regions := app.renderWithModalLayout(background)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Codex (#1)") || !strings.Contains(plain, "latest first") || strings.Contains(plain, "latest last") {
		t.Fatalf("modal initial position = %q", plain)
	}
	for _, region := range regions {
		if region.action.scrollable {
			if height := region.rect.bottom - region.rect.top; height > 18 {
				t.Fatalf("reference height = %d, want at most 18 including borders", height)
			}
			app.modalReferenceOffset = region.action.scrollMaxOffset
		}
	}
	if got := ansi.Strip(app.renderWithModal(background)); !strings.Contains(got, "latest last") {
		t.Fatal("latest reply's final line is unreachable")
	}
	app.modalReferenceOffset = 0
	if got := ansi.Strip(app.renderWithModal(background)); !strings.Contains(got, "context") {
		t.Fatal("original code context is unreachable")
	}
	app.tabs[0].state.Comments[0].Replies[0].Body = "short final reply"
	app.modalReferenceOffset = -1
	rendered, regions = app.renderWithModalLayout(background)
	rows := strings.Split(ansi.Strip(rendered), "\n")
	for _, region := range regions {
		if region.action.scrollable && !strings.Contains(rows[region.rect.bottom-2], "short final reply") {
			t.Fatalf("blank padding below the latest modal reply: %q", rows[region.rect.top:region.rect.bottom])
		}
	}
}

func TestResolvedThreadsOpenWithEnter(t *testing.T) {
	for _, surface := range []string{"inline", "sidebar", "drifted", "drifted-sidebar"} {
		t.Run(surface, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.width, app.height = 120, 40
			app.recalculateLayout()
			drifted := strings.HasPrefix(surface, "drifted")
			app.tab().state.Comments = []review.Comment{{ID: "resolved", StartLine: 1, EndLine: 1, Resolved: true, Drifted: drifted, Body: "resolved body", Author: "Agent"}}
			app.selectComment(0, app.commentTargets(0)[0])
			if strings.Contains(surface, "sidebar") {
				app.focused = commentPane
				app.updateCommentSidebar()
			}
			app = pressKey(app, 'h')
			view := app.contentViewport.View()
			if strings.Contains(surface, "sidebar") {
				view = app.commentViewport.View()
			}
			if !strings.Contains(ansi.Strip(view), "resolved body") {
				t.Fatal("focused resolved body hidden by h")
			}
			app = pressKey(app, tea.KeyEnter)
			if app.modal == noModal || app.editingID != "resolved" {
				t.Fatal("Enter did not open the resolved thread")
			}
			if !strings.Contains(ansi.Strip(app.View().Content), "resolved body") {
				t.Fatal("resolved body is not accessible in the editor")
			}
			if app.tab().expandedDrifted["resolved"] || !app.tab().state.Comments[0].Resolved {
				t.Fatal("opening changed expansion or resolution")
			}
		})
	}
}
