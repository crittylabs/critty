package tui

import (
	"net/url"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/knu/tcrit/internal/review"
)

func TestCopyReferenceAndYank(t *testing.T) {
	app := setupAppWithDoc(t, "one\ntwo\n")
	app.tab().path = "dir/日本 file\\(x).go"
	app.tab().cursorLine = 2
	editorMeta(&app, 'w')
	want := `@dir/日本\ file\\\(x\).go L2`
	if len(app.killRing.entries) != 1 || app.killRing.entries[0] != want {
		t.Fatalf("ring = %#v, want %q", app.killRing.entries, want)
	}
	app.tab().state.Comments = []review.Comment{{ID: "c_abcdef", Scope: "file", Body: "thread"}}
	app.focused = commentPane
	app.updateCommentSidebar()
	editorMeta(&app, 'w')
	editorMeta(&app, 'w')
	if len(app.killRing.entries) != 3 || app.killRing.entries[0] != "c_abcdef" || app.killRing.entries[1] != "c_abcdef" {
		t.Fatalf("copies should be separate entries: %#v", app.killRing.entries)
	}
	app.modal = replyModal
	app.modalTextarea.Focus()
	editorControl(&app, 'y')
	if app.modalTextarea.Value() != "c_abcdef" {
		t.Fatalf("yank = %q", app.modalTextarea.Value())
	}
	editorMeta(&app, 'y')
	editorMeta(&app, 'y')
	if app.modalTextarea.Value() != want {
		t.Fatalf("rotated yank = %q", app.modalTextarea.Value())
	}
	before := len(app.killRing.entries)
	editorMeta(&app, 'w')
	if len(app.killRing.entries) != before {
		t.Fatal("copy shortcut ran inside editor")
	}
}

func TestCopySourceReferenceWithoutNewSideLine(t *testing.T) {
	for _, old := range []bool{false, true} {
		app := setupAppWithDoc(t, "one\n")
		app.tab().path = "file.go"
		app.tab().cursorLine = 0
		if old {
			app.tab().cursorLine, app.tab().cursorSide = 1, "old"
		}
		editorMeta(&app, 'w')
		if app.killRing.entries[0] != "@file.go" {
			t.Fatalf("invalid new-side line copied: %q", app.killRing.entries[0])
		}
	}
}

func TestCommentReferenceNavigation(t *testing.T) {
	for _, scope := range []string{"line", "file"} {
		t.Run(scope, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			second := setupAppWithDoc(t, "target\n")
			app.tabs = append(app.tabs, second.tabs[0])
			app.multiFile = true
			app.tabs[1].state.Comments = []review.Comment{{ID: "c_abcdef", Scope: scope, StartLine: 1, EndLine: 1, Body: "target", Resolved: true}}
			body := "see `c_abcdef` and c_000000 and xc_abcdef and c_abcdef0"
			rendered := app.linkFileReferences(body)
			if ansi.Strip(rendered) != body || strings.Count(rendered, "critty://comment") != 1 {
				t.Fatalf("unexpected reference rendering: %q", rendered)
			}
			app.hideComments = true
			if !app.navigateCommentReference("critty://comment?id=c_abcdef") {
				t.Fatal("reference did not navigate")
			}
			if app.activeTab != 1 || app.hideComments || app.selectedCommentID() != "c_abcdef" || !app.tab().state.Comments[0].Resolved {
				t.Fatal("reference did not focus the resolved target")
			}
			editorMeta(&app, 'w')
			if app.killRing.entries[0] != "c_abcdef" {
				t.Fatal("focused inline thread did not copy its ID")
			}
		})
	}
}

