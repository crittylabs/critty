package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/review"
)

// fileReview holds the in-memory comments for one tab; they are written
// back to the session's CritJSON whenever the app persists.
type fileReview struct {
	Comments []review.Comment
}

// FileTab holds per-file state for a single tab in the code review TUI.
type FileTab struct {
	preview               imagePreview
	markdownImages        []markdownImage
	markdownImagesPending bool
	attachment            *review.AttachedDocument
	path                  string
	oldPath               string
	status                string
	textconv              gitpkg.Textconv
	loadErr               error
	binaryRevision        string
	unread                bool // changed or added since this tab was last displayed
	doc                   *document.Document
	state                 *fileReview
	diff                  *gitpkg.DiffInfo // original diff, retained when whitespace is ignored
	changedLines          map[int]bool     // line numbers that are added/modified
	commonLines           map[int]int
	pairedLines           map[int]int
	inlineChanges         map[int][]gitpkg.InlineSegment
	deletedAfter          map[int][]gitpkg.DeletedLine // deleted lines keyed by new-file line they appear after
	changeChunks          []changeChunk                // contiguous groups of changed lines
	cursorLine            int                          // 1-based
	cursorSide            string                       // "old" for deleted lines; empty means new side
	contentOffset         int
	commentOffset         int

	// Visual selection mode
	selecting    bool
	selectAnchor int
	selectSide   string

	// Sidebar annotation list and cursor
	sidebarItems  []sidebarItem
	sidebarCursor int

	// Annotation focus
	cursorOnAnnotation bool
	cursorOnForm       bool
	cursorAnnoIdx      int
	expandedDrifted    map[string]bool
	foldedComments     map[string]bool

	// Placeholder tabs
	isBinary  bool
	isDeleted bool
	// outsideChanges marks a tab kept only for its comments after the
	// file left the change set in a later round.
	outsideChanges bool

	// Cached rendering data (computed once per tab switch, not per keystroke)
	highlighted      bool // language detection and highlight caches are current
	lexer            chroma.Lexer
	outlineMask      *outlineMask // masked source from the highlight pass, nil when unavailable
	chromaLines      []string     // display-ready syntax-highlighted lines (nil for markdown)
	plainLines       []string     // display text without styling, parallel to chromaLines
	displayWidths    []int        // display widths of chromaLines
	lineNumbers      []string     // default gutter rendering of each line number
	lineNumbersDark  bool
	deletedLineCache map[int][]string // pre-highlighted deleted line content, keyed by afterLine
	isMarkdown       bool
	isCode           bool
	markdown         *markdownCache
	goOutline        *goOutlineCache
	codeOutline      *codeOutlineCache
	tableOffsets     map[int]int // horizontal offsets keyed by table start line
}

// changeChunk represents a contiguous block of changed lines.
type changeChunk struct {
	startLine int
	endLine   int
}

// newFileTab creates a new FileTab from a DiffInfo.
func newFileTab(path string, diff *gitpkg.DiffInfo) FileTab {
	ft := FileTab{
		path:       path,
		cursorLine: 1,
		diff:       diff,
	}
	if diff != nil {
		ft.commonLines = diff.CommonLines
		ft.pairedLines = diff.PairedLines
		ft.changedLines = diff.ChangedLines
		ft.inlineChanges = diff.InlineChanges
		ft.deletedAfter = diff.DeletedAfter
		ft.changeChunks = computeChangeChunks(diff)
	}
	return ft
}

func (m *AppModel) saveTabViewport() {
	if len(m.tabs) == 0 {
		return
	}
	t := m.tab()
	t.contentOffset = m.contentViewport.YOffset()
	t.commentOffset = m.commentViewport.YOffset()
}

func (m *AppModel) activateTab(index int) bool {
	if index < 0 || index >= len(m.tabs) || m.activeTab == index {
		return false
	}
	m.saveTabViewport()
	m.activeTab = index
	return true
}

