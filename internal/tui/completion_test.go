package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFindCompletionToken(t *testing.T) {
	for _, tt := range []struct {
		line       string
		ok         bool
		start      int
		queryStart int
		dir, query string
	}{
		{"@a", true, 0, 1, "", "a"},
		{"@", false, 0, 0, "", ""},
		{"x@a", false, 0, 0, "", ""},
		{"see @src/ap", true, 4, 9, "src/", "ap"},
		{`@my\ fi`, true, 0, 1, "", "my fi"},
		{"@a b", false, 0, 0, "", ""},
		{"@a b @c", true, 5, 6, "", "c"},
		{"@dir/", true, 0, 5, "dir/", ""},
		{"@日本/ファ", true, 0, 4, "日本/", "ファ"},
		{`\@a`, false, 0, 0, "", ""},
	} {
		line := []rune(tt.line)
		token, ok := findCompletionToken(line, len(line))
		if ok != tt.ok {
			t.Fatalf("%q: ok = %v, want %v", tt.line, ok, tt.ok)
		}
		if !ok {
			continue
		}
		if token.start != tt.start || token.queryStart != tt.queryStart || token.dir != tt.dir || token.query != tt.query {
			t.Errorf("%q: token = %+v", tt.line, token)
		}
	}
	if _, ok := findCompletionToken([]rune("@abc"), 1); ok {
		t.Error("cursor right after @ must not complete")
	}
	if token, ok := findCompletionToken([]rune("@abc def"), 3); !ok || token.query != "ab" {
		t.Errorf("mid-token cursor: %+v, %v", token, ok)
	}
}

func newCompletionApp(t *testing.T) AppModel {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"main.go", "main_test.go", "src/app.go", "src/.hidden", "src/my file.md", "src/日本.md"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	app := newKillRingApp("")
	app.modalTextarea.SetWidth(40)
	return app
}

