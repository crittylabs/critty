package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/review"
)

// setupSession points XDG state at a temp dir and opens a doc session there.
func setupSession(t *testing.T, docPath string) *review.Session {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := review.OpenDocSession("", docPath)
	if err != nil {
		t.Fatalf("opening session: %v", err)
	}
	return sess
}

func setupAppWithDoc(t *testing.T, content string) AppModel {
	t.Helper()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	app := NewApp(testFile, AppConfig{Session: setupSession(t, testFile), Author: "Tester"})
	app.tabs = []FileTab{{path: testFile}}
	app.activeTab = 0
	updated, _ := app.Update(docRenderedMsg{})
	a := updated.(AppModel)
	a.contentViewport.SetHeight(5)
	a.sidebarView = commentsView // most tests exercise the comment sidebar
	return a
}

func pressKey(app AppModel, code rune) AppModel {
	app, _ = pressKeyCmd(app, code)
	return app
}

func clickMouse(app AppModel, x, y int) AppModel {
	return releaseMouse(pressMouse(app, x, y), x, y)
}

func clickMouseCmd(app AppModel, x, y int) (AppModel, tea.Cmd) {
	return updateApp(app, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func pressMouse(app AppModel, x, y int) AppModel {
	app, _ = clickMouseCmd(app, x, y)
	return app
}

func moveMouse(app AppModel, x, y int) AppModel {
	app, _ = updateApp(app, tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	return app
}

func hoverMouse(app AppModel, x, y int) AppModel {
	app, _ = updateApp(app, tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseNone})
	return app
}

func releaseMouse(app AppModel, x, y int) AppModel {
	app, _ = updateApp(app, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return app
}

func clickModalAction(t *testing.T, app AppModel, needle string) AppModel {
	t.Helper()
	focus := -1
	switch {
	case strings.HasPrefix(needle, "Discard"):
		focus = 0
	case strings.HasPrefix(needle, "Delete") && app.modal == deleteConfirmModal:
		focus = 0
	case strings.HasPrefix(needle, "Keep Editing"):
		focus = 1
	case strings.HasPrefix(needle, "Keep"):
		focus = 1
	case strings.HasPrefix(needle, "Save"):
		focus = 1
	case strings.HasPrefix(needle, "Suggest"):
		focus = 3
	case strings.HasPrefix(needle, "Close") && app.modal == finishModal:
		focus = 1
	case strings.HasPrefix(needle, "Close"):
		focus = 2
	case strings.HasPrefix(needle, "Delete"):
		focus = app.modalDeleteStartFocus()
	}
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == focus {
			return clickMouse(app, region.rect.left, region.rect.top)
		}
	}
	t.Fatalf("modal action %q not found", needle)
	return app
}

func modalTextareaRect(t *testing.T, app AppModel) mouseRect {
	t.Helper()
	for _, region := range app.modalMouseRegions() {
		if region.action.textarea {
			return region.rect
		}
	}
	t.Fatal("modal textarea region not found")
	return mouseRect{}
}

func assertRegionContainsRenderedText(t *testing.T, app AppModel, rect mouseRect, text string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(app.View().Content), "\n")
	if rect.top < 0 || rect.top >= len(lines) {
		t.Fatalf("region row %d is outside %d rendered rows", rect.top, len(lines))
	}
	byteIndex := strings.Index(lines[rect.top], text)
	if byteIndex < 0 {
		t.Fatalf("rendered row %d does not contain %q: %q", rect.top, text, lines[rect.top])
	}
	x := lipgloss.Width(lines[rect.top][:byteIndex])
	if x < rect.left || x >= rect.right {
		t.Fatalf("rendered %q starts at x=%d outside region [%d,%d)", text, x, rect.left, rect.right)
	}
}

func renderedLineY(t *testing.T, app AppModel, needle string) int {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(app.View().Content), "\n") {
		if strings.Contains(line, needle) {
			return y
		}
	}
	t.Fatalf("rendered line %q not found", needle)
	return 0
}

