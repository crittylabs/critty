package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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
	chromaLines      []string         // syntax-highlighted lines (nil for markdown)
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

// ensureHighlightCache populates the Chroma highlight cache if needed.
// Call this once after loading a tab's document, not on every rebuild.
func (ft *FileTab) ensureHighlightCache() {
	if ft.doc == nil || ft.chromaLines != nil {
		return
	}
	ext := strings.ToLower(filepath.Ext(ft.path))
	ft.isMarkdown = ext == ".md" || ext == ".markdown"
	lexer := codeLexer(ft.path, ft.doc.Content)
	ft.isCode = isCodeLexer(lexer)
	if ft.isMarkdown {
		ft.ensureMarkdownCache()
		return
	}
	highlight := newCodeHighlighter(lexer)
	ft.chromaLines = highlight(ft.doc.Content)

	// Pre-highlight deleted lines
	ft.deletedLineCache = make(map[int][]string)
	for afterLine, dels := range ft.deletedAfter {
		highlighted := make([]string, len(dels))
		for i, del := range dels {
			hl := highlight(del.Content)
			if len(hl) > 0 {
				highlighted[i] = hl[0]
			} else {
				highlighted[i] = del.Content
			}
		}
		ft.deletedLineCache[afterLine] = highlighted
	}
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
