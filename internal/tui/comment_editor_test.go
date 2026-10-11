package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/review"
)

func TestSaveTextModalWithoutFeedback(t *testing.T) {
	for _, mode := range []modalType{commentModal, fileCommentModal, replyModal, editModal} {
		for _, body := range []string{"", " \n\t", "original"} {
			if mode == editModal && body != "original" {
				continue
			}
			if mode != editModal && body == "original" {
				continue
			}
			t.Run(fmt.Sprintf("%v/%q", mode, body), func(t *testing.T) {
				app := setupAppWithDoc(t, "first\n")
				app.modal = mode
				if mode == editModal || mode == replyModal {
					app.tab().state.Comments = []review.Comment{{ID: "c1", Body: "original", UpdatedAt: "unchanged"}}
					app.editingID = "c1"
				}
				if mode == editModal {
					app.modalInitial = "original"
				}
				app.modalTextarea.SetValue(body)
				updated, _ := app.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				app = *updated.(*AppModel)
				want := noModal
				if app.modal != want || app.newFeedback {
					t.Fatalf("modal = %v, newFeedback = %v; want %v, false", app.modal, app.newFeedback, want)
				}
				if mode == editModal || mode == replyModal {
					c := app.tab().state.Comments[0]
					if c.Body != "original" || c.UpdatedAt != "unchanged" || len(c.Replies) != 0 {
						t.Fatalf("comment changed: %+v", c)
					}
				} else if len(app.tab().state.Comments) != 0 {
					t.Fatal("empty comment created")
				}
			})
		}
	}
}

func TestReplyResolution(t *testing.T) {
	for _, edit := range []bool{false, true} {
		for _, resolved := range []bool{false, true} {
			t.Run(fmt.Sprintf("edit=%v/resolved=%v", edit, resolved), func(t *testing.T) {
				c := testComment()
				c.Author = "Other"
				c.Resolved = resolved
				if resolved {
					c.ResolvedRound = 1
				}
				if edit {
					c.Replies = []review.Reply{{ID: "own", Body: "reply", Author: "Tester", ReviewRound: 1}}
				}
				app, _ := newFinishTestApp(t, []review.Comment{c})
				app.width, app.height = 100, 35
				app.recalculateLayout()
				app.openCommentThread(c.ID)
				if app.tab().state.Comments[0].Resolved != resolved || !app.hasModalResolution() {
					t.Fatal("editor did not load the thread resolution")
				}
				updated, _ := app.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
				app = *updated.(*AppModel)
				if app.session.FileComments(app.tab().path)[0].Resolved == resolved {
					t.Fatal("Meta-r must persist resolution immediately")
				}
				app.closeTextModal()
				if app.modal != noModal {
					t.Fatal("resolution alone must not prompt before closing")
				}
				app.openCommentThread(c.ID)
				clicked := false
				for _, region := range app.modalMouseRegions() {
					if region.action.resolve {
						assertRegionContainsRenderedText(t, app, region.rect, "Resolved")
						for _, close := range app.modalMouseRegions() {
							if close.action.close && (close.rect.top != region.rect.top || close.rect.left <= region.rect.right) {
								t.Fatal("checkbox must be immediately left of the title's close button")
							}
						}
						app = clickMouse(app, region.rect.left, region.rect.top)
						clicked = true
						break
					}
				}
				if !clicked || app.session.FileComments(app.tab().path)[0].Resolved != resolved {
					t.Fatal("checkbox click did not toggle resolution")
				}
				app.handleTextModal(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
				app.modalSubmit()
				got := app.session.FileComments(app.tab().path)[0]
				if got.Resolved == resolved || app.modal != noModal {
					t.Fatalf("resolution-only save failed: %+v", got)
				}
				wantRound := 1
				if resolved {
					wantRound = 0
				}
				if got.ResolvedRound != wantRound || len(got.Replies) != len(c.Replies) {
					t.Fatalf("save changed replies or lost resolution round: %+v", got)
				}
				app.openCommentThread(c.ID)
				app.modalTextarea.SetValue("unsaved reply")
				app.handleTextModal(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
				app.closeTextModal()
				if app.modal != discardChangesModal {
					t.Fatal("unsaved reply text should still prompt before closing")
				}
				app.handleDiscardChangesModal(tea.KeyPressMsg{Code: 'y'})
				if app.session.FileComments(app.tab().path)[0].Resolved != resolved {
					t.Fatal("discarding reply text must not undo resolution")
				}
			})
		}
	}
}

func TestSaveClearedCommentDeletesEditedEntry(t *testing.T) {
	for _, reply := range []bool{false, true} {
		for _, body := range []string{"", " \n\t"} {
			t.Run(fmt.Sprintf("reply=%v/body=%q", reply, body), func(t *testing.T) {
				app := setupAppWithDoc(t, "first\n")
				app.openLineComment()
				app.modalTextarea.SetValue("original")
				app.modalSubmit()
				id := app.tab().state.Comments[0].ID
				if reply {
					app.tab().state.Comments[0].Replies = []review.Reply{
						{ID: "r1", Body: "keep", Author: "Other"},
						{ID: "r2", Body: "remove", Author: app.author, ReviewRound: app.reviewRound()},
					}
					app.persist()
				}
				app.openCommentThread(id)
				if app.modal != editModal {
					t.Fatalf("modal = %v, want edit", app.modal)
				}
				app.modalTextarea.SetValue(body)
				updated, _ := app.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				app = *updated.(*AppModel)
				if app.modal != noModal {
					t.Fatalf("modal = %v, want closed without confirmation", app.modal)
				}
				for _, comments := range [][]review.Comment{app.tab().state.Comments, app.session.FileComments(app.tab().path)} {
					if reply {
						if len(comments) != 1 || comments[0].Body != "original" || len(comments[0].Replies) != 1 || comments[0].Replies[0].ID != "r1" {
							t.Fatalf("unexpected remaining thread: %+v", comments)
						}
					} else if len(comments) != 0 {
						t.Fatalf("comment not deleted: %+v", comments)
					}
				}
			})
		}
	}
}

func TestMouseClickCommentModalSave(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.openLineComment()
	app.modalTextarea.SetValue("mouse comment")
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == 1 {
			assertRegionContainsRenderedText(t, app, region.rect, "Save ctrl+s")
			break
		}
	}

	app = clickModalAction(t, app, "Save ctrl+s")

	if app.modal != noModal || len(app.tab().state.Comments) != 1 || app.tab().state.Comments[0].Body != "mouse comment" {
		t.Fatalf("modal = %v, comments = %+v; want saved mouse comment", app.modal, app.tab().state.Comments)
	}
}

func TestNewCommentIsSelectedForImmediateDeletion(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  rune
	}{
		{"line", tea.KeyEnter},
		{"file", 'f'},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.tab().cursorLine = 2
			app = pressKey(app, tt.key)
			app.modalTextarea.SetValue("remove me")
			app.modalSubmit()

			if len(app.tab().state.Comments) != 1 {
				t.Fatal("expected one saved comment")
			}
			comment := app.tab().state.Comments[0]
			if app.focused != contentPane || app.selectedCommentID() != comment.ID {
				t.Fatalf("selected comment = %q in pane %v, want %q in content pane",
					app.selectedCommentID(), app.focused, comment.ID)
			}
			app = pressKey(app, 'd')
			if app.modal != deleteConfirmModal {
				t.Fatalf("d opened modal %v, want delete confirmation", app.modal)
			}
		})
	}
}