func TestRenderedCommentReferenceClick(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width, app.height = 100, 35
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{
		{ID: "c_123456", StartLine: 1, EndLine: 1, Body: "source", Replies: []review.Reply{{Body: "see `c_abcdef`"}}},
		{ID: "c_abcdef", StartLine: 15, EndLine: 15, Body: "target", Resolved: true},
	}
	app.tab().cursorLine, app.tab().cursorOnAnnotation = 1, true
	app.rebuildContent()
	app.updateCommentSidebar()
	screen, _ := app.renderReviewScreen()
	canvas := lipgloss.NewCanvas(app.width, app.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	for y := 0; y < app.height; y++ {
		for x := 0; x < app.contentViewport.Width(); x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			u, err := url.Parse(cell.Link.URL)
			if err != nil || u.Host != "comment" {
				continue
			}
			app, cmd := clickMouseCmd(app, x, y)
			if cmd != nil || app.selectedCommentID() != "c_abcdef" {
				t.Fatal("rendered thread reference did not navigate")
			}
			return
		}
	}
	t.Fatal("thread reference was not rendered")
}

func TestAmbiguousCommentReference(t *testing.T) {
	app := setupAppWithDoc(t, "one\n")
	second := setupAppWithDoc(t, "two\n")
	app.tabs = append(app.tabs, second.tabs[0])
	for i := range app.tabs {
		app.tabs[i].state.Comments = []review.Comment{{ID: "c_abcdef", Scope: "file", Body: "same ID"}}
	}
	if app.linkFileReferences("c_abcdef") != "c_abcdef" || app.navigateCommentReference("critty://comment?id=c_abcdef") {
		t.Fatal("ambiguous reference must not select an arbitrary thread")
	}
}

func TestFileReferenceSyntax(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{"file", "file.ext"} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp("file", AppConfig{})
	for _, tt := range []struct {
		body   string
		line   int
		linked bool
	}{
		{"@file", 0, true},
		{"@file.ext L40にある通り、...", 40, true},
		{"@file L40-45", 40, true},
		{"text @file  L999999x", 999999, true},
		{"@file\nL40", 0, true},
		{"@file\tL40", 0, true},
		{"@file\u3000L40", 0, true},
		{"@file。", 0, true},
		{"@file L0", 0, false},
		{"@file L9999999999999999999999999999", 0, false},
		{"name@file.ext", 0, false},
		{"@missing L40", 0, false},
		{"@. L40", 0, false},
	} {
		t.Run(tt.body, func(t *testing.T) {
			rendered := app.linkFileReferences(tt.body)
			if ansi.Strip(rendered) != tt.body {
				t.Fatalf("reference rendering changed text: %q", rendered)
			}
			canvas := lipgloss.NewCanvas(100, 5)
			canvas.Compose(lipgloss.NewLayer(rendered))
			var found bool
			for y := 0; y < 5; y++ {
				for x := 0; x < 100; x++ {
					cell := canvas.CellAt(x, y)
					if cell == nil {
						continue
					}
					location, ok := sourceLink(cell.Link.URL)
					if ok {
						found = true
						if location.line != tt.line {
							t.Fatalf("line = %d", location.line)
						}
					}
					if cell.Content == "に" || cell.Content == "-" {
						if cell.Link.URL != "" {
							t.Fatalf("suffix is linked: %q", cell.Content)
						}
					}
				}
			}
			if found != tt.linked {
				t.Fatalf("linked = %v, want %v", found, tt.linked)
			}
		})
	}
}

func TestRenderedReplyReferenceClick(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("line\n", 20))
	app.width, app.height = 100, 35
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{{
		ID: "c_test", StartLine: 1, EndLine: 1, Author: "Tester", Body: "original",
		Replies: []review.Reply{{ID: "r_test", Author: "Codex", Body: "日本語 " + strings.Repeat("text ", 5) + "@" + app.tab().path + " L15に追加しました。"}},
	}}
	app.tab().cursorLine, app.tab().cursorOnAnnotation = 1, true
	app.rebuildContent()
	app.updateCommentSidebar()
	// The absolute path is intentionally long enough to wrap.
	screen, _ := app.renderReviewScreen()
	canvas := lipgloss.NewCanvas(app.width, app.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	linkedRows := make(map[int]bool)
	var clickX, clickY int
	for y := 0; y < app.height; y++ {
		for x := 0; x < app.contentViewport.Width(); x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			if location, ok := sourceLink(cell.Link.URL); ok && location.line == 15 {
				linkedRows[y] = true
				clickX, clickY = x, y
			}
		}
	}
	if len(linkedRows) < 2 {
		t.Fatalf("wrapped link spans %d rows", len(linkedRows))
	}
	app, cmd := clickMouseCmd(app, clickX, clickY)
	if cmd != nil || app.modal != noModal || app.tab().cursorLine != 15 || app.tab().cursorOnAnnotation {
		t.Fatal("click on wrapped reply reference did not navigate")
	}
	app.modal = helpModal
	_, cmd = updateApp(app, tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	if cmd != nil {
		t.Fatal("background link activated under modal")
	}
}

