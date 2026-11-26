package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/v2/internal/review"
)

func commentByID(t *testing.T, app AppModel, tab int, id string) *review.Comment {
	t.Helper()
	for i := range app.tabs[tab].state.Comments {
		if app.tabs[tab].state.Comments[i].ID == id {
			return &app.tabs[tab].state.Comments[i]
		}
	}
	return nil
}

func TestResolveNextUndoReturnsToThreadAndRedoAdvances(t *testing.T) {
	t.Parallel()
	app := newCommentNavigationTestApp()
	app.rebuildContent()
	app.tab().cursorLine, app.tab().cursorOnAnnotation, app.tab().cursorAnnoIdx = 2, true, 0
	app = pressKey(app, 'R')
	if c := commentByID(t, app, 0, "first-a"); !c.Resolved || app.tab().cursorAnnoIdx != 1 || !app.tab().cursorOnAnnotation {
		t.Fatalf("R: resolved %v, cursor %d:%d on annotation %v", c.Resolved, app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation)
	}
	app = pressAltKey(app, 'z')
	if c := commentByID(t, app, 0, "first-a"); c.Resolved || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 0 || !app.tab().cursorOnAnnotation || app.focused != contentPane {
		t.Fatalf("undo: resolved %v, cursor %d:%d on annotation %v focused %v", c.Resolved, app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation, app.focused)
	}
	app = pressAltKey(app, '_')
	if c := commentByID(t, app, 0, "first-a"); !c.Resolved || app.tab().cursorAnnoIdx != 1 {
		t.Fatalf("redo: resolved %v, cursor %d:%d", c.Resolved, app.tab().cursorLine, app.tab().cursorAnnoIdx)
	}
	app = ctrlKey(app, 'z')
	app = pressKey(app, 'r')
	if c := commentByID(t, app, 0, "first-a"); !c.Resolved || len(app.reviewRedo) != 0 {
		t.Fatalf("a new resolve kept redo history: resolved %v redo %d", c.Resolved, len(app.reviewRedo))
	}
	app = ctrlKey(app, 'z')
	if c := commentByID(t, app, 0, "first-a"); c.Resolved {
		t.Fatal("undo of a plain resolve left the thread resolved")
	}
}

func TestDeleteUndoRestoresCommentAndReplyInPlace(t *testing.T) {
	t.Parallel()
	app := newCommentNavigationTestApp()
	app.rebuildContent()
	app.tabs[0].state.Comments[2].Replies = []review.Reply{{ID: "reply-1", Body: "one"}, {ID: "reply-2", Body: "two"}}
	app.tabs[0].state.Comments[1].StartLine = 1
	app.tabs[0].state.Comments[1].Author = app.author
	app.openCommentDelete("first-b")
	app = pressKey(app, 'y')
	if app.tab().cursorLine != 2 || app.tab().cursorOnAnnotation || app.focused != contentPane {
		t.Fatalf("delete landed at line %d, annotation %v, pane %v", app.tab().cursorLine, app.tab().cursorOnAnnotation, app.focused)
	}
	if ids := commentIDs(app, 0); len(ids) != 2 || ids[0] != "first-a" || ids[1] != "first-c" {
		t.Fatalf("delete left %v", ids)
	}
	app = pressAltKey(app, 'z')
	if ids := commentIDs(app, 0); len(ids) != 3 || ids[1] != "first-b" {
		t.Fatalf("undo restored %v, want first-b back in place", ids)
	}
	if !app.tab().cursorOnAnnotation || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 1 {
		t.Fatalf("undo did not focus the restored comment: %d:%d on annotation %v", app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation)
	}
	restored := commentByID(t, app, 0, "first-b")
	restored.Replies = []review.Reply{{ID: "new-reply", Body: "New reply"}}
	app = pressAltKey(app, 'Z')
	if c := commentByID(t, app, 0, "first-b"); c == nil || len(c.Replies) != 1 {
		t.Fatal("redo removed a thread with a new reply")
	}
	commentByID(t, app, 0, "first-b").Replies = nil
	app = pressAltKey(app, 'Z')
	if ids := commentIDs(app, 0); len(ids) != 2 || app.tab().cursorOnAnnotation {
		t.Fatalf("redo left %v on annotation %v", ids, app.tab().cursorOnAnnotation)
	}

	app.deleteFromThread(0, "first-c", "reply-1")
	if replies := commentByID(t, app, 0, "first-c").Replies; len(replies) != 1 || replies[0].ID != "reply-2" {
		t.Fatalf("reply delete left %+v", replies)
	}
	app = ctrlKey(app, 'z')
	if replies := commentByID(t, app, 0, "first-c").Replies; len(replies) != 2 || replies[0].ID != "reply-1" {
		t.Fatalf("undo restored replies %+v", replies)
	}
	if !app.tab().cursorOnAnnotation || app.tab().cursorLine != 4 {
		t.Fatalf("undo did not focus the parent thread: %d on annotation %v", app.tab().cursorLine, app.tab().cursorOnAnnotation)
	}
	app = pressAltKey(app, '_')
	if replies := commentByID(t, app, 0, "first-c").Replies; len(replies) != 1 || replies[0].ID != "reply-2" {
		t.Fatalf("redo deleted the wrong reply: %+v", replies)
	}
}