func TestRenderModalButtonSeparatesShortcutKeys(t *testing.T) {
	app := AppModel{}
	tests := []struct {
		name    string
		label   string
		hint    string
		focused bool
		want    string
	}{
		{name: "focused", label: "Approve", hint: "y", focused: true, want: " Approve y"},
		{name: "multiple shortcuts", label: "Keep Editing", hint: "n / esc", want: " Keep Editing n / esc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansi.Strip(app.renderModalButton(tt.label, tt.hint, tt.focused))
			if got != tt.want {
				t.Errorf("renderModalButton() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMouseClickCommentTextareaFocusesAndMovesCursor(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	app.openLineComment()
	app.modalTextarea.SetValue("first line\nsecond line\nthird line")
	app.modalTextarea.Blur()
	app.modalFocus = 1
	rect := modalTextareaRect(t, app)
	assertRegionContainsRenderedText(t, app, rect, "first line")

	x := rect.left + lipgloss.Width(app.modalTextarea.Prompt) + 2
	app = clickMouse(app, x, rect.top+1)

	if !app.modalTextarea.Focused() || app.modalFocus != 0 ||
		app.modalTextarea.Line() != 1 || app.modalTextarea.Column() != 2 {
		t.Fatalf("focused = %t, modal focus = %d, cursor = %d:%d; want textarea at 1:2",
			app.modalTextarea.Focused(), app.modalFocus,
			app.modalTextarea.Line(), app.modalTextarea.Column())
	}
}

func TestCommentTerminalCursor(t *testing.T) {
	for _, height := range []int{24, 12} {
		for _, body := range []string{"", "日本語abc", strings.Repeat("日本語", 40), strings.Repeat("line\n", 12) + "日本語abc"} {
			t.Run(fmt.Sprintf("height=%d/body=%q", height, body), func(t *testing.T) {
				app := setupAppWithDoc(t, "first\nsecond\n")
				app.width, app.height = 100, height
				app.recalculateLayout()
				app.openLineComment()
				app.modalTextarea.SetValue(body)
				_ = app.modalTextarea.View()
				app.modalTextarea.SetHeight(app.modalTextarea.Height())
				rect := modalTextareaRect(t, app)
				view := app.View()
				if view.Cursor == nil {
					t.Fatal("focused textarea has no terminal cursor")
				}
				c := view.Cursor
				if c.X < rect.left || c.X >= rect.right || c.Y < max(0, rect.top) || c.Y >= min(height, rect.bottom) {
					t.Fatalf("cursor %v outside visible textarea %v", c, rect)
				}
				app = clickMouse(app, c.X, c.Y)
				if app.modalTextarea.Column() != len([]rune(strings.Split(body, "\n")[app.modalTextarea.Line()])) {
					t.Fatalf("clicking terminal cursor moved away from end of line: %d", app.modalTextarea.Column())
				}
				app.modalTextarea.Blur()
				if app.View().Cursor != nil {
					t.Fatal("blurred textarea still has a terminal cursor")
				}
				app.modalTextarea.Focus()
				app.modal = finishModal
				if app.View().Cursor != nil {
					t.Fatal("dialog without textarea has a terminal cursor")
				}
			})
		}
	}
}

func TestMouseWheelScrollsCommentTextarea(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	app.openLineComment()
	app.modalTextarea.SetValue(strings.Repeat("line\n", 11) + "line")
	app.modalTextarea.MoveToBegin()
	rect := modalTextareaRect(t, app)

	app = wheelMouse(app, rect.left+1, rect.top+1, tea.MouseWheelDown)
	app = wheelMouse(app, rect.left+1, rect.top+1, tea.MouseWheelDown)

	if app.modalTextarea.Line() != 6 || app.modalTextarea.ScrollYOffset() == 0 {
		t.Fatalf("cursor line = %d, scroll offset = %d; want line 6 scrolled into view",
			app.modalTextarea.Line(), app.modalTextarea.ScrollYOffset())
	}
}

func TestMouseClickCommentModalSuggest(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.contentViewport.SetWidth(75)
	app.tabs[0].cursorLine = 1
	app.openLineComment()
	view := app.View().Content
	if !strings.Contains(view, "Suggest") || !strings.Contains(view, "alt+s") {
		t.Fatalf("missing suggestion button or shortcut: %q", view)
	}

	app = clickModalAction(t, app, "Suggest alt+s")

	if got := app.modalTextarea.Value(); !strings.Contains(got, "```suggestion\nfirst\n```") {
		t.Fatalf("textarea = %q, want suggestion block", got)
	}
}

func TestMouseClickCommentModalClose(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	app.openLineComment()

	app = clickModalAction(t, app, "Close esc")

	if app.modal != noModal {
		t.Fatalf("modal = %v, want closed", app.modal)
	}
}

func TestMouseClickDirtyCommentModalCloseConfirmsDiscard(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	app.openLineComment()
	app.modalTextarea.SetValue("unsaved comment")

	app = clickModalAction(t, app, "Close esc")

	if app.modal != discardChangesModal || app.discardReturn != commentModal {
		t.Fatalf("modal = %v, return = %v; want discard confirmation for comment", app.modal, app.discardReturn)
	}
	if got := app.modalTextarea.Value(); got != "unsaved comment" {
		t.Fatalf("textarea = %q, want unsaved comment preserved", got)
	}
	for _, region := range app.modalMouseRegions() {
		switch region.action.focus {
		case 0:
			assertRegionContainsRenderedText(t, app, region.rect, "Discard y")
		case 1:
			assertRegionContainsRenderedText(t, app, region.rect, "Keep Editing n / esc")
		}
	}

	app = clickModalAction(t, app, "Keep Editing n / esc")
	if app.modal != commentModal || app.modalTextarea.Value() != "unsaved comment" || !app.modalTextarea.Focused() {
		t.Fatalf("modal = %v, textarea = %q, focused = %t; want resumed editing",
			app.modal, app.modalTextarea.Value(), app.modalTextarea.Focused())
	}

	app = clickModalAction(t, app, "Close esc")
	app = clickModalAction(t, app, "Discard y")
	if app.modal != noModal || app.modalTextarea.Value() != "" {
		t.Fatalf("modal = %v, textarea = %q; want discarded and closed", app.modal, app.modalTextarea.Value())
	}
}

func TestDirtyCommentModalEscapeReturnsToEditing(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.openLineComment()
	app.modalTextarea.SetValue("unsaved comment")

	app = pressKey(app, tea.KeyEscape)
	if app.modal != discardChangesModal {
		t.Fatalf("modal = %v, want discard confirmation", app.modal)
	}

	app = pressKey(app, tea.KeyEscape)
	if app.modal != commentModal || !app.modalTextarea.Focused() {
		t.Fatalf("modal = %v, focused = %t; want resumed comment editing",
			app.modal, app.modalTextarea.Focused())
	}
}

func TestMouseClickDirtyEditModalCloseConfirmsDiscard(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	comment := review.Comment{
		ID: "c_edit", StartLine: 1, EndLine: 1, Body: "original",
		Author: app.author, ReviewRound: app.reviewRound(),
	}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)
	app.modalTextarea.SetValue("changed")

	app = clickModalAction(t, app, "Close esc")
	if app.modal != discardChangesModal || app.discardReturn != editModal {
		t.Fatalf("modal = %v, return = %v; want discard confirmation for edit", app.modal, app.discardReturn)
	}

	app = clickModalAction(t, app, "Discard y")
	if got := app.tab().state.Comments[0].Body; got != "original" {
		t.Fatalf("comment body = %q, want original", got)
	}
}

func TestMouseClickUnchangedEditModalCloseDoesNotConfirm(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	comment := review.Comment{
		ID: "c_edit", StartLine: 1, EndLine: 1, Body: "original",
		Author: app.author, ReviewRound: app.reviewRound(),
	}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)

	app = clickModalAction(t, app, "Close esc")

	if app.modal != noModal {
		t.Fatalf("modal = %v, want unchanged edit to close directly", app.modal)
	}
}

func TestMouseClickCommentModalDelete(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.contentViewport.SetWidth(75)
	comment := review.Comment{
		ID: "c_delete", StartLine: 1, EndLine: 1, Body: "delete me",
		Author: app.author, ReviewRound: app.reviewRound(),
	}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)

	app = clickModalAction(t, app, "Delete comment")
	if app.modal != deleteConfirmModal || len(app.tab().state.Comments) != 1 {
		t.Fatalf("modal = %v, comments = %+v; want delete confirmation", app.modal, app.tab().state.Comments)
	}
	for _, region := range app.modalMouseRegions() {
		switch region.action.focus {
		case 0:
			assertRegionContainsRenderedText(t, app, region.rect, "Delete y")
		case 1:
			assertRegionContainsRenderedText(t, app, region.rect, "Keep n / esc")
		}
	}

	app = clickModalAction(t, app, "Keep n / esc")
	if app.modal != editModal || len(app.tab().state.Comments) != 1 {
		t.Fatalf("modal = %v, comments = %+v; want edit resumed", app.modal, app.tab().state.Comments)
	}

	app = clickModalAction(t, app, "Delete comment")
	app = clickModalAction(t, app, "Delete y")

	if app.modal != noModal || len(app.tab().state.Comments) != 0 {
		t.Fatalf("modal = %v, comments = %+v; want deleted", app.modal, app.tab().state.Comments)
	}
}

func TestSuggestionButtonInsertsSelectedCodeAndPersistsComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.tabs[0].selecting = true
	app.tabs[0].selectAnchor = 1
	app.tabs[0].cursorLine = 2
	app.modal = commentModal
	app.modalTextarea.SetValue("Use clearer names.")

	updated, _ := app.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
	app = *updated.(*AppModel)

	wantBody := "Use clearer names.\n\n```suggestion\nfirst\nsecond\n```"
	if got := app.modalTextarea.Value(); got != wantBody {
		t.Fatalf("suggestion body = %q, want %q", got, wantBody)
	}
	if app.modalTextarea.Line() != 4 || app.modalTextarea.Column() != len("second") {
		t.Fatalf("suggestion cursor = %d:%d, want 4:%d",
			app.modalTextarea.Line(), app.modalTextarea.Column(), len("second"))
	}
	if app.modalFocus != 0 || !app.modalTextarea.Focused() {
		t.Fatalf("suggestion left focus at %d, focused=%t; want textarea", app.modalFocus, app.modalTextarea.Focused())
	}
	if got := app.modalTextarea.SelectedText(); got != "first\nsecond" {
		t.Fatalf("suggestion selection = %q", got)
	}

	app.modalSubmit()
	comments := app.session.FileComments(app.tab().path)
	if len(comments) != 1 {
		t.Fatalf("persisted comments = %+v, want one", comments)
	}
	comment := comments[0]
	if comment.StartLine != 1 || comment.EndLine != 2 || comment.Anchor != "first\nsecond" || comment.Body != wantBody {
		t.Fatalf("persisted suggestion = %+v", comment)
	}
}

