package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestRenderedContentLayoutTracksWrappedLines(t *testing.T) {
	for _, tt := range []struct {
		path     string
		long     string
		markdown bool
	}{
		{"test.go", strings.Repeat("x", 500), false},
		{"test.md", strings.Repeat("word ", 100), true},
	} {
		t.Run(tt.path, func(t *testing.T) {
			app := newScrollTestApp(tt.path, []string{"short", tt.long, "short"}, tt.markdown, 80, 24)
			if !tt.markdown {
				app.tabs[0].chromaLines[1] = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(tt.long)
				app.tabs[0].changedLines = map[int]bool{2: true}
			}
			app.rebuildContent()
			if r := app.contentLayout.lineRanges[2]; r.end-r.start <= 1 {
				t.Error("expected multiple rendered rows for the wrapped line")
			}
			for _, line := range []int{1, 3} {
				if r := app.contentLayout.lineRanges[line]; r.end-r.start != 1 {
					t.Errorf("line %d occupies %d rows, want 1", line, r.end-r.start)
				}
			}
			if tt.markdown {
				return
			}
			if got := strings.Count(app.contentViewport.View(), "x"); got != len(tt.long) {
				t.Errorf("rendered %d of %d highlighted characters", got, len(tt.long))
			}
		})
	}
}

func TestHighlightMarkdownNestedBoldLinkPreservesText(t *testing.T) {
	line := "- **[Crit](https://crit.md/)-compatible agent workflow** — review commands"
	want := "- Crit-compatible agent workflow — review commands"
	if got := ansi.Strip(highlightMarkdown(line)); got != want {
		t.Fatalf("highlighted text = %q, want %q", got, want)
	}
}

func TestMarkdownInlineFormattingAcrossWrap(t *testing.T) {
	for _, source := range []string{
		"before `code with several words across rows` after",
		"before **bold with several words across rows** after",
		"before [a label with several words across rows](https://example.org/a_(b)) after",
	} {
		t.Run(source, func(t *testing.T) {
			app := newScrollTestApp("test.md", []string{source}, true, 32, 24)
			app.rebuildContent()
			want := strings.Join(strings.Fields(ansi.Strip(highlightMarkdown(source))), "")
			var text strings.Builder
			for _, row := range strings.Split(app.contentViewport.GetContent(), "\n") {
				// Content rows have a seven-column source gutter.
				text.WriteString(strings.Join(strings.Fields(ansi.Strip(ansi.Cut(row, 7, 1000))), ""))
			}
			if got := text.String(); got != want {
				t.Fatalf("wrapped text = %q, want %q", got, want)
			}
			deleted := strings.Join(deletedDisplayLines(source, "", nil, true, 20, nil), "")
			if got := strings.Join(strings.Fields(ansi.Strip(deleted)), ""); got != want {
				t.Fatalf("deleted text = %q, want %q", got, want)
			}
		})
	}
}

func TestWrappedMarkdownCells(t *testing.T) {
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, source := range []string{
			"**日本語 é bold words across several wrapped rows**",
			"`日本語 é code words across several wrapped rows`",
			"[日本語 é linked words across several wrapped rows](https://example.org/a_(b))",
		} {
			app := newScrollTestApp("test.md", []string{source, "plain tail"}, true, 32, 24)
			app.rebuildContent()
			rows := uv.NewStyledString(app.contentViewport.GetContent()).Lines(ansi.GraphemeWidth)
			for y, row := range rows {
				x := 0
				for _, cell := range row {
					if x < 7 || y >= app.contentLayout.lineRanges[1].end {
						if cell.Link.URL != "" {
							t.Fatalf("link leaked to gutter or next line: (%d,%d) %+v", x, y, cell)
						}
					} else if strings.TrimSpace(cell.Content) != "" {
						switch source[0] {
						case '*':
							if cell.Style.Attrs&uv.AttrBold == 0 {
								t.Fatalf("bold lost after wrapping at (%d,%d)", x, y)
							}
						case '`':
							if cell.Style.Fg == nil {
								t.Fatalf("code style lost after wrapping at (%d,%d)", x, y)
							}
						case '[':
							if cell.Link.URL != "https://example.org/a_(b)" {
								t.Fatalf("link lost after wrapping at (%d,%d): %q", x, y, cell.Link.URL)
							}
						}
					}
					x += max(1, cell.Width)
				}
			}
		}
	}
}

func TestDeletedMarkdownLinesWrap(t *testing.T) {
	longLine := strings.Repeat("word ", 100)
	app := newScrollTestApp("test.md", []string{"current"}, true, 80, 24)
	app.tabs[0].deletedAfter = map[int][]gitpkg.DeletedLine{
		0: {{OldLineNum: 1, Content: longLine}},
	}

	lines := deletedDisplayLines(longLine, "", nil, true, app.contentViewport.Width()-8, nil)
	if len(lines) < 2 {
		t.Fatalf("expected deleted Markdown line to wrap, got %d display line", len(lines))
	}
	app.rebuildContent()
	oldRange := app.contentLayout.oldRanges[1]
	if got := oldRange.end - oldRange.start; got != len(lines) {
		t.Errorf("deleted line range has %d rows, want %d", got, len(lines))
	}
	if got := app.contentLayout.lineRanges[1].end - app.contentLayout.lineRanges[1].start; got != 1 {
		t.Errorf("source line range has %d rows, want 1", got)
	}
}

