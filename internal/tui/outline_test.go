package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestMarkdownOutlineParsing(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		known        map[int]bool
		want         []outlineHeading
	}{
		{"hierarchy", "intro\n# Root\n\n### Skipped **level**\nbody\n## 日本語 [link](https://example.org)\n#### `code` &amp; é\n# Last ###\n", nil, []outlineHeading{{2, 1, "Root"}, {4, 3, "Skipped level"}, {6, 2, "日本語 link"}, {7, 4, "code & é"}, {8, 1, "Last"}}},
		{"blocks", "```md\n# fenced\n```\n\n    ## indented\n\n<div>\n# html\n</div>\n\n\\# escaped\n#no-space\n####### too-many\n\n## Real\n", nil, []outlineHeading{{15, 2, "Real"}}},
		{"setext-and-containers", "Setext\n======\n\n> ## Quoted\n\n- ### Nested\n\n#\n", nil, []outlineHeading{{1, 1, "Setext"}, {4, 2, "Quoted"}, {6, 3, "Nested"}, {8, 1, "(untitled)"}}},
		{"partial", "# Known\n\n# Missing\n\n## Also known\n", map[int]bool{1: true, 2: true, 5: true, 6: true}, []outlineHeading{{1, 1, "Known"}, {5, 2, "Also known"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseMarkdownSource(strings.Split(tc.source, "\n"), tc.known)
			if !reflect.DeepEqual(parsed.headings, tc.want) {
				t.Fatalf("headings = %#v, want %#v", parsed.headings, tc.want)
			}
		})
	}
}

func outlineTestApp(t *testing.T, source string) AppModel {
	t.Helper()
	app := setupAppWithDoc(t, source)
	app.tab().path = "outline.md"
	app.sidebarView = filesView
	app.tab().isMarkdown = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 30})
	app.rebuildContent()
	app.updateCommentSidebar()
	return app
}

func TestOutlineFollowsSourceAndJumps(t *testing.T) {
	app := outlineTestApp(t, "intro\n# Root\nbody\n### Child\nbody\n## Sibling\nbody\n# Last\n")
	for _, tc := range []struct{ line, active int }{{1, -1}, {2, 0}, {3, 0}, {5, 1}, {6, 2}, {8, 3}} {
		app.tab().cursorLine = tc.line
		app.rebuildContent()
		if app.outline.active != tc.active {
			t.Fatalf("line %d: active = %d, want %d", tc.line, app.outline.active, tc.active)
		}
	}
	app = pressKey(app, 's')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
	app = pressKey(app, 'g')
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 4 || app.outline.active != 1 || !app.outline.focused {
		t.Fatal("outline navigation did not jump to the child heading")
	}
	app = pressKey(app, 'h')
	if app.tab().cursorLine != 2 {
		t.Fatal("left did not jump to the parent")
	}
	app = pressKey(app, 'l')
	if app.tab().cursorLine != 4 {
		t.Fatal("right did not jump to the child")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.focused != contentPane {
		t.Fatal("Enter did not return to the source")
	}
	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+2, top+app.fileTree.viewport.Height()+1+2)
	if app.tab().cursorLine != 6 || !app.outline.focused || app.focused != commentPane {
		t.Fatal("outline click did not jump to the sibling")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
	if app.outline.focused {
		t.Fatal("Ctrl+Up did not return to the file tree")
	}
}

func TestOutlineShortcut(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{
		{Code: 'G', Mod: tea.ModAlt},
		{Code: 'g', Mod: tea.ModAlt | tea.ModShift},
		{Code: 'G', Mod: tea.ModAlt | tea.ModShift},
	} {
		t.Run(msg.String(), func(t *testing.T) {
			app := outlineTestApp(t, "# Root\nbody\n## Child\n")
			app.sidebarView = commentsView
			app.viewMode = true
			app.tab().cursorLine = 2
			app, _ = updateApp(app, msg)
			if app.viewMode || app.sidebarView != filesView || app.focused != commentPane || !app.outline.focused || app.tab().cursorLine != 2 {
				t.Fatalf("%s did not focus the outline without moving the source cursor", msg.String())
			}
			app = pressKey(app, 'j')
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
			if app.focused != contentPane || app.tab().cursorLine != 3 {
				t.Fatal("Enter did not return to the selected source entry")
			}
		})
	}
}

func TestOutlineFooterHints(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		app := outlineTestApp(t, "# Root\nbody\n")
		app.readOnly = readOnly
		app.focused = contentPane
		if footer := ansi.Strip(app.renderFooter()); !strings.Contains(footer, "M-G outline") || strings.Contains(footer, "C-↑/↓") {
			t.Fatalf("source footer: %q", footer)
		}
		app.focused = commentPane
		if footer := ansi.Strip(app.renderFooter()); strings.Contains(footer, "M-G") || !strings.Contains(footer, "C-↑/↓ tree/outline") {
			t.Fatalf("Files footer: %q", footer)
		}
		app.sidebarView = commentsView
		if footer := ansi.Strip(app.renderFooter()); strings.Contains(footer, "M-G") || strings.Contains(footer, "C-↑/↓") {
			t.Fatalf("comments footer: %q", footer)
		}
		app.focused = contentPane
		app.outline.rows = nil
		if footer := ansi.Strip(app.renderFooter()); strings.Contains(footer, "M-G") || strings.Contains(footer, "C-↑/↓") {
			t.Fatalf("footer without outline: %q", footer)
		}
	}
}

