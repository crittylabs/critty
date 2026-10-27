package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/review"
)

func TestCommentFoldKeys(t *testing.T) {
	for _, drifted := range []bool{false, true} {
		for _, resolved := range []bool{false, true} {
			for _, sidebar := range []bool{false, true} {
				t.Run(fmt.Sprintf("drifted=%t/resolved=%t/sidebar=%t", drifted, resolved, sidebar), func(t *testing.T) {
					app := setupAppWithDoc(t, "source\n")
					app.width, app.height = 120, 40
					app.recalculateLayout()
					app.tab().state.Comments = []review.Comment{{ID: "thread", StartLine: 1, EndLine: 1, Body: "thread body", Drifted: drifted, Resolved: resolved}}
					app.selectComment(0, app.commentTargets(0)[0])
					if sidebar {
						app.focused, app.sidebarView = commentPane, commentsView
						app.updateCommentSidebar()
					}
					for _, step := range []struct {
						key    rune
						folded bool
					}{{'h', true}, {tea.KeyLeft, true}, {'l', false}, {tea.KeyRight, false}, {tea.KeyLeft, true}, {tea.KeyEnter, false}} {
						app = pressKey(app, step.key)
						if app.modal != noModal || app.threadCollapsed("thread", resolved, drifted, true) != step.folded {
							t.Fatalf("key %d: modal=%v, folded=%t; want folded=%t", step.key, app.modal, app.threadCollapsed("thread", resolved, drifted, true), step.folded)
						}
						view := app.contentViewport.GetContent()
						if sidebar {
							view = app.commentViewport.GetContent()
						}
						if strings.Contains(ansi.Strip(view), "thread body") == step.folded {
							t.Fatalf("key %d: rendered body does not match fold state", step.key)
						}
					}
					app = pressKey(app, tea.KeyEnter)
					if app.modal == noModal || app.editingID != "thread" {
						t.Fatal("Enter on an unfolded comment did not open its window")
					}
					if app.tab().state.Comments[0].Resolved != resolved {
						t.Fatal("folding changed resolution")
					}
				})
			}
		}
	}
}

func TestCommentFoldKeysPrioritizeFoldingOverTableScroll(t *testing.T) {
	app := setupAppWithDoc(t, "| First | Second |\n| --- | --- |\n| "+strings.Repeat("wide cell ", 30)+" | end |\n")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 60, Height: 35})
	app.tab().state.Comments = []review.Comment{{ID: "table", StartLine: 3, EndLine: 3, Body: "table feedback"}}
	app.selectComment(0, app.commentTargets(0)[0])
	app = pressKey(app, 'h')
	app = pressKey(app, 'l')
	if app.tableAtLine(3).window.offset != 0 {
		t.Fatal("unfolding scrolled the table")
	}
	app = pressKey(app, 'l')
	offset := app.tableAtLine(3).window.offset
	if offset == 0 {
		t.Fatal("right on an unfolded comment did not scroll the table")
	}
	app = pressKey(app, 'h')
	if app.tableAtLine(3).window.offset != offset {
		t.Fatal("folding scrolled the table")
	}
	app = pressKey(app, 'h')
	if app.tableAtLine(3).window.offset >= offset {
		t.Fatal("left on a folded comment did not scroll the table")
	}
}

func TestDriftedMouseRespectsKeyboardFolds(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.tab().state.Comments = []review.Comment{{ID: "thread", StartLine: 1, EndLine: 1, Body: "feedback", Drifted: true}}
	app.selectComment(0, app.commentTargets(0)[0])
	for _, key := range []rune{'h', 'l'} {
		app = pressKey(app, key)
		folded := app.threadCollapsed("thread", false, true, true)
		app.selectMarker([]string{"thread"})
		if app.threadCollapsed("thread", false, true, true) == folded || app.modal != noModal {
			t.Fatal("clicking the marker did not toggle the current keyboard fold state")
		}
	}
}

func TestCommentDisplayKeysAndHelp(t *testing.T) {
	for _, width := range []int{18, 80} {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/readOnly=%t", width, readOnly), func(t *testing.T) {
				app := setupAppWithDoc(t, "source\n")
				app.readOnly = readOnly
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: width, Height: 40})
				app = pressKey(app, 'O')
				if !app.openAll || app.viewMode {
					t.Fatal("O did not enable Open all")
				}
				app = pressKey(app, 'V')
				if !app.viewMode || !app.openAll || !app.commentCollapsed(false, false) {
					t.Fatal("View changed the saved expansion setting")
				}
				app = pressKey(app, 'V')
				if app.viewMode || !app.openAll {
					t.Fatal("V did not restore the previous expansion")
				}
				bars := ansi.Strip(app.renderReviewBar() + app.renderFooter())
				if strings.Contains(bars, "Open all") || strings.Contains(bars, "View: on") || strings.Contains(bars, "View: off") {
					t.Fatal("display modes have status controls outside Help")
				}
				help := ansi.Strip(app.renderHelp(80))
				if !strings.Contains(help, "reading view") || !strings.Contains(help, "open all") {
					t.Fatal("Help does not explain the display keys")
				}
			})
		}
	}
}

