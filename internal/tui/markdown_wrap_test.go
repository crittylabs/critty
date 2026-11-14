package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const wrapWords = "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima"

func TestMarkdownHangingRows(t *testing.T) {
	for _, tc := range []struct{ name, source, prefix string }{
		{"list", "- " + wrapWords, "  "},
		{"ordered", "123. " + wrapWords, "     "},
		{"nested list", "- Outer\n  - " + wrapWords, "    "},
		{"quote", "> " + wrapWords, "▎ "},
		{"compact quote", ">" + wrapWords, "▎ "},
		{"nested quote", "> > " + wrapWords, "▎ ▎ "},
		{"quoted list", "> - " + wrapWords, "▎   "},
		{"task", "- [ ] " + wrapWords, "  "},
		{"callout title", "> [!NOTE] " + wrapWords, "▎   "},
		{"callout body", "> [!NOTE]\n> " + wrapWords, "▎ "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseMarkdownSource(strings.Split(tc.source, "\n"), nil)
			line := &parsed.lines[len(parsed.lines)-1]
			content := line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			rows := line.wrapRows(content, 24, 0)
			if len(rows) < 2 {
				t.Fatal("test did not wrap")
			}
			for i, row := range rows {
				plain := ansi.Strip(row)
				if ansi.StringWidth(row) > 24 || i > 0 && !strings.HasPrefix(plain, tc.prefix) {
					t.Fatalf("row %d = %q, want prefix %q within 24 cells", i, plain, tc.prefix)
				}
			}
		})
	}
}

func TestHangingMarkdownSelection(t *testing.T) {
	const source = "- **first** " + wrapWords + " last\n\n> > *first* " + wrapWords + " last\n\n> [!NOTE] **first** " + wrapWords + " last"
	app := setupPrivateAppWithDoc(t, source)
	app.width, app.height, app.viewMode = 55, 45, true
	app.recalculateLayout()
	app.rebuildContent()
	spans := app.contentLayout.textSpans
	first, last := spans[0], spans[len(spans)-1]
	if last.left <= first.left {
		t.Fatal("continuation has no hanging indentation")
	}
	left, top, _, _ := app.contentBounds()
	app = pressMouse(app, left+first.left, top+first.row)
	app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
	if got := app.selectedText(); got != source {
		t.Fatalf("copied decorative prefix or lost markup:\ngot %q\nwant %q", got, source)
	}
}

func TestHangingCommentAndLiteralCode(t *testing.T) {
	app := NewApp("notes.md", AppConfig{})
	app.images.hidden = true
	view := app.renderCommentContent("Reviewer", "> > "+wrapWords, "", 24)
	for _, row := range strings.Split(ansi.Strip(view), "\n") {
		if !strings.HasPrefix(row, "▎ ▎ ") || ansi.StringWidth(row) > 24 {
			t.Fatalf("comment lost quote bars or width: %q", row)
		}
	}
	parsed := parseMarkdownSource([]string{"```md", "- " + wrapWords, "> " + wrapWords, "```"}, nil)
	for _, line := range parsed.lines {
		if line.wrapIndent != 0 {
			t.Fatal("literal Markdown in code acquired hanging indentation")
		}
	}
	line := &parseMarkdownSource([]string{"> > " + wrapWords}, nil).lines[0]
	for _, row := range line.wrapRows(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil), 2, 0) {
		if ansi.StringWidth(row) > 2 {
			t.Fatalf("narrow wrapping overflowed: %q", row)
		}
	}
}

func TestHangingSplitDiffSelection(t *testing.T) {
	old, next := "- **old** "+wrapWords+" last", "- **new** "+wrapWords+" last"
	patch := "diff --git a/notes.md b/notes.md\n--- a/notes.md\n+++ b/notes.md\n@@ -1,5 +1,5 @@\n Intro\n \n-" + old + "\n+" + next + "\n \n End\n"
	app := patchApp(t, patch)
	app.width, app.height, app.split = 140, 35, true
	app.recalculateLayout()
	app.rebuildContent()
	for _, tc := range []struct{ side, want string }{{"old", old}, {"", next}} {
		var spans []contentTextSpan
		for _, span := range app.contentLayout.textSpans {
			if span.ref.side == tc.side && span.ref.line == 3 {
				spans = append(spans, span)
			}
		}
		if len(spans) < 2 || spans[1].left != spans[0].left+2 {
			t.Fatalf("side=%s: wrapped text did not retain body indentation: %+v", tc.side, spans)
		}
		first, last := spans[0], spans[len(spans)-1]
		left, top, _, _ := app.contentBounds()
		app = pressMouse(app, left+first.left, top+first.row)
		app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
		if got := app.selectedText(); got != tc.want {
			t.Fatalf("side=%s: copied=%q, want=%q", tc.side, got, tc.want)
		}
	}
}