func TestOutlineLayoutScrollingAndResize(t *testing.T) {
	var source strings.Builder
	for i := range 40 {
		fmt.Fprintf(&source, "## Heading %02d 日本語 é\nbody\n", i)
	}
	app := outlineTestApp(t, source.String())
	initial := app.outline.viewport.Height()
	left, top, _, bottom := app.commentBounds()
	app = pressMouse(app, left+2, top+app.fileTree.viewport.Height())
	app = moveMouse(app, left+2, top)
	app = releaseMouse(app, left+2, top)
	if app.outline.dragging || app.outline.viewport.Height() <= initial || app.outline.viewport.Height()+1 > app.commentViewport.Height()*2/3 {
		t.Fatal("splitter did not resize within the two-thirds limit")
	}
	if app.fileTree.viewport.Height()+app.outline.viewport.Height()+1 != bottom-top {
		t.Fatal("Files body heights do not add up")
	}
	treeOffset := app.fileTree.viewport.YOffset()
	app, _ = updateApp(app, tea.MouseWheelMsg{X: left + 2, Y: bottom - 1, Button: tea.MouseWheelDown})
	if app.outline.viewport.YOffset() == 0 || app.fileTree.viewport.YOffset() != treeOffset {
		t.Fatal("outline wheel did not scroll independently")
	}
	app.tab().cursorLine = 79
	app.rebuildContent()
	if app.outline.active != 39 || app.outline.viewport.YOffset()+app.outline.viewport.Height() <= 39 {
		t.Fatal("source navigation did not reveal the current heading")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 16}, {Width: 100, Height: 40}} {
		app, _ = updateApp(app, size)
		if app.outline.viewport.Height()+1 > app.commentViewport.Height()*2/3 {
			t.Fatal("resize exceeded the outline height limit")
		}
		for _, line := range strings.Split(app.renderFilesSidebar(), "\n") {
			if ansi.StringWidth(line) > app.commentViewport.Width() {
				t.Fatal("Unicode outline exceeded the pane width")
			}
		}
	}
}

func TestOutlineRefreshAndFallback(t *testing.T) {
	app := outlineTestApp(t, "# Original\nbody\n")
	app.tab().doc = document.FromContent("outline.md", []byte("intro\n## Updated\nbody\n"))
	app.tab().cursorLine = 3
	app.rebuildContent()
	if len(app.outline.rows) != 1 || app.outline.rows[0].label != "Updated" || app.outline.rows[0].line != 2 || app.outline.active != 0 {
		t.Fatal("updated Markdown source did not refresh the outline")
	}
	app.outline.focused = true
	app.tab().path = "outline.go"
	app.tab().isMarkdown = false
	app.rebuildContent()
	if len(app.outline.rows) != 0 || app.outline.viewport.Height() != 0 || app.outline.focused || app.fileTree.viewport.Height() != app.commentViewport.Height() {
		t.Fatal("non-Markdown files did not return the whole sidebar to Files")
	}
	app.tab().path = "outline.md"
	app.tab().isMarkdown = true
	app.tab().chromaLines = nil
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 50, Height: 10})
	if app.commentViewport.Height() < 6 && app.outline.viewport.Height() != 0 {
		t.Fatal("outline crowded the file tree in a small terminal")
	}
}

func TestOutlineDeletedLineUsesNewSideSection(t *testing.T) {
	app := outlineTestApp(t, "# Root\nbody\n## Child\nbody\n")
	app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{2: {{OldLineNum: 30, Content: "removed"}}}
	app.tab().cursorSide, app.tab().cursorLine = "old", 30
	app.rebuildContent()
	if app.outline.active != 0 {
		t.Fatal("deleted line used its old line number to select a new heading")
	}
}