func TestAddCommentModalKeepsFullContextScrollable(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "context line " + strconv.Itoa(i+1)
	}
	app := setupAppWithDoc(t, strings.Join(lines, "\n"))
	app.width = 80
	app.height = 24
	app.recalculateLayout()
	app.tabs[0].selecting = true
	app.tabs[0].selectAnchor = 1
	app.tabs[0].cursorLine = len(lines)
	app.openLineComment()

	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered, regions := app.renderWithModalLayout(background)
	plainRendered := ansi.Strip(rendered)
	if strings.Contains(plainRendered, "more lines") {
		t.Fatal("add comment context is summarized instead of scrollable")
	}
	for _, action := range []string{"Save ctrl+s", "Close esc", "Suggest alt+s"} {
		if !strings.Contains(plainRendered, action) {
			t.Fatalf("add comment modal does not show %q", action)
		}
	}
	if !strings.Contains(plainRendered, "context line 30") {
		t.Fatal("add comment context does not initially show the final selected line")
	}

	var contextRegion modalMouseRegion
	for _, region := range regions {
		if region.action.scrollable {
			contextRegion = region
			break
		}
	}
	if contextRegion.action.scrollMaxOffset == 0 {
		t.Fatal("long add comment context is not scrollable")
	}
	app.modalReferenceOffset = 0
	if rendered := ansi.Strip(app.View().Content); !strings.Contains(rendered, "context line 1") {
		t.Fatal("first selected line is not reachable by scrolling")
	}
}