// positionTabViewport restores a tab for step zero, or starts directional
// navigation at its top (positive) or bottom (negative), after rendering.
func (m *AppModel) positionTabViewport(step int) {
	if len(m.tabs) == 0 {
		m.contentViewport.GotoTop()
		m.commentViewport.GotoTop()
		return
	}
	switch {
	case step > 0:
		m.contentViewport.GotoTop()
		m.commentViewport.GotoTop()
	case step < 0:
		m.contentViewport.GotoBottom()
		m.commentViewport.GotoBottom()
	default:
		m.contentViewport.SetYOffset(m.tab().contentOffset)
		m.commentViewport.SetYOffset(m.tab().commentOffset)
	}
}

// ensureHighlightCache populates the language and highlight caches if
// needed.  Call this once after loading a tab's document, not on every
// rebuild.
func (ft *FileTab) ensureHighlightCache() {
	if ft.doc == nil {
		return
	}
	if !ft.highlighted {
		ft.highlighted = true
		ft.detectLanguage()
		if ft.isMarkdown {
			ft.ensureMarkdownCache()
			return
		}
		ft.highlightSource()
	}
	if !ft.isMarkdown && ft.deletedLineCache == nil {
		ft.highlightDeleted()
	}
}

func markdownPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

// detectLanguage selects the lexer once per document so repeated highlight
// passes and the outline share it.
func (ft *FileTab) detectLanguage() {
	ft.isMarkdown = markdownPath(ft.path)
	ft.lexer = codeLexer(ft.path, ft.doc.Content)
	ft.isCode = isCodeLexer(ft.lexer)
}

// highlightSource renders the document once and keeps display-ready lines,
// their plain text and widths, and the masked source the outline reads.
func (ft *FileTab) highlightSource() {
	var mask *outlineMask
	var observers []func(chroma.Token)
	if language := outlineLanguage(ft.lexer); language != "" && language != "ruby" && ft.doc.Known == nil {
		mask = newOutlineMask(language, ft.doc.Content)
		observers = append(observers, mask.observe)
	}
	ft.chromaLines = newCodeHighlighter(ft.lexer, observers...)(ft.doc.Content)
	ft.plainLines = make([]string, len(ft.chromaLines))
	ft.displayWidths = make([]int, len(ft.chromaLines))
	for i, line := range ft.chromaLines {
		line = displaySourceText(line)
		ft.chromaLines[i], ft.displayWidths[i] = line, ansi.StringWidth(line)
		if i < len(ft.doc.Lines) {
			ft.plainLines[i] = displaySourceText(ft.doc.Lines[i])
		}
	}
	ft.outlineMask = nil
	if mask != nil {
		ft.outlineMask = mask.finish()
	}
}

func (ft *FileTab) highlightDeleted() {
	highlight := newCodeHighlighter(ft.lexer)
	ft.deletedLineCache = make(map[int][]string, len(ft.deletedAfter))
	for afterLine, dels := range ft.deletedAfter {
		ft.deletedLineCache[afterLine] = highlightDeletedLines(highlight, dels)
	}
}

// resetHighlightCache discards the highlight caches so the next
// ensureHighlightCache call recomputes them, such as after a recolor.
func (ft *FileTab) resetHighlightCache() {
	ft.highlighted = false
	ft.chromaLines, ft.plainLines, ft.displayWidths = nil, nil, nil
	ft.deletedLineCache, ft.outlineMask = nil, nil
}

// displayLineInfo returns the plain text and display width of a cached
// source line, or ok false when the line must be measured after rendering.
func (ft *FileTab) displayLineInfo(index int) (plain string, width int, ok bool) {
	if ft.isMarkdown || index >= len(ft.displayWidths) || index >= len(ft.plainLines) {
		return "", 0, false
	}
	return ft.plainLines[index], ft.displayWidths[index], true
}

// renderLineNumber caches the default gutter rendering of a line number
// until the color scheme changes.
func (ft *FileTab) renderLineNumber(n int) string {
	if ft.lineNumbersDark != terminalIsDark {
		ft.lineNumbers, ft.lineNumbersDark = nil, terminalIsDark
	}
	for len(ft.lineNumbers) <= n {
		ft.lineNumbers = append(ft.lineNumbers, lineNumStyle.Render(strconv.Itoa(len(ft.lineNumbers))))
	}
	return ft.lineNumbers[n]
}

