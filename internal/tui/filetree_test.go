package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestReviewTabsFollowExpandedTreeOrder(t *testing.T) {
	files := []gitpkg.FileChange{
		{Path: "root.go"}, {Path: "a.go"}, {Path: "a/y.go"},
		{Path: "b/x.go"}, {Path: "a/z/sub.go"}, {Path: "a/sub/z.go"},
		{Path: "a/a.go"}, {Path: ".config"}, {Path: "attachments/local.md"},
	}
	docs := []review.AttachedDocument{
		{ID: "z", Source: "/docs/z.md", Name: "z.md"},
		{ID: "a", Source: "/docs/a.md", Name: "a.md"},
		{ID: "removed", Source: "/docs/removed.md", Name: "removed.md", Removed: true},
	}
	sess := &review.Session{CJ: review.NewCritJSON()}
	sess.SetFileComments(docs[2].Path(), "", []review.Comment{{ID: "removed-comment", Body: "keep"}})
	sess.SetFileComments("c/outside.go", "", []review.Comment{{ID: "outside-comment", Body: "keep"}})
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Patch: &gitpkg.Patch{}, Session: sess, Documents: docs})
	want := []string{
		docs[0].Path(), docs[1].Path(), docs[2].Path(),
		"a/sub/z.go", "a/z/sub.go", "a/a.go", "a/y.go",
		"attachments/local.md", "b/x.go", "c/outside.go", ".config", "a.go", "root.go",
	}
	if len(app.tabs) != len(want) {
		t.Fatalf("tab count = %d, want %d", len(app.tabs), len(want))
	}
	for i, path := range want {
		if app.tabs[i].path != path {
			t.Fatalf("tab %d = %q, want %q", i, app.tabs[i].path, path)
		}
	}
	app.syncFileTree()
	index := 0
	for _, row := range app.fileTree.rows {
		if row.isDir {
			continue
		}
		if row.tabIndex != index || row.path != want[index] {
			t.Fatalf("tree leaf %d = %+v, want %q", index, row, want[index])
		}
		index++
	}
	if !app.tabs[9].outsideChanges || app.tabs[2].attachment == nil || !app.tabs[2].attachment.Removed {
		t.Fatal("sorting lost restored tab metadata")
	}
	app.toggleTreeDir(0)
	for i, path := range want {
		if app.tabs[i].path != path {
			t.Fatal("folding reordered tabs")
		}
	}
}

func newFileTreeTestApp() AppModel {
	app := NewApp("root.go", AppConfig{Project: "test-project"})
	app.multiFile = true
	app.tabs = nil
	for _, path := range []string{"a/sub/z.go", "a/y.go", "b/x.go", "root.go"} {
		app.tabs = append(app.tabs, FileTab{
			path: path, cursorLine: 1,
			doc:   &document.Document{Path: path, Content: "one", Lines: []string{"one"}},
			state: &fileReview{},
		})
	}
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.rebuildContent()
	app.updateCommentSidebar()
	return app
}

func treeRowPaths(app AppModel) []string {
	paths := make([]string, len(app.fileTree.rows))
	for i, row := range app.fileTree.rows {
		paths[i] = row.path
	}
	return paths
}

