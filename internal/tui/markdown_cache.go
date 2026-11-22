package tui

import (
	"github.com/crittylabs/critty/v2/internal/document"
	gitpkg "github.com/crittylabs/critty/v2/internal/git"
)

type markdownCache struct {
	doc     *document.Document
	diff    *gitpkg.DiffInfo
	content string
	partial bool
	after   *markdownSource
	before  *markdownSource
}

func (t *FileTab) ensureMarkdownCache() {
	if t.doc == nil || !t.isMarkdown || t.markdown != nil && t.markdown.doc == t.doc && t.markdown.diff == t.diff && t.markdown.content == t.doc.Content && t.markdown.partial == (t.doc.Known != nil) {
		return
	}
	cache := &markdownCache{doc: t.doc, diff: t.diff, content: t.doc.Content, partial: t.doc.Known != nil, after: parseMarkdownWithSyntax(t.doc.Lines, t.doc.Known, t.syntax, t.symbols)}
	if t.diff != nil {
		oldLines := t.beforeSourceLines()
		last := 0
		for n := range oldLines {
			last = max(last, n)
		}
		lines := make([]string, last)
		var known map[int]bool
		if t.doc.Known != nil {
			known = make(map[int]bool)
		}
		for n, line := range oldLines {
			lines[n-1] = line
			if known != nil {
				known[n] = true
			}
		}
		cache.before = parseMarkdownWithSyntax(lines, known, t.syntax, t.symbols)
	}
	t.markdown = cache
}

func (t *FileTab) markdownDisplayLine(line int, before bool, fallback string, inline []gitpkg.InlineSegment, images []linkSpan) string {
	t.ensureMarkdownCache()
	var source *markdownSource
	if t.markdown != nil {
		source = t.markdown.after
		if before {
			source = t.markdown.before
		}
	}
	base, changed := diffCommonTextBg, diffAddedTextBg
	if before {
		changed = diffDeletedTextBg
	}
	if source != nil && line > 0 && line <= len(source.lines) && source.lines[line-1].source == fallback {
		return source.lines[line-1].render(inline, base, changed, images)
	}
	// Standalone deleted lines and test fixtures may have no document cache.
	source = parseMarkdownWithSyntax([]string{fallback}, nil, t.syntax, t.symbols)
	return source.lines[0].render(inline, base, changed, images)
}

// markdownDisplayLineInfo renders a document line and also returns its plain
// text and display width when the cached rendering was reused.
func (t *FileTab) markdownDisplayLineInfo(line int, before bool, fallback string, inline []gitpkg.InlineSegment, images []linkSpan) (display, plain string, width int, measured bool) {
	display = t.markdownDisplayLine(line, before, fallback, inline, images)
	if t.markdown != nil {
		source := t.markdown.after
		if before {
			source = t.markdown.before
		}
		if source != nil && line > 0 && line <= len(source.lines) {
			if cached := source.lines[line-1].cacheSlot(inlineKey(inline)); cached.set && cached.text == display {
				return display, cached.plain, cached.width, true
			}
		}
	}
	return display, "", 0, false
}

func (t *FileTab) markdownTableCells(line int, links []linkSpan) []string {
	return t.markdownTableCellsWithInline(line, links, nil)
}

func (t *FileTab) markdownTableCellsWithInline(line int, links []linkSpan, inline []gitpkg.InlineSegment) []string {
	t.ensureMarkdownCache()
	return t.markdown.after.lines[line-1].renderTableCells(inline, diffCommonTextBg, diffAddedTextBg, links)
}
