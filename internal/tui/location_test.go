package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestStartupFocus(t *testing.T) {
	for _, lateWindow := range []bool{false, true} {
		for _, line := range []int{0, 25} {
			t.Run(fmt.Sprintf("line=%d/lateWindow=%t", line, lateWindow), func(t *testing.T) {
				t.Chdir(t.TempDir())
				for _, path := range []string{"a.txt", "z.txt"} {
					if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 40)), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				app := NewCodeReviewApp([]gitpkg.FileChange{{Path: "z.txt"}, {Path: "a.txt"}}, "HEAD", AppConfig{FocusPath: "./z.txt", FocusLine: line})
				probeID := app.images.probeID
				window := tea.WindowSizeMsg{Width: 100, Height: 18}
				if !lateWindow {
					app, _ = updateApp(app, window)
				}
				app.previousReplyIDs = []string{}
				app, _ = updateApp(app, docRenderedMsg{})
				app.tabs[0].state.Comments = []review.Comment{{ID: "thread", StartLine: 2, EndLine: 2, Replies: []review.Reply{{ID: "new", Author: "Agent"}}}}
				if lateWindow {
					app, _ = updateApp(app, window)
				}
				if app.activeTab != 1 || app.tab().cursorLine != max(1, line) || app.tab().cursorOnAnnotation {
					t.Fatalf("focus = tab %d, line %d, annotation %t", app.activeTab, app.tab().cursorLine, app.tab().cursorOnAnnotation)
				}
				if app.images.retriedOnKey || app.images.probeID != probeID {
					t.Fatal("startup focus retried image detection")
				}
				if line == 25 && app.contentViewport.YOffset() == 0 {
					t.Fatal("focused line was not scrolled into view")
				}
				app.tab().cursorLine = 3
				app, _ = updateApp(app, window)
				if app.tab().cursorLine != 3 {
					t.Fatal("resize reapplied startup focus")
				}
			})
		}
	}
}

func TestGotoLineAndEditorFallback(t *testing.T) {
	app := setupAppWithDoc(t, "one\ntwo\nthree")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.rebuildContent()
	app.updateCommentSidebar()
	app.tab().doc.Known = map[int]bool{1: true, 3: true}
	app.focused = commentPane
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'g', Mod: tea.ModAlt})
	if app.modal != gotoLineModal || app.View().Cursor == nil {
		t.Fatal("M-g did not open an input with a terminal cursor")
	}
	app.lineInput.SetValue("0")
	app = pressKey(app, tea.KeyEnter)
	if app.modal != gotoLineModal || app.locationError == "" {
		t.Fatal("invalid line number closed the prompt")
	}
	app.lineInput.SetValue("3")
	app = pressKey(app, tea.KeyEnter)
	if app.modal != noModal || app.tab().cursorLine != 3 || app.focused != contentPane {
		t.Fatal("known line did not navigate")
	}
	for _, line := range []string{"2", "99999"} {
		app, _ = updateApp(app, tea.KeyPressMsg{Code: 'g', Mod: tea.ModAlt})
		app, _ = updateApp(app, tea.PasteMsg{Content: line})
		app, cmd := pressKeyCmd(app, tea.KeyEnter)
		if cmd != nil || app.modal != openSourceModal || app.modalFocus != 1 {
			t.Fatal("missing line must ask before opening editor, defaulting to cancel")
		}
		if app.tab().cursorLine != 3 {
			t.Fatal("missing line changed the cursor")
		}
		cancelled, cmd := pressKeyCmd(app, tea.KeyEnter)
		if cmd != nil || cancelled.modal != noModal {
			t.Fatal("default confirmation did not cancel")
		}
		for _, region := range app.modalMouseRegions() {
			if region.action.focus == 0 {
				opened, cmd := clickMouseCmd(app, region.rect.left, region.rect.top)
				if cmd == nil || opened.modal != noModal {
					t.Fatal("Open click did not request editor launch")
				}
			}
		}
	}
}

