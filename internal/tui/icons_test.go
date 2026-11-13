package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/icons"
	"github.com/crittylabs/critty/internal/review"
)

func TestNerdFontFileNavigation(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := newFileTreeTestApp()
		app.icons = icons.Nerd
		app.recalculateLayout()
		app.tabs[0].unread, app.tabs[0].status = true, "modified"
		app.updateCommentSidebar()
		paths := strings.Join(treeRowPaths(app), "\n")
		view := ansi.Strip(app.fileTree.viewport.GetContent())
		for _, mark := range []string{icons.Nerd.Glyph(icons.FolderOpen, ""), icons.Nerd.Glyph(icons.Go, ""), icons.Nerd.Glyph(icons.Modified, ""), "●"} {
			if !strings.Contains(view, mark) {
				t.Fatalf("dark=%t: missing marker %q in %s", dark, mark, view)
			}
		}
		left, top, _, _ := app.commentBounds()
		app = clickMouse(app, left+1, top+5)
		if !app.fileTree.collapsed["b"] || !strings.Contains(ansi.Strip(app.fileTree.viewport.GetContent()), icons.Nerd.Glyph(icons.FolderClosed, "")) {
			t.Fatal("folder icon or mouse fold target did not follow collapse")
		}
		app.toggleTreeDir(5)
		app.tabs[0].unread = true
		app.icons = icons.Plain
		app.recalculateLayout()
		app.updateCommentSidebar()
		if strings.Join(treeRowPaths(app), "\n") != paths {
			t.Fatal("icon selection changed source paths or tree structure")
		}
		plain := ansi.Strip(app.fileTree.viewport.GetContent())
		if !strings.Contains(plain, "▾") || strings.Contains(plain, icons.Nerd.Glyph(icons.Go, "")) || !strings.Contains(plain, "●") {
			t.Fatalf("portable tree or unread marker changed: %s", plain)
		}
	}
}

func TestNerdFontCommentAndSidebarIcons(t *testing.T) {
	app := setupPrivateAppWithDoc(t, "# Section\n\nsource\n")
	app.icons = icons.Nerd
	app.width, app.height = 100, 40
	comment := review.Comment{ID: "thread", StartLine: 3, EndLine: 3, Author: "Reviewer", Body: "**Body**", Replies: []review.Reply{{ID: "reply", Author: "Agent", Body: "Reply body"}}}
	app.tab().state.Comments = []review.Comment{comment}
	app.selectComment(0, app.commentTargets(0)[0])
	app.focused, app.sidebarView = commentPane, commentsView
	app.recalculateLayout()
	app.rebuildContent()
	app.updateCommentSidebar()
	view := ansi.Strip(app.commentViewport.GetContent())
	for _, kind := range []icons.Kind{icons.Comment, icons.Reply, icons.Unchecked} {
		if !strings.Contains(view, icons.Nerd.Glyph(kind, "")) {
			t.Fatalf("comment icon missing: %d in %s", kind, view)
		}
	}
	x := app.contentPaneWidth() + 2
	for i, tab := range app.sidebarTabs() {
		got, ok := app.sidebarTabAt(x)
		if !ok || int(got) != i {
			t.Fatalf("sidebar icon shifted mouse target %d", i)
		}
		x += ansi.StringWidth(tab) + 1
	}
	if !strings.Contains(ansi.Strip(app.renderFilesSidebar()), icons.Nerd.Glyph(icons.Outline, "")) {
		t.Fatal("outline title icon missing")
	}
	if app.tab().state.Comments[0].Body != comment.Body || app.tab().state.Comments[0].StartLine != 3 {
		t.Fatal("comment icons changed source or anchors")
	}
	app.tab().state.Comments[0].Resolved = true
	app.updateCommentSidebar()
	if !strings.Contains(ansi.Strip(app.commentViewport.GetContent()), icons.Nerd.Glyph(icons.Checked, "")) {
		t.Fatal("resolved comment icon missing")
	}
}

func TestFileIconKindsAndStatusCounts(t *testing.T) {
	for _, tc := range []struct {
		path   string
		binary bool
		kind   icons.Kind
	}{
		{"src/main.GO", false, icons.Go},
		{"README.md", false, icons.Markdown},
		{"image.png", true, icons.Image},
		{"data.bin", true, icons.Binary},
		{"settings.toml", false, icons.Config},
		{"unknown.extension", false, icons.File},
	} {
		if got := fileIcon(tc.path, tc.binary); got != tc.kind {
			t.Fatalf("%s: icon kind = %d, want %d", tc.path, got, tc.kind)
		}
	}
	for _, tc := range []struct {
		status string
		kind   icons.Kind
	}{
		{"added", icons.Added}, {"untracked", icons.Added}, {"modified", icons.Modified}, {"deleted", icons.Removed}, {"renamed", icons.Renamed},
	} {
		tab := FileTab{status: tc.status, changedLines: map[int]bool{1: true}}
		label := ansi.Strip(tab.changeSummary(false, icons.Nerd))
		if !strings.Contains(label, icons.Nerd.Glyph(tc.kind, "")) || !strings.Contains(label, "+1") {
			t.Fatalf("%s: status or line counts missing: %s", tc.status, label)
		}
	}
}
