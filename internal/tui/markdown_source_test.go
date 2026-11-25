package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/v2/internal/document"
)

func TestMarkdownSourceToggle(t *testing.T) {
	lines := []string{
		"# Heading", "", "Plain **bold** and `code`.", "", "> Quote", "", "- [x] Task", "",
		"---", "", "[Link](https://example.org) ![Picture](image.png)", "",
		"| Name |Value|", "|-|-:|", "| `a\\|b`|   B |", "", "A &amp; B and \\*escaped.", "",
		"```go", "var count = 42", "```",
	}
	app := setupAppWithDoc(t, strings.Join(lines, "\n"))
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 120, Height: 42})
	app.images.enabled = true
	app.tab().markdownImagesPending = true
	app.tab().markdownImages = []markdownImage{{line: 11, path: "image.png", previewSide: previewSide{failed: true}}}
	app.tab().cursorLine = 11
	app.rebuildContent()
	rendered := ansi.Strip(app.contentViewport.GetContent())
	if !strings.Contains(rendered, "─") || !strings.Contains(rendered, "☑") || strings.Contains(rendered, "](image.png)") ||
		!strings.Contains(rendered, "Image preview unavailable") || len(app.contentLayout.horizontalBlocks) == 0 {
		t.Fatalf("fixture did not render Markdown: %q", rendered)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'u', Mod: tea.ModAlt})
	if !app.tab().markdownSourceView || app.tab().cursorLine != 11 || len(app.contentLayout.horizontalBlocks) != 0 {
		t.Fatalf("source mode did not preserve the cursor or disable table layout")
	}
	styledRows := strings.Split(app.contentViewport.GetContent(), "\n")
	for i, want := range lines {
		r := app.contentLayout.lineRanges[i+1]
		if r.end != r.start+1 {
			t.Fatalf("source line %d has %d rows", i+1, r.end-r.start)
		}
		row := ansi.Cut(styledRows[r.start], app.sourceGutterWidth(), ansi.StringWidth(styledRows[r.start]))
		if got := strings.TrimRight(ansi.Strip(row), " "); got != want {
			t.Errorf("line %d = %q, want %q", i+1, got, want)
		}
	}
	link := sourceWordStyle(t, styledRows[app.contentLayout.lineRanges[11].start], "Link")
	if link.Fg == nil || link.Underline == 0 {
		t.Fatal("link lost syntax styling")
	}
	rule := sourceWordStyle(t, styledRows[app.contentLayout.lineRanges[9].start], "---")
	if !sameColor(rule.Fg, mdHrStyle.GetForeground()) {
		t.Fatal("horizontal rule lost syntax styling")
	}
	for _, line := range []int{13, 15} {
		chars, styles := renderedCells(styledRows[app.contentLayout.lineRanges[line].start])
		for column, char := range chars {
			if char != '|' {
				continue
			}
			inCell := column > 0 && chars[column-1] == '\\'
			if sameColor(styles[column].Fg, mdHrStyle.GetForeground()) == inCell {
				t.Errorf("line %d column %d: delimiters should be subdued and escaped pipes should retain the cell style", line, column)
			}
		}
	}
	styledLink := app.tab().markdownDisplayLine(11, false, lines[10], nil, nil)
	wrapped := app.tab().markdownWrapRows(11, false, styledLink, 12, nil)
	if got := ansi.Strip(strings.Join(wrapped, "")); len(wrapped) < 2 || got != lines[10] {
		t.Fatalf("wrapping changed source: %q", wrapped)
	}
	var span contentTextSpan
	for _, candidate := range app.contentLayout.textSpans {
		if candidate.ref.line == 11 {
			span = candidate
			break
		}
	}
	x, y := contentScreenPoint(app, span.left, span.row)
	app = pressMouse(app, x, y)
	app = releaseMouse(app, x+len("[Link](https://example.org)"), y)
	if got := app.selectedText(); got != "[Link](https://example.org)" {
		t.Fatalf("source selection copied %q", got)
	}
	app = pressKey(app, tea.KeyEscape)
	_, layout := app.renderReviewScreen()
	if !layout.hasMarkdownView {
		t.Fatal("missing clickable Markdown control")
	}
	header := strings.Split(app.renderHeader(), "\n")[layout.markdownViewButton.top]
	if got := ansi.Strip(ansi.Cut(header, layout.markdownViewButton.left, layout.markdownViewButton.right)); got != "· Markdown:raw " {
		t.Fatalf("header control = %q, want raw syntax label", got)
	}
	if !strings.Contains(ansi.Strip(header), "Markdown:raw") || !strings.Contains(ansi.Strip(header), "%") ||
		strings.Index(ansi.Strip(header), "Markdown:raw") > strings.Index(ansi.Strip(header), "%") {
		t.Fatalf("syntax label must precede the line progress: %q", ansi.Strip(header))
	}
	if strings.Contains(ansi.Strip(app.renderFooter()), "Markdown") {
		t.Fatal("file syntax leaked into the shared footer")
	}
	app = clickMouse(app, layout.markdownViewButton.left, layout.markdownViewButton.top)
	if app.tab().markdownSourceView || ansi.Strip(app.contentViewport.GetContent()) != rendered {
		t.Fatal("Markdown control did not restore the formatted document")
	}
}

