package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTableHorizontalControlsAndResize(t *testing.T) {
	content := "| ID | Description | Notes |\n| --- | --- | --- |\n| 1 | " + strings.Repeat("word ", 20) + " | " + strings.Repeat("note ", 20) + " |"
	app := setupAppWithDoc(t, content+"\n\n"+content)
	app.viewMode = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 70, Height: 40})
	app.tab().cursorLine = 3
	first := *app.tableAtLine(3)
	if first.window.maxOffset() <= 0 || first.window.contentWidth != 92 {
		t.Fatalf("unexpected table width: %+v", first)
	}
	if !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "▶") {
		t.Fatal("missing overflow indicator")
	}
	app = pressKey(app, tea.KeyRight)
	if app.tableAtLine(3).window.offset == 0 || app.tableAtLine(7).window.offset != 0 {
		t.Fatal("keyboard did not scroll only the cursor's table")
	}
	app = pressKey(app, 'h')
	if app.tableAtLine(3).window.offset != 0 || app.openAll || !app.viewMode {
		t.Fatal("h did not scroll left while preserving comment display")
	}
	app = pressKey(app, 'l')
	if app.tableAtLine(3).window.offset == 0 {
		t.Fatal("l did not scroll right")
	}
	left, top, _, _ := app.contentBounds()
	row := app.contentLayout.lineRanges[1].start
	app = clickMouse(app, left+gutterWidth, top+row)
	if app.tableAtLine(3).window.offset != 0 {
		t.Fatal("left button did not scroll back")
	}
	row = app.contentLayout.lineRanges[5].start
	app, _ = updateApp(app, tea.MouseWheelMsg{X: left + gutterWidth + 2, Y: top + row, Button: tea.MouseWheelRight})
	if app.tableAtLine(3).window.offset != 0 || app.tableAtLine(7).window.offset == 0 {
		t.Fatal("wheel did not target the table under the pointer")
	}
	before := app.tableAtLine(7).window.contentWidth
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 140, Height: 40})
	if app.tableAtLine(7).window.offset != 0 || app.tableAtLine(7).window.contentWidth != before {
		t.Fatal("resize changed column widths or retained an invalid offset")
	}
}

func TestTableSearchRevealsHiddenLinkAndWrappedRow(t *testing.T) {
	content := "| First | Second |\n| --- | --- |\n| " + strings.Repeat("left ", 25) + " | " + strings.Repeat("filler ", 30) + " [日本語](https://hidden.example) |"
	app := setupAppWithDoc(t, content)
	app.viewMode = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 60, Height: 14})
	app = ctrlKey(app, 's')
	app = typeSearch(app, "hidden.example")
	table := app.tableAtLine(3)
	if table.window.offset == 0 || table.match == nil || table.match.top < app.contentViewport.YOffset() || table.match.bottom > app.contentViewport.YOffset()+app.contentViewport.Height() {
		t.Fatalf("match was not revealed: table=%+v, y=%d", table, app.contentViewport.YOffset())
	}
	assertISearchHighlight(t, app.contentViewport.GetContent(), "日本語", true)
	if !strings.Contains(app.contentViewport.GetContent(), ansi.SetHyperlink("https://hidden.example")) {
		t.Fatal("clipped link lost its destination")
	}
}

func TestScrolledTableSelectionKeepsMarkdownAndGutter(t *testing.T) {
	const label = "日本語 é label"
	const link = "[**" + label + "**](https://example.org/path)"
	app := setupAppWithDoc(t, "| First | Second |\n| --- | --- |\n| "+strings.Repeat("words ", 20)+" | "+link+" |")
	app.viewMode = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 45, Height: 30})
	table := *app.tableAtLine(3)
	app.scrollTable(table, table.window.maxOffset())
	var span contentTextSpan
	for _, candidate := range app.contentLayout.textSpans {
		if strings.Contains(candidate.text, label) {
			span = candidate
			break
		}
	}
	at := strings.Index(span.text, label)
	if at < 0 {
		t.Fatal("label span missing")
	}
	left, top, _, _ := app.contentBounds()
	x := left + span.left + ansi.StringWidth(span.text[:at])
	app = pressMouse(app, x, top+span.row)
	app = releaseMouse(app, x+ansi.StringWidth(label), top+span.row)
	if got := app.selectedText(); got != link {
		t.Fatalf("copied %q, want %q", got, link)
	}
	for _, row := range strings.Split(app.contentViewport.GetContent(), "\n") {
		if !utf8.ValidString(row) || ansi.StringWidth(row) > app.contentViewport.Width() {
			t.Fatalf("invalid or oversized clipped row: %q", row)
		}
	}
	// Scrolling keeps the selection's source anchors and cannot paint over the gutter.
	app.scrollTable(*app.tableAtLine(3), -1)
	if got := app.selectedText(); got != link {
		t.Fatalf("scroll changed selected source: %q", got)
	}
}