func TestSuggestionIsAvailableWhenReplyingToLineComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	comment := review.Comment{
		ID: "c_agent", StartLine: 1, EndLine: 2, Scope: "line",
		Body: "agent reply", Author: "Codex", ReviewRound: 1,
	}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)
	if app.modal != replyModal || !app.canSuggest() {
		t.Fatalf("line reply modal = %v, canSuggest=%t", app.modal, app.canSuggest())
	}

	updated, _ := app.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
	app = *updated.(*AppModel)
	want := "```suggestion\nfirst\nsecond\n```"
	if got := app.modalTextarea.Value(); got != want {
		t.Fatalf("reply suggestion = %q, want %q", got, want)
	}
	if app.modalTextarea.Line() != 2 || app.modalTextarea.Column() != len("second") {
		t.Fatalf("reply suggestion cursor = %d:%d, want 2:%d",
			app.modalTextarea.Line(), app.modalTextarea.Column(), len("second"))
	}

	app.width = 80
	app.height = 24
	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	if rendered := app.renderWithModal(background); !strings.Contains(rendered, "Suggest") {
		t.Fatalf("reply modal does not contain Suggest button: %q", rendered)
	}
}

func TestSuggestionCodeCanBeDeletedOrReplaced(t *testing.T) {
	for _, code := range []string{"one", "日本語\n\n\t" + strings.Repeat("長い行", 30)} {
		for _, replace := range []bool{false, true} {
			app := setupAppWithDoc(t, code+"\n")
			app.tab().selecting = true
			app.tab().selectAnchor = 1
			app.tab().cursorLine = strings.Count(code, "\n") + 1
			app.modal = commentModal
			app.modalTextarea.SetWidth(12)
			app.modalTextarea.SetHeight(3)
			app.modalTextarea.SetValue("説明\n\n以前の本文")
			app.insertSuggestion()
			wantCode := strings.ReplaceAll(code, "\t", "    ")
			if got := app.modalTextarea.SelectedText(); got != wantCode {
				t.Fatalf("selected code = %q, want %q", got, wantCode)
			}
			replacement := ""
			if replace {
				replacement = "変更"
				updated, _ := app.Update(tea.KeyPressMsg{Code: '変', Text: replacement})
				app = *updated.(*AppModel)
			} else {
				app = pressKey(app, tea.KeyDelete)
			}
			want := "説明\n\n以前の本文\n\n```suggestion\n" + replacement + "\n```"
			if got := app.modalTextarea.Value(); got != want {
				t.Fatalf("edited suggestion = %q, want %q", got, want)
			}
		}
	}
}

