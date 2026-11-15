package tui

import (
	"fmt"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/crittylabs/critty/internal/review"
)

func TestSourceFocusPreservesMarkdownFormatting(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	const patch = "diff --git a/notes.md b/notes.md\n--- a/notes.md\n+++ b/notes.md\n@@ -1 +1 @@\n-**Old** *italic*\n+**New** *italic*\n"
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, split := range []bool{false, true} {
			for _, side := range []string{"old", ""} {
				for _, focus := range []string{"none", "inline", "sidebar", "selection"} {
					t.Run(fmt.Sprintf("dark=%t/split=%t/side=%s/focus=%s", dark, split, side, focus), func(t *testing.T) {
						app := patchApp(t, patch)
						app.tab().doc.Known = nil
						app.split = split
						app.tab().cursorLine, app.tab().cursorSide = 1, side
						app.tab().state.Comments = []review.Comment{{ID: "c_focus", StartLine: 1, Side: side, Body: "Check formatting."}}
						switch focus {
						case "inline":
							app.tab().cursorOnAnnotation = true
						case "sidebar":
							app.focused, app.sidebarView = commentPane, commentsView
						case "selection":
							app.tab().selecting = true
							app.tab().selectAnchor, app.tab().selectSide = 1, side
						}
						app.updateCommentSidebar()
						app.rebuildContent()
						if focus == "inline" || focus == "sidebar" {
							start, end, gotSide := app.highlightedCommentLines()
							if start != 1 || end != 1 || gotSide != side {
								t.Fatalf("comment highlight = %d,%d,%q", start, end, gotSide)
							}
						}
						runes, styles := renderedCells(app.contentViewport.GetContent())
						label := "New"
						if side == "old" {
							label = "Old"
						}
						at := strings.Index(string(runes), label)
						if at < 0 {
							t.Fatalf("missing %s", label)
						}
						at = len([]rune(string(runes)[:at]))
						for i := at; i < at+len(label); i++ {
							if focus != "none" {
								expected := sidebarHighlightBg.GetBackground()
								if focus == "selection" {
									expected = selectedLineBg.GetBackground()
								}
								if !sameColor(styles[i].Bg, expected) {
									t.Fatalf("%s kept diff colors while %s has focus", label, focus)
								}
							}
							if styles[i].Attrs&uv.AttrBold == 0 {
								t.Fatalf("%s lost bold while %s has focus", label, focus)
							}
						}
					})
				}
			}
		}
	}
}