func TestGotoLineButtons(t *testing.T) {
	for _, focus := range []int{1, 2} {
		app := setupAppWithDoc(t, "one\ntwo\nthree")
		app.width, app.height = 80, 24
		app.recalculateLayout()
		app.rebuildContent()
		app.updateCommentSidebar()
		app.tab().cursorLine = 1
		app.openGotoLine()
		app.lineInput.SetValue("3")
		content, _ := app.locationModalContent(40)
		if !strings.Contains(ansi.Strip(content), "Go RET") || !strings.Contains(ansi.Strip(content), "Cancel ESC") {
			t.Fatal("dialog actions missing")
		}
		clicked := false
		for _, region := range app.modalMouseRegions() {
			if region.action.focus == focus {
				app, _ = clickMouseCmd(app, region.rect.left, region.rect.top)
				clicked = true
				break
			}
		}
		if !clicked || app.modal != noModal {
			t.Fatal("dialog button was not clickable")
		}
		want := 1
		if focus == 1 {
			want = 3
		}
		if app.tab().cursorLine != want {
			t.Fatalf("cursor = %d, want %d", app.tab().cursorLine, want)
		}
		app.openGotoLine()
		app.lineInput.SetValue("2")
		app = pressKey(app, tea.KeyTab)
		if app.modalFocus != 1 || app.lineInput.Focused() {
			t.Fatal("Tab did not focus Go")
		}
		app = pressKey(app, tea.KeyEnter)
		if app.tab().cursorLine != 2 || app.modal != noModal {
			t.Fatal("focused Go did not navigate")
		}
	}
}

func TestGotoLineInputClickMovesCursor(t *testing.T) {
	for _, tc := range []struct {
		name string
		x    int
		want int
	}{
		{"prompt", -2, 0},
		{"start", 0, 0},
		{"middle", 2, 2},
		{"end", 5, 5},
		{"past end", 8, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "one\ntwo\nthree")
			app.width, app.height = 80, 24
			app.recalculateLayout()
			app.openGotoLine()
			app.lineInput.SetValue("12345")
			app = pressKey(app, tea.KeyTab)
			for _, region := range app.modalMouseRegions() {
				if !region.action.lineInput {
					continue
				}
				app, _ = clickMouseCmd(app, region.rect.left+len(app.lineInput.Prompt)+tc.x, region.rect.top)
				if app.modalFocus != 0 || !app.lineInput.Focused() || app.lineInput.Position() != tc.want {
					t.Fatalf("input focus = %d, cursor = %d; want focused input at %d", app.modalFocus, app.lineInput.Position(), tc.want)
				}
				return
			}
			t.Fatal("input mouse region missing")
		})
	}
}

func TestSourceNavigationAcrossTabs(t *testing.T) {
	app := setupAppWithDoc(t, "first")
	path := filepath.Join(filepath.Dir(app.tab().path), "second.go")
	app.tabs = append(app.tabs, FileTab{path: path,
		doc: document.FromContent(path, []byte("a\nb\nc")), state: &fileReview{}})
	app.tabs[1].cursorOnAnnotation, app.tabs[1].selecting = true, true
	app.navigateSource(sourceLocation{path: path, line: 2})
	if app.activeTab != 1 || app.tab().cursorLine != 2 || app.tab().cursorOnAnnotation || app.tab().selecting {
		t.Fatal("did not focus source in second tab")
	}
	// M-g can navigate a snapshot even if its file is gone from disk.
	if regularFile(path) {
		t.Fatal("fixture should not exist on disk")
	}
	app.navigateSource(sourceLocation{path: path, line: 99})
	if app.modal != noModal || app.locationError == "" {
		t.Fatal("missing file must not offer to create it")
	}
}

func TestOpenCurrentSourceAndModalIsolation(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond")
	t.Setenv("EDITOR", "vi")
	editorGotoCache.Store("vi", false)
	t.Cleanup(func() { editorGotoCache.Delete("vi") })
	app.tab().cursorLine = 2
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'e', Mod: tea.ModAlt})
	if cmd == nil || app.modal != noModal {
		t.Fatal("M-e should launch without confirmation")
	}
	ready := cmd().(sourceEditorReadyMsg)
	if ready.err != nil || !strings.HasSuffix(strings.Join(ready.cmd.Args, "|"), "|+2|"+app.tab().path) {
		t.Fatalf("editor command = %#v, %v", ready.cmd, ready.err)
	}
	app.modal = commentModal
	app.modalTextarea.SetValue("draft")
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'g', Mod: tea.ModAlt})
	if app.modal != commentModal {
		t.Fatal("M-g escaped comment input")
	}
	app.modal = noModal
	if err := os.Remove(app.tab().path); err != nil {
		t.Fatal(err)
	}
	_, cmd = updateApp(app, tea.KeyPressMsg{Code: 'e', Mod: tea.ModAlt})
	if cmd != nil {
		t.Fatal("M-e must not create a missing file")
	}
}