func TestSuggestionScrollsCursorIntoView(t *testing.T) {
	lastLine := strings.Repeat("x", 48)
	app := setupAppWithDoc(t, "first\n"+lastLine+"\n")
	app.tabs[0].selecting = true
	app.tabs[0].selectAnchor = 1
	app.tabs[0].cursorLine = 2
	app.modal = commentModal
	app.modalTextarea.SetWidth(12)
	app.modalTextarea.SetHeight(3)

	app.insertSuggestion()
	app.modalTextarea.SetVirtualCursor(false)
	cursor := app.modalTextarea.Cursor()
	if cursor == nil {
		t.Fatal("suggestion cursor is hidden")
	}
	if cursor.Y < 0 || cursor.Y >= app.modalTextarea.Height() {
		t.Fatalf("suggestion cursor Y = %d outside textarea height %d",
			cursor.Y, app.modalTextarea.Height())
	}
}

func TestSuggestionIsUnavailableWhenReplyingToFileComment(t *testing.T) {
	app := setupAppWithDoc(t, "first\n")
	comment := review.Comment{ID: "c_file", Scope: "file", Body: "whole file", Author: "Codex"}
	app.tabs[0].state.Comments = []review.Comment{comment}
	app.openCommentThread(comment.ID)

	if app.modal != replyModal || app.canSuggest() {
		t.Fatalf("file reply modal = %v, canSuggest=%t", app.modal, app.canSuggest())
	}
}

func TestEditModalDeletesOwnCurrentRoundParent(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.modal = editModal
	app.editingID = comment.ID
	app.modalFocus = app.modalDeleteStartFocus()

	app = pressKey(app, tea.KeyEnter)
	if app.modal != deleteConfirmModal || len(app.tabs[0].state.Comments) != 1 {
		t.Fatalf("modal = %v, comments = %+v; want delete confirmation", app.modal, app.tabs[0].state.Comments)
	}
	app = pressKey(app, 'y')

	if len(app.tabs[0].state.Comments) != 0 {
		t.Fatalf("expected parent comment deleted, got %+v", app.tabs[0].state.Comments)
	}
}

func TestDeleteKeyDeletesSelectedSidebarCommentAfterConfirmation(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, 'd')
	if app.modal != deleteConfirmModal || app.editingID != comment.ID {
		t.Fatalf("modal = %v, editing ID = %q; want delete confirmation for %q",
			app.modal, app.editingID, comment.ID)
	}
	app = pressKey(app, 'n')
	if app.modal != noModal || app.editingID != "" || len(app.tabs[0].state.Comments) != 1 {
		t.Fatalf("cancel left modal = %v, editing ID = %q, comments = %+v",
			app.modal, app.editingID, app.tabs[0].state.Comments)
	}

	app = pressKey(app, 'd')
	app = pressKey(app, 'y')
	if app.modal != noModal || len(app.tabs[0].state.Comments) != 0 {
		t.Fatalf("modal = %v, comments = %+v; want selected comment deleted",
			app.modal, app.tabs[0].state.Comments)
	}
}

func TestDeleteKeyDeletesFocusedInlineCommentAfterConfirmation(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.tab().doc = document.FromContent("test.go", []byte("line\n"))
	app.focused = contentPane
	app.tabs[0].cursorLine = comment.EndAt()
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0

	app = pressKey(app, 'd')
	if app.modal != deleteConfirmModal {
		t.Fatalf("modal = %v, want delete confirmation", app.modal)
	}
	app = pressKey(app, 'y')
	if len(app.tabs[0].state.Comments) != 0 || app.tabs[0].cursorOnAnnotation {
		t.Fatalf("comments = %+v, annotation focus = %t; want focused comment deleted",
			app.tabs[0].state.Comments, app.tabs[0].cursorOnAnnotation)
	}
}

func TestDeleteKeyIgnoresIneligibleSelectedComment(t *testing.T) {
	comment := testComment()
	comment.Author = "another reviewer"
	comment.ReviewRound = 1
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.focused = commentPane
	app.updateCommentSidebar()

	app = pressKey(app, 'd')
	if app.modal != noModal || app.editingID != "" || len(app.tabs[0].state.Comments) != 1 {
		t.Fatalf("modal = %v, editing ID = %q, comments = %+v; want no action",
			app.modal, app.editingID, app.tabs[0].state.Comments)
	}
}

