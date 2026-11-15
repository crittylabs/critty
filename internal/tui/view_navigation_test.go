package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
)

func TestSingleFileViewSidebar(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		outline, nerd bool
	}{
		{"plain.txt", "Example text.\n", false, false},
		{"plain.md", "Example text.\n", false, true},
		{"outlined.md", "# Root\n\n## Child\n", true, false},
		{"nested.md", "# Reading notes\n\n## Navigation\n\n## Details\n", true, true},
		{"outlined.go", "package example\n\nfunc Run() {}\n", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			app := NewApp(path, AppConfig{ReadOnly: true, NerdFonts: tc.nerd})
			app, _ = updateApp(app, docRenderedMsg{})
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 25})
			if app.sidebarVisible() != tc.outline || app.fileTree.viewport.Height() != 0 {
				t.Fatalf("sidebar=%t tree height=%d", app.sidebarVisible(), app.fileTree.viewport.Height())
			}
			tabs := app.sidebarTabs()
			if tc.outline {
				if len(tabs) != 1 || !strings.Contains(ansi.Strip(tabs[0]), "Outline") || app.outline.viewport.Height() != app.commentViewport.Height() {
					t.Fatalf("outline does not fill its only sidebar tab: %v", tabs)
				}
			} else {
				if len(tabs) != 0 || app.contentViewport.Width() != app.width-2 {
					t.Fatal("view without an outline is not full width")
				}
				for _, key := range []rune{'s', 't'} {
					app = pressKey(app, key)
					if app.focused != contentPane || app.sidebarVisible() {
						t.Fatal("sidebar shortcut focused an unavailable sidebar")
					}
				}
			}
			if help := ansi.Strip(app.renderHelp(80)); strings.Contains(help, "files/comments") || strings.Contains(help, "tree/outline") || strings.Contains(help, "open all comments") {
				t.Fatal("single-file Help advertises unavailable comment or tree controls")
			}
			if screen := ansi.Strip(app.View().Content); strings.Contains(screen, "Comments (") || strings.Contains(screen, "files/comments") {
				t.Fatal("single-file view exposes comment controls")
			}
			rows := strings.Split(app.View().Content, "\n")
			_, top, _, bottom := app.contentBounds()
			for y := top; y < bottom; y++ {
				if ansi.StringWidth(rows[y]) != app.width || ansi.Strip(ansi.Cut(rows[y], app.width-1, app.width)) != "│" {
					t.Fatalf("row %d does not end with a border at column %d: %q", y, app.width, rows[y])
				}
			}
		})
	}
}

func TestSingleFileViewOutlineInteraction(t *testing.T) {
	app := newViewNavigationApp(t, "# Root\n\n## Child\n\n## Sibling\n")
	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+3, top+1)
	if app.tab().cursorLine != 3 || app.focused != commentPane || !app.outline.focused {
		t.Fatal("outline row click did not select the corresponding source line")
	}
	app = pressKey(app, tea.KeyEsc)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'G', Mod: tea.ModAlt})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 5 || !app.outline.focused {
		t.Fatal("outline keys focused the hidden tree or lost navigation")
	}
	app = pressKey(app, 'V')
	if app.sidebarVisible() || app.contentViewport.Width() != app.width-2 {
		t.Fatal("reading view did not hide the outline")
	}
	app = pressKey(app, 's')
	if !app.sidebarVisible() || !app.outline.focused || app.focused != commentPane {
		t.Fatal("sidebar shortcut did not restore the outline")
	}
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 45, Height: 12})
	app = pressKey(app, tea.KeyEsc)
	app = clickMouse(app, app.contentPaneWidth()+3, app.headerHeight())
	if app.focused != commentPane || !app.outline.focused || app.fileTree.viewport.Height() != 0 {
		t.Fatal("resized Outline tab did not focus its content")
	}
}

