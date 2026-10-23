package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestCopyNoticeExpires(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dark   bool
		height int
	}{
		{"light/short", false, 16}, {"light/tall", false, 30},
		{"dark/short", true, 16}, {"dark/tall", true, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initAdaptiveStyles(tc.dark)
			app := setupAppWithDoc(t, "one\ntwo\n")
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: tc.height})
			footer, viewport := app.renderFooter(), app.contentViewport.Height()
			app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			if cmd == nil || !strings.Contains(ansi.Strip(app.renderFooter()), "Copied to kill ring:") || !hasForeground(app.renderFooter(), footerNoticeStyle.GetForeground(), "Copied") {
				t.Fatal("copy did not show a styled timed notice")
			}
			app = pressKey(app, tea.KeyDown)
			if app.notice.text == "" {
				t.Fatal("navigation cleared the notice before its timeout")
			}
			app, _ = updateApp(app, noticeExpiredMsg(app.notice.generation))
			if app.notice.text != "" || app.renderFooter() != footer || app.contentViewport.Height() != viewport {
				t.Fatal("expiration did not restore footer and viewport")
			}
		})
	}
}

func TestNoticeReplacementAndPriority(t *testing.T) {
	app := setupAppWithDoc(t, "text\n")
	app.width, app.height = 40, 30
	app.flashNotice("old")
	old := noticeExpiredMsg(app.notice.generation)
	app.flashNotice("new")
	app, _ = updateApp(app, old)
	if app.notice.text != "new" {
		t.Fatal("old timer cleared the newer notice")
	}
	app.locationError = "persistent error"
	app, _ = updateApp(app, noticeExpiredMsg(app.notice.generation))
	if !strings.Contains(app.renderFooter(), "persistent error") {
		t.Fatal("notice timeout cleared an error")
	}
	app.locationError = ""
	app.flashNotice(strings.Repeat("日本語", 30))
	if lipgloss.Width(app.renderFooter()) > app.width {
		t.Fatal("notice overflowed terminal width")
	}
}

func TestReloadNoticeOnlyForVisibleChanges(t *testing.T) {
	dir := newGitRepo(t)
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "a base\n")
	write("b.txt", "b base\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-qm", "base")
	write("a.txt", "a one\n")
	write("b.txt", "b one\n")
	source := gitpkg.ReviewSource{Scope: "unstaged"}
	files, err := source.Files()
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Source: &source})
	app, _ = updateApp(app, docRenderedMsg{})
	app.width, app.height = 80, 30
	if app.tab().path != "a.txt" {
		t.Fatal("wrong initial tab")
	}
	reloadApp(t, &app)
	if app.notice.text != "" {
		t.Fatal("unchanged reload flashed a notice")
	}
	write("b.txt", "b two\n")
	reloadApp(t, &app)
	if app.notice.text != "" || app.tabs[1].doc.Content != "b two\n" {
		t.Fatal("offscreen update flashed a notice or did not load")
	}
	write("a.txt", "a two\n")
	reloadApp(t, &app)
	if !strings.Contains(app.notice.text, "a.txt") {
		t.Fatal("visible text update did not flash")
	}
	app.expireNotice(noticeExpiredMsg(app.notice.generation))
	write("a.txt", "a staged\n")
	runGitIn(t, dir, "add", "a.txt")
	write("a.txt", "a two\n")
	reloadApp(t, &app)
	if !strings.Contains(app.notice.text, "a.txt") {
		t.Fatal("old-side diff update did not flash")
	}
}

func TestBinaryReloadNotice(t *testing.T) {
	for _, scope := range []string{"all", "staged"} {
		t.Run(scope, func(t *testing.T) {
			dir := newGitRepo(t)
			path := "image.bin"
			if err := os.WriteFile(path, []byte{0, 1, 2}, 0600); err != nil {
				t.Fatal(err)
			}
			other := "第二 image.bin"
			if err := os.WriteFile(other, []byte{0, 1, 2}, 0600); err != nil {
				t.Fatal(err)
			}
			runGitIn(t, dir, "add", path, other)
			runGitIn(t, dir, "commit", "-qm", "base")
			if err := os.WriteFile(path, []byte{0, 3, 4}, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(other, []byte{0, 3, 4}, 0600); err != nil {
				t.Fatal(err)
			}
			if scope == "staged" {
				runGitIn(t, dir, "add", path, other)
			}
			source := gitpkg.ReviewSource{Scope: scope}
			files, err := source.Files()
			if err != nil {
				t.Fatal(err)
			}
			app := NewCodeReviewApp(files, "HEAD", AppConfig{Source: &source})
			app, _ = updateApp(app, docRenderedMsg{})
			app.width, app.height = 80, 30
			reloadApp(t, &app)
			if app.notice.text != "" {
				t.Fatal("unchanged binary flashed a notice")
			}
			if err := os.WriteFile(other, []byte{0, 5, 6}, 0600); err != nil {
				t.Fatal(err)
			}
			if scope == "staged" {
				runGitIn(t, dir, "add", other)
			}
			reloadApp(t, &app)
			if app.notice.text != "" {
				t.Fatal("offscreen binary update flashed a notice")
			}
			if err := os.WriteFile(path, []byte{0, 5, 6}, 0600); err != nil {
				t.Fatal(err)
			}
			if scope == "staged" {
				runGitIn(t, dir, "add", path)
			}
			reloadApp(t, &app)
			if !strings.Contains(app.notice.text, "image.bin") {
				t.Fatal("staged binary update did not flash")
			}
		})
	}
}