func TestInlineDiffDisplayLinesPreserveTextAndBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	before := "Review code with your coding agent."
	after := "Review documents with your agent."
	patch, err := gitpkg.ParsePatch(strings.NewReader("diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-" + before + "\n+" + after + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	if commonBackground == bgToAnsi(diffChangedLineBg.GetBackground()) ||
		commonBackground == bgToAnsi(diffDeletedLineBg.GetBackground()) {
		t.Fatal("expected replacement-line common text to use a neutral background")
	}
	diff := patch.Files[0].Diff
	for _, tc := range []struct {
		name, want string
		segments   []gitpkg.InlineSegment
		changed    lipgloss.Style
	}{
		{"old", before, diff.DeletedAfter[0][0].Inline, diffDeletedTextBg},
		{"new", after, diff.InlineChanges[1], diffAddedTextBg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(inlineDiffDisplayLines(tc.segments, true, 80, diffCommonTextBg, tc.changed, nil), "\n")
			if text := ansi.Strip(got); text != tc.want {
				t.Fatalf("rendered text = %q, want %q", text, tc.want)
			}
			changedBackground := bgToAnsi(tc.changed.GetBackground())
			if commonBackground == changedBackground {
				t.Fatal("expected common and changed text to use distinct backgrounds")
			}
			if !strings.Contains(got, commonBackground) || !strings.Contains(got, changedBackground) {
				t.Errorf("rendered line does not contain both backgrounds: %q", got)
			}
		})
	}
}

func TestInlineDiffMarkdownRestoresSegmentBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	segments := []gitpkg.InlineSegment{
		{Content: "- **common** `code` tail ", Changed: false},
		{Content: "**changed** `code` tail", Changed: true},
	}
	rendered := strings.Join(inlineDiffDisplayLines(
		segments, true, 200, diffCommonTextBg, diffAddedTextBg, nil), "\n")
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	changedBackground := bgToAnsi(diffAddedTextBg.GetBackground())
	if !strings.Contains(rendered, "\x1b[m"+commonBackground) {
		t.Fatalf("common background was not restored after Markdown style reset: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[m"+changedBackground) {
		t.Fatalf("changed background was not restored after Markdown style reset: %q", rendered)
	}
}

func TestInlineBackgroundResumesAfterInlineCode(t *testing.T) {
	initAdaptiveStyles(true)
	base := bgToAnsi(diffChangedLineBg.GetBackground())
	rendered := inlineBackground(diffChangedLineBg, "before "+mdCodeStyle.Render("code")+" after")
	beforeAfter := rendered[:strings.Index(rendered, "after")]
	reset := strings.LastIndex(beforeAfter, "\x1b[m")
	if reset < 0 {
		t.Fatalf("inline code has no background reset: %q", rendered)
	}
	if resumed := strings.LastIndex(beforeAfter, base); resumed < reset {
		t.Fatalf("line background was not resumed after inline code: %q", rendered)
	}
}

func TestFileCommentsRenderBeforeContent(t *testing.T) {
	for _, kind := range []string{"source", "empty", "deleted", "binary", "removed"} {
		t.Run(kind, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.contentViewport.SetHeight(20)
			app.contentViewport.SetWidth(80)
			switch kind {
			case "empty", "deleted":
				app.tab().doc.Lines = nil
				if kind == "deleted" {
					app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "deleted"}}}
				}
			case "binary":
				app.tab().isBinary, app.tab().doc = true, nil
			case "removed":
				app.tab().outsideChanges, app.tab().doc = true, nil
			}
			app.tab().state.Comments = []review.Comment{
				{ID: "resolved", Scope: "file", Body: "older", Resolved: true},
				{ID: "open", Scope: "file", Body: "newer"},
			}
			app.rebuildContent()
			if row := app.contentLayout.rows[0]; !row.annotation || row.line != 0 || row.annotationIndex != 0 {
				t.Fatalf("first row = %+v, want first file comment", row)
			}
			app.selectComment(0, app.commentTargets(0)[1])
			if app.selectedCommentID() != "open" || !strings.Contains(app.contentViewport.View(), "newer") {
				t.Fatal("file comment navigation did not reveal the selected thread")
			}
			app = pressKey(app, 'H')
			for _, row := range app.contentLayout.rows {
				if row.annotation {
					t.Fatal("hidden file comment remains in content layout")
				}
			}
		})
	}
}

func TestRenderAnnotationBoxCollapsesResolvedThread(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	ann := newAnnotation(review.Comment{
		ID: "c_resolved", StartLine: 1, EndLine: 1,
		Body: "please fix", Resolved: true,
		Replies: []review.Reply{{Author: "AI", Body: "fixed"}},
	})

	box, _ := app.renderAnnotationBox(ann, 40, false)
	if !strings.Contains(ansi.Strip(box), "Resolved") {
		t.Fatalf("resolved annotation box = %q", box)
	}
	if strings.Contains(box, "please fix") || strings.Contains(box, "fixed") {
		t.Fatalf("resolved annotation body remained visible: %q", box)
	}
	if got := strings.Count(box, "\n"); got != 3 {
		t.Fatalf("resolved annotation box height = %d lines, want 3", got)
	}
}

func TestDisplaySourceTextEscapesControlCharacters(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"plain", "plain"},
		{"a\tb", "a    b"},
		{"s/\r$//", "s/^M$//"},
		{"x\x00y\x7f", "x^@y^?"},
		{"one\ntwo", "one\ntwo"},
		{"\x1b[31mred\x1b[0m\t日本語", "\x1b[31mred\x1b[0m    日本語"},
	} {
		if got := displaySourceText(tt.in); got != tt.want {
			t.Errorf("displaySourceText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
