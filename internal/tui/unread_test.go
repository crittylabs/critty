package tui

import (
	"os"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestReloadUnreadTabs(t *testing.T) {
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
	source := gitpkg.ReviewSource{Scope: "all"}
	files, err := source.Files()
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Source: &source})
	app, _ = updateApp(app, docRenderedMsg{})
	app.width, app.height = 100, 30
	check := func(want map[string]bool) {
		t.Helper()
		for _, tab := range app.tabs {
			if tab.unread != want[tab.path] {
				t.Fatalf("%s: unread=%t, want %t", tab.path, tab.unread, want[tab.path])
			}
		}
	}
	check(nil)
	reloadApp(t, &app)
	check(nil)
	write("a.txt", "a two\n")
	write("b.txt", "b two\n")
	write("c.txt", "new\n")
	reloadApp(t, &app)
	check(map[string]bool{"b.txt": true, "c.txt": true})
	app.toggleWhitespace()
	reloadApp(t, &app)
	check(map[string]bool{"b.txt": true, "c.txt": true})
	app.selectTab(1)
	check(map[string]bool{"c.txt": true})
	reloadApp(t, &app)
	check(map[string]bool{"c.txt": true})
	app.selectTab(0)
	write("b.txt", "b three\n")
	reloadApp(t, &app)
	check(map[string]bool{"b.txt": true, "c.txt": true})
}

func TestUnreadRenderingAndNavigation(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, theme := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(theme.name, func(t *testing.T) {
			initAdaptiveStyles(theme.dark)
			for _, route := range []string{"tab", "number", "tree", "change"} {
				t.Run(route, func(t *testing.T) {
					app := newFileTreeTestApp()
					row := 4 // a/y.go
					tabBefore := app.renderTab(app.tabLabels(), 1)
					treeBefore := app.treeRowText(row, app.tabIndicators())
					app.tabs[1].unread = true
					app.tabs[2].unread = true
					assertDot := func(text string, want bool) {
						t.Helper()
						found := false
						for _, cells := range uv.NewStyledString(text).Lines(ansi.GraphemeWidth) {
							for _, cell := range cells {
								if cell.Content == "●" {
									found = true
									if !sameColor(cell.Style.Fg, unreadDotStyle.GetForeground()) {
										t.Fatalf("dot color: %+v", cell.Style)
									}
								}
								if cell.Content == "y" && sameColor(cell.Style.Fg, unreadDotStyle.GetForeground()) {
									t.Fatal("dot color leaked into filename")
								}
							}
						}
						if found != want {
							t.Fatalf("dot=%t, want %t", found, want)
						}
					}
					assertPosition := func(before, after string) {
						t.Helper()
						before, after = ansi.Strip(before), ansi.Strip(after)
						beforePrefix, _, _ := strings.Cut(before, "y.go")
						afterPrefix, _, _ := strings.Cut(after, "y.go")
						if ansi.StringWidth(beforePrefix) != ansi.StringWidth(afterPrefix) || ansi.StringWidth(before) != ansi.StringWidth(after) {
							t.Fatal("unread marker moved filename or changed row width")
						}
					}
					tabUnread := app.renderTab(app.tabLabels(), 1)
					treeUnread := app.treeRowText(row, app.tabIndicators())
					assertDot(tabUnread, false)
					assertDot(treeUnread, true)
					assertPosition(tabBefore, tabUnread)
					assertPosition(treeBefore, treeUnread)
					switch route {
					case "tab":
						app.selectTab(1)
					case "number":
						app = pressKey(app, '2')
					case "tree":
						app.setTreeCursor(row)
					case "change":
						app.selectChange(1, changeChunk{startLine: 1, endLine: 1}, 0)
					}
					if app.activeTab != 1 || app.tabs[1].unread || !app.tabs[2].unread {
						t.Fatal("opening a tab did not mark only that tab as read")
					}
					assertDot(app.renderTab(app.tabLabels(), 1), false)
					assertPosition(tabUnread, app.renderTab(app.tabLabels(), 1))
					assertDot(app.treeRowText(row, app.tabIndicators()), false)
					assertPosition(treeUnread, app.treeRowText(row, app.tabIndicators()))
				})
			}
		})
	}
}