func TestPatchMarkdownSharesDocumentStyles(t *testing.T) {
	lines := []string{
		"# Heading", "", "## Section", "", "### Detail", "", "#### Minor", "",
		"Text **bold** and *italic* and `code`.", "", "> Quoted", "", "- [ ] Task", "",
		"[Link](https://example.org) <kbd>Enter</kbd>", "", "```go", "var name = \"value\"", "```", "",
		"```text", "# literal", "```", "", "\\# escaped", "", "---", "", "A &amp; B and é.",
		"", "##### Subsection", "", "###### Note",
	}
	var patch strings.Builder
	fmt.Fprintf(&patch, "--- /dev/null\n+++ b/docs.md\n@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		fmt.Fprintf(&patch, "+%s\n", line)
	}
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		tab := FileTab{path: "sample.patch", doc: document.FromContent("sample.patch", []byte(patch.String()))}
		tab.ensureHighlightCache()
		parsed := parseMarkdownSource(lines, nil)
		for _, sample := range []struct {
			line int
			word string
		}{
			{0, "Heading"}, {2, "Section"}, {4, "Detail"}, {6, "Minor"},
			{8, "bold"}, {8, "italic"}, {8, "code"}, {10, "Quoted"}, {12, "Task"},
			{14, "Link"}, {14, "Enter"}, {17, "var"},
			{30, "Subsection"}, {32, "Note"},
		} {
			normal := parsed.lines[sample.line].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			want := sourceWordStyle(t, normal, sample.word)
			got := sourceWordStyle(t, tab.chromaLines[sample.line+3], sample.word)
			if !sameColor(got.Fg, want.Fg) && (got.Fg != nil || want.Fg != nil) || got.Attrs != want.Attrs {
				t.Fatalf("dark=%v, %q: source style %+v differs from document %+v", dark, sample.word, got, want)
			}
		}
		for i, line := range []int{0, 2, 4, 6, 30, 32} {
			rendered := parsed.lines[line].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			style := sourceWordStyle(t, rendered, "#")
			if !sameColor(style.Fg, markdownHeadingStyle(i+1).GetForeground()) || style.Attrs&uv.AttrItalic != 0 {
				t.Fatalf("dark=%v, H%d: got style %+v, want its heading color without italics", dark, i+1, style)
			}
		}
		for i, line := range lines {
			got := tab.chromaLines[i+3]
			if ansi.Strip(got) != "+"+line {
				t.Fatalf("line %d changed source: %q", i+1, got)
			}
		}
		for _, i := range []int{21, 24} {
			style := sourceWordStyle(t, tab.chromaLines[i+3], "#")
			if sameColor(style.Fg, markdownHeadingStyle(1).GetForeground()) || style.Attrs&uv.AttrBold != 0 {
				t.Fatalf("literal or escaped heading on line %d received heading styling", i+1)
			}
		}
	}
}

func sourceWordStyle(t *testing.T, rendered, word string) uv.Style {
	t.Helper()
	runes, styles := renderedCells(rendered)
	plain := string(runes)
	at := strings.Index(plain, word)
	if at < 0 {
		t.Fatalf("missing %q in %q", word, plain)
	}
	return styles[len([]rune(plain[:at]))]
}