func TestUndoDeleteOnRankingRow(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "- [ ] First <!-- form: id=order rank value=a -->\n- [ ] Second <!-- form: id=order rank value=b -->\n")
	app.tab().cursorLine = 1
	app = pressAltKey(app, tea.KeyDown)
	d := app.forms.Document(app.tab().path)
	app.tab().state.Comments = []review.Comment{{ID: "note", StartLine: 1, EndLine: 2, Body: "Note", Author: app.author, ReviewRound: app.reviewRound()}}
	app.persist()
	app.focusComment(app.activeTab, "note")
	app = pressKey(app, 'd')
	if app.modal != deleteConfirmModal {
		t.Fatalf("delete did not open confirmation: selected %q, modal %v", app.selectedCommentID(), app.modal)
	}
	app = pressKey(app, 'y')
	if app.tab().cursorLine != 2 || len(app.tab().state.Comments) != 0 {
		t.Fatalf("delete: line %d, comments %v", app.tab().cursorLine, app.tab().state.Comments)
	}
	_, screen := app.renderReviewScreen()
	if !screen.hasUndoButton {
		t.Fatal("missing Undo delete button")
	}
	app = clickMouse(app, screen.undoButton.left, screen.undoButton.top)
	if app.selectedCommentID() != "note" {
		t.Fatal("undo did not restore and focus the deleted comment")
	}
	assertRankAnswer(t, d, "order", []string{"b", "a"}, nil)
	_, screen = app.renderReviewScreen()
	if !screen.hasRedoButton {
		t.Fatal("missing Redo delete button")
	}
	app = clickMouse(app, screen.redoButton.left, screen.redoButton.top)
	saved, err := review.OpenSessionAt(app.session.Key, app.session.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.FileComments(app.tab().path)) != 0 || app.tab().cursorLine != 2 {
		t.Fatal("redo did not save the deletion and return to the source row")
	}
	for _, undo := range []rune{'/', '_', 'z'} {
		app = ctrlKey(app, undo)
		if app.selectedCommentID() != "note" {
			t.Fatalf("C-%c did not restore the comment", undo)
		}
		app = pressAltKey(app, '_')
		if len(app.tab().state.Comments) != 0 {
			t.Fatal("M-_ did not redo the deletion")
		}
	}
	app = pressAltKey(app, tea.KeyDown)
	app = pressAltKey(app, 'z')
	assertRankAnswer(t, d, "order", []string{"b", "a"}, nil)
	if len(app.tab().state.Comments) != 0 {
		t.Fatal("undo after moving the item restored a comment")
	}
}

func TestResolveNextUndoLeavesFinishDialog(t *testing.T) {
	t.Parallel()
	app := newCommentNavigationTestApp()
	app.rebuildContent()
	for tab := range app.tabs {
		for i := range app.tabs[tab].state.Comments {
			app.tabs[tab].state.Comments[i].Resolved = true
		}
	}
	app.tabs[0].state.Comments[0].Resolved = false
	app.tab().cursorLine, app.tab().cursorOnAnnotation, app.tab().cursorAnnoIdx = 2, true, 0
	app = pressKey(app, 'R')
	if app.modal != finishModal || !app.finishFromResolve {
		t.Fatalf("R with no thread left opened modal %v (from resolve %v)", app.modal, app.finishFromResolve)
	}
	app = pressKey(app, tea.KeyUp)
	app = typeSearch(app, "x")
	app = ctrlKey(app, '/')
	if app.modal != finishModal || app.modalTextarea.Value() != "" {
		t.Fatalf("undo in the overall comment: modal %v text %q", app.modal, app.modalTextarea.Value())
	}
	editorMeta(&app, '_')
	if got := app.modalTextarea.Value(); got != "x" {
		t.Fatalf("redo in the overall comment = %q", got)
	}
	app = ctrlKey(app, 's')
	app = pressAltKey(app, 'z')
	if app.modal != noModal || app.finishFromResolve || commentByID(t, app, 0, "first-a").Resolved || !app.tab().cursorOnAnnotation {
		t.Fatalf("undo from the finish dialog: modal %v resolved %v on annotation %v", app.modal, commentByID(t, app, 0, "first-a").Resolved, app.tab().cursorOnAnnotation)
	}
}

func commentIDs(app AppModel, tab int) []string {
	var ids []string
	for _, c := range app.tabs[tab].state.Comments {
		ids = append(ids, c.ID)
	}
	return ids
}
