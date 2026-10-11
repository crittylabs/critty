package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRankFileMatches(t *testing.T) {
	paths := []string{"docs/app.md", "internal/tui/app.go", "internal/tui/app_test.go", "cmd/critty/main.go", "internal/tui/thread.go"}
	order := func(query string) string {
		var names []string
		for _, match := range rankFileMatches(paths, query) {
			names = append(names, paths[match.index])
		}
		return strings.Join(names, ",")
	}
	if got := order(""); got != strings.Join(paths, ",") {
		t.Fatalf("empty query order = %q", got)
	}
	if got := order("app"); got != "docs/app.md,internal/tui/app.go,internal/tui/app_test.go" {
		t.Fatalf("basename matches = %q", got)
	}
	if got := order("tuiapp"); got != "internal/tui/app.go,internal/tui/app_test.go" {
		t.Fatalf("path-only matches = %q", got)
	}
	if got := order("tui test"); got != "internal/tui/app_test.go" {
		t.Fatalf("multi-word matches = %q", got)
	}
	if got := order("app  md"); got != "docs/app.md" {
		t.Fatalf("multi-word basename match = %q", got)
	}
	if got := order("app zzz"); got != "" {
		t.Fatalf("a word without a match must exclude the path: %q", got)
	}
	if got := order("zzz"); got != "" {
		t.Fatalf("no match = %q", got)
	}
	if positions := rankFileMatches(paths, "docs md")[0].positions; strings.Join(intStrings(positions), ",") != "0,1,2,3,9,0" {
		t.Fatalf("multi-word positions = %v", positions)
	}
	matches := rankFileMatches(paths, "thr")
	if len(matches) != 1 || matches[0].index != 4 {
		t.Fatalf("thread match = %+v", matches)
	}
	if want := []int{len("internal/tui/"), len("internal/tui/") + 1, len("internal/tui/") + 2}; strings.Join(intStrings(matches[0].positions), ",") != strings.Join(intStrings(want), ",") {
		t.Fatalf("positions = %v, want %v", matches[0].positions, want)
	}
}

func intStrings(values []int) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(rune('0' + v%10))
	}
	return out
}

func newFileSelectApp() AppModel {
	app := newCommentNavigationTestApp()
	app.tabs = append(app.tabs, FileTab{path: "lib/util_test.go", cursorLine: 1, state: &fileReview{}})
	app.activeTab = 1
	app.width, app.height = 80, 30
	return app
}