func contentScreenPoint(app AppModel, x, contentY int) (int, int) {
	left, top, _, _ := app.contentBounds()
	return left + x, top + contentY - app.contentViewport.YOffset()
}

func wheelMouse(app AppModel, x, y int, button tea.MouseButton) AppModel {
	app, _ = updateApp(app, tea.MouseWheelMsg{X: x, Y: y, Button: button})
	return app
}

func newCommentNavigationTestApp() AppModel {
	app := NewApp("first.go", AppConfig{})
	app.sidebarView = commentsView
	lines := []string{"one", "two", "three", "four"}
	tab := func(path string, comments ...review.Comment) FileTab {
		return FileTab{
			path:       path,
			cursorLine: 1,
			doc: &document.Document{
				Path: path, Content: strings.Join(lines, "\n"), Lines: lines,
			},
			state: &fileReview{Comments: comments},
		}
	}
	comment := func(id string, line int) review.Comment {
		return review.Comment{ID: id, StartLine: line, EndLine: line, Body: id}
	}
	app.tabs = []FileTab{
		tab("first.go", comment("first-a", 2), comment("first-b", 2), comment("first-c", 4)),
		tab("empty.go"),
		tab("last.go", comment("last-a", 1), comment("last-b", 3)),
	}
	app.multiFile = true
	app.contentViewport.SetWidth(80)
	app.contentViewport.SetHeight(20)
	return app
}

func newChangeNavigationTestApp() AppModel {
	app := newCommentNavigationTestApp()
	app.tabs[0].changeChunks = []changeChunk{
		{startLine: 2, endLine: 2},
		{startLine: 4, endLine: 4},
	}
	app.tabs[2].changeChunks = []changeChunk{
		{startLine: 1, endLine: 1},
		{startLine: 3, endLine: 3},
	}
	return app
}

func newScrollTestApp(path string, lines []string, isMarkdown bool, width, height int) AppModel {
	app := NewApp(path, AppConfig{})
	app.tabs[0].doc = &document.Document{
		Path:    path,
		Content: strings.Join(lines, "\n"),
		Lines:   lines,
	}
	app.tabs[0].isMarkdown = isMarkdown
	if !isMarkdown {
		// Stand in for highlightCode output: one rendered line per doc line.
		app.tabs[0].chromaLines = lines
	}
	app.contentViewport.SetWidth(width)
	app.contentViewport.SetHeight(height)
	return app
}

func newFinishTestApp(t *testing.T, comments []review.Comment) (AppModel, chan FinishEvent) {
	t.Helper()
	t.Chdir(t.TempDir())
	finishCh := make(chan FinishEvent, 4)
	app := NewApp("test.go", AppConfig{
		Session:  setupSession(t, "test.go"),
		Author:   "Tester",
		FinishCh: finishCh,
	})
	app.tabs[0].state = &fileReview{Comments: comments}
	app.sidebarView = commentsView
	return app, finishCh
}

func updateApp(app AppModel, msg tea.Msg) (AppModel, tea.Cmd) {
	updated, cmd := app.Update(msg)
	switch v := updated.(type) {
	case AppModel:
		return v, cmd
	case *AppModel:
		return *v, cmd
	default:
		panic("unexpected model type")
	}
}

func pressAltKey(app AppModel, code rune) AppModel {
	app, _ = updateApp(app, tea.KeyPressMsg{Code: code, Mod: tea.ModAlt})
	return app
}

func pressKeyCmd(app AppModel, code rune) (AppModel, tea.Cmd) {
	return updateApp(app, tea.KeyPressMsg{Code: code})
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func testComment() review.Comment {
	return review.Comment{ID: "c_test01", StartLine: 1, EndLine: 1, Body: "please fix", CreatedAt: review.Now()}
}

func takeEvent(t *testing.T, ch chan FinishEvent) FinishEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	default:
		t.Fatal("expected a finish event")
		return FinishEvent{}
	}
}

func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
