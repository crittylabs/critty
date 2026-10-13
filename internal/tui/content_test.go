package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestRenderedContentLayoutTracksWrappedChromaLines(t *testing.T) {
	longLine := strings.Repeat("x", 500)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.go", lines, false, 80, 24)
	app.tabs[0].chromaLines[1] = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(longLine)
	app.tabs[0].changedLines = map[int]bool{2: true}

	app.rebuildContent()
	if r := app.contentLayout.lineRanges[2]; r.end-r.start <= 1 {
		t.Error("expected multiple rendered rows for wrapped Chroma-highlighted line")
	}
	for _, line := range []int{1, 3} {
		if r := app.contentLayout.lineRanges[line]; r.end-r.start != 1 {
			t.Errorf("line %d occupies %d rows, want 1", line, r.end-r.start)
		}
	}
	if got := strings.Count(app.contentViewport.View(), "x"); got != len(longLine) {
		t.Errorf("rendered %d of %d highlighted characters", got, len(longLine))
	}
}

func TestRenderedContentLayoutTracksWrappedMarkdown(t *testing.T) {
	longLine := strings.Repeat("word ", 100) // 500 chars, wraps in markdown
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.md", lines, true, 80, 24)

	app.rebuildContent()
	if r := app.contentLayout.lineRanges[2]; r.end-r.start <= 1 {
		t.Error("expected multiple rendered rows for wrapped Markdown line")
	}
	for _, line := range []int{1, 3} {
		if r := app.contentLayout.lineRanges[line]; r.end-r.start != 1 {
			t.Errorf("line %d occupies %d rows, want 1", line, r.end-r.start)
		}
	}
}

func TestHighlightMarkdownNestedBoldLinkPreservesText(t *testing.T) {
	line := "- **[Crit](https://crit.md/)-compatible agent workflow** — review commands"
	want := "- Crit-compatible agent workflow — review commands"
	if got := ansi.Strip(highlightMarkdown(line)); got != want {
		t.Fatalf("highlighted text = %q, want %q", got, want)
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

func TestInlineDiffDisplayLinesUseDistinctBackgrounds(t *testing.T) {
	initAdaptiveStyles(true)
	segments := []gitpkg.InlineSegment{
		{Content: "common ", Changed: false},
		{Content: "added", Changed: true},
	}

	rendered := strings.Join(inlineDiffDisplayLines(segments, true, 80, diffCommonTextBg, diffAddedTextBg, nil), "\n")
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	changedBackground := bgToAnsi(diffAddedTextBg.GetBackground())
	if commonBackground == changedBackground {
		t.Fatal("expected common and changed text to use distinct backgrounds")
	}
	if commonBackground == bgToAnsi(diffChangedLineBg.GetBackground()) ||
		commonBackground == bgToAnsi(diffDeletedLineBg.GetBackground()) {
		t.Fatal("expected replacement-line common text to use a neutral background")
	}
	if !strings.Contains(rendered, commonBackground) || !strings.Contains(rendered, changedBackground) {
		t.Errorf("rendered line does not contain both backgrounds: %q", rendered)
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