func TestFileSelectFlow(t *testing.T) {
	app := newFileSelectApp()
	if app = pressKey(app, '/'); app.modal != noModal {
		t.Fatal("/ still opens a search")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	if app.modal != fileSelectModal || app.fileSelect.selected != 1 {
		t.Fatalf("modal = %v, selected = %d; want current tab preselected", app.modal, app.fileSelect.selected)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.modal != noModal || app.activeTab != 1 {
		t.Fatalf("esc: modal = %v, tab = %d", app.modal, app.activeTab)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	for _, r := range "utl" {
		app, _ = updateApp(app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := app.fileSelect.input.Value(); got != "utl" || len(app.fileSelect.matches) != 1 || app.fileSelect.selected != 0 {
		t.Fatalf("query = %q, matches = %+v", got, app.fileSelect.matches)
	}
	rendered := ansi.Strip(app.View().Content)
	if !strings.Contains(rendered, "Open file") || !strings.Contains(rendered, "lib/util_test.go") {
		t.Fatalf("modal not rendered: %q", rendered)
	}
	filtered := modalFrameRows(app, rendered)
	if height := filtered[1] - filtered[0] + 1; height != len(app.tabs)+10 {
		t.Fatalf("dialog height = %d for %d tabs", height, len(app.tabs))
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if unfiltered := modalFrameRows(app, ansi.Strip(app.View().Content)); unfiltered != filtered {
		t.Fatalf("dialog frame moved from %v to %v while typing", filtered, unfiltered)
	}
	for _, r := range "utl" {
		app, _ = updateApp(app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.modal != noModal || app.tab().path != "lib/util_test.go" {
		t.Fatalf("enter: modal = %v, path = %q", app.modal, app.tab().path)
	}

	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'z', Text: "z"})
	if len(app.fileSelect.matches) != 0 || !strings.Contains(ansi.Strip(app.View().Content), "No matching files") {
		t.Fatalf("no-match state: %+v", app.fileSelect.matches)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.modal != noModal || app.tab().path != "lib/util_test.go" {
		t.Fatal("enter without matches changed the tab or kept the modal")
	}

	app.focused = commentPane
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	start := app.fileSelect.selected
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyUp})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyDown})
	if start != len(app.tabs)-1 || app.fileSelect.selected != (start+1)%len(app.tabs) {
		t.Fatalf("selected = %d after moving from %d", app.fileSelect.selected, start)
	}
	want := app.tabs[app.fileSelect.matches[app.fileSelect.selected].index].path
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.tab().path != want || app.focused != commentPane {
		t.Fatalf("path = %q, want %q; focused = %v", app.tab().path, want, app.focused)
	}
}

func TestFileSelectMouse(t *testing.T) {
	app := newFileSelectApp()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	rows := strings.Split(ansi.Strip(app.View().Content), "\n")
	target := app.tabs[0].path
	titleRow := renderedLineY(t, app, "Open file")
	for y, row := range rows {
		if x := strings.Index(row, target); y > titleRow && x >= 0 {
			app = clickMouse(app, x+1, y)
			if app.modal != noModal || app.tab().path != target {
				t.Fatalf("click: modal = %v, path = %q", app.modal, app.tab().path)
			}
			return
		}
	}
	t.Fatalf("row for %q not rendered: %q", target, rows)
}

func TestFileSelectCloseButton(t *testing.T) {
	app := newFileSelectApp()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
	titleRow := renderedLineY(t, app, "Open file")
	for _, region := range app.modalMouseRegions() {
		if !region.action.close {
			continue
		}
		if region.rect.top != titleRow {
			t.Fatalf("close button row = %d, title row = %d", region.rect.top, titleRow)
		}
		app = clickMouse(app, region.rect.left, region.rect.top)
		if app.modal != noModal || app.activeTab != 1 {
			t.Fatalf("x: modal = %v, tab = %d", app.modal, app.activeTab)
		}
		return
	}
	t.Fatal("file select dialog has no close button")
}

func TestRenderFileRowHighlightsMatches(t *testing.T) {
	row := renderFileRow("src/app.go", []int{4, 5}, "(+1)", false)
	if ansi.Strip(row) != "src/app.go (+1)" {
		t.Fatalf("row text = %q", ansi.Strip(row))
	}
	if !strings.Contains(row, "\x1b[1;") && !strings.Contains(row, "\x1b[1m") {
		t.Fatalf("matched runes are not bold: %q", row)
	}
	selected := renderFileRow("日本/語.md", []int{len("日本/")}, "", true)
	if !strings.Contains(selected, "\x1b[7") && !strings.Contains(selected, ";7m") {
		t.Fatalf("selected row is not reversed: %q", selected)
	}
	if start, end := fileSelectWindow(30, 25, 10); start != 20 || end != 30 {
		t.Fatalf("window = %d-%d", start, end)
	}
	if start, end := fileSelectWindow(5, 4, 10); start != 0 || end != 5 {
		t.Fatalf("short window = %d-%d", start, end)
	}
}

// modalFrameRows returns the first and last rows of the dialog frame,
// identified by full-width border rows of the dialog's width so the pane
// frames behind the dialog do not match.
func modalFrameRows(app AppModel, rendered string) [2]int {
	inner := strings.Repeat("─", app.modalWidth()-2)
	rows := [2]int{-1, -1}
	for y, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "╭"+inner+"╮") {
			rows[0] = y
		}
		if strings.Contains(line, "╰"+inner+"╯") {
			rows[1] = y
		}
	}
	return rows
}
