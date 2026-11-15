package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/v2/internal/review"
)

func assertCopiedText(t *testing.T, app AppModel, cmd tea.Cmd, want string) {
	t.Helper()
	if app.killRing.current() != want {
		t.Fatalf("kill ring=%q, want %q", app.killRing.current(), want)
	}
	if cmd == nil {
		t.Fatal("missing clipboard command")
	}
	msg := cmd()
	for {
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			break
		}
		if len(batch) == 0 {
			t.Fatal("empty clipboard batch")
		}
		msg = batch[0]()
	}
	if !reflect.DeepEqual(msg, tea.SetClipboard(want)()) {
		t.Fatalf("clipboard message=%#v", msg)
	}
}

func TestCopyFocusedSource(t *testing.T) {
	const source = "before **bold**\t<kbd>X</kbd>\r\n\nlast"
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(readOnly), func(t *testing.T) {
			app := setupPrivateAppWithDoc(t, source)
			app.readOnly = readOnly
			app.tab().cursorLine = 1
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
			var cmd tea.Cmd
			app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			assertCopiedText(t, app, cmd, "before **bold**\t<kbd>X</kbd>\r")
			app.tab().cursorLine = 3
			app = pressKey(app, 'v')
			app = pressKey(app, 'k')
			app = pressKey(app, 'k')
			app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			assertCopiedText(t, app, cmd, source)
			if !app.tab().selecting {
				t.Fatal("line selection was cleared")
			}
			app, cmd = pressKeyCmd(app, '@')
			assertCopiedText(t, app, cmd, "@"+escapeReferencePath(app.tab().path)+" L1")
		})
	}
}

func TestCopyDiffSourceAndUnavailableRanges(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			app := patchApp(t, formatsPatch)
			app.split = split
			app.tab().cursorLine, app.tab().cursorSide = 11, "old"
			app.rebuildContent()
			app = pressKey(app, 'v')
			app.tab().cursorLine = 13
			var cmd tea.Cmd
			app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			assertCopiedText(t, app, cmd, "old one\nold two\nafter")
			app.tab().selectAnchor, app.tab().cursorLine = 20, 22
			app.tab().selectSide, app.tab().cursorSide = "", ""
			app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			assertCopiedText(t, app, cmd, "before\nnew one\nafter")
			for _, side := range []string{"", "old"} {
				app.tab().selectSide, app.tab().cursorSide = side, side
				app.tab().selectAnchor, app.tab().cursorLine = 20, 50
				app, _ = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
				if !strings.Contains(app.notice.text, "source lines unavailable") || app.killRing.current() != "before\nnew one\nafter" {
					t.Fatal("unavailable text replaced the clipboard")
				}
			}
		})
	}
}

func TestCopyFocusedComments(t *testing.T) {
	for _, inline := range []bool{false, true} {
		for _, replies := range []bool{false, true} {
			t.Run(fmt.Sprintf("inline=%t replies=%t", inline, replies), func(t *testing.T) {
				app := setupPrivateAppWithDoc(t, "source")
				comment := review.Comment{ID: "c_abcdef", StartLine: 1, Author: "Re_viewer", Body: "Keep **bold** and `code`."}
				want := comment.Body
				if replies {
					comment.Replies = []review.Reply{{Author: "Agent", Body: "[Link](https://example.org)"}, {Body: "Final reply."}}
					want = "**Re\\_viewer**\n\n" + comment.Body + "\n\n**Agent**\n\n[Link](https://example.org)\n\n**anonymous**\n\nFinal reply."
				}
				app.tab().state.Comments = []review.Comment{comment}
				app.tab().cursorLine = 1
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: 160, Height: 40})
				if inline {
					app.tab().cursorOnAnnotation = true
				} else {
					app.focused, app.sidebarView = commentPane, commentsView
				}
				app.updateCommentSidebar()
				app.rebuildContent()
				var cmd tea.Cmd
				copyKey := tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt}
				if inline {
					copyKey = tea.KeyPressMsg{Code: 'Y'}
				}
				app, cmd = updateApp(app, copyKey)
				assertCopiedText(t, app, cmd, want)
				app, cmd = pressKeyCmd(app, '@')
				assertCopiedText(t, app, cmd, comment.ID)
			})
		}
	}
}

func TestReferenceKeyTypesInEditor(t *testing.T) {
	app := setupPrivateAppWithDoc(t, "source")
	app = pressKey(app, 'f')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: '@', Text: "@"})
	if app.modalTextarea.Value() != "@" || len(app.killRing.entries) != 0 {
		t.Fatal("reference shortcut intercepted editor input")
	}
}