func typeText(m *AppModel, text string) {
	for _, r := range text {
		editorMessage(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func completionLabels(app AppModel) []string {
	labels := make([]string, 0, len(app.completion.items))
	for _, item := range app.completion.items {
		labels = append(labels, item.label())
	}
	return labels
}

func TestFileCompletionFlow(t *testing.T) {
	app := newCompletionApp(t)
	typeText(&app, "@")
	if app.completion.active() {
		t.Fatal("@ alone opened the menu")
	}
	typeText(&app, "ma")
	if got := strings.Join(completionLabels(app), ","); got != "main.go,main_test.go" {
		t.Fatalf("candidates = %q", got)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := app.modalTextarea.Value(); got != "@main.go " || app.completion.active() || app.modalFocus != 0 {
		t.Fatalf("after tab: value = %q, active = %v, focus = %d", got, app.completion.active(), app.modalFocus)
	}
	typeText(&app, "@s")
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := app.modalTextarea.Value(); got != "@main.go @src/" {
		t.Fatalf("directory completion = %q", got)
	}
	if got := strings.Join(completionLabels(app), ","); got != "app.go,my file.md,日本.md" {
		t.Fatalf("directory listing = %q", got)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyDown})
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyDown})
	if app.completion.selected != 1 {
		t.Fatalf("selected = %d", app.completion.selected)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := app.modalTextarea.Value(); got != `@main.go @src/my\ file.md ` {
		t.Fatalf("menu completion = %q", got)
	}
	typeText(&app, "@src/.h")
	if got := strings.Join(completionLabels(app), ","); got != ".hidden" {
		t.Fatalf("dot query = %q", got)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyUp})
	if app.completion.selected != 0 {
		t.Fatalf("up from typing selected %d", app.completion.selected)
	}
}

func TestFileCompletionDismissAndFallthrough(t *testing.T) {
	app := newCompletionApp(t)
	typeText(&app, "@ma")
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.completion.active() || app.modal != commentModal || app.modalTextarea.Value() != "@ma" {
		t.Fatalf("esc: active = %v, modal = %v, value = %q", app.completion.active(), app.modal, app.modalTextarea.Value())
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyDown})
	if app.completion.active() {
		t.Fatal("dismissed menu reopened without typing")
	}
	typeText(&app, "i")
	if got := strings.Join(completionLabels(app), ","); got != "main.go,main_test.go" {
		t.Fatalf("typing after esc: %q", got)
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := app.modalTextarea.Value(); got != "@mai\n" || app.completion.active() {
		t.Fatalf("enter without selection: %q, active = %v", got, app.completion.active())
	}
	typeText(&app, "@zz")
	if app.completion.active() {
		t.Fatal("no match kept the menu")
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if app.modalFocus != 1 || app.modalTextarea.Value() != "@mai\n@zz" {
		t.Fatalf("tab without candidates: focus = %d, value = %q", app.modalFocus, app.modalTextarea.Value())
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	typeText(&app, " x@ma")
	if app.completion.active() {
		t.Fatal("@ inside a word opened the menu")
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.modal != discardChangesModal {
		t.Fatalf("esc without menu: modal = %v", app.modal)
	}
}

func TestFileCompletionMidLineReplacesQueryOnly(t *testing.T) {
	app := newCompletionApp(t)
	typeText(&app, "see @ma later")
	for range len(" later") {
		editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	if !app.completion.active() {
		t.Fatal("cursor inside token did not open the menu")
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := app.modalTextarea.Value(); got != "see @main.go  later" || app.modalTextarea.Column() != len("see @main.go ") {
		t.Fatalf("mid-line completion = %q, column = %d", got, app.modalTextarea.Column())
	}
	editorMessage(&app, tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	if got := app.modalTextarea.Value(); got != "see @main.go  later" {
		t.Fatalf("yank-pop after completion changed text: %q", got)
	}
}

func TestRenderCompletionMenu(t *testing.T) {
	items := []completionItem{{name: "first"}, {name: "second", dir: true}, {name: "third"}}
	menu := renderCompletionMenu(items, -1, 0, 80)
	lines := strings.Split(menu, "\n")
	if len(lines) != 5 || ansi.Strip(lines[2]) != "│ second/ │" {
		t.Fatalf("menu = %q", menu)
	}
	if !strings.Contains(lines[1], "\x1b[1;2m") && !strings.Contains(lines[1], "\x1b[2;1m") {
		t.Errorf("first row is not faint bold: %q", lines[1])
	}
	if strings.Contains(lines[2], "\x1b[1") {
		t.Errorf("second row is bold: %q", lines[2])
	}
	focused := strings.Split(renderCompletionMenu(items, 1, 0, 80), "\n")
	if strings.Count(focused[2], "\x1b[2m") != 2 || !strings.Contains(ansi.Strip(focused[2]), "second/") {
		t.Errorf("focused row should only fade its borders: %q", focused[2])
	}
	if !strings.Contains(focused[1], "\x1b[1;2m") && !strings.Contains(focused[1], "\x1b[2;1m") {
		t.Errorf("first row loses bold while another row is focused: %q", focused[1])
	}
	narrow := strings.Split(renderCompletionMenu(items, -1, 0, 9), "\n")
	if ansi.Strip(narrow[2]) != "│ seco… │" {
		t.Errorf("narrow menu = %q", narrow[2])
	}
	many := make([]completionItem, 15)
	for i := range many {
		many[i] = completionItem{name: strings.Repeat("x", i+1)}
	}
	if got := len(strings.Split(renderCompletionMenu(many, 12, 3, 80), "\n")); got != completionMenuRows+2 {
		t.Errorf("visible rows = %d", got-2)
	}
}

func TestCompletionMenuPlacement(t *testing.T) {
	app := setupAppWithDoc(t, "one\ntwo\n")
	root := filepath.Dir(app.tabs[0].path)
	for _, name := range []string{"alpha.go", "alpine.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app.session.Meta.CWD = root
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 30})
	app.modal = commentModal
	app.modalTextarea.Focus()
	typeText(&app, "note @al")
	if got := strings.Join(completionLabels(app), ","); got != "alpha.go,alpine.go" {
		t.Fatalf("candidates = %q", got)
	}
	view := app.View()
	rows := strings.Split(ansi.Strip(view.Content), "\n")
	menuRow := -1
	for y, row := range rows {
		if strings.Contains(row, "│ alpha.go") {
			menuRow = y
		}
	}
	if menuRow < 0 || view.Cursor == nil || menuRow != view.Cursor.Y+2 {
		t.Fatalf("menu row = %d, cursor = %+v", menuRow, view.Cursor)
	}
	atX := strings.Index(rows[view.Cursor.Y], "note @al") + len("note ")
	if menuX := strings.Index(rows[menuRow-1], "╭"); menuX != atX {
		t.Fatalf("menu x = %d, want %d (under @)", menuX, atX)
	}

	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 12})
	rows = strings.Split(ansi.Strip(app.View().Content), "\n")
	if !strings.Contains(strings.Join(rows, "\n"), "│ alpha.go") {
		t.Fatal("menu missing on a short screen with room above")
	}
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 4})
	if strings.Contains(ansi.Strip(app.View().Content), "alpha.go │") {
		t.Fatal("menu drawn without room")
	}
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := app.modalTextarea.Value(); got != "note @alpha.go " {
		t.Fatalf("tab without menu room = %q", got)
	}
}
