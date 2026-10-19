package tui

import (
	"strings"
	"testing"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

// BenchmarkMarkdownRebuild includes the complete viewport rebuild, including
// wrapping and source gutters, rather than only inline parsing.
func BenchmarkMarkdownRebuild(b *testing.B) {
	paragraphs := strings.Repeat("A paragraph with **bold**, `code`, and [a link](https://example.org).\nIts continuation has ordinary text.\n\n", 100)
	long := "**first\n" + strings.Repeat("words ", 20000) + "\nlast**"
	for _, tc := range []struct{ name, source, mode string }{
		{"document", paragraphs, "document"},
		{"long-paragraph", long, "document"},
		{"unified-word-diff", paragraphs, "unified"},
		{"split-word-diff", paragraphs, "split"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			lines := strings.Split(tc.source, "\n")
			makeApp := func() AppModel {
				app := newScrollTestApp("benchmark.md", lines, true, 120, 40)
				app.width, app.height = 120, 40
				app.tab().state = &fileReview{}
				if tc.mode != "document" {
					diff := &gitpkg.DiffInfo{ChangedLines: make(map[int]bool), CommonLines: make(map[int]int), InlineChanges: make(map[int][]gitpkg.InlineSegment), DeletedAfter: make(map[int][]gitpkg.DeletedLine)}
					for i, line := range lines {
						n := i + 1
						if i%3 == 0 && line != "" {
							diff.ChangedLines[n] = true
							diff.InlineChanges[n] = []gitpkg.InlineSegment{{Content: "A paragraph with **"}, {Content: "bold", Changed: true}, {Content: "**, `code`, and [a link](https://example.org)."}}
							diff.DeletedAfter[n-1] = []gitpkg.DeletedLine{{OldLineNum: n, Content: strings.Replace(line, "bold", "old", 1), Inline: []gitpkg.InlineSegment{{Content: "A paragraph with **"}, {Content: "old", Changed: true}, {Content: "**, `code`, and [a link](https://example.org)."}}}}
						} else {
							diff.CommonLines[n] = n
						}
					}
					tab := app.tab()
					tab.diff, tab.changedLines, tab.commonLines, tab.inlineChanges, tab.deletedAfter = diff, diff.ChangedLines, diff.CommonLines, diff.InlineChanges, diff.DeletedAfter
					app.multiFile, app.split = true, tc.mode == "split"
				}
				return app
			}
			b.Run("cold", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					app := makeApp()
					app.tab().ensureHighlightCache()
					app.rebuildContent()
				}
			})
			b.Run("warm", func(b *testing.B) {
				app := makeApp()
				app.tab().ensureHighlightCache()
				app.rebuildContent()
				b.ReportAllocs()
				for b.Loop() {
					app.rebuildContent()
				}
			})
		})
	}
}