// Deleted lines of one hunk are contiguous in the old file, so tokenise them
// together like the surrounding source: multi-line constructs keep their
// styling and each hunk costs one lexer pass.  Fall back to one pass per line
// when the lexer reflows the block.
func highlightDeletedLines(highlight func(string) []string, dels []gitpkg.DeletedLine) []string {
	contents := make([]string, len(dels))
	for i, del := range dels {
		contents[i] = del.Content
	}
	if hl := highlight(strings.Join(contents, "\n") + "\n"); len(hl) == len(dels) {
		return hl
	}
	for i, content := range contents {
		if hl := highlight(content); len(hl) > 0 {
			contents[i] = hl[0]
		}
	}
	return contents
}

func (t *FileTab) unreadMarker() string {
	if t.unread {
		return unreadDotStyle.Render("●") + " "
	}
	return "  "
}

func (t *FileTab) displayPath() string {
	if t.path == overallCommentsPath {
		return "Overall comments"
	}
	if t.attachment != nil {
		return "attachments/" + t.attachment.Name
	}
	return t.path
}

// computeChangeChunks groups contiguous changed/deleted lines into navigable chunks.
// Each chunk represents one visual change region that n/N should jump to.
func computeChangeChunks(diff *gitpkg.DiffInfo) []changeChunk {
	if diff == nil {
		return nil
	}

	// Collect all lines that are visually part of a change.
	// Added/modified lines are directly involved.
	// Deleted lines render before afterLine+1, so mark that line.
	involved := make(map[int]bool)
	for ln := range diff.ChangedLines {
		involved[ln] = true
	}
	for afterLine := range diff.DeletedAfter {
		// Deleted lines render just above afterLine+1, so that's
		// the line the cursor should land on.
		target := afterLine + 1
		if target < 1 {
			target = 1
		}
		involved[target] = true
	}

	if len(involved) == 0 {
		return nil
	}

	// Sort line numbers
	lines := make([]int, 0, len(involved))
	for ln := range involved {
		lines = append(lines, ln)
	}
	sort.Ints(lines)

	// Group strictly contiguous lines into chunks.
	var chunks []changeChunk
	start := lines[0]
	end := lines[0]
	for _, ln := range lines[1:] {
		if ln == end+1 {
			end = ln
		} else {
			chunks = append(chunks, changeChunk{startLine: start, endLine: end})
			start = ln
			end = ln
		}
	}
	chunks = append(chunks, changeChunk{startLine: start, endLine: end})

	return chunks
}

// changeSummary combines file status and line counts, or just colored markers.
func (t *FileTab) changeSummary(compact bool) string {
	if t.attachment != nil {
		return ""
	}
	var counts []string
	if !t.outsideChanges {
		switch t.status {
		case "added", "untracked":
			counts = append(counts, tabAddedCount.Render("A"))
		case "deleted":
			counts = append(counts, tabDeletedCount.Render("D"))
		default:
			if t.isBinary {
				marker := "M"
				if t.status == "renamed" {
					marker = "R"
				}
				counts = append(counts, tabCountDelimiter.Foreground(warning).Render(marker))
			}
		}
	}
	if t.isBinary {
		if compact {
			return strings.Join(counts, "")
		}
		return tabCountDelimiter.Render("(") + strings.Join(counts, " ") + tabCountDelimiter.Render(")")
	}
	if n := len(t.changedLines); n > 0 {
		text := "+"
		if !compact {
			text = fmt.Sprintf("+%d", n)
		}
		counts = append(counts, tabAddedCount.Render(text))
	}
	deleted := 0
	for _, lines := range t.deletedAfter {
		deleted += len(lines)
	}
	if deleted > 0 {
		text := "-"
		if !compact {
			text = fmt.Sprintf("-%d", deleted)
		}
		counts = append(counts, tabDeletedCount.Render(text))
	}
	if len(counts) == 0 {
		return ""
	}
	if compact {
		return strings.Join(counts, "")
	}
	return tabCountDelimiter.Render("(") + strings.Join(counts, " ") + tabCountDelimiter.Render(")")
}