func TestEditModalDeletesOnlyOwnCurrentRoundReply(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	comment.Replies = []review.Reply{
		{ID: "rp_own", Author: "Tester", Body: "mine", ReviewRound: 1},
		{ID: "rp_old", Author: "Tester", Body: "old", ReviewRound: 0},
		{ID: "rp_other", Author: "AI", Body: "other", ReviewRound: 1},
	}
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.modal = editModal
	app.editingID = comment.ID
	app.editingReplyID = "rp_own"
	targets := app.modalDeleteTargets()
	if len(targets) != 1 || targets[0].replyID != "rp_own" {
		t.Fatalf("delete targets = %+v, want only rp_own", targets)
	}
	app.modalFocus = app.modalDeleteStartFocus()

	app = pressKey(app, tea.KeyEnter)
	if app.modal != deleteConfirmModal || len(app.tabs[0].state.Comments[0].Replies) != 3 {
		t.Fatalf("modal = %v, replies = %+v; want delete confirmation", app.modal, app.tabs[0].state.Comments[0].Replies)
	}
	app = pressKey(app, 'y')

	comments := app.tabs[0].state.Comments
	if len(comments) != 1 {
		t.Fatalf("expected parent preserved, got %+v", comments)
	}
	if len(comments[0].Replies) != 2 || comments[0].Replies[0].ID != "rp_old" || comments[0].Replies[1].ID != "rp_other" {
		t.Fatalf("remaining replies = %+v, want old and other", comments[0].Replies)
	}
}

func TestDriftedReplyModalStartsAtLatestReply(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(fmt.Sprintf("edit=%v", edit), func(t *testing.T) {
			comment := testComment()
			comment.Author = "Other"
			comment.Drifted = true
			comment.Quote = "original source"
			comment.Body = strings.Repeat("old comment\n", 30)
			comment.Replies = []review.Reply{{ID: "latest", Author: "Other", Body: "LATEST REPLY", ReviewRound: 1}}
			if edit {
				comment.Replies = append(comment.Replies, review.Reply{ID: "own", Author: "Tester", Body: "edit this reply", ReviewRound: 1})
			}
			app, _ := newFinishTestApp(t, []review.Comment{comment})
			app.width, app.height = 100, 40
			app.recalculateLayout()
			app.openCommentThread(comment.ID)
			if view := ansi.Strip(app.View().Content); !strings.Contains(view, "LATEST REPLY") {
				t.Fatal("drifted reply dialog did not show the latest preceding reply")
			}
			app.scrollModalReference(-100)
			if view := ansi.Strip(app.View().Content); !strings.Contains(view, "original source") {
				t.Fatal("original drifted context is no longer accessible by scrolling")
			}
		})
	}
}

func TestEditReplyModalKeepsDeleteButtonVisibleWithLongThread(t *testing.T) {
	comment := testComment()
	comment.EndLine = 12
	comment.Author = "Reviewer"
	comment.Body = strings.Repeat("long parent comment ", 12)
	for i := range 12 {
		comment.Replies = append(comment.Replies, review.Reply{
			ID: "rp_" + string(rune('a'+i)), Author: "AI",
			Body: strings.Repeat("long reply body ", 12), ReviewRound: 1,
		})
	}
	comment.Replies = append(comment.Replies, review.Reply{
		ID: "rp_own", Author: "Tester", Body: "edit me\nsecond line\nlast line", ReviewRound: 1,
	})
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	lines := make([]string, comment.EndLine)
	for i := range lines {
		lines[i] = "context line " + strconv.Itoa(i+1)
	}
	app.tabs[0].doc = &document.Document{
		Path: "test.go", Content: strings.Join(lines, "\n"), Lines: lines,
	}
	app.width = 128
	app.height = 30
	app.recalculateLayout()
	app.modal = editModal
	app.editingID = comment.ID
	app.editingReplyID = "rp_own"
	app.modalReferenceOffset = -1
	app.modalTextarea.SetValue(strings.Repeat("unsaved reply text ", 12))

	const displayHeight = 24
	background := lipgloss.NewStyle().Width(app.width).Height(displayHeight).Render("")
	staleHeightRendered, _ := app.renderWithModalLayout(background)
	for _, action := range []string{"Save ctrl+s", "Close esc", "Suggest alt+s", "Delete reply"} {
		if !strings.Contains(ansi.Strip(staleHeightRendered), action) {
			t.Fatalf("modal using display height does not show %q", action)
		}
	}
	app.height = displayHeight
	app.recalculateLayout()
	rendered, regions := app.renderWithModalLayout(background)
	if height := lipgloss.Height(rendered); height > displayHeight {
		t.Fatalf("modal height = %d, display height = %d", height, displayHeight)
	}
	plainRendered := ansi.Strip(rendered)
	for _, action := range []string{"Save ctrl+s", "Close esc", "Suggest alt+s", "Delete reply"} {
		if !strings.Contains(plainRendered, action) {
			t.Fatalf("modal does not show %q", action)
		}
	}
	var referenceRegion modalMouseRegion
	deleteFound := false
	for _, region := range regions {
		if region.rect.bottom > displayHeight {
			t.Fatalf("modal region bottom = %d, display height = %d", region.rect.bottom, displayHeight)
		}
		if region.action.scrollable {
			referenceRegion = region
		}
		if region.action.deleteIndex == 0 && region.action.focus == app.modalDeleteStartFocus() {
			deleteFound = true
			break
		}
	}
	if !deleteFound {
		t.Fatal("delete reply button region not found")
	}
	if referenceRegion.rect.bottom == 0 {
		t.Fatal("scrollable reference region not found")
	}
	if referenceRegion.action.scrollMaxOffset == 0 {
		t.Fatal("long reference content is not scrollable")
	}
	before := ansi.Strip(rendered)
	if !strings.Contains(before, "long reply body") {
		t.Fatal("combined reference region does not initially show the preceding reply")
	}
	if strings.Contains(before, "edit me") || strings.Contains(before, "last line") {
		t.Fatal("combined reference region includes the reply being edited")
	}
	app = wheelMouse(app, referenceRegion.rect.left, referenceRegion.rect.top, tea.MouseWheelUp)
	if app.modalReferenceOffset >= referenceRegion.action.scrollMaxOffset {
		t.Fatal("wheel up did not scroll reference content")
	}
	after := ansi.Strip(app.View().Content)
	if before == after {
		t.Fatal("reference content did not change after scrolling")
	}
	app.modalReferenceOffset = 0
	if rendered := ansi.Strip(app.View().Content); !strings.Contains(rendered, "context line 1") {
		t.Fatal("first line of code context is not reachable by scrolling")
	}

	const shortDisplayHeight = 16
	shortBackground := lipgloss.NewStyle().Width(app.width).Height(shortDisplayHeight).Render("")
	shortRendered, shortRegions := app.renderWithModalLayout(shortBackground)
	if !strings.Contains(ansi.Strip(shortRendered), "Delete reply") {
		t.Fatal("oversized modal is not initially scrolled to the delete action")
	}
	for _, region := range shortRegions {
		if region.action.focus == app.modalDeleteStartFocus() && region.rect.bottom > shortDisplayHeight {
			t.Fatalf("delete action bottom = %d, display height = %d", region.rect.bottom, shortDisplayHeight)
		}
	}
}

