package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/crittylabs/critty/v2/internal/icons"
	"github.com/crittylabs/critty/v2/internal/review"
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
						diffBg, wordBg := diffChangedLineBg, diffAddedTextBg
						if side == "old" {
							diffBg, wordBg = diffDeletedLineBg, diffDeletedTextBg
						}
						for i := at; i < at+len(label); i++ {
							switch focus {
							case "inline", "sidebar":
								if !sameColor(styles[i].Bg, sidebarHighlightBg.GetBackground()) {
									t.Fatalf("%s lost the comment highlight while %s has focus", label, focus)
								}
							default:
								if !sameColor(styles[i].Bg, diffBg.GetBackground()) && !sameColor(styles[i].Bg, wordBg.GetBackground()) {
									t.Fatalf("%s lost diff colors while %s has focus", label, focus)
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

func TestSourceLineBackgroundsCoverGutterAndPadding(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			initAdaptiveStyles(dark)
			app := setupPrivateAppWithDoc(t, strings.Repeat("wrapped word ", 10)+"\n\nOutside")
			app.width, app.height = 64, 20
			app.viewMode = true
			app.recalculateLayout()
			app.tab().selecting, app.tab().selectAnchor, app.tab().cursorLine = true, 1, 2
			for _, hidden := range []bool{false, true} {
				app.hideLineNumbers = hidden
				app.rebuildContent()
				rows := strings.Split(app.contentViewport.GetContent(), "\n")
				for line, style := range []lipgloss.Style{selectedLineBg, lipgloss.NewStyle().Background(buttonBackground)} {
					r := app.contentLayout.lineRanges[line+1]
					if line == 0 && r.end-r.start < 2 {
						t.Fatal("selection did not contain wrapped rows")
					}
					for i := r.start; i < r.end; i++ {
						runes, cells := renderedCells(rows[i])
						if len(cells) != app.contentViewport.Width() || runes[0] != ' ' {
							t.Fatalf("line %d row %d has an incomplete background or marker: %q", line+1, i, rows[i])
						}
						if !hidden && (runes[gutterWidth-1] != '┃' || !sameColor(cells[gutterWidth-1].Fg, commentFocusedBorderColor)) {
							t.Fatalf("line %d row %d is missing the blue selection bar", line+1, i)
						}
						for col, cell := range cells {
							if !sameColor(cell.Bg, style.GetBackground()) {
								t.Fatalf("line %d row %d column %d background = %v, want %v", line+1, i, col, cell.Bg, style.GetBackground())
							}
						}
					}
				}
				r := app.contentLayout.lineRanges[3]
				runes, cells := renderedCells(rows[r.start])
				if cells[0].Bg != nil || (!hidden && runes[gutterWidth-1] != ' ') {
					t.Fatal("selection marked a line outside the range")
				}
			}
			app.maxWrapWidth = 20
			app.recalculateLayout()
			app.rebuildContent()
			app.scrollToCursor()
			row := renderedLineY(t, app, "wrapped word")
			view := strings.Split(app.View().Content, "\n")
			_, cells := renderedCells(view[row])
			if !sameColor(cells[app.contentPaneWidth()-2].Bg, selectedLineBg.GetBackground()) {
				t.Fatal("wrap limit stopped the selection background before the pane border")
			}
		})
	}
}

func TestRankCursorOverridesFixedAndMovableColors(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			initAdaptiveStyles(dark)
			app, _ := formTestAppWithContent(t, `- [ ] Inbox <!-- form: id=r rank value=inbox fixed -->
- [ ] Draft <!-- form: id=r rank value=draft -->
`)
			for _, current := range []int{1, 2} {
				app.tab().cursorLine = current
				app.rebuildContent()
				rows := strings.Split(app.contentViewport.GetContent(), "\n")
				for line, background := range []lipgloss.Style{
					lipgloss.NewStyle().Background(formChip.fixedRankBackground),
					lipgloss.NewStyle().Background(formChip.rankBackground),
				} {
					row := rows[app.contentLayout.lineRanges[line+1].start]
					runes, cells := renderedCells(row)
					if hasHandle := strings.Contains(string(runes), app.icons.Glyph(icons.MoveFourDirections, "✥")); hasHandle != (line == 1) {
						t.Fatalf("unexpected drag handle on line %d", line+1)
					}
					handle := cells[app.sourceGutterWidth()+1]
					if !sameColor(handle.Fg, formChip.rankHandleForeground) {
						t.Fatalf("handle on line %d lost its gray foreground: %v", line+1, handle.Fg)
					}
					label := []string{"Inbox", "Draft"}[line]
					labelAt := len([]rune(strings.SplitN(string(runes), label, 2)[0]))
					want := background.GetBackground()
					if line+1 == current {
						want = buttonBackground
					}
					badge := formChip.rankPendingBadge
					if line == 0 {
						badge = formChip.fixedRankBadge
					}
					badgeStart := app.sourceGutterWidth() + 3
					for col := app.sourceGutterWidth() + 1; col < len(cells); col++ {
						if col >= badgeStart && col < badgeStart+3 {
							if !sameColor(cells[col].Bg, badge) || !sameColor(cells[col].Fg, formChip.rankBadgeSymbol) {
								t.Fatalf("rank badge changed at column %d on line %d", col, line+1)
							}
						} else if line+1 == current && !sameColor(cells[col].Bg, want) {
							t.Fatalf("cursor lost background at column %d on line %d", col, line+1)
						}
					}
					if !sameColor(cells[labelAt].Bg, want) {
						t.Fatalf("%s background = %v, want %v", label, cells[labelAt].Bg, want)
					}
				}
			}
		})
	}
}