func TestEscapedFileReferences(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp("file", AppConfig{})
	for _, tt := range []struct{ path, reference string }{
		{"my file.md", `@my\ file.md L40にある通り`},
		{`back\slash.go`, `@back\\slash.go L40-45`},
		{"file(@name).go", `@file\(\@name\).go L40`},
		{"日本.md", `@\日\本.md L40`},
		{"日本.md", `@日本.md L40`},
		{"__init__.py", `@\_\_init\_\_.py L40`},
		{"__init__.py", `@__init__.py L40`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			if err := os.WriteFile(tt.path, []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			rendered := app.linkFileReferences(tt.reference)
			if ansi.Strip(rendered) != tt.reference {
				t.Fatal("display must preserve escapes as written")
			}
			canvas := lipgloss.NewCanvas(100, 1)
			canvas.Compose(lipgloss.NewLayer(rendered))
			cell := canvas.CellAt(0, 0)
			location, ok := sourceLink(cell.Link.URL)
			if !ok || location.path != tt.path || location.line != 40 {
				t.Fatalf("location = %#v, linked = %v", location, ok)
			}
		})
	}
	if err := os.WriteFile("prefix", []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"@prefix\\", "@prefix\\\nnext"} {
		if strings.Contains(app.linkFileReferences(body), "critty://") {
			t.Fatal("incomplete escape linked a shorter filename")
		}
	}
}

func TestEscapeReferencePath(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"dir/x_test.go", "dir/x_test.go"},
		{"__init__.py", `\_\_init\_\_.py`},
		{"src/_private/foo_.txt", `src/\_private/foo\_.txt`},
		{"my _file", `my\ \_file`},
		{"日本_語.md", "日本_語.md"},
		{"日本 語（1）.md", `日本\ 語\（1\）.md`},
		{"a*b`c[d].go", `a\*b\` + "`" + `c\[d\].go`},
		{"全角\u3000空白", `全角\` + "\u3000" + `空白`},
	} {
		if got := escapeReferencePath(tt.path); got != tt.want {
			t.Errorf("escape(%q) = %q, want %q", tt.path, got, tt.want)
		}
		if got := unescapeReferencePath(escapeReferencePath(tt.path)); got != tt.path {
			t.Errorf("round trip of %q = %q", tt.path, got)
		}
	}
}

func TestFileReferenceFollowedByProse(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp("file", AppConfig{})
	for _, path := range []string{"日本.md", "main.go"} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct{ body, path string }{
		{"@日本.mdを見て", "日本.md"},
		{"@main.goを参照。", "main.go"},
		{"@main.go L3の後", "main.go"},
		{"@main.goの L3", "main.go"},
		{"@missing.goを参照", ""},
	} {
		rendered := app.linkFileReferences(tt.body)
		if ansi.Strip(rendered) != tt.body {
			t.Fatalf("rendering changed text: %q", rendered)
		}
		canvas := lipgloss.NewCanvas(40, 1)
		canvas.Compose(lipgloss.NewLayer(rendered))
		var linked string
		for x := 0; x < 40; x++ {
			cell := canvas.CellAt(x, 0)
			if cell == nil || cell.Link.URL == "" {
				continue
			}
			location, ok := sourceLink(cell.Link.URL)
			if !ok || location.path != tt.path {
				t.Fatalf("%q linked %#v", tt.body, location)
			}
			linked += cell.Content
		}
		want := ""
		if tt.path != "" {
			want = "@" + tt.path
			if strings.Contains(tt.body, " L3の") {
				want += " L3"
			}
		}
		if linked != want {
			t.Errorf("%q linked text = %q, want %q", tt.body, linked, want)
		}
	}
}