func TestKeyboardScrollsModalReferenceByPage(t *testing.T) {
	comment := testComment()
	comment.Author = "Reviewer"
	comment.Body = strings.Repeat("long parent comment\n", 30)
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.width = 80
	app.height = 24
	app.recalculateLayout()
	app.openCommentThread(comment.ID)

	var scrollRegion modalMouseRegion
	for _, region := range app.modalMouseRegions() {
		if region.action.scrollable {
			scrollRegion = region
			break
		}
	}
	if scrollRegion.action.scrollMaxOffset == 0 {
		t.Fatal("long modal reference is not scrollable")
	}

	pageSize := max(1, scrollRegion.rect.bottom-scrollRegion.rect.top-2)
	updated, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModCtrl})
	app = *updated.(*AppModel)
	want := max(0, scrollRegion.action.scrollOffset-pageSize)
	if app.modalReferenceOffset != want {
		t.Fatalf("Ctrl-PgUp offset = %d, want %d", app.modalReferenceOffset, want)
	}
	if !app.modalTextarea.Focused() {
		t.Fatal("Ctrl-PgUp moved focus away from textarea")
	}

	updated, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgDown, Mod: tea.ModCtrl})
	app = *updated.(*AppModel)
	want = min(scrollRegion.action.scrollMaxOffset, want+pageSize)
	if app.modalReferenceOffset != want {
		t.Fatalf("Ctrl-PgDown offset = %d, want %d", app.modalReferenceOffset, want)
	}
}

func TestScrollableModalBoxHonorsMinimumHeight(t *testing.T) {
	box, _, maxOffset := renderScrollableModalBox(strings.Repeat("long content ", 20), 40, 3, 0)
	if height := lipgloss.Height(box); height > 3 {
		t.Fatalf("box height = %d, max height = 3", height)
	}
	if width := lipgloss.Width(box); width != 40 {
		t.Fatalf("box width = %d, want 40", width)
	}
	if maxOffset == 0 {
		t.Fatal("clipped box is not scrollable")
	}
}

func TestModalBoxKeepsTabbedLinesInsideBorders(t *testing.T) {
	box, _, _ := renderScrollableModalBox("15 \t"+strings.Repeat("a", 23), 30, 10, 0)
	for i, row := range strings.Split(ansi.Strip(box), "\n") {
		if width := lipgloss.Width(row); width != 30 {
			t.Errorf("row %d width = %d, want 30: %q", i, width, row)
		}
		if !strings.ContainsAny(row[:3], "╭│╰") {
			t.Errorf("row %d lacks a left border: %q", i, row)
		}
	}
}

func TestContextPreviewIndentsWrappedTabbedLines(t *testing.T) {
	app := setupAppWithDoc(t, "x\n\tn, err := strconv.Atoi(os.Getenv(\"GREET_COUNT\"))\n")
	preview := ansi.Strip(app.renderContextPreview("", 2, 2, 30, 0))
	lines := strings.Split(preview, "\n")
	if len(lines) < 2 {
		t.Fatalf("preview did not wrap: %q", preview)
	}
	for i, line := range lines {
		if strings.Contains(line, "\t") {
			t.Errorf("line %d keeps a tab: %q", i, line)
		}
		if width := lipgloss.Width(line); width > 30 {
			t.Errorf("line %d width = %d, max 30: %q", i, width, line)
		}
		if i > 0 && !strings.HasPrefix(line, strings.Repeat(" ", 6)) {
			t.Errorf("continuation line %d is not indented: %q", i, line)
		}
	}
}