func TestSingleFileViewOutlineAvailability(t *testing.T) {
	app := newViewNavigationApp(t, "# Root\n\n## Child\n")
	plain := filepath.Join(filepath.Dir(app.tab().path), "plain.md")
	if err := os.WriteFile(plain, []byte("Example text.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.focusOutline()
	followViewTestLink(t, &app, "plain.md")
	if app.sidebarVisible() || app.focused != contentPane || app.contentViewport.Width() != app.width-2 {
		t.Fatal("navigation to a document without headings retained the outline pane or focus")
	}
	app.stepViewHistory(false)
	if !app.sidebarVisible() || len(app.sidebarTabs()) != 1 || app.outline.viewport.Height() != app.commentViewport.Height() {
		t.Fatal("Back did not restore the document's outline")
	}
}

func newViewNavigationApp(t *testing.T, content string) AppModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "start.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(path, AppConfig{ReadOnly: true})
	app, _ = updateApp(app, docRenderedMsg{})
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 25})
	return app
}

func followViewTestLink(t *testing.T, app *AppModel, destination string) {
	t.Helper()
	cmd, ok := app.followLink(destination)
	if !ok {
		t.Fatalf("link not handled: %s", destination)
	}
	if cmd != nil {
		msg, ok := cmd().(viewLinkLoadedMsg)
		if !ok {
			t.Fatal("local navigation did not load a document")
		}
		*app, _ = updateApp(*app, msg)
	}
}

func TestViewNavigationHistory(t *testing.T) {
	app := newViewNavigationApp(t, "[Next](next.md#details)\n"+strings.Repeat("start\n", 100))
	start := app.tab().path
	dir := filepath.Dir(start)
	next := filepath.Join(dir, "next.md")
	for name, content := range map[string]string{
		"next.md":  "# Next\n\n## Details\n" + strings.Repeat("next\n", 100),
		"other.md": "# Other\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app.tab().cursorLine = 45
	app.contentViewport.SetYOffset(35)
	followViewTestLink(t, &app, "next.md#details")
	if app.tab().path != next || app.tab().cursorLine != 3 || len(app.tabs) != 1 || app.session != nil {
		t.Fatalf("navigation: path=%s line=%d tabs=%d", app.tab().path, app.tab().cursorLine, len(app.tabs))
	}
	if !strings.Contains(app.View().Content, "next.md") || len(app.browsing.back) != 1 {
		t.Fatal("navigation did not update the visible path and history")
	}
	app.tab().cursorLine = 60
	app.contentViewport.SetYOffset(50)
	if err := os.Remove(start); err != nil {
		t.Fatal(err)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if app.tab().path != start || app.tab().cursorLine != 45 || app.contentViewport.YOffset() != 35 {
		t.Fatalf("Back lost snapshot or position: %s line=%d offset=%d", app.tab().path, app.tab().cursorLine, app.contentViewport.YOffset())
	}
	_, layout := app.renderReviewScreen()
	if !layout.hasForwardButton {
		t.Fatal("Forward button missing")
	}
	app, _ = clickMouseCmd(app, layout.forwardButton.left, layout.forwardButton.top)
	if app.tab().path != next || app.tab().cursorLine != 60 || app.contentViewport.YOffset() != 50 {
		t.Fatal("Forward click did not restore reading position")
	}
	app.stepViewHistory(false)
	followViewTestLink(t, &app, "other.md")
	if len(app.browsing.forward) != 0 || len(app.tabs) != 1 {
		t.Fatal("a new destination must replace the forward branch without adding tabs")
	}
	if cmd := app.openSourceEditor(sourceLocation{path: next}); cmd != nil {
		t.Fatal("view navigation enabled editing")
	}
}

func TestViewNavigationExistingTabsAndAnchors(t *testing.T) {
	app := newViewNavigationApp(t, "# Start\n"+strings.Repeat("start\n", 80))
	path := filepath.Join(filepath.Dir(app.tab().path), "existing.md")
	app.tabs = append(app.tabs, FileTab{path: path, doc: document.FromContent(path, []byte("# Existing\n\n## Details\n"+strings.Repeat("text\n", 80))), state: &fileReview{}, cursorLine: 40, contentOffset: 30})
	followViewTestLink(t, &app, "existing.md")
	if app.activeTab != 1 || len(app.tabs) != 2 || app.tab().cursorLine != 40 || app.contentViewport.YOffset() != 30 {
		t.Fatal("existing tab did not retain its reading position")
	}
	followViewTestLink(t, &app, "#details")
	if app.tab().cursorLine != 3 {
		t.Fatal("same-document heading did not navigate")
	}
	// Ghostty's default Option+Left/Right sends ESC b/f rather than Alt arrows.
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt})
	if app.tab().cursorLine != 40 || app.contentViewport.YOffset() != 30 {
		t.Fatal("Back did not restore position before an anchor jump")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt})
	if app.tab().cursorLine != 3 {
		t.Fatal("Forward did not restore the heading position")
	}
	app.stepViewHistory(false)
	app.stepViewHistory(false)
	if app.activeTab != 0 || len(app.tabs) != 2 {
		t.Fatal("Back did not reuse the original tab")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt})
	if app.activeTab != 1 || len(app.tabs) != 2 {
		t.Fatal("Forward did not reuse the destination tab")
	}
}

