package tui

import (
	"testing"

	"github.com/crittylabs/critty/internal/review"
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
	app := newCommentNavigationTestApp()
	app.rebuildContent()
	app.tab().cursorLine, app.tab().cursorOnAnnotation, app.tab().cursorAnnoIdx = 2, true, 0
	app = pressKey(app, 'R')
	if c := commentByID(t, app, 0, "first-a"); !c.Resolved || app.tab().cursorAnnoIdx != 1 || !app.tab().cursorOnAnnotation {
		t.Fatalf("R: resolved %v, cursor %d:%d on annotation %v", c.Resolved, app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation)
	}
	app = ctrlKey(app, 'z')
	if c := commentByID(t, app, 0, "first-a"); c.Resolved || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 0 || !app.tab().cursorOnAnnotation || app.focused != contentPane {
		t.Fatalf("undo: resolved %v, cursor %d:%d on annotation %v focused %v", c.Resolved, app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation, app.focused)
	}
	app = ctrlKey(app, 'y')
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
	app := newCommentNavigationTestApp()
	app.rebuildContent()
	app.tabs[0].state.Comments[2].Replies = []review.Reply{{ID: "reply-1", Body: "one"}, {ID: "reply-2", Body: "two"}}
	app.deleteFromThread(0, "first-b", "")
	if ids := commentIDs(app, 0); len(ids) != 2 || ids[0] != "first-a" || ids[1] != "first-c" {
		t.Fatalf("delete left %v", ids)
	}
	app = ctrlKey(app, 'z')
	if ids := commentIDs(app, 0); len(ids) != 3 || ids[1] != "first-b" {
		t.Fatalf("undo restored %v, want first-b back in place", ids)
	}
	if !app.tab().cursorOnAnnotation || app.tab().cursorLine != 2 || app.tab().cursorAnnoIdx != 1 {
		t.Fatalf("undo did not focus the restored comment: %d:%d on annotation %v", app.tab().cursorLine, app.tab().cursorAnnoIdx, app.tab().cursorOnAnnotation)
	}
	app = ctrlKey(app, 'y')
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
}

func TestResolveNextUndoLeavesFinishDialog(t *testing.T) {
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
	app = ctrlKey(app, 'z')
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
