package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCodeBlockBackgroundAndCopy(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	const content = "Before\n\n```unknown-language\n" + "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima\n\nlast\n```\n\nAfter"
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := setupPrivateAppWithDoc(t, content)
		app.width, app.height, app.viewMode = 60, 35, true
		app.recalculateLayout()
		app.rebuildContent()
		rows := strings.Split(app.contentViewport.GetContent(), "\n")
		var first, last contentTextSpan
		for _, span := range app.contentLayout.textSpans {
			if span.ref.line < 3 || span.ref.line > 7 {
				continue
			}
			if span.ref.line == 3 {
				first = span
			}
			last = span
			_, cells := renderedCells(rows[span.row])
			if len(cells) != app.contentViewport.Width() || !sameColor(cells[len(cells)-1].Bg, codeBlockBackground()) {
				t.Fatalf("dark=%t: code row %d lacks full background: %q", dark, span.row, rows[span.row])
			}
			if strings.HasSuffix(span.text, " ") {
				t.Fatal("decorative padding entered selectable source")
			}
		}
		left, top, _, _ := app.contentBounds()
		app = pressMouse(app, left+first.left, top+first.row)
		app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
		want := strings.Join(strings.Split(content, "\n")[2:7], "\n")
		if got := app.selectedText(); got != want {
			t.Fatalf("copied = %q, want %q", got, want)
		}
	}
}

func TestCodeBackgroundPreservesOverlays(t *testing.T) {
	const text = "first match last"
	content := "first " + isearchMatchStyle.Render("match") + " last"
	row := shadeCodeRow(content, 0, 25)
	_, cells := renderedCells(row)
	if ansi.Strip(row) != text+strings.Repeat(" ", 25-len(text)) || !sameColor(cells[6].Bg, isearchMatchStyle.GetBackground()) || !sameColor(cells[24].Bg, codeBlockBackground()) {
		t.Fatalf("search overlay or padding lost: %q", row)
	}
	row = shadeCodeRow(inlineBackground(diffAddedTextBg, text), 0, 25)
	_, cells = renderedCells(row)
	if !sameColor(cells[0].Bg, diffAddedTextBg.GetBackground()) {
		t.Fatal("code background replaced word-diff background")
	}
}

func TestCodeBlockBackgroundInComments(t *testing.T) {
	app := NewApp("notes.md", AppConfig{})
	app.images.hidden = true
	view := app.renderCommentContent("Reviewer", "```go\nvar count = 1\n\n```", "", 32)
	for _, row := range strings.Split(view, "\n") {
		_, cells := renderedCells(row)
		if len(cells) != 32 || !sameColor(cells[31].Bg, codeBlockBackground()) {
			t.Fatalf("comment code row has incomplete background: %q", row)
		}
	}
	plain := parseMarkdownSource([]string{"Before", "", "    literal", "", "After"}, nil)
	if !plain.lines[2].codeBlock || plain.lines[0].codeBlock || plain.lines[4].codeBlock {
		t.Fatal("indented code detection changed surrounding prose")
	}
	if rendered := plain.lines[2].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil); ansi.Strip(rendered) != "    literal" {
		t.Fatal("code background changed source text")
	}
}

func TestCodeBackgroundInSplitDiff(t *testing.T) {
	patch := "diff --git a/notes.md b/notes.md\n--- a/notes.md\n+++ b/notes.md\n@@ -1,6 +1,6 @@\n ```go\n-var count = 1\n+var count = 2\n \n last\n ```\n After\n"
	app := patchApp(t, patch)
	app.width, app.height, app.split = 160, 35, true
	app.recalculateLayout()
	app.rebuildContent()
	row := app.contentLayout.lineRanges[2].start
	split := app.contentLayout.columns[row].split
	if split == 0 {
		t.Fatal("test did not render a split diff")
	}
	_, cells := renderedCells(strings.Split(app.contentViewport.GetContent(), "\n")[row])
	for _, column := range []int{split - 1, len(cells) - 1} {
		if !sameColor(cells[column].Bg, codeBlockBackground()) {
			t.Fatalf("code background did not fill diff column %d", column)
		}
	}
	if sameColor(cells[split+1].Bg, codeBlockBackground()) {
		t.Fatal("code background covered the split divider")
	}
}