func TestViewNavigationFailuresAndExternalLinks(t *testing.T) {
	opened := stubOpener(t, false)
	app := newViewNavigationApp(t, "# Start\n")
	start := app.tab().path
	for _, link := range []string{"missing.md", "#missing"} {
		followViewTestLink(t, &app, link)
		if app.tab().path != start || app.locationError == "" || len(app.browsing.back) != 0 {
			t.Fatal("failed navigation changed the document or history")
		}
	}
	path := filepath.Join(filepath.Dir(start), "binary.dat")
	if err := os.WriteFile(path, []byte{0, 1, 2}, 0o600); err != nil {
		t.Fatal(err)
	}
	followViewTestLink(t, &app, "binary.dat")
	if app.modal != openExternalModal || app.modalFocus != 1 || len(*opened) != 0 {
		t.Fatal("unsupported file must offer external opening, defaulting to cancel")
	}
	app = pressKey(app, tea.KeyEnter)
	if app.modal != noModal || len(*opened) != 0 {
		t.Fatal("default action opened an external application")
	}
	followViewTestLink(t, &app, "binary.dat")
	app, cmd := pressKeyCmd(app, 'y')
	if status := openStatus(cmd); status != "Opened "+path || app.modal != noModal || len(*opened) != 1 {
		t.Fatalf("external open: %s, %v", status, *opened)
	}
	cmd, ok := app.followLink("https://example.org")
	if !ok || openStatus(cmd) != "Opened https://example.org" || len(*opened) != 2 {
		t.Fatal("web link did not use the browser")
	}
	for _, uri := range []string{"javascript:alert(1)", "file:///etc/passwd", "//example.org/file"} {
		if _, ok := app.followLink(uri); ok {
			t.Fatalf("unsupported URI accepted: %s", uri)
		}
	}
}

func TestViewHeadingFragments(t *testing.T) {
	content := "---\ntitle: metadata\n---\n# Hello, *World*!\n\n## Hello, World!\n\n## 日本語\n\n```md\n# Hidden\n```\n"
	tab := FileTab{path: "notes.md", doc: document.FromContent("notes.md", []byte(content))}
	for _, tc := range []struct {
		fragment string
		line     int
	}{
		{"hello-world", 4}, {"hello-world-1", 6}, {"日本語", 8}, {"user-content-hello-world", 4}, {"hidden", 0}, {"L4junk", 0},
	} {
		t.Run(tc.fragment, func(t *testing.T) {
			line, ok := viewFragmentLine(&tab, tc.fragment)
			if ok != (tc.line > 0) || line != tc.line {
				t.Fatalf("line=%d ok=%t, want %d", line, ok, tc.line)
			}
		})
	}
}

func TestViewNavigationReloadAndStaleLoads(t *testing.T) {
	app := newViewNavigationApp(t, "# Start\n")
	path := filepath.Join(filepath.Dir(app.tab().path), "next.md")
	if err := os.WriteFile(path, []byte("# Next\nold\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, _ := app.followLink("next.md")
	msg := cmd().(viewLinkLoadedMsg)
	followViewTestLink(t, &app, "#start")
	app.finishViewLink(msg)
	if app.tab().path == path {
		t.Fatal("stale load replaced a later navigation")
	}
	followViewTestLink(t, &app, "next.md")
	if err := os.WriteFile(path, []byte("# Next\nnew\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = app.requestReload(true)
	if cmd == nil {
		t.Fatal("linked file cannot reload")
	}
	before := len(app.browsing.back)
	app.finishViewLink(cmd().(viewLinkLoadedMsg))
	if app.tab().doc.Content != "# Next\nnew\n" || app.reload.busy || len(app.browsing.back) != before {
		t.Fatal("reload did not refresh the linked file without adding history")
	}
}
