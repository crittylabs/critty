package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/icons"
)

func TestNerdFontFileNavigation(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := newFileTreeTestApp()
		app.icons = icons.Nerd
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