func TestReadingViewRestoresExpansion(t *testing.T) {
	for _, opened := range []bool{false, true} {
		for _, exit := range []string{"V", "s", "t", "reference"} {
			t.Run(fmt.Sprintf("opened=%t/exit=%s", opened, exit), func(t *testing.T) {
				app := setupAppWithDoc(t, "source\nnext\n")
				app.session.CJ.ReviewRound = 3
				app.tab().state.Comments = []review.Comment{
					{ID: "c_abcdef", StartLine: 1, EndLine: 1, Body: "open inline body"},
					{ID: "resolved", StartLine: 1, EndLine: 1, Resolved: true, ResolvedRound: 3, Body: "resolved inline body"},
					{ID: "file", Scope: "file", Body: "open file body"},
					{ID: "manual", StartLine: 2, EndLine: 2, Drifted: true, Body: "manually folded body"},
				}
				app.tab().expandedDrifted = map[string]bool{"manual": false}
				app.openAll = opened
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: 120, Height: 50})
				originalWidth := app.contentViewport.Width()
				app = pressKey(app, 'V')
				content := ansi.Strip(app.contentViewport.View())
				if !app.viewMode || app.openAll != opened || !app.commentCollapsed(false, false) || app.contentViewport.Width() <= originalWidth || app.commentPanelWidth() != 0 {
					t.Fatal("View did not widen the source and temporarily fold every comment")
				}
				if strings.Contains(content, "open inline body") || strings.Contains(content, "open file body") || strings.Contains(content, "resolved inline body") {
					t.Fatalf("wrong comments visible in View: %q", content)
				}

				for _, target := range app.commentTargets(0) {
					if target.id == "c_abcdef" {
						app.selectComment(0, target)
					}
				}
				if !app.viewMode || strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "open inline body") {
					t.Fatal("focusing a comment unfolded it inside View")
				}
				if exit == "reference" {
					if !app.navigateCommentReference("critty://comment?id=c_abcdef") {
						t.Fatal("comment reference did not resolve")
					}
				} else {
					app = pressKey(app, []rune(exit)[0])
				}
				if app.viewMode || app.openAll != opened || app.contentViewport.Width() != originalWidth {
					t.Fatal("leaving View did not restore the previous display options")
				}
				app.tab().cursorOnAnnotation = false
				app.focused = contentPane
				app.rebuildContent()
				content = ansi.Strip(app.contentViewport.GetContent())
				if strings.Contains(content, "resolved inline body") != opened {
					t.Fatal("restored expansion preference is not reflected in inline comments")
				}
				if app.tab().expandedDrifted["manual"] || strings.Contains(content, "manually folded body") != opened {
					t.Fatal("Open all or View changed the saved individual expansion")
				}
			})
		}
	}
}

func TestReadingViewKeepsDeletedCommentAnchors(t *testing.T) {
	patch := "diff --git a/review.go b/review.go\n--- a/review.go\n+++ b/review.go\n@@ -10,3 +20,3 @@\n context\n-" + strings.Repeat("old ", 30) + "\n+new\n tail\n"
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%t", split), func(t *testing.T) {
			app := patchApp(t, patch)
			app.split = split
			app.tab().state.Comments = []review.Comment{{ID: "old", StartLine: 11, EndLine: 11, Side: "old", Body: "deleted line comment"}}
			app = pressKey(app, 'V')
			if !app.viewMode || strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "deleted line comment") {
				t.Fatal("View did not fold the comment on a wrapped deleted line")
			}
			if _, ok := app.contentLayout.oldRanges[11]; !ok {
				t.Fatal("View lost the deleted-line anchor")
			}
			app = pressKey(app, 'V')
			if !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "deleted line comment") {
				t.Fatal("leaving View lost the deleted comment body")
			}
		})
	}
}

func TestOpenAllLeavesView(t *testing.T) {
	for _, opened := range []bool{false, true} {
		t.Run(fmt.Sprintf("previousOpenAll=%t", opened), func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.tab().state.Comments = []review.Comment{{ID: "resolved", StartLine: 1, EndLine: 1, Resolved: true, Body: "resolved body"}}
			app.openAll = opened
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
			width := app.contentViewport.Width()
			app = pressKey(app, 'V')
			app = pressKey(app, 'O')
			if app.viewMode || !app.openAll || app.contentViewport.Width() != width || !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "resolved body") {
				t.Fatal("O did not leave View and open all comments")
			}
			app = pressKey(app, 'O')
			if app.openAll || app.viewMode || strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "resolved body") {
				t.Fatal("O did not return to default display")
			}
		})
	}
}

func TestResolveFoldsCurrentRoundAndOpenAllRestoresBody(t *testing.T) {
	for _, drifted := range []bool{false, true} {
		t.Run(fmt.Sprintf("drifted=%t", drifted), func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.width, app.height = 120, 40
			app.recalculateLayout()
			app.session.CJ.ReviewRound = 3
			app.tab().state.Comments = []review.Comment{{ID: "thread", StartLine: 1, EndLine: 1, Drifted: drifted, Body: "thread body"}}
			app.selectComment(0, app.commentTargets(0)[0])
			app.clickResolve("thread")
			if strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "thread body") {
				t.Fatal("current-round resolved body did not fold")
			}
			for _, opened := range []bool{true, false} {
				app = pressKey(app, 'O')
				visible := strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "thread body")
				if app.openAll != opened || visible != opened {
					t.Fatal("Open all did not switch between full expansion and defaults")
				}
				comment := app.tab().state.Comments[0]
				if !comment.Resolved || comment.ResolvedRound != 3 {
					t.Fatal("display toggle changed the resolution data")
				}
			}
		})
	}
}