func TestFileTreeRowsListDirectoriesFirstAndFoldChildren(t *testing.T) {
	app := newFileTreeTestApp()
	want := " a a/sub a/sub/z.go a/y.go b b/x.go root.go"
	if got := strings.Join(treeRowPaths(app), " "); got != want {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	for i, depth := range []int{0, 1, 2, 3, 2, 1, 2, 1} {
		if app.fileTree.rows[i].depth != depth {
			t.Fatalf("row %d depth = %d, want %d", i, app.fileTree.rows[i].depth, depth)
		}
	}

	app.toggleTreeDir(1)
	if got := strings.Join(treeRowPaths(app), " "); got != " a b b/x.go root.go" {
		t.Fatalf("folded rows = %q", got)
	}
}

func TestFileTreeFollowsActiveTabAndUnfoldsItsDirectories(t *testing.T) {
	app := newFileTreeTestApp()
	if app.sidebarView != filesView {
		t.Fatalf("default view = %v, want the file tree", app.sidebarView)
	}
	app = pressKey(app, 's')
	if app.focused != commentPane {
		t.Fatalf("s: focus = %v", app.focused)
	}
	if row := app.fileTree.rows[app.fileTree.cursor]; row.path != "a/sub/z.go" {
		t.Fatalf("cursor on %q, want the active tab", row.path)
	}

	app.toggleTreeDir(1) // fold a/
	app.activeTab = 3    // root.go, as any tab switch elsewhere would do
	app.updateCommentSidebar()
	if row := app.fileTree.rows[app.fileTree.cursor]; row.path != "root.go" {
		t.Fatalf("cursor on %q after switching tabs, want root.go", row.path)
	}

	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if app.activeTab != 2 {
		t.Fatalf("shift+tab: active tab = %d", app.activeTab)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if app.activeTab != 0 {
		t.Fatalf("shift+tab x3: active tab = %d", app.activeTab)
	}
	if app.fileTree.collapsed["a"] || app.fileTree.collapsed["a/sub"] {
		t.Fatal("switching to a file inside a folded directory did not unfold it")
	}
	if row := app.fileTree.rows[app.fileTree.cursor]; row.path != "a/sub/z.go" {
		t.Fatalf("cursor on %q, want a/sub/z.go", row.path)
	}
}

func TestFileTreeCursorSelectsFilesAndKeysFoldDirectories(t *testing.T) {
	app := newFileTreeTestApp()
	app = pressKey(app, 's')
	app = pressKey(app, 'G')
	if app.activeTab != 3 || app.fileTree.rows[app.fileTree.cursor].path != "root.go" {
		t.Fatalf("G: tab = %d, cursor = %q", app.activeTab, app.fileTree.rows[app.fileTree.cursor].path)
	}
	app = pressKey(app, 'k')
	if app.activeTab != 2 {
		t.Fatalf("k onto b/x.go: tab = %d", app.activeTab)
	}
	app = pressKey(app, 'h') // to parent b/
	if row := app.fileTree.rows[app.fileTree.cursor]; row.path != "b" || app.activeTab != 2 {
		t.Fatalf("h: cursor = %q, tab = %d", row.path, app.activeTab)
	}
	app = pressKey(app, 'h') // fold b/
	if !app.fileTree.collapsed["b"] {
		t.Fatal("h on an open directory did not fold it")
	}
	app = pressKey(app, 'l') // unfold b/
	if app.fileTree.collapsed["b"] {
		t.Fatal("l on a folded directory did not unfold it")
	}
	app = pressKey(app, tea.KeyEnter) // fold again
	if !app.fileTree.collapsed["b"] {
		t.Fatal("enter on a directory did not toggle it")
	}
	app = pressKey(app, 'G')
	app = pressKey(app, tea.KeyEnter)
	if app.focused != contentPane || app.activeTab != 3 {
		t.Fatalf("enter on a file: focus = %v, tab = %d", app.focused, app.activeTab)
	}
}

func TestFileTreeRendersOneColumnIndentAndActiveFile(t *testing.T) {
	app := newFileTreeTestApp()
	app = pressKey(app, 's')
	lines := strings.Split(ansi.Strip(app.fileTree.viewport.View()), "\n")
	want := []string{
		"  ▾ test-project/",
		"   ▾ a/",
		"    ▾ sub/",
		">      z.go",
		"      y.go",
		"   ▾ b/",
		"      x.go",
		"     root.go",
	}
	for i, line := range want {
		if strings.TrimRight(lines[i], " ") != line {
			t.Fatalf("row %d = %q, want %q", i, lines[i], line)
		}
	}
	active := strings.Split(app.fileTree.viewport.View(), "\n")[3]
	if !strings.Contains(active, "\x1b[1") {
		t.Fatalf("active file row = %q, want bold", active)
	}
}

func TestFileTreeMouse(t *testing.T) {
	app := newFileTreeTestApp()
	left, top, _, _ := app.commentBounds()

	app = clickMouse(app, left+1, top+5) // b/
	if !app.fileTree.collapsed["b"] || app.focused != commentPane {
		t.Fatalf("clicking a directory: folded = %t, focus = %v", app.fileTree.collapsed["b"], app.focused)
	}
	app = clickMouse(app, left+1, top+6) // root.go, now right below the folded b/
	if app.activeTab != 3 {
		t.Fatalf("clicking a file: tab = %d, want root.go", app.activeTab)
	}

	app = pressKey(app, 't')
	if app.sidebarView != commentsView || app.focused != commentPane {
		t.Fatalf("t: view = %v, focus = %v; want comments focused", app.sidebarView, app.focused)
	}
	app = pressKey(app, 't')
	if app.sidebarView != filesView {
		t.Fatalf("second t: view = %v, want the file tree", app.sidebarView)
	}
}

func TestFileTreeScrollsTruncatedCursorRow(t *testing.T) {
	app := newFileTreeTestApp()
	long := "b/" + strings.Repeat("directory-name-", 4) + "file.go"
	app.tabs = append(app.tabs, FileTab{path: long, cursorLine: 1,
		doc: &document.Document{Path: long, Content: "one", Lines: []string{"one"}}, state: &fileReview{}})
	app.activeTab = len(app.tabs) - 1
	app.updateCommentSidebar()
	app = pressKey(app, 's')
	row := func() string {
		return ansi.Strip(strings.Split(app.fileTree.viewport.View(), "\n")[app.fileTree.cursor])
	}
	if !strings.HasSuffix(strings.TrimRight(row(), " "), "…") {
		t.Fatalf("long row = %q, want truncated", row())
	}
	app = pressKey(app, tea.KeyRight)
	if app.fileTree.hscroll == 0 || !strings.HasPrefix(row(), "> …") {
		t.Fatalf("right: scroll = %d, row = %q", app.fileTree.hscroll, row())
	}
	for range 10 {
		app = pressKey(app, tea.KeyRight)
	}
	if !strings.HasSuffix(strings.TrimRight(row(), " "), "file.go") {
		t.Fatalf("scrolled row = %q, want its end visible", row())
	}
	cursor := app.fileTree.cursor
	app = pressKey(app, 'h')
	if app.fileTree.cursor != cursor || app.fileTree.hscroll >= app.maxTreeScroll() {
		t.Fatalf("h while scrolled moved the cursor to %d (scroll %d)", app.fileTree.cursor, app.fileTree.hscroll)
	}
	for app.fileTree.hscroll > 0 {
		app = pressKey(app, 'h')
	}
	if app.fileTree.hscroll != 0 {
		t.Fatalf("scroll = %d after scrolling back", app.fileTree.hscroll)
	}
	app = pressKey(app, 'h') // now climbs to b/
	if got := app.fileTree.rows[app.fileTree.cursor].path; got != "b" {
		t.Fatalf("h at the left edge moved to %q, want b/", got)
	}
	app = pressKey(app, 'h') // folds b/
	app = pressKey(app, 'l') // unfolds it rather than scrolling
	if app.fileTree.collapsed["b"] || app.fileTree.hscroll != 0 {
		t.Fatalf("l on a folded directory: folded = %t, scroll = %d", app.fileTree.collapsed["b"], app.fileTree.hscroll)
	}
}

func TestFileTreeRootAndCascade(t *testing.T) {
	app := newFileTreeTestApp()
	app.tabs = app.tabs[:1]
	app.tabs[0].path = "a/sub/deep/z.go"
	app.fileTree = newFileTree()
	app.updateCommentSidebar()
	if root := app.fileTree.rows[0]; root.name != "test-project" || root.collapsed {
		t.Fatalf("root = %+v", root)
	}
	app.toggleTreeDir(0)
	if len(app.fileTree.rows) != 1 {
		t.Fatalf("folded root leaves %d rows", len(app.fileTree.rows))
	}
	for _, path := range []string{"", "a", "a/sub", "a/sub/deep"} {
		if !app.fileTree.collapsed[path] {
			t.Fatalf("%q was not folded", path)
		}
	}
	app.toggleTreeDir(0)
	if len(app.fileTree.rows) != 5 || len(app.fileTree.collapsed) != 0 {
		t.Fatalf("cascade did not reopen the chain: %+v", app.fileTree)
	}
	app.toggleTreeDir(0)
	app.fileTree.syncedTab = -1
	app.updateCommentSidebar()
	if len(app.fileTree.rows) != 5 {
		t.Fatal("following the active file did not reopen the root")
	}
}
