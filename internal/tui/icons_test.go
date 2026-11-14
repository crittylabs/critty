package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/icons"
	"github.com/crittylabs/critty/internal/review"
)

func TestNerdFontHeaderControls(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		for _, width := range []int{45, 80, 120} {
			t.Run(fmt.Sprintf("dark=%t/width=%d", dark, width), func(t *testing.T) {
				initAdaptiveStyles(dark)
				app := patchApp(t, whitespacePatch)
				app.icons = icons.Nerd
				app.width, app.patch = width, nil
				app.project, app.branch = "sample", "feature/viewer"
				app.reload.watch, app.images.enabled = true, true
				app.recalculateLayout()
				app.rebuildContent()
				app.updateCommentSidebar()
				if width == 120 {
					bar := ansi.Strip(app.renderReviewBar())
					for _, label := range []string{icons.Nerd.Label(icons.Project, "sample"), icons.Nerd.Label(icons.Branch, "feature/viewer")} {
						if !strings.Contains(bar, label) {
							t.Fatalf("header context missing: %q in %s", label, bar)
						}
					}
				}
				_, layout := app.renderReviewScreen()
				for _, control := range []struct {
					icon    string
					button  string
					rect    mouseRect
					visible bool
				}{
					{icons.Nerd.Glyph(icons.Watch, ""), app.watchIndicator(), layout.watchIndicator, layout.hasWatchIndicator},
					{"", app.splitButton(), layout.splitButton, layout.hasSplitButton},
					{"", app.whitespaceButton(), layout.whitespaceButton, layout.hasWhitespaceButton},
					{"", app.imagesButton(), layout.imagesButton, layout.hasImagesButton},
				} {
					if !control.visible || control.rect.left < 0 || control.rect.right > width || control.rect.bottom > app.height {
						t.Fatalf("control %q unreachable: %+v", control.button, control.rect)
					}
					if control.icon != "" && !strings.Contains(control.button, control.icon) {
						t.Fatalf("control %q icon missing", control.button)
					}
				}
				app = clickMouse(app, layout.splitButton.left, layout.splitButton.top)
				if !app.split {
					t.Fatal("split button did not toggle split")
				}
				_, after := app.renderReviewScreen()
				if after.splitButton != layout.splitButton {
					t.Fatal("split state shifted header targets")
				}
				app = clickMouse(app, after.whitespaceButton.left, after.whitespaceButton.top)
				if !app.ignoreWhitespace {
					t.Fatal("whitespace button did not toggle filtering")
				}
				_, after = app.renderReviewScreen()
				app = clickMouse(app, after.watchIndicator.left, after.watchIndicator.top)
				if app.reload.watch {
					t.Fatal("watch icon did not turn off watching")
				}
				_, after = app.renderReviewScreen()
				app = clickMouse(app, after.imagesButton.left, after.imagesButton.top)
				if !app.images.hidden {
					t.Fatal("image button did not hide previews")
				}
				for _, line := range strings.Split(app.renderReviewBar(), "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatal("header exceeds terminal width")
					}
				}
			})
		}
	}
}

func TestOutlineSymbolKinds(t *testing.T) {
	for _, tc := range []struct {
		language, source string
		want             []icons.Kind
		names            []string
	}{
		{"go", "package p\ntype Server struct{}\ntype Reader interface{}\ntype Name = string\nfunc New() {}\nfunc (*Server) Run() {}\n", []icons.Kind{icons.Struct, icons.Interface, icons.Class, icons.Function, icons.Method}, []string{"Server", "Reader", "Name", "New", "(*Server) Run"}},
		{"cpp", "namespace example {\nclass Server {\n void run() {}\n};\nvoid finish() {}\n}\n", []icons.Kind{icons.Namespace, icons.Class, icons.Method, icons.Function}, []string{"example", "Server", "run()", "finish()"}},
		{"python", "class Server:\n def run(self):\n  def nested():\n   pass\ndef finish():\n pass\n", []icons.Kind{icons.Class, icons.Method, icons.Function, icons.Function}, []string{"Server", "run", "nested", "finish"}},
		{"typescript", "interface Server {\n run(): void;\n}\nconst start = () => {};\nconst api = {\n stop() {}\n};\n", []icons.Kind{icons.Interface, icons.Method, icons.Function, icons.Class, icons.Method}, []string{"Server", "run()", "start", "api", "stop()"}},
		{"rust", "mod example {\n struct Server;\n impl Server {\n  fn run(&self) {}\n }\n fn finish() {}\n}\n", []icons.Kind{icons.Namespace, icons.Struct, icons.Class, icons.Method, icons.Function}, []string{"example", "Server", "Server", "run", "finish"}},
		{"zig", "const Kind = enum { ready };\nconst Server = struct {\n fn run() void {}\n};\n", []icons.Kind{icons.Enum, icons.Struct, icons.Method}, []string{"Kind", "Server", "run"}},
		{"emacs-lisp", "(use-package example\n :config\n (defun run ()\n  (cl-defun nested () t)))\n(defconst ready t)\n(defvar value nil)\n", []icons.Kind{icons.Namespace, icons.Function, icons.Function, icons.Constant, icons.Variable}, []string{"example", "run", "nested", "ready", "value"}},
	} {
		t.Run(tc.language, func(t *testing.T) {
			rows := parseCodeOutline(tc.language, tc.source, 0)
			if tc.language == "go" {
				rows = parseGoOutline(tc.source, 0)
			}
			var got []icons.Kind
			for i, row := range rows {
				got = append(got, row.kind)
				if i >= len(tc.names) {
					t.Fatalf("unexpected row: %+v", row)
				}
				if label := ansi.Strip(row.displayLabel(icons.Nerd)); label != icons.Nerd.Label(row.kind, tc.names[i]) {
					t.Fatalf("Nerd Font label = %q, want name %q", label, tc.names[i])
				}
				if row.displayLabel(icons.Plain) != row.label {
					t.Fatal("plain label changed")
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("kinds = %v, want %v; rows = %+v", got, tc.want, rows)
			}
		})
	}
}

func TestNerdFontOutlineNavigation(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := outlineTestApp(t, "# Root\n## Child\n")
		app.icons = icons.Nerd
		app.recalculateLayout()
		app.updateCommentSidebar()
		view := ansi.Strip(app.outline.viewport.GetContent())
		if !hasForeground(app.outline.viewport.GetContent(), mdCodeStyle.GetForeground(), icons.Nerd.Glyph(icons.Heading, "")) {
			t.Fatal("outline icon does not use the inline-code foreground")
		}
		if !strings.Contains(view, icons.Nerd.Label(icons.Heading, "Child")) {
			t.Fatalf("heading icon missing: %s", view)
		}
		left, top, _, _ := app.commentBounds()
		app = clickMouse(app, left+3, top+app.fileTree.viewport.Height()+2)
		if app.tab().cursorLine != 2 {
			t.Fatal("outline icon shifted the mouse target")
		}
		rows := outlineHeadings(app.outline.rows)
		app.icons = icons.Plain
		app.recalculateLayout()
		app.updateCommentSidebar()
		if !reflect.DeepEqual(outlineHeadings(app.outline.rows), rows) || strings.Contains(app.outline.viewport.GetContent(), icons.Nerd.Glyph(icons.Heading, "")) {
			t.Fatal("portable outline changed source metadata or retained icons")
		}
	}
}

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
