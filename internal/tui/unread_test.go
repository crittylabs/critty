package tui

import (
	"os"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/v2/internal/form"
	gitpkg "github.com/crittylabs/critty/v2/internal/git"
	"github.com/crittylabs/critty/v2/internal/review"
)

func TestUnansweredFormGutter(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] All <!-- form: id=all select-all=checks -->
- [ ] Include notes <!-- form: id=notes tags=checks -->
- [ ] Brief <!-- form: id=length value=brief -->
- [ ] Detailed <!-- form: id=length value=detailed -->
- [ ] Reason <!-- form: id=reason text required -->
`)
	assertDots := func(want map[int]bool) {
		t.Helper()
		app.rebuildContent()
		rows := strings.Split(app.contentViewport.GetContent(), "\n")
		for line := 1; line <= 5; line++ {
			runes, cells := renderedCells(rows[app.contentLayout.lineRanges[line].start])
			if (runes[0] == '●') != want[line] {
				t.Errorf("line %d gutter = %q, want pending dot %t", line, runes[0], want[line])
			}
			if want[line] && !sameColor(cells[0].Fg, unreadDotStyle.GetForeground()) {
				t.Errorf("line %d pending dot lost its color", line)
			}
		}
	}
	assertDots(map[int]bool{2: true, 3: true, 5: true})
	answers := app.forms.Document(app.tab().path).Answers
	answers["notes"] = form.Answer{Value: true, Human: true}
	answers["length"] = form.Answer{Value: "detailed", Human: true}
	answers["reason"] = form.Answer{Value: "For review", Human: true}
	assertDots(nil)
}

func TestUnresolvedCommentGutter(t *testing.T) {
	t.Parallel()
	const patch = "diff --git a/notes.md b/notes.md\n--- a/notes.md\n+++ b/notes.md\n@@ -1 +1 @@\n-Old\n+New\n"
	app := patchApp(t, patch)
	app.tab().state.Comments = []review.Comment{
		{ID: "before", Side: "old", StartLine: 1, Body: "Previous text."},
		{ID: "after", StartLine: 1, Body: "New text."},
		{ID: "file", Scope: "file", Body: "File note."},
	}
	for _, split := range []bool{false, true} {
		app.split = split
		for _, reading := range []bool{false, true} {
			app.viewMode = reading
			for _, resolved := range []bool{false, true} {
				for i := range app.tab().state.Comments {
					app.tab().state.Comments[i].Resolved = resolved
				}
				app.rebuildContent()
				want := 3
				if resolved {
					want = 0
				}
				if dots := strings.Count(ansi.Strip(app.contentViewport.GetContent()), "●"); dots != want {
					t.Errorf("split=%t reading=%t resolved=%t: %d dots, want %d", split, reading, resolved, dots, want)
				}
			}
		}
	}
}

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
	app, _ = updateApp(app, app.loadDocuments()())
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
					tabBefore := app.renderTabWidth(app.tabLabels(), 1, 0)
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
					assertPosition := func(before, after string, shift int) {
						t.Helper()
						before, after = ansi.Strip(before), ansi.Strip(after)
						beforePrefix, _, _ := strings.Cut(before, "y.go")
						afterPrefix, _, _ := strings.Cut(after, "y.go")
						if ansi.StringWidth(beforePrefix)+shift != ansi.StringWidth(afterPrefix) || ansi.StringWidth(before)+shift != ansi.StringWidth(after) {
							t.Fatalf("unread marker did not change filename position and width by %d cells", shift)
						}
					}
					tabUnread := app.renderTabWidth(app.tabLabels(), 1, 0)
					treeUnread := app.treeRowText(row, app.tabIndicators())
					assertDot(tabUnread, true)
					assertDot(treeUnread, true)
					assertPosition(tabBefore, tabUnread, 2)
					assertPosition(treeBefore, treeUnread, 0)
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
					assertDot(app.renderTabWidth(app.tabLabels(), 1, 0), false)
					assertPosition(tabUnread, app.renderTabWidth(app.tabLabels(), 1, 0), -2)
					assertDot(app.treeRowText(row, app.tabIndicators()), false)
					assertPosition(treeUnread, app.treeRowText(row, app.tabIndicators()), 0)
				})
			}
		})
	}
}