func TestEnterAddsThenEditsOwnCurrentRoundReply(t *testing.T) {
	comment := testComment()
	comment.Author = "Tester"
	comment.ReviewRound = 1
	comment.Resolved = true
	comment.ResolvedRound = 1
	comment.Replies = []review.Reply{{
		ID: "rp_ai", Author: "AI", Body: "addressed", ReviewRound: 1,
	}}
	app, _ := newFinishTestApp(t, []review.Comment{comment})
	app.focused = contentPane
	app.tabs[0].doc = &document.Document{Path: "test.go", Content: "line", Lines: []string{"line"}}
	app.tabs[0].cursorLine = comment.EndAt()
	app.tabs[0].cursorOnAnnotation = true
	app.tabs[0].cursorAnnoIdx = 0

	app = pressKey(app, tea.KeyEnter)
	if app.modal != replyModal || app.editingID != comment.ID {
		t.Fatalf("enter opened modal %v for %q; want reply modal for %q", app.modal, app.editingID, comment.ID)
	}

	app.modalTextarea.SetValue("follow-up")
	app.modalSubmit()
	replies := app.tabs[0].state.Comments[0].Replies
	if len(replies) != 2 {
		t.Fatalf("replies = %+v, want added follow-up", replies)
	}
	added := replies[1]
	if added.Author != "Tester" || added.ReviewRound != 1 || added.Body != "follow-up" {
		t.Fatalf("added reply = %+v", added)
	}
	if !app.tabs[0].state.Comments[0].Resolved || app.tabs[0].state.Comments[0].ResolvedRound != 1 {
		t.Error("adding a reply should preserve the displayed resolution")
	}

	app = pressKey(app, tea.KeyEnter)
	if app.modal != editModal || app.editingReplyID != added.ID {
		t.Fatalf("enter opened modal %v for reply %q; want edit modal for %q", app.modal, app.editingReplyID, added.ID)
	}
	if got := app.modalTextarea.Value(); got != "follow-up" {
		t.Errorf("edit reply body = %q, want follow-up", got)
	}
	if targets := app.modalDeleteTargets(); len(targets) != 1 || targets[0].replyID != added.ID {
		t.Fatalf("delete targets = %+v, want added reply", targets)
	}
	app.width, app.height = 128, 40
	app.recalculateLayout()
	app.modalTextarea.SetValue("revised follow-up")
	background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
	rendered, _ := app.renderWithModalLayout(background)
	plain := ansi.Strip(rendered)
	if strings.Count(plain, "follow-up") != 1 || !strings.Contains(plain, "revised follow-up") {
		t.Errorf("edited reply should appear only in the textarea:\n%s", plain)
	}
	if !strings.Contains(plain, "addressed") {
		t.Error("edit modal should preserve the preceding reply")
	}
}

func TestEnterDoesNotEditOwnReplyAfterFollowup(t *testing.T) {
	comment := testComment()
	comment.Replies = []review.Reply{
		{ID: "rp_own", Author: "Tester", Body: "question", ReviewRound: 1},
		{ID: "rp_ai", Author: "AI", Body: "answer", ReviewRound: 1},
	}
	app, _ := newFinishTestApp(t, []review.Comment{comment})

	app.openCommentThread(comment.ID)

	if app.modal != replyModal || app.editingReplyID != "" {
		t.Fatalf("modal = %v, editing reply = %q; want a new reply", app.modal, app.editingReplyID)
	}
}

func TestEnterEditsOnlyOwnUnrepliedCurrentRoundParent(t *testing.T) {
	tests := []struct {
		name      string
		author    string
		round     int
		wantModal modalType
	}{
		{name: "own current", author: "Tester", round: 1, wantModal: editModal},
		{name: "another author", author: "AI", round: 1, wantModal: replyModal},
		{name: "previous round", author: "Tester", round: 0, wantModal: replyModal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := testComment()
			comment.Author = tt.author
			comment.ReviewRound = tt.round
			app, _ := newFinishTestApp(t, []review.Comment{comment})

			app.openCommentThread(comment.ID)

			if app.modal != tt.wantModal {
				t.Errorf("modal = %v, want %v", app.modal, tt.wantModal)
			}
		})
	}
}

func TestEditModalHidesDeleteForIneligibleParent(t *testing.T) {
	tests := []struct {
		name    string
		author  string
		round   int
		replies []review.Reply
	}{
		{name: "another author", author: "AI", round: 1},
		{name: "previous round", author: "Tester", round: 0},
		{name: "thread with reply", author: "Tester", round: 1, replies: []review.Reply{{ID: "rp_1", Author: "AI"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := testComment()
			comment.Author = tt.author
			comment.ReviewRound = tt.round
			comment.Replies = tt.replies
			app, _ := newFinishTestApp(t, []review.Comment{comment})
			app.modal = editModal
			app.editingID = comment.ID

			if targets := app.modalDeleteTargets(); len(targets) != 0 {
				t.Fatalf("delete targets = %+v, want none", targets)
			}
		})
	}
}
