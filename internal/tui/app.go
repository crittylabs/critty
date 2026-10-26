package tui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/form"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

type pane int

const (
	contentPane pane = iota
	commentPane
)

type modalType int

const (
	noModal modalType = iota
	commentModal
	fileCommentModal
	replyModal
	editModal
	discardChangesModal
	deleteConfirmModal
	finishModal
	finishResolveModal
	finishFormsModal
	finishAbortModal
	helpModal
	gotoLineModal
	openSourceModal
	fileSelectModal
	lightboxModal
	formTextModal
	formListModal
)

// FinishEvent is emitted on the finish channel when the reviewer finishes a
// round.  The runner turns it into a payload for blocked agent clients.
type FinishEvent struct {
	Approved bool
	Result   *review.FormResult
	Files    []gitpkg.FileChange
}

// AppConfig carries the cross-cutting dependencies of the TUI.
type AppConfig struct {
	Context          context.Context
	ReadOnly         bool
	Documents        []review.AttachedDocument
	DocumentSnapshot *review.SnapshotFile
	Project          string
	Topic            string
	Version          string
	Session          *review.Session
	Author           string
	Split            bool
	Watch            bool
	// Staged reads code-review files and diffs from the Git index.
	Staged    bool
	Patch     *gitpkg.Patch
	Source    *gitpkg.ReviewSource
	FinishCh  chan<- FinishEvent
	FocusPath string
	FocusLine int
	// Answers labels Comment as Confirm answers and selects it by default.
	Answers bool
}

// gutterWidth is the total width of the left gutter: line number (5) + marker (1) + space (1).
const gutterWidth = 7

const displayTabWidth = 4

type AppModel struct {
	textSelection contentTextSelection
	readOnly      bool
	reload        reloadState
	images        imageDisplay
	commentImages map[commentImageKey]*previewSide
	imageBodies   map[string][]markdownImageRef
	editorImages  editorImageState
	lightbox      lightboxState
	forms         *review.FormState
	formRows      map[string]map[int]formRow
	formViewed    map[string]map[int]bool
	// formTargets maps select-all IDs to item indices by document path.
	formTargets   map[string]map[string][]int
	formList      formListState
	answerMode    bool
	attachments   map[string]review.AttachedDocument
	project       string
	topic         string
	version       string
	width, height int
	focused       pane
	modal         modalType

	// Multi-file tabs (code review mode)
	tabs       []FileTab
	activeTab  int
	multiFile  bool // true when in code review mode
	fileSelect fileSelectState

	// Single-file mode (legacy)
	filePath         string
	documentSnapshot *review.SnapshotFile

	// Review session backing all tabs, and the author stamped on new comments.
	session          *review.Session
	author           string
	authorColors     map[string]int
	threadScrolls    map[threadViewKey]threadScroll
	viewMode         bool
	openAll          bool
	ignoreWhitespace bool
	split            bool
	viewAttention    *diffAttention
	previousReplyIDs []string // submission baseline pending initial window dimensions
	initialFocus     *sourceLocation

	// Finish-flow state (see AppConfig).
	finishCh chan<- FinishEvent
	baseRef  string
	staged   bool
	patch    *gitpkg.Patch
	source   *gitpkg.ReviewSource

	host string // multiplexer hosting an agent-launched review, if any

	contentViewport   viewport.Model
	commentViewport   viewport.Model
	sidebarView       sidebarView
	fileTree          fileTree
	outline           documentOutline
	modalTextarea     textarea.Model
	clipboardID       uint64
	clipboardPending  bool
	clipboardStatus   string
	killRing          killRing
	completion        completionState
	pathInfo          formPathState
	mouseSelecting    bool
	rankDrag          rankDragState
	hoveredGutterLine int
	hoveredGutterSide string
	contentLayout     renderedContentLayout
	sidebarTargets    []int
	sidebarActions    []commentHeaderRegion

	// Editing state
	editingID            string // ID of the parent comment being edited or replied to
	editingReplyID       string // ID of the reply being edited; empty when editing the parent
	pendingFormAnswer    *form.Answer
	editingFormLine      int    // line of the form item whose text is being edited
	modalInitial         string // textarea value when the current text modal opened
	modalOpenedAt        time.Time
	modalOpeningClick    tea.Mouse
	modalReferenceOffset int
	helpOffset           int
	discardReturn        modalType
	deleteReturn         modalType
	pendingDelete        int
	modalFocus           int  // focus index within the active modal
	newFeedback          bool // true after adding or editing a comment in this round
	finishSaveFailed     bool
	finishDraft          string
	finishVerdict        review.Verdict
	lineInput            textinput.Model
	pendingLocation      sourceLocation
	locationError        string
	notice               footerNotice
	isearch              isearchState

	err error
}

type renderedRange struct {
	start int
	end   int
}

type mouseRect struct {
	left   int
	top    int
	right  int
	bottom int
}

func (r mouseRect) contains(mouse tea.Mouse) bool {
	return mouse.X >= r.left && mouse.X < r.right && mouse.Y >= r.top && mouse.Y < r.bottom
}

type renderedContentLayout struct {
	tables       []tableViewport
	textSpans    []contentTextSpan
	columns      []diffMouseRow
	formControls []formControl
	rows         []contentMouseTarget
	lineRanges   map[int]renderedRange
	oldRanges    map[int]renderedRange
	actions      []commentHeaderRegion
	markers      []commentMarker
}

type commentHeaderRegion struct {
	resolve mouseRect
	up      mouseRect
	down    mouseRect
	delete  mouseRect
	id      string
}

func (r *commentHeaderRegion) translate(x, y int) {
	for _, rect := range []*mouseRect{&r.resolve, &r.up, &r.down, &r.delete} {
		rect.left += x
		rect.right += x
		rect.top += y
		rect.bottom += y
	}
}

type renderedScreenLayout struct {
	watchIndicator      mouseRect
	hasWatchIndicator   bool
	copyTextButton      mouseRect
	hasCopyTextButton   bool
	whitespaceButton    mouseRect
	hasWhitespaceButton bool
	splitButton         mouseRect
	hasSplitButton      bool
	finishButton        mouseRect
	hasFinishButton     bool
	footerHelp          mouseRect
	hasFooterHelp       bool
	formListButton      mouseRect
	hasFormListButton   bool
	imagesButton        mouseRect
	hasImagesButton     bool
	modalRegions        []modalMouseRegion
	isearchCursor       *tea.Cursor
	isearchRegions      []isearchMouseRegion
	progressRegions     []progressRegion
}

func newRenderedContentLayout() renderedContentLayout {
	return renderedContentLayout{
		lineRanges: make(map[int]renderedRange),
		oldRanges:  make(map[int]renderedRange),
	}
}

func (l *renderedContentLayout) appendBlock(b *strings.Builder, block string, target contentMouseTarget) {
	block = strings.TrimSuffix(block, "\n")
	start := len(l.rows)
	for _, row := range strings.Split(block, "\n") {
		b.WriteString(row)
		b.WriteByte('\n')
		l.rows = append(l.rows, target)
	}
	if target.passive || target.line <= 0 && !target.annotation {
		return
	}
	ranges := l.lineRanges
	if target.side == "old" {
		ranges = l.oldRanges
	}
	r, ok := ranges[target.line]
	if !ok {
		r.start = start
	}
	r.end = len(l.rows)
	ranges[target.line] = r
}

// tab returns the active FileTab. Panics if no tabs exist.
func (m *AppModel) tab() *FileTab {
	return &m.tabs[m.activeTab]
}

func NewApp(filePath string, cfg AppConfig) AppModel {
	if cfg.ReadOnly {
		cfg.Session = nil
	}
	ta := textarea.New()
	ta.KeyMap = modalTextareaKeyMap()
	ta.SetVirtualCursor(false)
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false

	tab := FileTab{
		path:       filePath,
		cursorLine: 1,
		isBinary:   imagePath(filePath),
	}

	m := AppModel{
		readOnly:         cfg.ReadOnly,
		isearch:          isearchState{isearchOptions: isearchOptions{allFiles: true}},
		reload:           reloadState{watch: cfg.Watch, context: cfg.Context},
		images:           newImageDisplay(),
		project:          cfg.Project,
		topic:            cfg.Topic,
		version:          cfg.Version,
		filePath:         filePath,
		documentSnapshot: cfg.DocumentSnapshot,
		tabs:             []FileTab{tab},
		activeTab:        0,
		session:          cfg.Session,
		author:           cfg.Author,
		answerMode:       cfg.Answers,
		authorColors:     make(map[string]int),
		finishCh:         cfg.FinishCh,
		host:             reviewHost(),
		initialFocus:     startupLocation(cfg),
		contentViewport:  viewport.New(),
		commentViewport:  viewport.New(),
		fileTree:         newFileTree(),
		outline:          newDocumentOutline(),
		modalTextarea:    ta,
	}
	m.addDocuments(cfg.Documents)
	m.ensureOverallCommentsTab()
	m.orderTabsByTree()
	m.loadBinaryRevisions()
	return m
}

// NewCodeReviewApp creates a multi-file code review TUI.
func NewCodeReviewApp(files []gitpkg.FileChange, ref string, cfg AppConfig) AppModel {
	if cfg.ReadOnly {
		cfg.Session = nil
	}
	ta := textarea.New()
	ta.KeyMap = modalTextareaKeyMap()
	ta.SetVirtualCursor(false)
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false

	tabs := make([]FileTab, 0, len(files))
	for _, f := range files {
		var diff *gitpkg.DiffInfo
		var diffErr error
		if cfg.Patch != nil {
			if pf := cfg.Patch.File(f.Path); pf != nil {
				diff = pf.Diff
			}
		} else if !f.IsBinary() {
			if cfg.Source != nil {
				diff, diffErr = cfg.Source.Diff(f.Path, f.OldPath)
			} else {
				diff, _ = codeDiff(f.Path, f.OldPath, ref, cfg.Staged)
			}
		}
		ft := newFileTab(f.Path, diff)
		ft.oldPath = f.OldPath
		ft.textconv = f.Textconv
		ft.loadErr = diffErr
		ft.status = f.Status.String()
		ft.isBinary = f.IsBinary() || cfg.Patch == nil && imagePath(f.Path) && f.Textconv.Command == ""
		if f.Status == gitpkg.StatusBinary {
			ft.status = "modified"
		}
		if f.Status == gitpkg.StatusDeleted {
			ft.isDeleted = true
		}
		tabs = append(tabs, ft)
	}

	if cfg.Session != nil {
		present := make(map[string]bool, len(tabs))
		for _, tab := range tabs {
			present[tab.path] = true
		}
		for _, doc := range cfg.Documents {
			present[doc.Path()] = true
		}
		for path, file := range cfg.Session.CJ.Files {
			snapshot, hasSnapshot := cfg.Session.CJ.RoundState.Files[path]
			if !present[path] && (len(file.Comments) > 0 || len(files) == 0 && hasSnapshot && !snapshot.Partial) {
				tab := newFileTab(path, nil)
				tab.outsideChanges = true
				if hasSnapshot && !snapshot.Partial {
					tab.doc = document.FromContent(path, []byte(snapshot.Content))
					tab.ensureHighlightCache()
				}
				tabs = append(tabs, tab)
			}
		}
	}

	m := AppModel{
		readOnly:        cfg.ReadOnly,
		isearch:         isearchState{isearchOptions: isearchOptions{allFiles: true}},
		reload:          reloadState{watch: cfg.Watch, context: cfg.Context},
		images:          newImageDisplay(),
		project:         cfg.Project,
		topic:           cfg.Topic,
		version:         cfg.Version,
		tabs:            tabs,
		activeTab:       0,
		multiFile:       true,
		split:           cfg.Split,
		session:         cfg.Session,
		author:          cfg.Author,
		answerMode:      cfg.Answers,
		authorColors:    make(map[string]int),
		finishCh:        cfg.FinishCh,
		baseRef:         ref,
		staged:          cfg.Staged,
		patch:           cfg.Patch,
		source:          cfg.Source,
		host:            reviewHost(),
		initialFocus:    startupLocation(cfg),
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		fileTree:        newFileTree(),
		outline:         newDocumentOutline(),
		modalTextarea:   ta,
	}
	m.addDocuments(cfg.Documents)
	m.ensureOverallCommentsTab()
	m.orderTabsByTree()
	m.loadBinaryRevisions()
	return m
}

func (m *AppModel) addDocuments(docs []review.AttachedDocument) {
	var active, removed []FileTab
	m.attachments = make(map[string]review.AttachedDocument, len(docs))
	for _, d := range docs {
		if d.Removed && (m.session == nil || len(m.session.FileComments(d.Path())) == 0) {
			continue
		}
		tab := newFileTab(d.Path(), nil)
		m.attachments[d.Path()] = d
		tab.attachment = &d
		tab.isBinary = d.Image != ""
		tab.binaryRevision = d.Revision
		if d.Removed {
			removed = append(removed, tab)
		} else {
			active = append(active, tab)
		}
	}
	if len(active)+len(removed) == 0 {
		return
	}
	m.tabs = append(append(active, m.tabs...), removed...)
	m.multiFile = true
}

func (m AppModel) Init() tea.Cmd {
	var tick tea.Cmd
	if m.reload.watch && m.CanWatch() {
		tick = m.startWatch()
	}
	return tea.Batch(m.loadDocuments(), tea.RequestBackgroundColor, tick)
}

func (m AppModel) loadDocuments() tea.Cmd {
	return func() tea.Msg {
		for _, tab := range m.tabs {
			if tab.loadErr != nil {
				return errMsg{tab.loadErr}
			}
			if tab.isBinary || tab.isDeleted || tab.outsideChanges {
				continue
			}
			if _, err := m.loadDocument(tab.path); err != nil {
				return errMsg{err}
			}
		}
		return docRenderedMsg{}
	}
}

func (m AppModel) loadDocument(path string) (*document.Document, error) {
	if path == m.filePath && m.documentSnapshot != nil {
		return document.FromContent(path, []byte(m.documentSnapshot.Content)), nil
	}
	if path == overallCommentsPath {
		return overallCommentsDocument(), nil
	}
	if d, ok := m.attachments[path]; ok {
		return document.FromContent(path, []byte(d.Content)), nil
	}
	if m.source != nil {
		var conversion gitpkg.Textconv
		for _, tab := range m.tabs {
			if tab.path == path {
				conversion = tab.textconv
				break
			}
		}
		content, err := m.source.TextContent(path, conversion)
		if err != nil {
			return nil, err
		}
		return document.FromContent(path, content), nil
	}
	if m.patch != nil {
		if f := m.patch.File(path); f != nil {
			doc := document.FromContent(path, []byte(f.Content))
			doc.Known = f.Known
			return doc, nil
		}
		return nil, fmt.Errorf("file absent from diff: %s", path)
	}
	if !m.staged {
		return document.Load(path)
	}
	content, err := gitpkg.FileContentFromIndex(path)
	if err != nil {
		return nil, err
	}
	return document.FromContent(path, content), nil
}

func codeDiff(path, oldPath, ref string, staged bool) (*gitpkg.DiffInfo, error) {
	if staged {
		return gitpkg.DiffFileStaged(path, oldPath)
	}
	return gitpkg.DiffFile(path, oldPath, ref)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	emptyBefore := m.modal == finishModal && m.modalFocus == finishText && m.modalTextarea.Value() == ""
	previousHeaderHeight := m.headerHeight()
	model, cmd := m.update(msg)
	switch app := model.(type) {
	case AppModel:
		app.updateFinishVerdictOnInput(emptyBefore)
		app.refreshCompletion()
		app.refreshHeaderLayout(previousHeaderHeight)
		app.markVisibleForms()
		reloadCmd := app.resumeReload()
		return app, tea.Batch(cmd, reloadCmd, app.recheckImagesOnSelection(m.activeTab, msg), app.prepareImage(),
			app.scheduleEditorImages(), app.prepareCommentImages(), app.prepareLightbox())
	case *AppModel:
		app.updateFinishVerdictOnInput(emptyBefore)
		app.refreshCompletion()
		app.refreshHeaderLayout(previousHeaderHeight)
		app.markVisibleForms()
		reloadCmd := app.resumeReload()
		return app, tea.Batch(cmd, reloadCmd, app.recheckImagesOnSelection(m.activeTab, msg), app.prepareImage(),
			app.scheduleEditorImages(), app.prepareCommentImages(), app.prepareLightbox())
	}
	return model, cmd
}

func (m *AppModel) refreshHeaderLayout(previousHeight int) {
	if len(m.tabs) > 0 && m.width > 0 && m.headerHeight() != previousHeight {
		m.recalculateLayout()
		m.rebuildContent()
		m.scrollToCursor()
	}
}

func (m AppModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msg, cmd := m.images.decodeResponse(msg)
	if msg == nil {
		return m, cmd
	}
	if replay, ok := msg.(imageResponseReplayMsg); ok {
		var commands []tea.Cmd
		for _, key := range replay.keys {
			model, cmd := m.updateMessage(key)
			switch app := model.(type) {
			case AppModel:
				m = app
			case *AppModel:
				m = *app
			}
			commands = append(commands, cmd)
		}
		return m, tea.Batch(commands...)
	}
	return m.updateMessage(msg)
}

func (m AppModel) updateMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.clipboardPending {
		switch input := msg.(type) {
		case tea.KeyPressMsg:
			if input.String() == "esc" || input.String() == "ctrl+c" {
				m.clipboardPending = false
				m.clipboardStatus = ""
			}
			return m, nil
		case tea.PasteMsg, tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent, tea.CursorPositionMsg, imageProbeMsg, imageProbeTimeoutMsg:
		return m.updateImageCapability(msg)
	case imageProbeSkippedMsg:
		// The pane may be selected moments later, as a launcher focuses it
		// after creation, so the pending timeout retries like an unanswered
		// query instead of ending the check.
		if msg.id == m.images.probeID {
			m.images.skipped = true
		}
		return m, nil
	case imageLoadedMsg:
		return m.finishImageLoad(msg)
	case imageInfoMsg:
		for i := range m.tabs {
			if m.tabs[i].path == msg.path {
				m.tabs[i].preview.side(msg.before).info = msg.info
			}
		}
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil
	case imageTransmittedMsg:
		if msg.comment != nil {
			if side := m.commentImages[*msg.comment]; side != nil {
				side.image = &inlineImage{id: msg.id, width: msg.width, height: msg.height}
			}
			return m, nil
		}
		for i := range m.tabs {
			if m.tabs[i].path == msg.path {
				m.tabs[i].imageSide(msg.before, msg.markdown).image = &inlineImage{id: msg.id, width: msg.width, height: msg.height}
			}
		}
		return m, nil
	case imageReadyMsg:
		if msg.comment != nil {
			if side := m.commentImages[*msg.comment]; side != nil && side.image != nil {
				side.image.ready = side.image.columns == msg.columns && side.image.rows == msg.rows
			}
			m.rebuildContent()
			m.updateCommentSidebar()
			return m, nil
		}
		for i := range m.tabs {
			if m.tabs[i].path != msg.path {
				continue
			}
			img := m.tabs[i].imageSide(msg.before, msg.markdown).image
			if img != nil && img.columns == msg.columns && img.rows == msg.rows {
				img.ready = true
			}
		}
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil
	case markdownImagesMsg:
		for i := range m.tabs {
			if m.tabs[i].path == msg.path {
				m.tabs[i].markdownImages = msg.images
			}
		}
		return m, nil
	case lightboxReadyMsg:
		m.finishLightboxPlacement(msg)
		return m, nil
	case lightboxLoadedMsg:
		return m, m.finishLightboxLoad(msg)
	case editorImagesMsg:
		m.finishEditorImagesDelay(msg)
		return m, nil
	case noticeExpiredMsg:
		m.expireNotice(msg)
		return m, nil
	case externalOpenMsg:
		m.locationError = msg.status
		return m, nil
	case watchReadyMsg:
		if msg.epoch != m.reload.epoch || !m.reload.watch {
			if msg.watcher != nil {
				msg.watcher.Close()
			}
			return m, nil
		}
		if msg.err != nil {
			m.reload.watch, m.reload.err = false, msg.err
			return m, nil
		}
		m.reload.watcher = msg.watcher
		m.reload.dirty = true
		return m, msg.watcher.next(msg.epoch)
	case watchChangedMsg:
		if !m.reload.watch || msg.epoch != m.reload.epoch {
			return m, nil
		}
		if msg.err != nil {
			m.reload.watch, m.reload.err = false, msg.err
			m.reload.watcher.Close()
			return m, nil
		}
		if msg.changed {
			m.reload.dirty = true
		}
		if msg.refresh {
			m.reload.watcher.Close()
			m.reload.watcher = nil
			return m, m.startWatch()
		}
		return m, m.reload.watcher.next(msg.epoch)
	case reloadMsg:
		cmd := m.finishReload(msg)
		return m, cmd
	case tea.BackgroundColorMsg:
		recolor := msg.IsDark() != terminalIsDark
		initAdaptiveStyles(msg.IsDark())
		for i := range m.tabs {
			if recolor && m.tabs[i].chromaLines != nil {
				m.tabs[i].chromaLines, m.tabs[i].deletedLineCache = nil, nil
				m.tabs[i].ensureHighlightCache()
			}
		}
		m.modalTextarea.SetStyles(textarea.DefaultStyles(msg.IsDark()))
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
			m.updateCommentSidebar()
		}
		return m, nil

	case tea.WindowSizeMsg:
		attention := m.captureDiffAttention()
		m.width = msg.Width
		m.height = msg.Height
		m.images.cell = cellPixelSize()
		m.recalculateLayout()
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
			m.updateCommentSidebar()
			m.restoreDiffAttention(attention)
			m.focusStartupLocation()
			m.focusNewReply()
			if m.isearch.active && m.isearch.found {
				m.revealTableMatch()
			}
		}
		return m, nil

	case docRenderedMsg:
		if err := m.restoreRound(); err != nil {
			m.err = err
			return m, nil
		}
		// Load documents and existing review comments for each tab
		for i := range m.tabs {
			t := &m.tabs[i]
			t.state = &fileReview{Comments: m.sessionComments(t.path)}
			if t.isBinary {
				continue
			}
			if t.outsideChanges {
				continue
			}
			if t.isDeleted {
				t.doc = &document.Document{Path: t.path}
				t.ensureHighlightCache()
				if lines := m.visualLines(t); len(lines) > 0 {
					t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
				}
				continue
			}
			doc, _ := m.loadDocument(t.path)
			t.doc = doc
			t.ensureHighlightCache()
			if m.patch != nil || m.alternateDiff(t) {
				if lines := m.visualLines(t); len(lines) > 0 {
					t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
				}
			}
		}

		m.recalculateLayout()
		m.rebuildContent()
		m.updateCommentSidebar()
		m.focusStartupLocation()
		m.focusNewReply()
		return m, nil

	case sourceEditorReadyMsg:
		if msg.err != nil {
			m.locationError = msg.err.Error()
			return m, nil
		}
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg { return sourceEditorFinishedMsg{err: err} })

	case sourceEditorFinishedMsg:
		if msg.err != nil {
			m.locationError = fmt.Sprintf("running $EDITOR: %v", msg.err)
		}
		return m, nil

	case clipboardImageMsg:
		cmd := m.finishClipboardImage(msg)
		return m, cmd

	case editorFinishedMsg:
		m.killRing.interrupt()
		m.finishExternalEdit(msg)
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.MouseClickMsg:
		m.killRing.interrupt()
		if m.handleISearchMouse(msg.Mouse()) {
			return m, nil
		}
		if m.isearch.active && m.modal == noModal && msg.Mouse().Button == tea.MouseLeft {
			m.exitISearch(false)
		}
		return m.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		m.killRing.interrupt()
		return m.handleMouseMotion(msg)
	case tea.MouseReleaseMsg:
		m.killRing.interrupt()
		return m.handleMouseRelease(msg)
	case tea.MouseWheelMsg:
		m.killRing.interrupt()
		return m.handleMouseWheel(msg)

	case tea.KeyPressMsg:
		kill, _ := killDirection(msg, m.modalTextarea.KeyMap)
		if !m.isTextModal() || m.modalFocus != 0 || (!kill && msg.String() != "ctrl+y" && msg.String() != "alt+y" && msg.String() != "alt+w") {
			m.killRing.interrupt()
		}
		return m.handleKeyPress(msg)
	case tea.PasteMsg:
		if m.isearch.active && m.modal == noModal {
			m.killRing.interrupt()
			m.isearchPaste(msg.Content)
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.modal == gotoLineModal {
		m.lineInput, cmd = m.lineInput.Update(msg)
		return m, cmd
	}
	if m.modal == fileSelectModal {
		m.fileSelect.input, cmd = m.fileSelect.input.Update(msg)
		m.refreshFileSelect()
		return m, cmd
	}
	if m.isTextModal() && m.modalFocus == 0 {
		before := m.modalTextarea.Value()
		cmd = m.updateModalTextarea(msg)
		if _, pasted := msg.(tea.PasteMsg); pasted || m.modalTextarea.Value() != before {
			m.killRing.interrupt()
		}
		return m, cmd
	}

	switch m.focused {
	case contentPane:
		m.contentViewport, cmd = m.contentViewport.Update(msg)
	case commentPane:
		if m.sidebarView == filesView {
			m.fileTree.viewport, cmd = m.fileTree.viewport.Update(msg)
		} else {
			m.commentViewport, cmd = m.commentViewport.Update(msg)
		}
	}

	return m, cmd
}

func (m *AppModel) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if m.finishSaveFailed && (msg.String() == "enter" || msg.String() == "y") {
			return m.doFinish()
		}
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.finishSaveFailed && msg.String() == "esc" {
			m.err, m.finishSaveFailed, m.modal = nil, false, noModal
		}
		return m, nil
	}
	if m.modal == gotoLineModal || m.modal == openSourceModal {
		return m.handleLocationModal(msg)
	}
	if m.modal == fileSelectModal {
		return m.handleFileSelectModal(msg)
	}
	if m.modal == formListModal {
		return m.handleFormListKey(msg)
	}
	m.locationError = ""
	if m.modal == lightboxModal {
		return m.handleLightboxKey(msg)
	}
	if m.hoveredGutterLine != 0 {
		m.hoveredGutterLine = 0
		m.hoveredGutterSide = ""
		m.rebuildContent()
	}
	if m.modal == helpModal {
		if key.Matches(msg, keys.Help) || key.Matches(msg, keys.Cancel) {
			m.modal = noModal
		} else {
			switch msg.String() {
			case "up", "k":
				m.scrollHelp(-1)
			case "down", "j":
				m.scrollHelp(1)
			case "pgup", "ctrl+u":
				m.scrollHelp(-max(1, m.height/2))
			case "pgdown", "ctrl+d":
				m.scrollHelp(max(1, m.height/2))
			case "home", "g":
				m.helpOffset = 0
			case "end", "G":
				m.scrollHelp(len(strings.Split(m.renderHelp(m.modalInnerWidth()), "\n")))
			}
		}
		return m, nil
	}
	if m.modal == discardChangesModal {
		return m.handleDiscardChangesModal(msg)
	}
	if m.modal == deleteConfirmModal {
		return m.handleDeleteConfirmModal(msg)
	}
	if m.modal == commentModal || m.modal == fileCommentModal || m.modal == replyModal || m.modal == editModal || m.modal == formTextModal {
		return m.handleTextModal(msg)
	}
	if m.modal == finishModal {
		return m.handleFinishModal(msg)
	}
	if m.modal == finishResolveModal || m.modal == finishAbortModal {
		return m.handleFinishConfirmationKey(msg)
	}
	if m.modal == finishFormsModal {
		return m.handleFinishFormsKey(msg)
	}

	if m.modal == noModal && !m.isearch.active {
		switch {
		case key.Matches(msg, keys.Reload):
			return m, m.requestReload(true)
		case key.Matches(msg, keys.Watch):
			return m, m.toggleWatch()
		}
	}
	if len(m.tabs) == 0 {
		if key.Matches(msg, keys.Quit) {
			if m.readOnly {
				return m, tea.Quit
			}
			m.openFinishModal()
		}
		return m, nil
	}

	t := m.tab()
	if m.textSelection.active && msg.String() != "alt+w" && !key.Matches(msg, keys.Cancel) {
		m.clearTextSelection()
	}
	if m.isearch.active && m.handleISearchKey(msg) {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+s", "/", "ctrl+r", "?":
		if t.doc != nil {
			m.startISearch(msg.String() == "ctrl+s" || msg.String() == "/")
			return m, nil
		}
	}
	if msg.String() == "space" || msg.String() == " " {
		if m.focused == contentPane && !t.selecting && !t.cursorOnAnnotation && t.cursorSide == "" && m.toggleForm(t.cursorLine) {
			return m, nil
		}
	}
	if direction := map[string]int{"alt+up": -1, "alt+down": 1}[msg.String()]; direction != 0 {
		if m.focused == contentPane && !t.selecting && !t.cursorOnAnnotation && t.cursorSide == "" && m.moveRank(t.cursorLine, direction) {
			return m, nil
		}
	}
	if msg.String() == "alt+e" {
		m.locationError = ""
		return m, m.openSourceEditor(sourceLocation{path: t.path, line: max(1, t.cursorLine)})
	}
	if msg.String() == "alt+g" {
		return m, m.openGotoLine()
	}
	if msg.String() == "alt+G" || msg.String() == "alt+shift+g" || msg.String() == "alt+shift+G" {
		m.focusOutline()
		return m, nil
	}
	if msg.String() == "alt+w" {
		if m.focused == contentPane && m.hasTextSelection() {
			return m, m.copySelectedText()
		}
		cmd := m.copyReference()
		return m, cmd
	}
	if (msg.String() == "o" || msg.String() == "alt+o") && !t.selecting {
		return m, m.openReferenceAtCursor()
	}
	if msg.String() == "alt+p" && m.multiFile {
		return m, m.openFileSelect()
	}
	if key.Matches(msg, keys.FileTree) && !t.selecting {
		m.toggleSidebarView()
		return m, nil
	}
	if m.focused == commentPane && m.sidebarView == filesView && !t.selecting && m.handleFilesSidebarKey(msg) {
		return m, nil
	}
	if m.focused == contentPane && !t.cursorOnAnnotation && t.cursorSide == "" &&
		(msg.String() == "left" || msg.String() == "right" || msg.String() == "h" || msg.String() == "l") {
		if table := m.tableAtLine(t.cursorLine); table != nil {
			direction := 1
			if msg.String() == "left" || msg.String() == "h" {
				direction = -1
			}
			m.scrollTable(*table, direction*max(1, table.window.width/2))
			return m, nil
		}
	}

	if msg.String() == "ctrl+pgup" || msg.String() == "ctrl+pgdown" {
		direction := 1
		if msg.String() == "ctrl+pgup" {
			direction = -1
		}
		if id := m.selectedCommentID(); id != "" {
			m.scrollThread(threadViewKey{id: id, sidebar: m.focused == commentPane}, direction, true)
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Quit):
		if m.readOnly {
			return m, tea.Quit
		}
		// Finishing is an explicit act: q opens the Approve/Finish modal.
		m.openFinishModal()
		return m, nil

	case key.Matches(msg, keys.Cancel):
		// Esc cancels selection
		if m.textSelection.active {
			m.clearTextSelection()
			return m, nil
		}
		if t.selecting {
			t.selecting = false
			m.rebuildContent()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Help):
		m.helpOffset = 0
		m.modal = helpModal
		return m, nil

	case key.Matches(msg, keys.IgnoreWhitespace):
		if m.multiFile {
			m.toggleWhitespace()
		}
		return m, nil
	case key.Matches(msg, keys.Split):
		m.toggleSplit()
		return m, nil
	case (msg.String() == "left" || msg.String() == "right") && m.focused == contentPane && m.alternateDiff(t):
		m.moveDiffSide(msg.String() == "left")
		return m, nil
	case key.Matches(msg, keys.Images):
		cmd := m.toggleImages()
		return m, cmd

	case key.Matches(msg, keys.CommentDisplay):
		m.toggleCommentView()
		return m, nil
	case key.Matches(msg, keys.OpenAll):
		m.toggleOpenAll()
		return m, nil

	case key.Matches(msg, keys.Tab):
		if !t.selecting {
			if m.viewMode {
				m.viewMode = false
				m.recalculateLayout()
			}
			if m.focused == contentPane {
				m.focused = commentPane
			} else {
				m.focused = contentPane
			}
			m.updateCommentSidebar()
			m.rebuildContent()
		}
		return m, nil

	case key.Matches(msg, keys.VisualMode):
		if m.focused == contentPane && t.doc != nil && t.cursorLine > 0 {
			if t.selecting {
				t.selecting = false
			} else {
				t.selecting = true
				t.selectAnchor = t.cursorLine
				t.selectSide = t.cursorSide
				t.cursorOnAnnotation = false
				t.cursorAnnoIdx = 0
			}
			m.rebuildContent()
			return m, nil
		}

	case key.Matches(msg, keys.FormList):
		m.openFormList()
		return m, nil

	case key.Matches(msg, keys.FileComment):
		if m.readOnly {
			return m, nil
		}
		if !t.selecting && t.state != nil {
			for _, c := range t.state.Comments {
				if isGeneralCommentScope(c.Scope) {
					m.editingID = c.ID
					m.editingReplyID = ""
					m.modalReferenceOffset = -1
					m.modal = replyModal
					m.modalFocus = 0
					m.modalTextarea.Placeholder = "Write a reply..."
					m.modalTextarea.Reset()
					m.modalInitial = ""
					m.modalTextarea.Focus()
					return m, nil
				}
			}
			m.modal = fileCommentModal
			m.modalFocus = 0
			m.modalTextarea.Placeholder = "Type your file comment..."
			m.modalTextarea.Reset()
			m.modalInitial = ""
			m.modalTextarea.Focus()
		}
		return m, nil

	case key.Matches(msg, keys.Delete):
		if !t.selecting {
			m.openSelectedCommentDelete()
		}
		return m, nil
	}

	// Tab switching (multi-file mode)
	if m.multiFile && !t.selecting {
		switch {
		case key.Matches(msg, keys.PrevTab):
			if m.activeTab > 0 {
				m.activeTab--
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		case key.Matches(msg, keys.NextTab):
			if m.activeTab < len(m.tabs)-1 {
				m.activeTab++
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		}
	}

	if m.multiFile && m.focused == contentPane && !t.selecting {
		// Number keys 1-9 for direct tab access
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0]-'0') - 1
			if idx < len(m.tabs) {
				m.activeTab = idx
				m.rebuildContent()
				m.updateCommentSidebar()
			}
			return m, nil
		}
	}

	if !t.selecting {
		switch {
		case key.Matches(msg, keys.NextChange):
			m.jumpToChange(1)
			return m, nil
		case key.Matches(msg, keys.PrevChange):
			m.jumpToChange(-1)
			return m, nil
		}
	}

	if m.focused == contentPane && !t.selecting {
		switch {
		case key.Matches(msg, keys.NextUnresolved):
			m.jumpToCommentOrFinish(1, false)
			return m, nil
		case key.Matches(msg, keys.PrevUnresolved):
			m.jumpToCommentOrFinish(-1, false)
			return m, nil
		case key.Matches(msg, keys.NextComment):
			m.jumpToCommentOrFinish(1, true)
			return m, nil
		case key.Matches(msg, keys.PrevComment):
			m.jumpToCommentOrFinish(-1, true)
			return m, nil
		}
	}

	if id := m.selectedCommentID(); id != "" {
		if _, drifted := m.driftedComment(id); drifted {
			if key.Matches(msg, keys.Confirm) {
				m.toggleDrifted(id)
				return m, nil
			}
			if msg.String() == "e" {
				m.openCommentThread(id)
				return m, nil
			}
		}
	}

	// Content pane cursor movement (annotation-aware)
	if m.focused == contentPane && (t.doc != nil || t.cursorOnAnnotation) {
		moved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns)-1 {
					t.cursorAnnoIdx++
				} else {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
					if next, ok := m.adjacentLine(t, 1); ok && (!t.selecting || next.side == t.selectSide) {
						t.cursorLine, t.cursorSide = next.line, next.side
					}
				}
			} else {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if len(anns) > 0 && !t.selecting {
					t.cursorOnAnnotation = true
					t.cursorAnnoIdx = 0
				} else if next, ok := m.adjacentLine(t, 1); ok && (!t.selecting || next.side == t.selectSide) {
					t.cursorLine, t.cursorSide = next.line, next.side
				}
			}
			moved = true
		case key.Matches(msg, keys.Up):
			if t.cursorOnAnnotation {
				if t.cursorAnnoIdx > 0 {
					t.cursorAnnoIdx--
				} else if t.cursorLine != 0 {
					t.cursorOnAnnotation = false
					t.cursorAnnoIdx = 0
				}
			} else {
				if prev, ok := m.adjacentLine(t, -1); ok && (!t.selecting || prev.side == t.selectSide) {
					anns := m.annotationsAfterLine(prev.line, prev.side)
					if len(anns) > 0 && !t.selecting {
						t.cursorLine, t.cursorSide = prev.line, prev.side
						t.cursorOnAnnotation = true
						t.cursorAnnoIdx = len(anns) - 1
					} else {
						t.cursorLine, t.cursorSide = prev.line, prev.side
					}
				} else if !t.selecting {
					if anns := m.annotationsAfterLine(0, ""); len(anns) > 0 {
						t.cursorLine, t.cursorSide = 0, ""
						t.cursorOnAnnotation, t.cursorAnnoIdx = true, len(anns)-1
					}
				}
			}
			moved = true
		case key.Matches(msg, keys.HalfPageDown):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.moveCursorBy(t, 1, jump)
			moved = true
		case key.Matches(msg, keys.HalfPageUp):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.moveCursorBy(t, -1, jump)
			moved = true
		case key.Matches(msg, keys.Top):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			if lines := m.visualLines(t); len(lines) > 0 && (!t.selecting || lines[0].side == t.selectSide) {
				t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
			}
			moved = true
		case key.Matches(msg, keys.Bottom):
			t.cursorOnAnnotation = false
			t.cursorAnnoIdx = 0
			if lines := m.visualLines(t); len(lines) > 0 && (!t.selecting || lines[len(lines)-1].side == t.selectSide) {
				last := lines[len(lines)-1]
				t.cursorLine, t.cursorSide = last.line, last.side
			}
			moved = true
		case key.Matches(msg, keys.Resolve, keys.ResolveNext):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns) {
					if key.Matches(msg, keys.ResolveNext) {
						m.resolveNext(anns[t.cursorAnnoIdx].id)
					} else {
						m.toggleResolve(anns[t.cursorAnnoIdx].id)
					}
				}
				return m, nil
			}
		case key.Matches(msg, keys.Confirm):
			if t.cursorOnAnnotation {
				anns := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
				if t.cursorAnnoIdx < len(anns) {
					m.openCommentThread(anns[t.cursorAnnoIdx].id)
					return m, nil
				}
			} else if t.state != nil {
				m.openLineComment()
				return m, nil
			}
		}

		if moved {
			t.cursorOnForm = false
			if t.cursorLine == 0 && !t.cursorOnAnnotation {
				m.moveCursorBy(t, 1, 1)
			}
			m.rebuildContent()
			m.scrollToCursor()
			return m, nil
		}
	}

	if m.focused == commentPane {
		switch {
		case key.Matches(msg, keys.NextUnresolved):
			m.jumpToCommentOrFinish(1, false)
			return m, nil
		case key.Matches(msg, keys.PrevUnresolved):
			m.jumpToCommentOrFinish(-1, false)
			return m, nil
		case key.Matches(msg, keys.NextComment):
			m.jumpToCommentOrFinish(1, true)
			return m, nil
		case key.Matches(msg, keys.PrevComment):
			m.jumpToCommentOrFinish(-1, true)
			return m, nil
		}
	}

	// Comment pane navigation
	if m.focused == commentPane && m.sidebarView == commentsView && len(t.sidebarItems) > 0 {
		sidebarMoved := false
		switch {
		case key.Matches(msg, keys.Down):
			if t.sidebarCursor < len(t.sidebarItems)-1 {
				t.sidebarCursor++
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Up):
			if t.sidebarCursor > 0 {
				t.sidebarCursor--
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Top):
			t.sidebarCursor = 0
			sidebarMoved = true
		case key.Matches(msg, keys.Bottom):
			t.sidebarCursor = len(t.sidebarItems) - 1
			sidebarMoved = true
		}
		if sidebarMoved {
			m.updateCommentSidebar()
			m.scrollToSidebarCursor()
			m.rebuildContent()
			sel := t.sidebarItems[t.sidebarCursor]
			if !isGeneralCommentScope(sel.scope) {
				t.cursorLine, t.cursorSide = sel.line, sel.side
				m.scrollToAnnotation(sel.side, sel.line, sel.endLine)
			}
			return m, nil
		}

		// Toggle resolution on the selected annotation
		if key.Matches(msg, keys.ResolveNext) {
			m.resolveNext(t.sidebarItems[t.sidebarCursor].id)
			return m, nil
		}
		if key.Matches(msg, keys.Resolve) {
			m.toggleResolve(t.sidebarItems[t.sidebarCursor].id)
			return m, nil
		}

		// Enter to edit selected annotation
		if key.Matches(msg, keys.Confirm) {
			sel := t.sidebarItems[t.sidebarCursor]
			m.openCommentThread(sel.id)
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) openCommentThread(id string) {
	t := m.tab()
	if t.state == nil {
		return
	}
	for i := range t.state.Comments {
		c := &t.state.Comments[i]
		if c.ID != id {
			continue
		}
		m.editingID = c.ID
		m.editingReplyID = ""
		m.modalReferenceOffset = -1
		m.modalFocus = 0
		m.modalTextarea.Reset()
		if reply := m.latestOwnReply(c); reply != nil {
			m.editingReplyID = reply.ID
			m.modal = editModal
			m.modalTextarea.SetValue(reply.Body)
			m.modalTextarea.Placeholder = "Edit reply..."
		} else if len(c.Replies) > 0 || !m.authoredThisRound(c.Author, c.ReviewRound) {
			m.modal = replyModal
			m.modalTextarea.Placeholder = "Write a reply..."
		} else {
			m.modal = editModal
			m.modalTextarea.SetValue(c.Body)
			m.modalTextarea.Placeholder = "Edit comment..."
		}
		m.modalInitial = m.modalTextarea.Value()
		m.modalTextarea.Focus()
		return
	}
}

func (m *AppModel) selectedCommentID() string {
	t := m.tab()
	switch m.focused {
	case contentPane:
		if !t.cursorOnAnnotation {
			return ""
		}
		annotations := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
		if t.cursorAnnoIdx < len(annotations) {
			return annotations[t.cursorAnnoIdx].id
		}
	case commentPane:
		if m.sidebarView == commentsView && t.sidebarCursor < len(t.sidebarItems) {
			return t.sidebarItems[t.sidebarCursor].id
		}
	}
	return ""
}

func (m *AppModel) openSelectedCommentDelete() {
	m.openCommentDelete(m.selectedCommentID())
}

func (m *AppModel) canDeleteComment(id string) bool {
	if m.tab().state == nil {
		return false
	}
	for _, c := range m.tab().state.Comments {
		if c.ID == id {
			return len(c.Replies) == 0 && m.authoredThisRound(c.Author, c.ReviewRound)
		}
	}
	return false
}

func (m *AppModel) openCommentDelete(id string) {
	if id == "" {
		return
	}
	m.editingID = id
	m.editingReplyID = ""
	if len(m.modalDeleteTargets()) == 0 {
		m.editingID = ""
		return
	}
	m.openDeleteConfirmation(0)
}

func (m *AppModel) openLineComment() {
	if m.readOnly {
		return
	}
	if m.tab().path == overallCommentsPath {
		return
	}
	if m.tab().cursorLine == 0 {
		m.moveCursorBy(m.tab(), 1, 1)
		if m.tab().cursorLine == 0 {
			return
		}
	}
	if m.patch != nil && len(m.visualLines(m.tab())) == 0 {
		return
	}
	if m.tab().state == nil {
		return
	}
	m.modal = commentModal
	m.modalReferenceOffset = -1
	m.modalFocus = 0
	m.modalTextarea.Placeholder = "Type your comment..."
	m.modalTextarea.Reset()
	m.modalInitial = ""
	m.modalTextarea.Focus()
}

func (m *AppModel) latestOwnReply(c *review.Comment) *review.Reply {
	if len(c.Replies) == 0 {
		return nil
	}
	reply := &c.Replies[len(c.Replies)-1]
	if !m.authoredThisRound(reply.Author, reply.ReviewRound) {
		return nil
	}
	return reply
}

func (m *AppModel) modalSubmit() {
	if m.modal == formTextModal {
		m.saveFormText()
		return
	}
	t := m.tab()
	body := strings.TrimSpace(m.modalTextarea.Value())
	if m.modalTextarea.Value() == m.modalInitial || (body == "" && m.modalInitial == "") {
		m.discardTextModal()
		return
	}
	if body == "" {
		m.modalDelete(0)
		return
	}
	if t.state == nil {
		return
	}
	var addedLineComment *commentTarget
	var addedFileCommentID string

	switch m.modal {
	case editModal:
		for i := range t.state.Comments {
			c := &t.state.Comments[i]
			if c.ID != m.editingID {
				continue
			}
			if m.editingReplyID == "" {
				c.Body = body
			} else {
				for j := range c.Replies {
					if c.Replies[j].ID == m.editingReplyID {
						c.Replies[j].Body = body
						break
					}
				}
			}
			c.UpdatedAt = review.Now()
			m.noteFinishFeedback(t.path, c.ID)
			break
		}
	case replyModal:
		now := review.Now()
		for i := range t.state.Comments {
			c := &t.state.Comments[i]
			if c.ID != m.editingID {
				continue
			}
			c.Replies = append(c.Replies, review.Reply{
				ID:          review.RandomReplyID(),
				Body:        body,
				Author:      m.author,
				CreatedAt:   now,
				ReviewRound: m.reviewRound(),
			})
			c.UpdatedAt = now
			m.noteFinishFeedback(t.path, c.ID)
			break
		}
	case commentModal:
		startLine, endLine := m.selectionRange()
		side := m.selectionSide()
		now := review.Now()
		c := review.Comment{
			ID:        review.RandomCommentID(),
			StartLine: startLine,
			EndLine:   endLine,
			Side:      side,
			Anchor:    m.anchorText(t, side, startLine, endLine),
			Body:      body,
			Author:    m.author,
			Scope:     "line",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if m.session != nil {
			c.ReviewRound = m.session.CJ.ReviewRound
		}
		addedLineComment = &commentTarget{
			line: endLine, side: side, annoIdx: len(m.annotationsAfterLine(endLine, side)),
		}
		t.state.Comments = append(t.state.Comments, c)
		m.noteFinishFeedback(t.path, c.ID)
	case fileCommentModal:
		now := review.Now()
		c := review.Comment{
			ID:        review.RandomCommentID(),
			Body:      body,
			Author:    m.author,
			Scope:     "file",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if m.session != nil {
			c.ReviewRound = m.session.CJ.ReviewRound
		}
		if t.path == overallCommentsPath {
			c.Scope = "review"
		}
		addedFileCommentID = c.ID
		t.state.Comments = append(t.state.Comments, c)
		m.noteFinishFeedback(t.path, c.ID)
	}
	m.newFeedback = true
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.discardReturn = noModal
	m.deleteReturn = noModal

	m.persist()
	m.modal = noModal
	m.modalTextarea.Blur()
	t.selecting = false
	if addedLineComment != nil {
		m.focused = contentPane
		m.selectComment(m.activeTab, *addedLineComment)
		return
	}
	m.rebuildContent()
	m.updateCommentSidebar()
	if addedFileCommentID != "" {
		for _, target := range m.commentTargets(m.activeTab) {
			if target.id == addedFileCommentID {
				m.selectComment(m.activeTab, target)
				break
			}
		}
	}
}

type modalDeleteTarget struct {
	replyID string
	label   string
}

func (m *AppModel) reviewRound() int {
	if m.session == nil {
		return 0
	}
	return m.session.CJ.ReviewRound
}

func (m *AppModel) authoredThisRound(author string, round int) bool {
	return author == m.author && round == m.reviewRound()
}

func (m *AppModel) modalDeleteTargets() []modalDeleteTarget {
	t := m.tab()
	if t.state == nil || m.editingID == "" {
		return nil
	}
	for _, c := range t.state.Comments {
		if c.ID != m.editingID {
			continue
		}
		if m.editingReplyID == "" {
			if m.canDeleteComment(c.ID) {
				return []modalDeleteTarget{{label: "Delete comment"}}
			}
			return nil
		}
		for i := range c.Replies {
			reply := &c.Replies[i]
			if reply.ID == m.editingReplyID && m.authoredThisRound(reply.Author, reply.ReviewRound) {
				return []modalDeleteTarget{{
					replyID: reply.ID,
					label:   fmt.Sprintf("Delete reply %d", i+1),
				}}
			}
		}
		return nil
	}
	return nil
}

func (m *AppModel) modalDelete(targetIndex int) {
	t := m.tab()
	if t.state == nil || m.editingID == "" {
		return
	}
	targets := m.modalDeleteTargets()
	if targetIndex < 0 || targetIndex >= len(targets) {
		return
	}
	target := targets[targetIndex]
	for i, c := range t.state.Comments {
		if c.ID != m.editingID {
			continue
		}
		if target.replyID == "" {
			t.state.Comments = append(t.state.Comments[:i], t.state.Comments[i+1:]...)
		} else {
			for j, reply := range c.Replies {
				if reply.ID == target.replyID {
					t.state.Comments[i].Replies = append(c.Replies[:j], c.Replies[j+1:]...)
					t.state.Comments[i].UpdatedAt = review.Now()
					break
				}
			}
		}
		break
	}
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.discardReturn = noModal
	m.deleteReturn = noModal
	m.persist()
	m.modal = noModal
	m.modalTextarea.Blur()
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	if t.cursorLine == 0 {
		m.moveCursorBy(t, 1, 1)
	}
	m.rebuildContent()
	m.updateCommentSidebar()
}

// toggleResolve flips a comment's resolution state, stamping ResolvedRound
// with the current round on resolve (mirroring crit's reply semantics).
// toggleResolve flips the resolution of thread id.  With advance, resolving
// moves on to the next unresolved thread across files, or returns focus to
// the source when none remain; otherwise the cursor stays where it is.
// toggleResolve flips the resolution of thread id in place.
func (m *AppModel) toggleResolve(id string) {
	m.resolveThread(id, false)
}

// resolveNext resolves thread id if it is open, keeps it resolved otherwise,
// and moves on to the next unresolved thread across files, or returns focus
// to the source when none remain.
func (m *AppModel) resolveNext(id string) {
	m.resolveThread(id, true)
}

func (m *AppModel) resolveThread(id string, advance bool) {
	t := m.tab()
	if t.state == nil {
		return
	}
	round := 0
	if m.session != nil {
		round = m.session.CJ.ReviewRound
	}
	nextTab, next, hasNext := m.adjacentComment(1, false)
	found := false
	for i := range t.state.Comments {
		if t.state.Comments[i].ID != id {
			continue
		}
		c := &t.state.Comments[i]
		found = true
		switch {
		case c.Resolved && advance:
			// Already resolved: leave it and just move on.
		case c.Resolved:
			c.Resolved = false
			c.ResolvedRound = 0
			c.UpdatedAt = review.Now()
		default:
			c.Resolved = true
			c.ResolvedRound = round
			c.UpdatedAt = review.Now()
			delete(t.expandedDrifted, id)
		}
		break
	}
	if advance && found {
		m.releaseThreadFocus()
	}
	m.persist()
	if advance && found && hasNext && (nextTab != m.activeTab || next.id != id) {
		m.selectComment(nextTab, next)
		return
	}
	m.rebuildContent()
	m.updateCommentSidebar()
	if m.focused == contentPane && !t.cursorOnAnnotation {
		m.scrollToCursor()
	}
	// No unresolved thread is left to advance to: move on to submitting.
	if advance && found {
		m.openFinishModal()
	}
}

// releaseThreadFocus returns focus to the source line under the cursor.
func (m *AppModel) releaseThreadFocus() {
	t := m.tab()
	m.focused = contentPane
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	if t.cursorLine == 0 {
		m.moveCursorBy(t, 1, 1)
	}
}

// clickResolve toggles thread id from its header button.  Resolving releases
// focus; reopening keeps the thread
// focused so its history is visible again.
func (m *AppModel) clickResolve(id string) {
	wasResolved := false
	if t := m.tab(); t.state != nil {
		for _, c := range t.state.Comments {
			if c.ID == id {
				wasResolved = c.Resolved
				break
			}
		}
	}
	m.toggleResolve(id)
	if wasResolved {
		return
	}
	m.releaseThreadFocus()
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

// resolveAll marks every comment thread resolved in the current round.
func (m *AppModel) resolveAll() {
	round := 0
	if m.session != nil {
		round = m.session.CJ.ReviewRound
	}
	now := review.Now()
	resolve := func(c *review.Comment) {
		if c.Resolved {
			return
		}
		c.Resolved = true
		c.ResolvedRound = round
		c.UpdatedAt = now
	}

	for i := range m.tabs {
		if m.tabs[i].state == nil {
			continue
		}
		for j := range m.tabs[i].state.Comments {
			resolve(&m.tabs[i].state.Comments[j])
		}
	}
	if m.session != nil {
		for i := range m.session.CJ.ReviewComments {
			resolve(&m.session.CJ.ReviewComments[i])
		}
		for path, file := range m.session.CJ.Files {
			for i := range file.Comments {
				resolve(&file.Comments[i])
			}
			m.session.CJ.Files[path] = file
		}
	}
	m.persist()
}

// sessionComments returns the session's stored comments for a file path.
func (m *AppModel) sessionComments(path string) []review.Comment {
	if m.session == nil {
		return []review.Comment{}
	}
	comments := m.session.FileComments(path)
	if path == overallCommentsPath {
		comments = m.session.CJ.ReviewComments
	}
	if comments == nil {
		comments = []review.Comment{}
	}
	return comments
}

// anchorText joins the full text of lines start..end as the comment's
// drift-correction anchor.
func (m *AppModel) anchorText(t *FileTab, side string, start, end int) string {
	if t.doc == nil {
		return ""
	}
	lines := make([]string, 0, end-start+1)
	if side == "old" {
		byLine := make(map[int]string)
		for _, dels := range t.deletedAfter {
			for _, del := range dels {
				if del.OldLineNum >= start && del.OldLineNum <= end {
					byLine[del.OldLineNum] = del.Content
				}
			}
		}
		for line := start; line <= end; line++ {
			if content, ok := byLine[line]; ok {
				lines = append(lines, content)
			}
		}
		return strings.Join(lines, "\n")
	}
	for l := start; l <= end && l <= t.doc.LineCount(); l++ {
		if !t.doc.HasLine(l) {
			return ""
		}
		lines = append(lines, t.doc.LineAt(l))
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) insertSuggestion() {
	start, end, ok := m.suggestionRange()
	if !ok {
		return
	}
	code := m.anchorText(m.tab(), "", start, end)
	if code == "" {
		return
	}
	body := strings.TrimRight(m.modalTextarea.Value(), "\n")
	if body != "" {
		body += "\n\n"
	}
	m.modalTextarea.SetValue(body + "```suggestion\n" + code + "\n```")
	m.modalFocus = 0
	m.modalTextarea.Focus()
	m.modalTextarea.CursorStart()
	m.modalTextarea.CursorUp()
	firstLine := strings.Count(body, "\n") + 1
	for m.modalTextarea.Line() > firstLine {
		m.modalTextarea.CursorUp()
	}
	m.modalTextarea.CursorStart()
	// Anchor the selection at the code start, then extend it back from the
	// closing fence to the code end without selecting either fence.
	m.modalTextarea, _ = m.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	m.modalTextarea.MoveToEnd()
	m.modalTextarea.CursorStart()
	m.modalTextarea, _ = m.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	// Populate the internal viewport before repositioning it around the cursor.
	_ = m.modalTextarea.View()
	m.modalTextarea.SetHeight(m.modalTextarea.Height())
}

func (m *AppModel) suggestionRange() (int, int, bool) {
	if m.modal == commentModal {
		if m.selectionSide() == "old" {
			return 0, 0, false
		}
		start, end := m.selectionRange()
		return start, end, true
	}
	if m.modal != replyModal && m.modal != editModal {
		return 0, 0, false
	}
	t := m.tab()
	if t.state == nil {
		return 0, 0, false
	}
	for _, c := range t.state.Comments {
		if c.ID == m.editingID && !isGeneralCommentScope(c.Scope) && c.Side != "old" && c.StartLine > 0 {
			return c.StartLine, c.EndAt(), true
		}
	}
	return 0, 0, false
}

func (m *AppModel) canSuggest() bool {
	start, end, ok := m.suggestionRange()
	if !ok {
		return false
	}
	if doc := m.tab().doc; doc != nil && doc.Known != nil {
		for line := start; line <= end; line++ {
			if !doc.HasLine(line) {
				return false
			}
		}
	}
	return true
}

func (m *AppModel) modalDeleteStartFocus() int {
	if m.canSuggest() {
		return 4
	}
	return 3
}

// persist writes every tab's comments back into the session and saves it.
func (m *AppModel) persist() {
	if m.readOnly || m.session == nil {
		return
	}
	for i := range m.tabs {
		if m.tabs[i].state == nil {
			continue
		}
		if m.tabs[i].path == overallCommentsPath {
			m.session.CJ.ReviewComments = m.tabs[i].state.Comments
		} else {
			m.session.SetFileComments(m.tabs[i].path, "", m.tabs[i].state.Comments)
		}
	}
	m.session.CJ.RoundState.NewFeedback = m.newFeedback
	if err := m.session.Save(); err != nil {
		m.err = err
		return
	}
	m.refreshSavedComments()
}

func (m *AppModel) refreshSavedComments() {
	m.ensureOverallCommentsTab()
	for i := range m.tabs {
		if m.tabs[i].state != nil {
			m.tabs[i].state.Comments = m.sessionComments(m.tabs[i].path)
		}
	}
}

// unresolvedTotal counts unresolved comments across tabs and the session's
// review-level comments.
func (m *AppModel) unresolvedTotal() int {
	n := 0
	for i := range m.tabs {
		if m.tabs[i].state == nil || m.tabs[i].path == overallCommentsPath {
			continue
		}
		for _, c := range m.tabs[i].state.Comments {
			if !c.Resolved {
				n++
			}
		}
	}
	if m.session != nil {
		for _, c := range m.session.CJ.ReviewComments {
			if !c.Resolved {
				n++
			}
		}
	}
	return n
}

// doFinish saves the completed round before closing the TUI.
func (m *AppModel) doFinish() (tea.Model, tea.Cmd) {
	if m.finishSaveFailed {
		m.err, m.finishSaveFailed = nil, false
	}
	verdict := m.finishVerdict
	if !verdict.Valid() {
		verdict = m.defaultFinishVerdict()
	}
	if verdict == review.VerdictApprove {
		if m.modal == finishModal {
			m.saveFinishDraft()
		} else {
			m.persist()
		}
		if m.err != nil {
			m.finishSaveFailed = true
			return m, nil
		}
		if len(m.missingForms()) > 0 {
			m.finishVerdict = verdict
			m.modal, m.modalFocus = finishFormsModal, finishReviewForms
			m.modalTextarea.Blur()
			return m, nil
		}
	}
	m.finishVerdict = verdict
	if verdict == review.VerdictApprove && len(m.unviewedForms()) > 0 {
		m.modal, m.modalFocus = finishFormsModal, finishReviewForms
		m.modalTextarea.Blur()
		return m, nil
	}
	return m.continueFinish()
}

// continueFinish submits the selected verdict after form checks.
func (m *AppModel) continueFinish() (tea.Model, tea.Cmd) {
	if m.finishVerdict == review.VerdictApprove {
		return m.confirmFormApproval()
	}
	if m.modal == finishFormsModal {
		m.modal = finishModal
	}
	return m.submitReview(m.finishVerdict)
}

func (m *AppModel) submitReview(verdict review.Verdict) (tea.Model, tea.Cmd) {
	if !verdict.Valid() {
		m.err = fmt.Errorf("invalid review verdict: %q", verdict)
		return m, nil
	}
	if verdict == review.VerdictApprove && len(m.missingForms()) > 0 {
		m.clipboardStatus = "Complete required confirmations before approving."
		return m, nil
	}
	m.finishVerdict = verdict
	if m.modal == finishModal {
		m.finishDraft = m.modalTextarea.Value()
	}
	if m.session != nil {
		m.persist()
		if m.err != nil {
			m.finishSaveFailed = true
			return m, nil
		}
		m.session.SetSubmission(verdict, m.finishDraft, m.author)
		m.refreshSavedComments()
		m.session.CJ.RoundState.ReviewCommentDraft = m.finishDraft
	}
	if verdict == review.VerdictApprove {
		m.resolveAll()
	} else {
		m.persist()
	}
	if m.err != nil {
		m.finishSaveFailed = true
		return m, nil
	}
	if verdict == review.VerdictApprove && m.unresolvedTotal() > 0 {
		m.err = fmt.Errorf("new feedback arrived while approving; review it before retrying")
		m.finishSaveFailed = true
		return m, nil
	}
	if m.session != nil {
		previous := m.session.CJ.RoundState
		m.session.CJ.RoundState.Finished = true
		m.session.CJ.RoundState.SubmittedReplies = m.replyIDs()
		if err := m.session.Save(); err != nil {
			m.session.CJ.RoundState = previous
			m.err, m.finishSaveFailed = err, true
			return m, nil
		}
		m.refreshSavedComments()
		if verdict == review.VerdictApprove && m.unresolvedTotal() > 0 {
			m.failSubmission(fmt.Errorf("new feedback arrived while approving; review it before retrying"))
			return m, nil
		}
	}
	approved := verdict == review.VerdictApprove
	var result *review.FormResult
	if m.forms != nil && m.forms.HasForms() {
		value := m.forms.Result(approved, m.session.CJ.ReviewRound)
		value.Session, value.ImageBase = m.session.Key, m.session.Dir
		value.Verdict = verdict
		value.ReviewComment = m.session.SubmissionComment()
		if err := m.session.SaveFormResult(value); err != nil {
			m.failSubmission(err)
			return m, nil
		}
		result = &value
	}
	if m.finishCh != nil {
		files := make([]gitpkg.FileChange, 0, len(m.tabs))
		for _, tab := range m.tabs {
			if tab.attachment == nil && tab.path != overallCommentsPath && !tab.outsideChanges {
				files = append(files, gitpkg.FileChange{Path: tab.path})
			}
		}
		m.finishCh <- FinishEvent{Approved: approved, Result: result, Files: files}
	}
	m.modal = noModal
	return m, tea.Quit
}

func (m *AppModel) handleTextModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.clipboardStatus = ""
	if msg.String() == "alt+o" && m.modalFocus == 0 {
		return m, m.openReferenceAtCursor()
	}
	if msg.String() == "alt+r" && m.hasModalResolution() {
		m.toggleResolve(m.editingID)
		return m, nil
	}
	if m.handleCompletionKey(msg) {
		return m, nil
	}
	focusOrder := []int{0, 2}
	if m.canSuggest() {
		focusOrder = append(focusOrder, 3)
	}
	focusOrder = append(focusOrder, 1)
	if m.modal == editModal {
		for i := range m.modalDeleteTargets() {
			focusOrder = append(focusOrder, m.modalDeleteStartFocus()+i)
		}
	}

	switch msg.String() {
	case "esc":
		m.closeTextModal()
		return m, nil
	case "ctrl+pgup":
		m.scrollModalReference(-1)
		return m, nil
	case "ctrl+pgdown":
		m.scrollModalReference(1)
		return m, nil
	case "tab", "shift+tab":
		m.cycleModalFocus(focusOrder, msg.String() == "shift+tab")
		if m.modalFocus == 0 {
			m.modalTextarea.Focus()
		} else {
			m.modalTextarea.Blur()
		}
		return m, nil
	case "enter":
		if m.modalFocus == 1 {
			m.modalSubmit()
			return m, nil
		} else if m.modalFocus == 2 {
			m.closeTextModal()
			return m, nil
		} else if m.canSuggest() && m.modalFocus == 3 {
			m.insertSuggestion()
			return m, nil
		} else if m.modal == editModal && m.modalFocus >= m.modalDeleteStartFocus() {
			m.openDeleteConfirmation(m.modalFocus - m.modalDeleteStartFocus())
			return m, nil
		}
		if m.modalFocus == 0 && m.pathInputType() != "" {
			m.modalSubmit()
			return m, nil
		}
	case "ctrl+s":
		m.modalSubmit()
		return m, nil
	case "ctrl+v":
		if m.modalFocus == 0 {
			return m, m.pasteClipboardImage()
		}
		return m, nil
	case "ctrl+o":
		if m.modalFocus == 0 {
			return m, m.openExternalEditor()
		}
		return m, nil
	case "alt+s":
		if m.canSuggest() {
			m.insertSuggestion()
		}
		return m, nil
	case "alt+w":
		// Experimental: copy the current kill ring entry to the system
		// clipboard through OSC 52 without disturbing a yank sequence.
		if text := m.killRing.current(); m.modalFocus == 0 && text != "" {
			m.clipboardStatus = "Copied to system clipboard: " + ansi.Truncate(strings.ReplaceAll(text, "\n", "⏎"), 40, "…")
			return m, tea.SetClipboard(text)
		}
		return m, nil
	}

	if m.modalFocus == 0 {
		if handled, cmd := m.updateKillRing(msg); handled {
			return m, cmd
		}
		var cmd tea.Cmd
		cmd = m.updateModalTextarea(msg)
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) scrollModalReference(direction int) {
	for _, region := range m.modalMouseRegions() {
		if !region.action.scrollable {
			continue
		}
		pageSize := max(1, region.rect.bottom-region.rect.top-2)
		offset := region.action.scrollOffset + direction*pageSize
		m.modalReferenceOffset = max(0, min(region.action.scrollMaxOffset, offset))
		return
	}
}

func (m *AppModel) closeTextModal() {
	if m.modalTextarea.Value() != m.modalInitial {
		m.discardReturn = m.modal
		m.modal = discardChangesModal
		m.modalFocus = 1
		m.modalTextarea.Blur()
		return
	}
	m.discardTextModal()
}

func (m *AppModel) discardTextModal() {
	m.modal = noModal
	m.discardReturn = noModal
	m.deleteReturn = noModal
	m.editingID = ""
	m.editingReplyID = ""
	m.modalInitial = ""
	m.modalTextarea.Blur()
	m.modalTextarea.Reset()
	m.modalTextarea.SetHeight(6)
	m.completion = completionState{}
	m.pathInfo = formPathState{}
	m.pendingFormAnswer = nil
}

func (m *AppModel) resumeTextModal() {
	m.modal = m.discardReturn
	m.discardReturn = noModal
	m.modalFocus = 0
	m.modalTextarea.Focus()
}

func (m *AppModel) handleDiscardChangesModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.discardTextModal()
	case "n", "N", "esc":
		m.resumeTextModal()
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			m.discardTextModal()
		} else {
			m.resumeTextModal()
		}
	}
	return m, nil
}

func (m *AppModel) openDeleteConfirmation(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(m.modalDeleteTargets()) {
		return
	}
	m.deleteReturn = m.modal
	m.pendingDelete = targetIndex
	m.modal = deleteConfirmModal
	m.modalFocus = 1
	m.modalTextarea.Blur()
}

func (m *AppModel) cancelDeleteConfirmation() {
	if m.deleteReturn == noModal {
		m.modal = noModal
		m.editingID = ""
		m.editingReplyID = ""
		m.pendingDelete = 0
		m.modalFocus = 0
		return
	}
	m.modal = m.deleteReturn
	m.deleteReturn = noModal
	m.modalFocus = m.modalDeleteStartFocus() + m.pendingDelete
}

func (m *AppModel) confirmDelete() {
	target := m.pendingDelete
	m.deleteReturn = noModal
	m.modalDelete(target)
}

func (m *AppModel) handleDeleteConfirmModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirmDelete()
	case "n", "N", "esc":
		m.cancelDeleteConfirmation()
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			m.confirmDelete()
		} else {
			m.cancelDeleteConfirmation()
		}
	}
	return m, nil
}

func (m *AppModel) recalculateLayout() {
	headerHeight := m.headerHeight()
	tabBarHeight := m.tabBarHeight()
	footerHeight := 0
	if len(m.tabs) > 0 && m.tab().state != nil {
		if footer := m.renderFooter(); footer != "" {
			footerHeight = lipgloss.Height(footer)
		}
	}
	// The tab bar serves as the top border of the content pane; one row
	// remains for the bottom border.
	mainHeight := max(0, m.height-headerHeight-tabBarHeight-footerHeight-1)

	// The comment pane takes
	// commentPanelWidth columns; the content pane's borders take two.
	contentWidth := m.width - m.commentPanelWidth() - 2

	m.contentViewport.SetWidth(contentWidth)
	m.contentViewport.SetHeight(mainHeight)
	m.commentViewport.SetWidth(max(0, m.commentPanelWidth()-2-paneHorizontalPadding*2))
	m.commentViewport.SetHeight(mainHeight)
	m.fileTree.viewport.SetWidth(m.commentViewport.Width())
	m.layoutFilesSidebar()

	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	m.modalTextarea.SetWidth(modalWidth - 10)
	m.modalTextarea.SetHeight(6)
	if m.pathInputType() != "" {
		m.resizePathInput()
	}
	if m.modal == finishModal {
		m.modalTextarea.SetHeight(m.finishTextareaHeight())
	}
}

// annotationsAfterLine returns annotations that render after the given line
// (keyed by their endLine).
func (m *AppModel) annotationsAfterLine(lineNum int, side string) []annotation {
	t := m.tab()
	if t.state == nil {
		return nil
	}
	var anns []annotation
	for _, c := range t.state.Comments {
		if commentLocation(t, c) == (lineRef{line: lineNum, side: side}) {
			anns = append(anns, tabAnnotation(t, c))
		}
	}
	return anns
}

type commentTarget struct {
	id       string
	scope    string
	resolved bool
	line     int
	side     string
	annoIdx  int
	formID   string
	required bool
	first    bool
}

// sidebarItem represents a comment in the sidebar list.
type sidebarItem struct {
	drifted  bool
	id       string
	scope    string
	line     int
	endLine  int
	side     string
	body     string
	author   string
	replies  []review.Reply
	resolved bool
}

// annotation represents an inline comment to render.
type annotation struct {
	drifted  bool
	id       string
	scope    string
	body     string
	line     int
	endLine  int
	side     string
	author   string
	resolved bool
	replies  []review.Reply
}

func newAnnotation(c review.Comment) annotation {
	return annotation{
		id: c.ID, body: c.Body, scope: c.Scope,
		line: c.StartLine, endLine: c.EndLine, side: c.Side,
		author: c.Author, resolved: c.Resolved, replies: c.Replies,
	}
}

func (m *AppModel) updateCommentSidebar() {
	m.syncFileTree()
	m.syncOutline()
	m.renderFileTree()
	if len(m.tabs) == 0 {
		m.commentViewport.SetContent("")
		return
	}
	t := m.tab()
	if t.state == nil {
		return
	}
	m.sidebarTargets = nil
	m.sidebarActions = nil

	t.sidebarItems = nil
	for _, c := range t.state.Comments {
		t.sidebarItems = append(t.sidebarItems, sidebarItem{
			id: c.ID, scope: c.Scope, line: c.StartLine, endLine: c.EndLine, drifted: commentDrifted(t, c),
			side: c.Side, body: c.Body, author: c.Author, replies: c.Replies,
			resolved: c.Resolved,
		})
	}
	sort.SliceStable(t.sidebarItems, func(i, j int) bool {
		if isGeneralCommentScope(t.sidebarItems[i].scope) {
			if !isGeneralCommentScope(t.sidebarItems[j].scope) {
				return true
			}
			return !t.sidebarItems[i].resolved && t.sidebarItems[j].resolved
		}
		if isGeneralCommentScope(t.sidebarItems[j].scope) {
			return false
		}
		return t.sidebarItems[i].line < t.sidebarItems[j].line
	})

	if t.sidebarCursor >= len(t.sidebarItems) {
		t.sidebarCursor = len(t.sidebarItems) - 1
	}
	if t.sidebarCursor < 0 {
		t.sidebarCursor = 0
	}
	if m.viewMode {
		m.commentViewport.SetContent("")
		return
	}

	var b strings.Builder

	if len(t.sidebarItems) == 0 {
		message := "No comments yet.\n\nPress RET for a line comment,\nor 'f' for a file comment."
		if len(t.state.Comments) > 0 {
			message = "All comments resolved."
		}
		b.WriteString(commentStyle.Render(message))
		m.commentViewport.SetContent(b.String())
		return
	}

	for idx, it := range t.sidebarItems {
		isSelected := m.focused == commentPane && idx == t.sidebarCursor
		collapsed := m.commentCollapsed(it.resolved, isSelected)
		if it.drifted {
			collapsed = m.viewMode || !m.driftedExpanded(it.id, it.resolved) && (!isSelected || !it.resolved)
		}
		var item strings.Builder

		var lineInfo string
		if isGeneralCommentScope(it.scope) {
			lineInfo = "File"
			if it.scope == "review" {
				lineInfo = "Review"
			}
		} else if it.endLine > it.line {
			lineInfo = fmt.Sprintf("L%d-%d", it.line, it.endLine)
		} else {
			lineInfo = fmt.Sprintf("L%d", it.line)
		}
		if it.side == "old" {
			lineInfo += " (deleted)"
		}
		if it.drifted {
			lineInfo += " (drifted)"
		}
		lineInfo = commentLineStyle.Foreground(lipgloss.Cyan).Render(lineInfo)
		if len(it.replies) > 0 {
			lineInfo += commentLineStyle.Render(" · ") + inlineLabelComment.Bold(false).Render(countNoun(len(it.replies), "reply", "replies"))
		}
		cursorCol := lipgloss.NewStyle().Width(2)
		prefix := cursorCol.Render("")
		if isSelected {
			prefix = cursorCol.Render(cursorMarker.Render(">"))
		}
		header, button := renderCommentHeader(prefix+lineInfo, it.resolved, m.canDeleteComment(it.id), isSelected && !m.readOnly, max(1, m.commentViewport.Width()))
		button.translate(0, len(m.sidebarTargets))
		button.id = it.id
		m.sidebarActions = append(m.sidebarActions, button)

		if collapsed {
			item.WriteString(header)
			wrapped := lipgloss.Wrap(expandDisplayTabs(item.String()), max(m.commentViewport.Width(), 1), "")
			for _, row := range strings.Split(wrapped, "\n") {
				b.WriteString(row)
				b.WriteByte('\n')
				m.sidebarTargets = append(m.sidebarTargets, idx)
			}
			b.WriteByte('\n')
			m.sidebarTargets = append(m.sidebarTargets, idx)
			continue
		}

		fmt.Fprintf(&item, "%s\n", header)

		thread := m.renderThread(threadViewKey{id: it.id, sidebar: true}, it.author, it.body, it.replies,
			max(1, m.commentViewport.Width()-1), isSelected)
		item.WriteString(" " + strings.ReplaceAll(thread, "\n", "\n "))

		wrapped := lipgloss.Wrap(expandDisplayTabs(item.String()), max(m.commentViewport.Width(), 1), "")
		for _, row := range strings.Split(wrapped, "\n") {
			b.WriteString(row)
			b.WriteByte('\n')
			m.sidebarTargets = append(m.sidebarTargets, idx)
		}
		b.WriteByte('\n')
		m.sidebarTargets = append(m.sidebarTargets, idx)
	}

	m.commentViewport.SetContent(b.String())
}

func unresolvedCommentCount(comments []review.Comment) int {
	count := 0
	for _, c := range comments {
		if !c.Resolved {
			count++
		}
	}
	return count
}

func truncateLeftToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	stringWidth := ansi.StringWidth(s)
	if stringWidth <= width {
		return s
	}
	const prefix = "…"
	prefixWidth := ansi.StringWidth(prefix)
	if width <= prefixWidth {
		return ansi.Truncate(prefix, width, "")
	}
	return prefix + ansi.Cut(s, stringWidth-width+prefixWidth, stringWidth)
}

func (m AppModel) reviewScopeLabel() string {
	if !m.multiFile || m.filePath != "" {
		return ""
	}
	if m.patch != nil {
		return "Supplied diff"
	}
	if m.source != nil {
		switch m.source.Scope {
		case "all":
			return "All"
		case "staged":
			return "Staged"
		case "unstaged":
			return "Unstaged"
		case "range":
			return "Range: " + m.source.Range
		}
	}
	if m.staged {
		return "Staged"
	}
	if m.baseRef == "" || m.baseRef == "HEAD" {
		return "Working tree"
	}
	return "Base: " + m.baseRef
}

func (m AppModel) renderHeader() string {
	header, _ := m.renderHeaderWithProgress()
	return header
}

func (m AppModel) renderHeaderWithProgress() (string, []progressRegion) {
	if len(m.tabs) == 0 {
		return m.renderReviewBar(), nil
	}
	t := m.tab()
	displayPath := t.displayPath()
	if t.attachment != nil {
		displayPath = t.attachment.Source
		if t.attachment.Removed {
			displayPath += " (history, read-only)"
		}
	} else if m.filePath != "" && t.path != overallCommentsPath {
		displayPath = m.filePath
	}

	prefix := " "
	var suffix string
	if t.selecting {
		start, end := m.selectionRange()
		selLabel := visualModeIndicator.Render("VISUAL")
		deleted := ""
		if m.selectionSide() == "old" {
			deleted = " (deleted)"
		}
		suffix = fmt.Sprintf("  %s L%d-%d%s", selLabel, start, end, deleted)
	} else if t.doc != nil {
		var position string
		if t.cursorSide == "old" {
			position = fmt.Sprintf("L%d (deleted)", t.cursorLine)
		} else {
			position = fmt.Sprintf("L%d/%d", t.cursorLine, t.doc.LineCount())
		}
		suffix = " " + headerPathStyle.Foreground(lipgloss.Cyan).Render(position)
	}
	headerWidth := max(0, m.width-headerStyle.GetHorizontalFrameSize())
	progress, regions := m.layoutProgressCounts(0, 0, -1)
	progress = " " + progress
	for i := range regions {
		regions[i].rect.left++
		regions[i].rect.right++
	}
	wrapProgress := m.width > 0 && ansi.StringWidth(prefix+suffix)+3+ansi.StringWidth(progress) > headerWidth
	if t.selecting {
		progress, regions = "", nil
	}
	progressWidth := 0
	if progress != "" && !wrapProgress {
		progressWidth = 3 + ansi.StringWidth(progress)
	}
	if m.width > 0 {
		pathWidth := max(0, headerWidth-ansi.StringWidth(prefix)-ansi.StringWidth(suffix)-progressWidth)
		displayPath = truncateLeftToWidth(displayPath, pathWidth)
	}
	headerContent := prefix + inlineBackground(headerPathStyle, headerPathStyle.Render(displayPath+suffix))
	progressLeft, progressTop := ansi.StringWidth(headerContent)+3, 0
	if progress != "" {
		if wrapProgress {
			progress, regions = m.layoutProgressCounts(max(1, headerWidth-1), 0, -1)
			lines := strings.Split(progress, "\n")
			for i, line := range lines {
				padding := max(0, headerWidth-1-ansi.StringWidth(line))
				lines[i] = strings.Repeat(" ", padding) + inlineBackground(lipgloss.NewStyle().Background(buttonBackground), " "+line)
				for j := range regions {
					if regions[j].rect.top == i {
						regions[j].rect.left += padding + 1
						regions[j].rect.right += padding + 1
					}
				}
			}
			progress = strings.Join(lines, "\n")
			headerContent += "\n"
			progressLeft, progressTop = 0, 1
		} else {
			headerContent += "   "
			if m.width > 0 {
				progressLeft = headerWidth - ansi.StringWidth(progress)
				headerContent += strings.Repeat(" ", max(0, progressLeft-ansi.StringWidth(headerContent)))
			}
			progress = inlineBackground(lipgloss.NewStyle().Background(buttonBackground), progress)
		}
		headerContent += progress
	}
	if m.width > 0 {
		lines := strings.Split(headerContent, "\n")
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], headerWidth, "")
		}
		headerContent = strings.Join(lines, "\n")
	}
	bar := m.renderReviewBar()
	for i := range regions {
		regions[i].rect.left += progressLeft + headerStyle.GetPaddingLeft()
		regions[i].rect.right += progressLeft + headerStyle.GetPaddingLeft()
		regions[i].rect.top += lipgloss.Height(bar) + progressTop + headerStyle.GetPaddingTop()
		regions[i].rect.bottom += lipgloss.Height(bar) + progressTop + headerStyle.GetPaddingTop()
	}
	return bar + "\n" + inlineBackground(headerStyle, headerStyle.Width(m.width).Render(headerContent)), regions
}

// renderReviewBar renders the top bar: what is under review, the review-wide
// toggles, and a right-aligned Submit button.  The parts are styled one by
// one so the bar's background resumes after each embedded style.
func (m AppModel) renderReviewBar() string {
	scope, subject := m.reviewSubject()
	if m.host != "" {
		subject += " on " + m.host
	}
	subject += "."
	text := renderBrand() + reviewBarStyle.Render(" ")
	if m.project != "" {
		text += reviewBarStyle.Bold(true).Render(headerLabel(m.project))
		if m.topic != "" {
			text += reviewBarStyle.Render(" · " + headerLabel(m.topic))
		}
		text += reviewBarStyle.Render(" — ")
	} else if m.topic != "" {
		text += reviewBarStyle.Render(headerLabel(m.topic) + " — ")
	}
	if !m.readOnly && m.session != nil && m.session.InMemory() {
		text += reviewBarStyle.Bold(true).Render("Demo · not saved · ")
	}
	action := "review"
	if m.readOnly {
		action = "view"
	}
	text += reviewBarStyle.Render(action + " the ")
	if style, ok := scopeWordStyle(scope); ok {
		text += style.Render(scope) + reviewBarStyle.Render(" ")
	}
	text += reviewBarStyle.Render(subject)
	buttons := []string{m.watchIndicator(), m.splitButton(), m.whitespaceButton(), m.submitButton()}
	var rows []string
	row := ""
	for _, button := range buttons {
		if button == "" {
			continue
		}
		if row != "" && m.width > 0 && lipgloss.Width(row)+1+lipgloss.Width(button) > m.width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += reviewBarStyle.Render(" ")
		}
		row += button
	}
	rows = append(rows, row)
	for i := range rows {
		if m.width > 0 {
			rows[i] += reviewBarStyle.Render(strings.Repeat(" ", max(0, m.width-lipgloss.Width(rows[i]))))
		}
	}
	right := row
	if m.width <= 0 || lipgloss.Width(right) < m.width {
		right += reviewBarStyle.Render(" ")
	}
	if len(rows) > 1 {
		return strings.Join(rows, "\n")
	}
	if m.width <= 0 {
		return text + reviewBarStyle.Render(" ") + right
	}
	available := m.width - lipgloss.Width(right) - 1
	if m.width >= 60 && lipgloss.Width(text) > available {
		return reviewBarStyle.Width(m.width).Render(ansi.Truncate(text, m.width, "…")) + "\n" + reviewBarStyle.Width(m.width).Render(right)
	}
	if available < 1 {
		return right
	}
	if lipgloss.Width(text) > available {
		text = ansi.Truncate(text, max(0, available-1), "") + reviewBarStyle.Render("…")
	}
	gap := m.width - lipgloss.Width(text) - lipgloss.Width(right)
	return text + reviewBarStyle.Render(strings.Repeat(" ", gap)) + right
}

func headerLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(value))
}

// submitButton renders the button that opens the finish dialog.
func (m AppModel) submitButton() string {
	if m.readOnly {
		return renderBarButton("Close", "q")
	}
	return renderBarButton("Submit", "q")
}

// scopeWordStyle styles the scope word in the top bar: white on the
// inline-change background of the matching diff side, which follows the
// terminal's light or dark theme.  Other scopes have no word.
func scopeWordStyle(scope string) (lipgloss.Style, bool) {
	switch scope {
	case "staged":
		return diffAddedTextBg.Foreground(lipgloss.BrightWhite), true
	case "unstaged":
		return diffDeletedTextBg.Foreground(lipgloss.BrightWhite), true
	}
	return lipgloss.Style{}, false
}

// reviewSubject names what the review covers, for the top bar.  The scope
// is "staged" or "unstaged" when the review is limited to one of them and
// empty otherwise.
func (m AppModel) reviewSubject() (scope, subject string) {
	if !m.multiFile || m.filePath != "" {
		return "", "document"
	}
	if m.source != nil {
		scope = m.source.Scope
	} else if m.staged {
		scope = "staged"
	}
	if _, ok := scopeWordStyle(scope); !ok {
		scope = ""
	}
	if len(m.attachments) > 0 {
		hasChanges := false
		for _, tab := range m.tabs {
			if tab.attachment == nil && !tab.outsideChanges {
				hasChanges = true
				break
			}
		}
		if !hasChanges {
			return scope, "attachments (no file changes)"
		}
	}
	return scope, "changes"
}

func (m AppModel) View() tea.View {
	if m.err != nil {
		hint := "Press q to quit."
		if m.finishSaveFailed {
			hint = "RET: retry saving · ESC: return to review · q: quit without finishing"
		}
		v := tea.NewView(fmt.Sprintf("Error: %v\n\n%s", m.err, hint))
		v.AltScreen = true
		return v
	}

	if m.multiFile && len(m.tabs) == 0 && m.width > 0 {
		body, regions := m.renderEmptyReview()
		v := tea.NewView(body)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeAllMotion
		m.placeModalCursor(&v, regions)
		return v
	}
	if m.width == 0 || len(m.tabs) == 0 || m.tab().state == nil {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	full, layout := m.renderReviewScreen()

	v := tea.NewView(full)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	if c := layout.isearchCursor; c != nil && c.X >= 0 && c.X < m.width && c.Y >= 0 && c.Y < m.height {
		v.Cursor = c
	}
	m.placeModalCursor(&v, layout.modalRegions)
	return v
}

func (m AppModel) placeModalCursor(v *tea.View, regions []modalMouseRegion) {
	cursor := m.editorCursor()
	switch m.modal {
	case gotoLineModal:
		cursor = m.lineInput.Cursor()
	case fileSelectModal:
		cursor = m.fileSelect.input.Cursor()
	}
	if c := cursor; c != nil {
		for _, region := range regions {
			if !region.action.textarea && !region.action.lineInput {
				continue
			}
			c.X += region.rect.left
			c.Y += region.rect.top
			if c.X >= max(0, region.rect.left) && c.X < min(m.width, region.rect.right) &&
				c.Y >= max(0, region.rect.top) && c.Y < min(m.height, region.rect.bottom) {
				v.Cursor = c
			}
			break
		}
	}
}

func (m AppModel) renderReviewScreen() (string, renderedScreenLayout) {
	t := m.tab()

	header, progressRegions := m.renderHeaderWithProgress()

	content := m.contentViewport.View()
	if m.modal != noModal {
		content = hideImagePlaceholders(content)
	}
	contentBox := lipgloss.NewStyle().
		Width(m.contentViewport.Width()).
		Height(m.contentViewport.Height()).
		Render(content)
	contentPane := renderPane(m.renderTabBar(), contentBox, lipgloss.RoundedBorder(), lipgloss.NewStyle().Foreground(accent))

	mainRow := contentPane
	if !m.viewMode {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, contentPane, m.renderSidebarPane())
	}

	footer := m.renderFooter()

	sections := []string{header, mainRow}
	footerTop := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, sections...))
	if footer != "" {
		sections = append(sections, footer)
	}
	layout := renderedScreenLayout{progressRegions: progressRegions}
	if button := m.watchIndicator(); button != "" {
		layout.watchIndicator, layout.hasWatchIndicator = buttonRect(header, 0, button)
	}
	if button := m.whitespaceButton(); button != "" {
		layout.whitespaceButton, layout.hasWhitespaceButton = buttonRect(header, 0, button)
	}
	if button := m.splitButton(); button != "" {
		layout.splitButton, layout.hasSplitButton = buttonRect(header, 0, button)
	}
	if rect, ok := buttonRect(footer, footerTop, m.searchHint()); ok && m.searchHint() != "" && !m.isearch.active {
		layout.isearchRegions = append(layout.isearchRegions, isearchMouseRegion{rect: rect, action: isearchStart})
	}
	if m.isearch.active && m.modal == noModal {
		_, x := m.isearchPrompt()
		layout.isearchCursor = tea.NewCursor(x, footerTop)
		_, regions := m.isearchControls(footerTop + 1)
		layout.isearchRegions = append(layout.isearchRegions, regions...)
		layout.isearchRegions = append(layout.isearchRegions, isearchMouseRegion{
			rect: mouseRect{left: 0, right: m.width, top: footerTop, bottom: footerTop + 1}, action: isearchInput,
		})
	}
	if !t.selecting {
		layout.finishButton, layout.hasFinishButton = buttonRect(header, 0, m.submitButton())
	}
	layout.copyTextButton, layout.hasCopyTextButton = buttonRect(footer, footerTop, m.copyTextButton())
	layout.footerHelp, layout.hasFooterHelp = buttonRect(footer, footerTop, m.footerHelpButton())
	layout.imagesButton, layout.hasImagesButton = buttonRect(footer, footerTop, m.imagesButton())
	if button := m.formListButton(); button != "" {
		layout.formListButton, layout.hasFormListButton = buttonRect(footer, footerTop, button)
	}

	full := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != noModal {
		full = hideImagePlaceholders(full)
		underlyingModal := noModal
		switch m.modal {
		case discardChangesModal:
			underlyingModal = m.discardReturn
		case deleteConfirmModal:
			underlyingModal = m.deleteReturn
		case lightboxModal:
			underlyingModal = m.lightbox.returnTo
		}
		if underlyingModal != noModal {
			underlying := m
			underlying.modal = underlyingModal
			underlying.modalFocus = 0
			full = hideImagePlaceholders(underlying.renderWithModal(full))
		}
		full, layout.modalRegions = m.renderWithModalLayout(full)
	}

	return full, layout
}

type tabLabel struct {
	text       string
	rendered   string
	width      int
	indicators string
}

func (m *AppModel) tabLabels() []tabLabel {
	labels := make([]tabLabel, len(m.tabs))
	indicators := m.tabIndicators()
	for i, t := range m.tabs {
		label := abbreviatedTabPath(t.displayPath())
		if t.attachment != nil {
			label = t.attachment.Name
		}
		start := strings.LastIndexByte(label, '/') + 1
		if i == m.activeTab {
			label = label[:start] + ansi.NewStyle().Bold().String() + label[start:] + ansi.NewStyle().Normal().Underline(false).String()
		}
		if counts := t.changeSummary(); counts != "" {
			label += " " + counts
		}
		if indicators[i] != "" {
			label += " " + indicators[i]
		}
		labels[i] = tabLabel{text: label, indicators: indicators[i]}
	}
	return labels
}

func abbreviatedTabPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts[:len(parts)-1] {
		for j, r := range part {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				if end := j + utf8.RuneLen(r); end < len(part) {
					parts[i] = ansi.NewStyle().Italic(true).String() + part[:end] + ansi.NewStyle().Italic(false).String()
				}
				break
			}
		}
	}
	return strings.Join(parts, "/")
}

func (m *AppModel) renderTab(labels []tabLabel, i int, isFirst bool) string {
	style := inactiveTabStyle
	if i == m.activeTab {
		style = activeTabStyle
	}
	if m.tabs[i].attachment != nil {
		style = style.Foreground(warning)
		if i == m.activeTab {
			style = style.Foreground(activeAttachmentColor)
		}
	}
	border, _, _, _, _ := style.GetBorder()
	if isFirst && i == m.activeTab {
		border.BottomLeft = "│"
	} else if isFirst {
		border.BottomLeft = "├"
	}
	label := labels[i].text
	paneWidth := m.contentPaneWidth()
	total := 0
	for _, label := range labels {
		total += ansi.StringWidth(label.text) + style.GetHorizontalFrameSize()
	}
	if total > paneWidth {
		if m.activeTab > 0 {
			paneWidth -= lipgloss.Width(m.renderTabOverflowIndicator(m.tabOverflowText(m.activeTab, true), false))
		}
		if m.activeTab < len(labels)-1 {
			paneWidth -= lipgloss.Width(m.renderTabOverflowIndicator(m.tabOverflowText(len(labels)-m.activeTab-1, false), false))
		}
	}
	available := max(1, paneWidth-style.GetHorizontalFrameSize())
	if m.width > 0 && ansi.StringWidth(label) > available {
		suffix := ""
		if labels[i].indicators != "" {
			suffix = " " + labels[i].indicators
		}
		label = ansi.Truncate(label, max(0, available-ansi.StringWidth(suffix)), "…") + suffix
	}
	return style.Border(border).Render(label)
}

func (m *AppModel) renderTabOverflowIndicator(text string, isFirst bool) string {
	style := inactiveTabStyle.Foreground(subtle)
	if m.contentPaneWidth() < 32 {
		style = style.Padding(0)
	}
	border, _, _, _, _ := style.GetBorder()
	if isFirst {
		border.BottomLeft = "├"
	}
	return style.Border(border).Render(text)
}

func (m *AppModel) tabOverflowText(count int, backwards bool) string {
	if m.contentPaneWidth() < 32 {
		if backwards {
			return "↤"
		}
		return "↦"
	}
	if backwards {
		return fmt.Sprintf("↤ %d more", count)
	}
	return fmt.Sprintf("%d more ↦", count)
}

func (m *AppModel) visibleTabWindow(labels []tabLabel) (int, int) {
	totalWidth := 0
	for _, label := range labels {
		totalWidth += label.width
	}
	paneWidth := m.contentPaneWidth()
	if totalWidth <= paneWidth {
		return 0, len(labels)
	}

	indicatorWidth := func(text string) int {
		return lipgloss.Width(m.renderTabOverflowIndicator(text, false))
	}
	leftWidth, rightWidth := 0, 0
	if m.activeTab > 0 {
		leftWidth = indicatorWidth(m.tabOverflowText(m.activeTab, true))
	}
	if m.activeTab < len(labels)-1 {
		rightWidth = indicatorWidth(m.tabOverflowText(len(labels)-m.activeTab-1, false))
	}

	available := paneWidth - leftWidth - rightWidth
	start, end := m.activeTab, m.activeTab+1
	used := labels[m.activeTab].width
	for {
		expanded := false
		if start > 0 && used+labels[start-1].width <= available {
			start--
			used += labels[start].width
			expanded = true
		}
		if end < len(labels) && used+labels[end].width <= available {
			used += labels[end].width
			end++
			expanded = true
		}
		if !expanded {
			return start, end
		}
	}
}

// modalClickGuard is how long a click that opened a dialog blocks another
// click at the same cell.
const modalClickGuard = 500 * time.Millisecond

func (m *AppModel) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	// Terminal mouse reports carry no click count, so a double-click that
	// opens a dialog would let its second click activate the control newly
	// placed beneath the pointer.  Ignore a click at the opening click's
	// cell within modalClickGuard instead.  The guard checks only that cell
	// and time, so it also applies after the dialog has been closed or
	// replaced, for example by a dialog opened from the keyboard.
	if m.modal != noModal && mouse.X == m.modalOpeningClick.X && mouse.Y == m.modalOpeningClick.Y &&
		time.Since(m.modalOpenedAt) < modalClickGuard {
		return m, nil
	}
	previousModal := m.modal
	defer func() {
		if previousModal != m.modal && m.modal != noModal {
			m.modalOpenedAt = time.Now()
			m.modalOpeningClick = mouse
		}
	}()
	if len(m.tabs) == 0 {
		return m.handleEmptyReviewClick(msg)
	}
	m.hoveredGutterLine = 0
	m.hoveredGutterSide = ""
	if m.modal == noModal && m.hasTextSelection() {
		_, layout := m.renderReviewScreen()
		if layout.hasCopyTextButton && layout.copyTextButton.contains(mouse) {
			return m, m.copySelectedText()
		}
	}
	if m.modal == noModal && m.textSelection.active {
		m.clearTextSelection()
	}
	if m.modal != noModal && m.clickedModalDismiss(mouse) {
		return m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	if m.modal == gotoLineModal || m.modal == openSourceModal {
		return m.handleLocationModalMouse(mouse)
	}
	if m.modal == fileSelectModal {
		return m.handleFileSelectMouse(mouse)
	}
	if m.modal == formListModal {
		return m.handleFormListMouse(mouse)
	}
	if m.modal == helpModal {
		for _, region := range m.modalMouseRegions() {
			if region.action.scrollable && region.rect.contains(mouse) {
				m.scrollHelp(region.action.scrollOffset)
				break
			}
		}
		return m, nil
	}
	if m.modal == finishResolveModal || m.modal == finishAbortModal {
		return m.handleFinishConfirmationMouse(mouse)
	}
	if m.modal == finishFormsModal {
		return m.handleFinishFormsMouse(mouse)
	}
	if m.modal == finishModal {
		return m.handleFinishModalMouse(mouse)
	}
	if m.modal == lightboxModal {
		return m.handleLightboxMouse(mouse)
	}
	if m.isTextModal() {
		if cmd, ok := m.openImageLink(m.referenceAt(mouse.X, mouse.Y)); ok {
			return m, cmd
		}
		return m.handleTextModalMouse(mouse)
	}
	if m.modal == discardChangesModal {
		return m.handleDiscardChangesModalMouse(mouse)
	}
	if m.modal == deleteConfirmModal {
		return m.handleDeleteConfirmModalMouse(mouse)
	}
	if m.modal != noModal {
		return m, nil
	}
	if m.handleTableScrollClick(mouse) {
		return m, nil
	}
	pendingTextLink := ""
	if !m.tab().selecting {
		uri := m.referenceAt(mouse.X, mouse.Y)
		if uri != "" {
			if !strings.HasPrefix(uri, "critty://image?") {
				m.startTextSelection(mouse)
			}
			if m.textSelection.dragging {
				pendingTextLink = uri
			} else if cmd, ok := m.activateReference(uri); ok {
				return m, cmd
			}
		}
	}
	if rect, ok := m.finishButtonRect(); ok && rect.contains(mouse) {
		if m.readOnly {
			return m, tea.Quit
		}
		m.openFinishModal()
		return m, nil
	}
	if rect, ok := m.footerHelpRect(); ok && rect.contains(mouse) {
		m.helpOffset = 0
		m.modal = helpModal
		return m, nil
	}
	_, imageLayout := m.renderReviewScreen()
	if imageLayout.hasWatchIndicator && imageLayout.watchIndicator.contains(mouse) {
		return m, m.toggleWatch()
	}
	if imageLayout.hasWhitespaceButton && imageLayout.whitespaceButton.contains(mouse) {
		m.toggleWhitespace()
		return m, nil
	}
	if imageLayout.hasSplitButton && imageLayout.splitButton.contains(mouse) {
		m.toggleSplit()
		return m, nil
	}
	for _, region := range imageLayout.progressRegions {
		if region.rect.contains(mouse) {
			m.jumpToProgress(region.kind)
			return m, nil
		}
	}
	if imageLayout.hasFormListButton && imageLayout.formListButton.contains(mouse) {
		m.openFormList()
		return m, nil
	}
	if imageLayout.hasImagesButton && imageLayout.imagesButton.contains(mouse) {
		cmd := m.toggleImages()
		return m, cmd
	}

	headerHeight := m.headerHeight()
	if mouse.Y >= headerHeight && mouse.Y < headerHeight+m.tabBarHeight() {
		if view, ok := m.sidebarTabAt(mouse.X); ok {
			m.sidebarView = view
			m.focused = commentPane
			m.updateCommentSidebar()
			m.rebuildContent()
			return m, nil
		}
		labels := m.tabLabels()
		for i := range labels {
			labels[i].rendered = m.renderTab(labels, i, i == 0)
			labels[i].width = lipgloss.Width(labels[i].rendered)
		}
		start, end := m.visibleTabWindow(labels)
		x := 0
		if start > 0 {
			indicator := m.renderTabOverflowIndicator(m.tabOverflowText(start, true), true)
			width := lipgloss.Width(indicator)
			if mouse.X >= x && mouse.X < x+width {
				m.selectTab(start - 1)
				return m, nil
			}
			x += width
		}
		for i := start; i < end; i++ {
			if mouse.X >= x && mouse.X < x+labels[i].width {
				m.selectTab(i)
				return m, nil
			}
			x += labels[i].width
		}
		if end < len(labels) {
			indicator := m.renderTabOverflowIndicator(m.tabOverflowText(len(labels)-end, false), false)
			if mouse.X >= x && mouse.X < x+lipgloss.Width(indicator) {
				m.selectTab(end)
				return m, nil
			}
		}
		return m, nil
	}

	left, top, right, bottom := m.contentBounds()
	if mouse.X >= left && mouse.X < right && mouse.Y >= top && mouse.Y < bottom {
		if !m.tab().selecting {
			point := tea.Mouse{X: mouse.X - left, Y: mouse.Y - top + m.contentViewport.YOffset()}
			for _, control := range m.contentLayout.formControls {
				if control.rect.contains(point) {
					m.focused = contentPane
					m.tab().cursorLine, m.tab().cursorSide, m.tab().cursorOnAnnotation = control.line, "", false
					m.tab().cursorOnForm = true
					if !m.startRankDrag(control.line) {
						m.toggleForm(control.line)
					}
					return m, nil
				}
			}
			for _, marker := range m.contentLayout.markers {
				if marker.rect.contains(point) {
					m.selectMarker(marker.ids)
					return m, nil
				}
			}
		}
		previousFocus := m.focused
		wasFocused := m.focused == contentPane
		m.focused = contentPane
		if target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset()); ok {
			t := m.tab()
			if !target.annotation && !m.isContentGutter(mouse.X-left) {
				m.startTextSelection(mouse)
				if m.textSelection.dragging {
					m.textSelection.target = target
					m.textSelection.link = pendingTextLink
					m.focused = previousFocus
					return m, nil
				}
			}
			if m.isContentGutter(mouse.X-left) && !target.annotation && t.selecting {
				start, end := m.selectionRange()
				if start < end && target.side == t.selectSide && target.line == end {
					m.openLineComment()
					m.rebuildContent()
					return m, nil
				}
			}
			openThread := wasFocused && target.annotation && t.cursorOnAnnotation &&
				t.cursorLine == target.line && t.cursorSide == target.side && t.cursorAnnoIdx == target.annotationIndex
			t.cursorLine, t.cursorSide = target.line, target.side
			t.cursorOnAnnotation = target.annotation
			t.cursorOnForm = false
			t.cursorAnnoIdx = target.annotationIndex
			if !t.selecting && target.annotation {
				point := tea.Mouse{X: mouse.X - left, Y: mouse.Y - top + m.contentViewport.YOffset()}
				for _, region := range m.contentLayout.actions {
					if m.handleCommentHeaderClick(region, point) {
						return m, nil
					}
				}
			}
			if openThread {
				annotations := m.annotationsAfterLine(target.line, target.side)
				if target.annotationIndex < len(annotations) {
					m.openCommentThread(annotations[target.annotationIndex].id)
					return m, nil
				}
			}
			if m.isContentGutter(mouse.X-left) && !target.annotation {
				t.selecting = true
				t.selectAnchor = target.line
				t.selectSide = target.side
				m.mouseSelecting = true
			}
		}
		m.updateCommentSidebar()
		m.rebuildContent()
		return m, nil
	}

	left, top, right, bottom = m.commentBounds()
	if mouse.X >= left && mouse.X < right && mouse.Y >= top && mouse.Y < bottom {
		if m.sidebarView == filesView {
			m.handleFilesSidebarClick(mouse.Y - top)
			m.rebuildContent()
			return m, nil
		}
		wasFocused := m.focused == commentPane
		m.focused = commentPane
		row := mouse.Y - top + m.commentViewport.YOffset()
		if i, ok := m.sidebarMouseTarget(row); ok {
			t := m.tab()
			point := tea.Mouse{X: mouse.X - left - paneHorizontalPadding, Y: row}
			openThread := wasFocused && t.sidebarCursor == i
			m.selectSidebarItem(i)
			if !t.selecting {
				for _, region := range m.sidebarActions {
					if m.handleCommentHeaderClick(region, point) {
						return m, nil
					}
				}
			}
			if openThread {
				m.openCommentThread(t.sidebarItems[i].id)
				return m, nil
			}
		}
		m.updateCommentSidebar()
		m.rebuildContent()
		m.scrollToSidebarCursor()
	}
	return m, nil
}

func (m *AppModel) handleCommentHeaderClick(region commentHeaderRegion, point tea.Mouse) bool {
	switch {
	case region.resolve.contains(point):
		m.clickResolve(region.id)
	case region.up.contains(point):
		m.jumpToComment(-1, true)
	case region.down.contains(point):
		m.jumpToCommentOrFinish(1, true)
	case region.delete.contains(point):
		m.openCommentDelete(region.id)
	default:
		return false
	}
	return true
}

func (m *AppModel) selectTab(index int) {
	if index < 0 || index >= len(m.tabs) || m.activeTab == index {
		return
	}
	m.activeTab = index
	m.rebuildContent()
	m.updateCommentSidebar()
}

type modalMouseAction struct {
	resolve         bool
	openImage       bool
	dialog          bool // the whole dialog frame
	close           bool
	lineInput       bool
	pick            bool
	pickIndex       int
	focus           int
	deleteIndex     int
	textarea        bool
	scrollable      bool
	scrollOffset    int
	scrollMaxOffset int
}

type modalMouseRegion struct {
	rect   mouseRect
	action modalMouseAction
}

type modalButtonSpec struct {
	rendered string
	action   modalMouseAction
}

func (m *AppModel) isTextModal() bool {
	return m.modal == commentModal || m.modal == fileCommentModal ||
		m.modal == replyModal || m.modal == editModal || m.modal == finishModal || m.modal == formTextModal
}

func (m *AppModel) hasModalResolution() bool {
	return m.modal == replyModal || (m.modal == editModal && m.editingReplyID != "")
}

func (m *AppModel) handleTextModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if m.handleCompletionClick(mouse) {
		return m, nil
	}
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		action := region.action
		if action.resolve {
			m.toggleResolve(m.editingID)
			return m, nil
		}
		if action.textarea {
			m.focusTextareaAt(mouse, region.rect)
			return m, nil
		}
		if action.scrollable {
			return m, nil
		}
		m.modalFocus = action.focus
		switch {
		case action.focus == 1:
			m.modalSubmit()
		case action.focus == 2:
			m.closeTextModal()
		case m.canSuggest() && action.focus == 3:
			m.insertSuggestion()
		case m.modal == editModal && action.focus >= m.modalDeleteStartFocus():
			m.openDeleteConfirmation(action.deleteIndex)
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) focusTextareaAt(mouse tea.Mouse, rect mouseRect) {
	viewRow := m.editorTextRow(mouse.Y - rect.top)
	if viewRow < 0 {
		return
	}
	targetRow := m.modalTextarea.ScrollYOffset() + viewRow
	m.modalTextarea.MoveToBegin()
	for range targetRow {
		line, column := m.modalTextarea.Line(), m.modalTextarea.Column()
		m.modalTextarea.CursorDown()
		if m.modalTextarea.Line() == line && m.modalTextarea.Column() == column {
			break
		}
	}

	lineInfo := m.modalTextarea.LineInfo()
	textX := max(0, mouse.X-rect.left-lipgloss.Width(m.modalTextarea.Prompt))
	lines := strings.Split(m.modalTextarea.Value(), "\n")
	line := []rune(lines[m.modalTextarea.Line()])
	column := lineInfo.StartColumn
	end := min(len(line), lineInfo.StartColumn+lineInfo.Width)
	for column < end && ansi.StringWidth(string(line[lineInfo.StartColumn:column+1])) <= textX {
		column++
	}
	m.modalTextarea.SetCursorColumn(column)
	m.modalFocus = 0
	m.modalTextarea.Focus()
}

func (m *AppModel) handleDiscardChangesModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		if region.action.focus == 0 {
			m.discardTextModal()
		} else {
			m.resumeTextModal()
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleDeleteConfirmModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		m.modalFocus = region.action.focus
		if region.action.focus == 0 {
			m.confirmDelete()
		} else {
			m.cancelDeleteConfirmation()
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.outline.dragging {
		if m.modal == noModal && msg.Mouse().Button == tea.MouseLeft && !m.viewMode && m.sidebarView == filesView {
			m.resizeOutlineAt(msg.Mouse().Y)
		}
		return m, nil
	}
	if m.modal == noModal && m.textSelection.dragging && msg.Mouse().Button == tea.MouseLeft {
		m.updateTextSelection(msg.Mouse(), true)
		return m, nil
	}
	if len(m.tabs) == 0 {
		return m, nil
	}
	if m.rankDrag.active {
		m.dragRank(msg.Mouse())
		return m, nil
	}
	if !m.mouseSelecting {
		m.updateGutterHover(msg.Mouse())
		return m, nil
	}
	if msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	m.updateMouseSelection(msg.Mouse(), true)
	return m, nil
}

func (m *AppModel) updateGutterHover(mouse tea.Mouse) {
	line := 0
	side := ""
	if m.modal == noModal && len(m.tabs) > 0 && m.tab().state != nil {
		left, top, _, bottom := m.contentBounds()
		if m.isContentGutter(mouse.X-left) && mouse.Y >= top && mouse.Y < bottom {
			if target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset()); ok && !target.annotation {
				line = target.line
				side = target.side
			}
		}
	}
	if m.hoveredGutterLine == line && m.hoveredGutterSide == side {
		return
	}
	m.hoveredGutterLine = line
	m.hoveredGutterSide = side
	m.rebuildContent()
}

func (m *AppModel) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if m.outline.dragging {
		m.outline.dragging = false
		return m, nil
	}
	if m.modal == noModal && m.textSelection.dragging && msg.Mouse().Button == tea.MouseLeft {
		m.updateTextSelection(msg.Mouse(), false)
		m.textSelection.dragging = false
		if !m.textSelection.active {
			return m, nil
		}
		target := m.textSelection.target
		t := m.tab()
		t.cursorLine, t.cursorSide, t.cursorAnnoIdx = target.line, target.side, 0
		t.cursorOnAnnotation, t.cursorOnForm = false, false
		m.focused = contentPane
		m.updateCommentSidebar()
		m.rebuildContent()
		if !m.hasTextSelection() && m.textSelection.link != "" {
			uri := m.textSelection.link
			m.clearTextSelection()
			cmd, _ := m.activateReference(uri)
			return m, cmd
		}
		return m, nil
	}
	if len(m.tabs) == 0 {
		return m, nil
	}
	if m.rankDrag.active {
		m.finishRankDrag()
		return m, nil
	}
	if !m.mouseSelecting || msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	m.updateMouseSelection(msg.Mouse(), false)
	m.mouseSelecting = false
	t := m.tab()
	t.selecting = t.cursorLine != t.selectAnchor || t.cursorSide != t.selectSide
	m.openLineComment()
	m.rebuildContent()
	return m, nil
}

func (m *AppModel) updateMouseSelection(mouse tea.Mouse, autoScroll bool) {
	left, top, _, bottom := m.contentBounds()
	viewportY := mouse.Y - top + m.contentViewport.YOffset()
	if autoScroll && mouse.Y <= top {
		m.contentViewport.ScrollUp(1)
		viewportY = m.contentViewport.YOffset()
	} else if autoScroll && mouse.Y >= bottom-1 {
		m.contentViewport.ScrollDown(1)
		viewportY = m.contentViewport.YOffset() + m.contentViewport.Height() - 1
	} else if mouse.Y < top || mouse.Y >= bottom {
		return
	}

	if target, ok := m.contentMouseTargetAt(mouse.X-left, viewportY); ok && !target.annotation {
		t := m.tab()
		if t.selecting && target.side != t.selectSide {
			return
		}
		t.cursorLine, t.cursorSide = target.line, target.side
		t.cursorOnAnnotation = false
		t.cursorOnForm = false
		t.cursorAnnoIdx = 0
		t.selecting = true
		m.rebuildContent()
	}
}

type contentMouseTarget struct {
	passive         bool // repeated context maps to a source line without extending its scroll range
	line            int
	side            string
	annotation      bool
	annotationIndex int
	drifted         bool
}

func (m *AppModel) contentMouseTarget(y int) (contentMouseTarget, bool) {
	if y < 0 || y >= len(m.contentLayout.rows) {
		return contentMouseTarget{}, false
	}
	return m.contentLayout.rows[y], true
}

func (m *AppModel) highlightedCommentLines() (int, int, string) {
	t := m.tab()
	if m.focused == commentPane && len(t.sidebarItems) > 0 && t.sidebarCursor < len(t.sidebarItems) {
		item := t.sidebarItems[t.sidebarCursor]
		if item.drifted {
			return 0, 0, ""
		}
		return item.line, max(item.line, item.endLine), item.side
	}
	if m.focused == contentPane && t.cursorOnAnnotation {
		annotations := m.annotationsAfterLine(t.cursorLine, t.cursorSide)
		if t.cursorAnnoIdx < len(annotations) {
			ann := annotations[t.cursorAnnoIdx]
			if ann.drifted {
				return 0, 0, ""
			}
			return ann.line, max(ann.line, ann.endLine), ann.side
		}
	}
	return 0, 0, ""
}

func (m AppModel) commentPanelWidth() int {
	if m.viewMode {
		return 0
	}
	return max(m.width/4, 26) // room for the Comments and Files tabs
}

// paneHorizontalPadding is the space between a pane's side borders and its body.
const paneHorizontalPadding = 1

// contentPaneWidth returns the total width of the tabbed content pane,
// including its borders.  View mode uses the full terminal width.
func (m AppModel) contentPaneWidth() int {
	if m.viewMode {
		return m.width
	}
	return m.width - m.commentPanelWidth()
}

// renderPane frames body under a tab row: the tab row's bottom edge serves as
// the top border, and body is wrapped by side borders and a bottom border.
func renderPane(tabRow, body string, border lipgloss.Border, style lipgloss.Style) string {
	left, right := style.Render(border.Left), style.Render(border.Right)
	var b strings.Builder
	b.WriteString(tabRow)
	b.WriteByte('\n')
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(left + line + right + "\n")
	}
	width := lipgloss.Width(tabRow)
	b.WriteString(style.Render(border.BottomLeft + strings.Repeat(border.Bottom, max(0, width-2)) + border.BottomRight))
	return b.String()
}

// commentPaneTabLabel returns the label of the comment pane's tab, truncated
// to fit the pane.
// sidebarBorderColor is the sidebar pane's border color for its focus state.
func (m AppModel) sidebarBorderColor() color.Color {
	if m.focused == commentPane {
		return commentFocusedBorderColor
	}
	return commentBorderColor
}

// sidebarTabs renders the sidebar's tabs in order, marking the active view.
func (m AppModel) sidebarTabs() []string {
	labels := []string{
		"Files",
		fmt.Sprintf("Comments (%d)", unresolvedCommentCount(m.tab().state.Comments)),
	}
	fg := m.sidebarBorderColor()
	tabs := make([]string, len(labels))
	for i, label := range labels {
		style := inactiveTabStyle.BorderForeground(fg)
		if sidebarView(i) == m.sidebarView {
			style = activeTabStyle.BorderForeground(fg)
		}
		border, _, _, _, _ := style.GetBorder()
		if i == 0 {
			border.BottomLeft = "├"
			if sidebarView(i) == m.sidebarView {
				border.BottomLeft = "│"
			}
		}
		tabs[i] = style.Border(border).Render(label)
	}
	return tabs
}

// sidebarTabAt returns the sidebar view whose tab spans screen column x.
func (m AppModel) sidebarTabAt(x int) (sidebarView, bool) {
	if m.viewMode {
		return 0, false
	}
	left := m.contentPaneWidth()
	for i, tab := range m.sidebarTabs() {
		right := left + lipgloss.Width(tab)
		if x >= left && x < right {
			return sidebarView(i), true
		}
		left = right
	}
	return 0, false
}

// renderSidebarPane renders the sidebar as a pane with Comments and Files
// tabs, matching the height of the content pane.  Focus brightens the
// border color, as with inline comment boxes.
func (m AppModel) renderSidebarPane() string {
	width := m.commentPanelWidth()
	border := lipgloss.RoundedBorder()
	line := lipgloss.NewStyle().Foreground(m.sidebarBorderColor())
	tabRow := fillTabRow(lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarTabs()...), width, border, line)
	view := m.commentViewport.View()
	if m.sidebarView == filesView {
		view = m.renderFilesSidebar()
	}
	body := lipgloss.NewStyle().
		Width(width-2).
		Height(m.commentViewport.Height()).
		Padding(0, paneHorizontalPadding).
		Render(view)
	return renderPane(tabRow, body, border, line)
}

// fillTabRow fits a rendered tab row to width.  A shorter row is extended
// with the pane's top border ending in the top-right corner; a row that fills
// the width exactly gets its last corner joined to the pane's right border.
func fillTabRow(row string, width int, border lipgloss.Border, style lipgloss.Style) string {
	gap := width - lipgloss.Width(row)
	if gap < 0 {
		return row
	}
	if gap == 0 {
		return joinTabRowRight(row, style)
	}
	blank := strings.Repeat(" ", gap)
	filler := style.Render(
		blank + "\n" + blank + "\n" + strings.Repeat(border.Top, gap-1) + border.TopRight)
	return lipgloss.JoinHorizontal(lipgloss.Top, row, filler)
}

// joinTabRowRight replaces the bottom-right corner of the last tab, which
// points outward, with a glyph that meets the pane's right border below it.
func joinTabRowRight(row string, style lipgloss.Style) string {
	lines := strings.Split(row, "\n")
	bottom := lines[len(lines)-1]
	corner := "┤"
	if strings.HasSuffix(ansi.Strip(bottom), activeTabBorder.BottomRight) {
		corner = "│"
	}
	lines[len(lines)-1] = ansi.Cut(bottom, 0, lipgloss.Width(bottom)-1) + style.Render(corner)
	return strings.Join(lines, "\n")
}

func (m *AppModel) commentBounds() (left, top, right, bottom int) {
	_, top, left, bottom = m.contentBounds()
	if m.viewMode {
		return left, top, left + m.commentPanelWidth(), bottom
	}
	left = m.contentPaneWidth() + 1
	return left, top, m.width - 1, bottom
}

func (m *AppModel) sidebarMouseTarget(y int) (int, bool) {
	if y < 0 || y >= len(m.sidebarTargets) {
		return 0, false
	}
	i := m.sidebarTargets[y]
	if i < 0 || i >= len(m.tab().sidebarItems) {
		return 0, false
	}
	return i, true
}

func (m *AppModel) selectSidebarItem(i int) {
	t := m.tab()
	item := t.sidebarItems[i]
	t.sidebarCursor = i
	if !isGeneralCommentScope(item.scope) {
		t.cursorLine, t.cursorSide = item.line, item.side
		t.cursorOnAnnotation = false
		t.cursorOnForm = false
		t.cursorAnnoIdx = 0
		m.scrollToAnnotation(item.side, item.line, item.endLine)
	}
}

func (m *AppModel) headerHeight() int {
	if m.width > 0 && len(m.tabs) > 0 {
		return lipgloss.Height(m.renderHeader())
	}
	return 1
}

func (m *AppModel) tabBarHeight() int {
	return lipgloss.Height(m.renderTabBar())
}

func (m *AppModel) contentBounds() (left, top, right, bottom int) {
	left = 1 // inside the pane's left border
	top = m.headerHeight() + m.tabBarHeight()
	right = left + m.contentViewport.Width()
	bottom = top + m.contentViewport.Height()
	return left, top, right, bottom
}

// finishButtonRect returns the screen rectangle of the Submit button in the
// top bar.
func (m *AppModel) finishButtonRect() (mouseRect, bool) {
	if len(m.tabs) == 0 || m.tab().state == nil || m.tab().selecting {
		return mouseRect{}, false
	}
	_, layout := m.renderReviewScreen()
	return layout.finishButton, layout.hasFinishButton
}

func (m *AppModel) footerHelpRect() (mouseRect, bool) {
	if len(m.tabs) == 0 || m.tab().state == nil {
		return mouseRect{}, false
	}
	_, layout := m.renderReviewScreen()
	return layout.footerHelp, layout.hasFooterHelp
}

func (m *AppModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if len(m.tabs) == 0 {
		return m, nil
	}
	if m.isTextModal() {
		return m.handleTextModalWheel(msg.Mouse())
	}
	if m.modal == lightboxModal {
		m.handleLightboxWheel(msg.Mouse())
		return m, nil
	}
	if m.modal == helpModal {
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			m.scrollHelp(-3)
		case tea.MouseWheelDown:
			m.scrollHelp(3)
		}
		return m, nil
	}
	if m.modal != noModal {
		return m, nil
	}
	mouse := msg.Mouse()
	if mouse.Button == tea.MouseWheelLeft || mouse.Button == tea.MouseWheelRight {
		if table := m.tableAtMouse(mouse); table != nil {
			direction := 1
			if mouse.Button == tea.MouseWheelLeft {
				direction = -1
			}
			m.scrollTable(*table, direction*max(1, table.window.width/2))
		}
		return m, nil
	}
	direction := 1
	if mouse.Button == tea.MouseWheelUp {
		direction = -1
	} else if mouse.Button != tea.MouseWheelDown {
		return m, nil
	}
	left, top, right, bottom := m.commentBounds()
	if !m.viewMode && mouse.X >= left && mouse.X < right && mouse.Y >= top && mouse.Y < bottom {
		if m.sidebarView == filesView {
			m.scrollFilesSidebar(mouse.Y-top, direction*3)
			return m, nil
		}
		if i, ok := m.sidebarMouseTarget(mouse.Y - top + m.commentViewport.YOffset()); ok {
			m.focused = commentPane
			m.tab().sidebarCursor = i
			m.updateCommentSidebar()
			m.rebuildContent()
			if m.scrollThread(threadViewKey{id: m.tab().sidebarItems[i].id, sidebar: true}, direction, false) {
				return m, nil
			}
		}
		if direction < 0 {
			m.commentViewport.ScrollUp(3)
		} else {
			m.commentViewport.ScrollDown(3)
		}
		return m, nil
	}
	left, top, right, bottom = m.contentBounds()
	if mouse.X < left || (!m.viewMode && mouse.X >= right) || mouse.Y < top || mouse.Y >= bottom {
		return m, nil
	}
	if target, ok := m.contentMouseTargetAt(mouse.X-left, mouse.Y-top+m.contentViewport.YOffset()); ok && target.annotation && mouse.X >= left+gutterWidth {
		annotations := m.annotationsAfterLine(target.line, target.side)
		if target.annotationIndex < len(annotations) {
			m.focused = contentPane
			t := m.tab()
			t.cursorLine, t.cursorSide = target.line, target.side
			t.cursorOnAnnotation, t.cursorAnnoIdx = true, target.annotationIndex
			m.rebuildContent()
			m.updateCommentSidebar()
			if m.scrollThread(threadViewKey{id: annotations[target.annotationIndex].id}, direction, false) {
				return m, nil
			}
		}
	}

	switch mouse.Button {
	case tea.MouseWheelUp:
		m.contentViewport.ScrollUp(m.contentViewport.MouseWheelDelta)
	case tea.MouseWheelDown:
		m.contentViewport.ScrollDown(m.contentViewport.MouseWheelDelta)
	}
	m.updateGutterHover(mouse)
	return m, nil
}

func (m *AppModel) handleTextModalWheel(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.scrollable {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.modalReferenceOffset = max(0, region.action.scrollOffset-3)
			case tea.MouseWheelDown:
				m.modalReferenceOffset = min(region.action.scrollMaxOffset, region.action.scrollOffset+3)
			}
			return m, nil
		}
		if !region.action.textarea {
			continue
		}
		m.modalFocus = 0
		m.modalTextarea.Focus()
		for range 3 {
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.modalTextarea.CursorUp()
			case tea.MouseWheelDown:
				m.modalTextarea.CursorDown()
			}
		}
		return m, nil
	}
	return m, nil
}

// renderTabBar renders the tab bar for multi-file mode.
func (m *AppModel) renderTabBar() string {
	labels := m.tabLabels()
	for i := range labels {
		rendered := m.renderTab(labels, i, i == 0)
		labels[i].rendered = rendered
		labels[i].width = lipgloss.Width(rendered)
	}

	addFiller := func(row string) string {
		return fillTabRow(row, m.contentPaneWidth(), lipgloss.RoundedBorder(), lipgloss.NewStyle().Foreground(accent))
	}

	start, end := m.visibleTabWindow(labels)
	if start == 0 && end == len(labels) {
		var tabs []string
		for i := range labels {
			tabs = append(tabs, labels[i].rendered)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
		return addFiller(row)
	}

	var parts []string
	if start > 0 {
		ind := m.renderTabOverflowIndicator(m.tabOverflowText(start, true), true)
		parts = append(parts, ind)
	}
	for i := start; i < end; i++ {
		parts = append(parts, m.renderTab(labels, i, i == start && start == 0))
	}
	if end < len(labels) {
		ind := m.renderTabOverflowIndicator(m.tabOverflowText(len(labels)-end, false), false)
		parts = append(parts, ind)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return addFiller(row)
}

// footerMinHeight is the terminal height above which the footer's key hints
// are shown; shorter terminals keep every row for the review itself.
const footerMinHeight = 22

// renderFooter renders the key hints and the Help button, or "" when the
// terminal is too short for them.
func (m AppModel) renderFooter() string {
	k := func(key, desc string) string {
		return footerKeyStyle.Render(keyHintNames.Replace(key)) + " " + footerDescStyle.Render(desc)
	}
	if m.reload.err != nil && !m.isearch.active {
		return footerStyle.Width(m.width).Render("Reload failed: " + headerLabel(m.reload.err.Error()))
	}
	if m.modal == noModal && m.hasTextSelection() {
		return ansi.Truncate(m.copyTextButton()+footerStyle.Render(" · esc: cancel text selection"), m.width, "")
	}
	if m.readOnly && !m.isearch.active && m.locationError == "" && m.notice.text == "" && m.height >= footerMinHeight {
		return m.renderFooterItems([]string{m.footerHelpButton(), m.outlineHint(), k("v", "select lines"), k("M-w", "copy reference"), k("q", "close"), m.searchHint()})
	}
	if m.isearch.active {
		controls, _ := m.isearchControls(0)
		return m.renderISearchPrompt() + "\n" + controls
	}
	if m.locationError != "" {
		return footerStyle.Width(m.width).Render(m.locationError)
	}
	if m.notice.text != "" {
		return footerNoticeStyle.Width(m.width).Render(ansi.Truncate(headerLabel(m.notice.text), max(0, m.width), "…"))
	}
	if m.height < footerMinHeight {
		return ""
	}
	t := m.tabs[m.activeTab]
	var items []string
	if t.selecting {
		items = []string{
			k("enter", "comment selection"),
			k("esc", "cancel"),
			k("v", "toggle select"),
		}
	} else {
		enterAction := "comment"
		comment, drifted := m.driftedComment(m.selectedCommentID())
		drifted = drifted && !comment.Resolved
		if drifted {
			enterAction = "toggle thread"
		}
		items = []string{
			k("[/]", "threads/forms"),
			k("{/}", "unresolved"),
			k("s", "sidebar"),
			k("t", "files/comments"),
			k("v", "select lines"),
			k("enter", enterAction),
			k("f", "file comment"),
		}
		if drifted {
			items = append(items, k("e", "edit/reply"))
		}
		if len(t.state.Comments) > 0 {
			items = append(items, k("r/space/R", "resolve/+next"))
		}
		if m.multiFile {
			items = append([]string{
				k("tab/S-tab", "next/prev tab"),
				k("n/N", "change/open/form"),
			}, items...)
		}
	}
	if row, ok := m.formRows[t.path][t.cursorLine]; ok && row.item != nil && !t.selecting && !t.cursorOnAnnotation && t.cursorSide == "" {
		action := "answer"
		if row.item.SelectAll != nil {
			action = "select all"
			if d := m.forms.Document(t.path); d != nil {
				if selected, total := m.selectAllState(d, row.item); selected == total {
					action = "clear all"
				}
			}
		}
		items = append([]string{k("space", action)}, items...)
	}
	if button := m.formListButton(); button != "" {
		items = append([]string{button}, items...)
	}
	items = append([]string{m.footerHelpButton(), m.imagesButton()}, items...)
	if !t.selecting {
		items = append(items, m.outlineHint())
	}
	items = append(items, m.searchHint())

	return m.renderFooterItems(items)
}

// Keep each key hint together so its clickable region survives wrapping.
func (m AppModel) renderFooterItems(items []string) string {
	var rows []string
	row := ""
	for _, item := range items {
		if item == "" {
			continue
		}
		if row != "" && lipgloss.Width(row)+3+lipgloss.Width(item) > m.width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += " · "
		}
		row += item
	}
	rows = append(rows, row)
	return footerStyle.Width(m.width).Render(strings.Join(rows, "\n"))
}

// formListButton opens the answer list when any attachment has a form.
func (m AppModel) formListButton() string {
	if m.forms == nil || !m.forms.HasForms() {
		return ""
	}
	return renderBarButton("Answers", "F")
}

func (m AppModel) footerHelpButton() string {
	return renderBarButton("Help", "M-?")
}

// buttonRect locates a rendered button in a block that starts at screen row
// top and may span several rows, and returns its screen rectangle.
func buttonRect(block string, top int, button string) (mouseRect, bool) {
	plain := ansi.Strip(button)
	for y, line := range strings.Split(block, "\n") {
		stripped := ansi.Strip(line)
		i := strings.Index(stripped, plain)
		if i < 0 {
			continue
		}
		left := lipgloss.Width(stripped[:i])
		return mouseRect{left: left, right: left + lipgloss.Width(plain), top: top + y, bottom: top + y + 1}, true
	}
	return mouseRect{}, false
}

type helpItem struct {
	keys string
	desc string
}

func renderHelpGroup(title string, items []helpItem, width int) string {
	if width < 6 {
		width = 6
	}
	keyWidth := 0
	for _, item := range items {
		if w := lipgloss.Width(keyHintNames.Replace(item.keys)); w > keyWidth {
			keyWidth = w
		}
	}
	keyWidth++
	if keyWidth > width-5 {
		keyWidth = width - 5
	}

	var b strings.Builder
	b.WriteString(helpHeadingStyle.Render(title))
	b.WriteString("\n")
	for i, item := range items {
		key := helpKeyStyle.Width(keyWidth).Render(keyHintNames.Replace(item.keys))
		b.WriteString(key + helpDescriptionStyle.Render(item.desc))
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func (m AppModel) renderHelp(innerWidth int) string {
	if m.readOnly {
		return renderHelpGroup("View (read-only)", []helpItem{
			{keys: "q/C-c", desc: "close without saving"},
			{keys: "up/down,j/k · PgUp/PgDn", desc: "move / scroll"},
			{keys: "left/right,h/l · horizontal wheel · ◀/▶", desc: "scroll a table horizontally; long cells wrap at 40 columns"},
			{keys: "V · O", desc: "reading view / open all comments"},
			{keys: "tab/shift+tab · 1-9", desc: "switch file"},
			{keys: "/ ? · C-s/C-r", desc: "search forward / backward"},
			{keys: "v · esc", desc: "select lines / cancel"},
			{keys: "M-w", desc: "copy selected text or reference"},
			{keys: "s/t", desc: "toggle sidebar / view"},
			{keys: "M-G", desc: "focus outline"},
			{keys: "C-↑/↓ · C-S-↑/↓", desc: "Files: focus tree/outline · resize outline"},
			{keys: "enter/esc", desc: "outline: return to source"},
			{keys: "w · S · I", desc: "whitespace / split / images"},
			{keys: "F5/M-r · W/S-F5/M-R", desc: "reload sources / toggle watch (default off)"},
			{keys: "M-g · M-p", desc: "go to line / open file"},
			{keys: "M-? · esc", desc: "help / close help"},
		}, innerWidth) + renderHostHelp(reviewHost(), innerWidth)
	}
	columnWidth := (innerWidth - 4) / 3
	general := renderHelpGroup("General", []helpItem{
		{keys: "enter", desc: "comment/open"},
		{keys: "f", desc: "file comment"},
		{keys: "F", desc: "answer list"},
		{keys: "M-↑/↓", desc: "move ranked item"},
		{keys: "v/space", desc: "select/form"},
		{keys: "s/t", desc: "sidebar/view"},
		{keys: "r/space · R", desc: "resolve / resolve and go to next"},
		{keys: "V · O", desc: "reading view / open all"},
		{keys: "w · I", desc: "WS changes · images"},
		{keys: "F5/M-r", desc: "reload sources"},
		{keys: "W/S-F5/M-R", desc: "watch (default off)"},
		{keys: "o/M-o", desc: "open image/link/reference on line"},
		{keys: "SPC/BS·M-o·RET·esc", desc: "zoom·next·open·close"},
		{keys: "d", desc: "delete comment"},
		{keys: "q/C-c", desc: "submit review"},
	}, columnWidth)

	navigation := renderHelpGroup("Navigation", []helpItem{
		{keys: "↑/↓,j/k", desc: "move"},
		{keys: "←/→,h/l", desc: "scroll table"},
		{keys: "PgUp/PgDn", desc: "half page"},
		{keys: "shift+↑/↓,C-u/d", desc: "half page"},
		{keys: "Home/End,g/G,</>", desc: "top/bottom"},
		{keys: "[/]", desc: "threads/forms"},
	}, columnWidth)
	codeReview := renderHelpGroup("Code review / search", []helpItem{
		{keys: "{/}", desc: "unresolved"},
		{keys: "/ ?", desc: "search text"},
		{keys: "M-e/g", desc: "editor/line"},
		{keys: "M-w", desc: "copy text/ref"},
		{keys: "M-p", desc: "open file"},
		{keys: "tab/shift+tab", desc: "next/prev file"},
		{keys: "alt+}/alt+{", desc: "next/prev file"},
		{keys: "1-9", desc: "file tab"},
		{keys: "n/N", desc: "diff/open/form"},
		{keys: "S · ←/→", desc: "split diff · diff side"},
		{keys: "↑/↓", desc: "choose file"},
		{keys: "enter/esc", desc: "open/cancel"},
	}, columnWidth)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, general, "  ", navigation, "  ", codeReview)
	contexts := renderHelpGroup("Selection and dialogs", []helpItem{
		{keys: "drag text · M-w · esc", desc: "select text · copy to kill ring and system clipboard · cancel"},
		{keys: "←/→,h/l · horizontal wheel · ◀/▶", desc: "scroll the cursor's table with keys, or the table under the pointer with the wheel or edge buttons; long cells wrap at 40 columns"},
		{keys: "V", desc: "toggle reading view: hide sidebar and fold all comments; leaving restores the previous expansion setting"},
		{keys: "O", desc: "toggle all comments open / default (resolved folded); in View, leave View and open all comments"},
		{keys: "C-↑/↓ · C-S-↑/↓", desc: "Files: focus tree/outline · grow/shrink outline (up to two thirds); drag the Outline divider to resize"},
		{keys: "M-G", desc: "focus the current file's outline, opening the Files sidebar"},
		{keys: "↑/↓,j/k · ←/→,h/l · enter/esc", desc: "outline: jump to entry · parent/child · return to source; Markdown headings and source declarations follow the source cursor"},
		{keys: "drag gutter", desc: "select lines for a comment"},
		{keys: "↑/↓,j/k · enter/v/esc · C-PgUp/PgDn", desc: "extend · comment/toggle/cancel selection · scroll thread"},
		{keys: "C-s/o/v · M-s · tab/S-tab · enter/esc", desc: "save/edit/paste · suggest · focus · activate/close"},
		{keys: "C-k/u/w,M-d · C-y/M-y · @path,tab", desc: "kill · yank · complete"},
		{keys: "tab · ↑/↓ · enter (path answers)", desc: "complete · choose · accept/save; no @ needed"},
		{keys: "a/r/c · tab/S-tab · enter · esc · q", desc: "select approve/reject/comment · focus · submit · back · abort (outside input)"},
		{keys: "←/→ (verdict) · S-tab/↑ · C-s (input)", desc: "select verdict · edit overall comment · save and return to verdict"},
		{keys: "]/} · [/{ (outside input)", desc: "first thread/form item · last one; braces skip resolved; close without one"},
		{keys: "space (form checkbox)", desc: "unchecked: check and edit input; checked: uncheck, keep input draft"},
		{keys: "r/esc (forms)", desc: "review/cancel"},
		{keys: "tab · enter (dialog counters)", desc: "focus count · jump to first pending/completed item"},
	}, innerWidth)

	search := renderHelpGroup("Incremental search", []helpItem{
		{keys: "C-s / C-r", desc: "next / previous match; repeat to wrap"},
		{keys: "enter / esc,C-g/c", desc: "keep match / cancel and return"},
		{keys: "backspace / C-y", desc: "undo input piece / paste kill ring"},
		{keys: "C-d / C-w", desc: "add following character / word"},
		{keys: "M-s i/c/w/r/m", desc: "toggle case / word / regexp / multi-file"},
		{keys: "terminal paste", desc: "paste text into the query"},
	}, innerWidth)
	help := columns + "\n\n" + search + "\n\n" + contexts
	return help + renderHostHelp(reviewHost(), innerWidth)
}

func renderHostHelp(host string, width int) string {
	var title string
	var items []helpItem
	switch host {
	case "tmux":
		title = "tmux controls (default bindings, 3.8+)"
		items = []helpItem{
			{keys: "prefix @", desc: "toggle floating / tiled pane"},
			{keys: "prefix ;", desc: "return to previous pane"},
		}
	case "Herdr":
		title = "Herdr controls (default bindings; prefix: C-b)"
		items = []helpItem{
			{keys: "prefix z", desc: "toggle pane zoom"},
			{keys: "prefix h/j/k/l", desc: "focus pane left/down/up/right"},
			{keys: "prefix tab/shift+tab", desc: "next / previous pane"},
			{keys: "prefix ?", desc: "show Herdr keybindings"},
		}
	case "Zellij":
		title = "Zellij controls (default bindings)"
		items = []helpItem{
			{keys: "C-g", desc: "lock / unlock Zellij controls"},
			{keys: "C-p e", desc: "toggle floating / tiled pane"},
			{keys: "C-p f", desc: "toggle pane fullscreen"},
			{keys: "C-p ; esc", desc: "return to previous pane"},
			{keys: "M-f", desc: "show / hide floating panes"},
			{keys: "M-←/↓/↑/→", desc: "move focus (left/right can switch tabs)"},
			{keys: "C-g p", desc: "Unlock First preset: then e/f/; as above; esc locks"},
		}
	case "cmux":
		title = "cmux controls (default bindings)"
		items = []helpItem{
			{keys: "cmd+shift+enter", desc: "toggle pane zoom"},
			{keys: "cmd+alt+←/↓/↑/→", desc: "focus pane left/down/up/right"},
			{keys: "cmd+[ / cmd+]", desc: "focus back / forward"},
			{keys: "cmd+shift+[ / ]", desc: "previous / next tab in pane"},
		}
	default:
		return ""
	}
	return "\n\n" + renderHelpGroup(title, items, width)
}

func (m *AppModel) scrollHelp(delta int) {
	lines := strings.Split(m.renderHelp(m.modalInnerWidth()), "\n")
	available := m.helpPageSize(m.modalInnerWidth(), m.height, len(lines))
	maxOffset := max(0, len(lines)-available)
	m.helpOffset = max(0, min(min(m.helpOffset, maxOffset)+delta, maxOffset))
}

func (m AppModel) helpPageSize(width, height, lineCount int) int {
	header, _ := m.helpHeader(width)
	footer, _ := m.helpFooter(width, 0, 0)
	available := max(1, height-modalStyle.GetVerticalFrameSize()-lipgloss.Height(header)-lipgloss.Height(footer))
	if lineCount > available {
		footer, _ = m.helpFooter(width, 1, 0)
		available = max(1, height-modalStyle.GetVerticalFrameSize()-lipgloss.Height(header)-lipgloss.Height(footer))
	}
	return available
}

func (m AppModel) helpFooter(width, pageSize, top int) (string, []modalMouseRegion) {
	brand := ansi.Truncate(BrandVersion(m.version, terminalIsDark), width, "…")
	if pageSize == 0 {
		return "\n" + lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(brand), nil
	}
	buttons, regions := layoutModalButtonRow([]modalButtonSpec{
		{rendered: m.renderModalButton("Up", "↑/PgUp", false), action: modalMouseAction{scrollable: true, scrollOffset: -pageSize}},
		{rendered: m.renderModalButton("Down", "↓/PgDn", false), action: modalMouseAction{scrollable: true, scrollOffset: pageSize}},
	}, width, top+1)
	if lipgloss.Height(buttons) == 1 && lipgloss.Width(buttons)+2+lipgloss.Width(brand) <= width {
		return "\n" + buttons + strings.Repeat(" ", width-lipgloss.Width(buttons)-lipgloss.Width(brand)) + brand, regions
	}
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(brand) + "\n" + buttons, regions
}

func (m AppModel) helpHeader(innerWidth int) (string, modalMouseRegion) {
	title, closeRegion := modalTitleRow(modalTitleStyle.MarginBottom(0).Render("Keyboard Help  (M-? / ESC to close)"), innerWidth)
	legend := helpLegendStyle.Render("C-: ctrl+, M-: meta/alt+, S-: shift+, RET: enter, DEL: backspace")
	return title + "\n" + lipgloss.Wrap(legend, innerWidth, ""), closeRegion
}

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalBtnFocusedLabel, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalBtnNormalLabel, modalBtnNormalKey)
}

func (m AppModel) renderPrimaryModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalBtnPrimaryFocusedLabel, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalBtnPrimaryLabel, modalBtnNormalKey)
}

// renderBarButton renders a button for the banner or footer.  Its label is
// bold so it stands out from the surrounding key hints.
func renderBarButton(label, hint string) string {
	return renderButton(label, hint, modalBtnFocusedLabel.Bold(true), modalBtnFocusedKey)
}

// renderAccessKeyBarButton highlights the first letter with the bar key style.
func renderAccessKeyBarButton(label string) string {
	_, size := utf8.DecodeRuneInString(label)
	style := modalBtnFocusedLabel.Padding(0)
	return style.Render(" ") + modalBtnFocusedKey.Render(label[:size]) + style.Render(label[size:]+" ")
}

// renderAccessKey underlines the first character of label to show that it
// is the key that triggers the control.
func renderAccessKey(label string, style lipgloss.Style) string {
	_, size := utf8.DecodeRuneInString(label)
	return style.Underline(true).Render(label[:size]) + style.Padding(0).Render(label[size:])
}

func renderButton(label, hint string, labelStyle, keyStyle lipgloss.Style) string {
	if hint == "" {
		return labelStyle.Render(label + " ")
	}
	keys := strings.Split(keyHintNames.Replace(hint), " / ")
	var renderedHint strings.Builder
	for i, key := range keys {
		if i > 0 {
			renderedHint.WriteString(labelStyle.Padding(0).Render(" / "))
		}
		renderedHint.WriteString(keyStyle.Render(key))
	}
	return labelStyle.Render(label+" ") + renderedHint.String()
}

func layoutModalTextarea(before, textareaView string, width int) (string, modalMouseRegion) {
	before = lipgloss.Wrap(before, width, "")
	textareaView = lipgloss.Wrap(textareaView, width, "")
	top := strings.Count(before, "\n")
	return before + textareaView + "\n\n", modalMouseRegion{
		rect: mouseRect{
			left: 0, top: top,
			right: min(width, lipgloss.Width(textareaView)), bottom: top + lipgloss.Height(textareaView),
		},
		action: modalMouseAction{textarea: true},
	}
}

func renderScrollableModalBox(content string, width, maxHeight, offset int) (string, int, int) {
	return renderModalBoxAt(content, width, maxHeight, offset, -1)
}

func renderModalBoxAt(content string, width, maxHeight, offset, initialOffset int) (string, int, int) {
	if content == "" || maxHeight < 3 {
		return "", 0, 0
	}

	contentWidth := max(1, width-2)
	// Measure and render with the same tab width; lipgloss expands tabs only when rendering.
	wrapped := lipgloss.Wrap(expandDisplayTabs(content), contentWidth, "")
	lines := strings.Split(wrapped, "\n")
	maxContentHeight := maxHeight - 2
	maxOffset := max(0, len(lines)-maxContentHeight)
	if initialOffset < 0 {
		initialOffset = maxOffset
	}
	if offset < 0 {
		offset = initialOffset
	}
	offset = min(offset, maxOffset)
	moreBelow := offset+maxContentHeight < len(lines)
	lines = lines[offset:min(len(lines), offset+maxContentHeight)]

	borderStyle := lipgloss.NewStyle().Foreground(subtle)
	topBorder, bottomBorder := strings.Repeat("─", contentWidth), strings.Repeat("─", contentWidth)
	if offset > 0 {
		topBorder = "↑" + strings.Repeat("─", contentWidth-1)
	}
	if moreBelow {
		bottomBorder = "↓" + strings.Repeat("─", contentWidth-1)
	}
	top := borderStyle.Render("╭" + topBorder + "╮")
	bottom := borderStyle.Render("╰" + bottomBorder + "╯")
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, line := range lines {
		line = ansi.Truncate(line, contentWidth, "")
		rows = append(rows, borderStyle.Render("│")+
			lipgloss.NewStyle().Width(contentWidth).Render(line)+borderStyle.Render("│"))
	}
	rows = append(rows, bottom)

	return strings.Join(rows, "\n"), offset, maxOffset
}

// modalMouseRegions returns the clickable regions inside the open dialog,
// excluding the dialog frame itself.
func (m AppModel) modalMouseRegions() []modalMouseRegion {
	var regions []modalMouseRegion
	if len(m.tabs) == 0 {
		_, regions = m.renderEmptyReview()
	} else {
		_, layout := m.renderReviewScreen()
		regions = layout.modalRegions
	}
	return slices.DeleteFunc(slices.Clone(regions), func(r modalMouseRegion) bool { return r.action.dialog })
}

// clickedModalDismiss reports whether mouse hit the dialog's close button or
// landed outside the dialog, either of which dismisses it like esc.
func (m AppModel) clickedModalDismiss(mouse tea.Mouse) bool {
	_, layout := m.renderReviewScreen()
	return dismissesModal(layout.modalRegions, mouse)
}

func dismissesModal(regions []modalMouseRegion, mouse tea.Mouse) bool {
	inside := false
	for _, region := range regions {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.close {
			return true
		}
		if region.action.dialog {
			inside = true
		}
	}
	return !inside
}

// modalTitleRow renders a dialog title with a close "x" at the right edge of
// the dialog body, returning the row and the region that closes the dialog.
func modalTitleRow(title string, width int) (string, modalMouseRegion) {
	closeButton := closeButtonStyle.Render("x")
	title = ansi.Truncate(title, max(0, width-1-lipgloss.Width(closeButton)), "…")
	gap := max(1, width-lipgloss.Width(title)-lipgloss.Width(closeButton))
	region := modalMouseRegion{
		rect:   mouseRect{left: width - 1, right: width, bottom: 1},
		action: modalMouseAction{close: true, focus: -1}, // not a button focus index
	}
	return title + strings.Repeat(" ", gap) + closeButton, region
}

// modalTitle renders a dialog title row with a close "x" followed by a blank
// line, as modalTitleStyle's bottom margin used to provide.
func modalTitle(text string, width int) (string, modalMouseRegion) {
	row, region := modalTitleRow(modalTitleStyle.MarginBottom(0).Render(text), width)
	return row + "\n", region
}

func (m AppModel) renderDangerModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalDangerBtnFocused, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalDangerBtnLabel, modalBtnNormalKey)
}

func (m AppModel) renderContextPreview(side string, start, end, maxWidth, maxLines int) string {
	t := m.tabs[m.activeTab]
	if t.doc == nil {
		return ""
	}
	var lines []string
	maxLineText := maxWidth - 7
	if maxLineText < 10 {
		maxLineText = 10
	}
	for i := start; i <= end; i++ {
		lineText := displaySourceText(m.anchorText(&t, side, i, i))
		if lineText == "" {
			continue
		}
		wrapped := lipgloss.Wrap(lineText, maxLineText, "")
		num := lineNumStyle.Render(fmt.Sprintf("%d", i))
		wrapLines := strings.Split(wrapped, "\n")
		for wi, wl := range wrapLines {
			if wi == 0 {
				lines = append(lines, num+" "+wl)
			} else {
				lines = append(lines, lipgloss.NewStyle().Width(6).Render("")+wl)
			}
		}
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], footerStyle.Render("  ... +"+countNoun(len(lines)-maxLines+1, "more line", "more lines")))
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderWithModal(background string) string {
	rendered, _ := m.renderWithModalLayout(background)
	return rendered
}

func (m AppModel) renderWithModalLayout(background string) (string, []modalMouseRegion) {
	frameStyle := modalStyle
	switch m.modal {
	case commentModal, fileCommentModal, replyModal, editModal, formTextModal:
		frameStyle = frameStyle.BorderForeground(commentFocusedBorderColor)
	}
	var modalContent string
	var modalNoticeHeight int
	var regions []modalMouseRegion
	bgW := lipgloss.Width(background)
	bgH := lipgloss.Height(background)
	modalWidth := m.modalWidth()
	innerWidth := modalWidth - 6

	switch m.modal {
	case gotoLineModal, openSourceModal:
		content, locationRegions := m.locationModalContent(innerWidth)
		regions = append(regions, locationRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)

	case formListModal:
		content, listRegions := m.formListModalContent(innerWidth, max(3, bgH-14))
		regions = append(regions, listRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)

	case fileSelectModal:
		// Size the list for the whole tab set, which is fixed while it is open.
		content, fileRegions := m.fileSelectModalContent(innerWidth, min(len(m.tabs), 20, max(3, bgH-12)))
		regions = append(regions, fileRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)

	case helpModal:
		header, closeRegion := m.helpHeader(innerWidth)
		regions = append(regions, closeRegion)
		lines := strings.Split(m.renderHelp(innerWidth), "\n")
		available := m.helpPageSize(innerWidth, bgH, len(lines))
		offset := max(0, min(m.helpOffset, len(lines)-available))
		body := strings.Join(lines[offset:min(len(lines), offset+available)], "\n")
		prefix := header + "\n" + body + "\n"
		pageSize := 0
		if len(lines) > available {
			pageSize = available
		}
		footer, scrollRegions := m.helpFooter(innerWidth, pageSize, strings.Count(prefix, "\n"))
		regions = append(regions, scrollRegions...)
		content := prefix + footer
		modalContent = frameStyle.Width(modalWidth).Render(content)

	case commentModal:
		start, end := m.selectionRange()
		side := m.selectionSide()
		titleText := fmt.Sprintf("Add Comment (line %d)", start)
		if start != end {
			titleText = fmt.Sprintf("Add Comment (lines %d-%d)", start, end)
		}
		title, closeRegion := modalTitle(titleText, innerWidth)
		contextContent := m.renderContextPreview(side, start, end, innerWidth-4, 0)
		buttonSpecs := []modalButtonSpec{
			{rendered: m.renderPrimaryModalButton("Save", "C-s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}
		if m.canSuggest() {
			buttonSpecs = append(buttonSpecs, modalButtonSpec{
				rendered: m.renderModalButton("Suggest", "M-s", m.modalFocus == 3),
				action:   modalMouseAction{focus: 3},
			})
		}

		buildContent := func(contextSection string, scrollOffset, scrollMaxOffset int) (string, []modalMouseRegion) {
			content := title + "\n"
			var contentRegions []modalMouseRegion
			if contextSection != "" {
				contextTop := strings.Count(content, "\n")
				content += contextSection + "\n\n"
				contentRegions = append(contentRegions, modalMouseRegion{
					rect: mouseRect{
						left: 0, top: contextTop,
						right: lipgloss.Width(contextSection), bottom: contextTop + lipgloss.Height(contextSection),
					},
					action: modalMouseAction{
						scrollable: true, scrollOffset: scrollOffset, scrollMaxOffset: scrollMaxOffset,
					},
				})
			}
			content, textareaRegion := layoutModalTextarea(content, m.clipboardTextareaView(), innerWidth)
			contentRegions = append(contentRegions, textareaRegion)
			buttons, buttonRegions := layoutModalActions(buttonSpecs, innerWidth, strings.Count(content, "\n"))
			content += buttons
			contentRegions = append(contentRegions, buttonRegions...)
			return content, contentRegions
		}

		fixedContent, _ := buildContent("", 0, 0)
		fixedHeight := lipgloss.Height(frameStyle.Width(modalWidth).Render(fixedContent))
		contextHeight := max(3, bgH-fixedHeight-3)
		contextSection, scrollOffset, scrollMaxOffset := renderScrollableModalBox(
			contextContent, innerWidth-2, contextHeight, m.modalReferenceOffset)
		content, contentRegions := buildContent(contextSection, scrollOffset, scrollMaxOffset)
		modalContent = frameStyle.Width(modalWidth).Render(content)
		regions = append(regions, contentRegions...)
		regions = append(regions, closeRegion)

	case fileCommentModal, formTextModal:
		titleText, context := "Add File Comment", m.tab().displayPath()
		if m.modal == formTextModal {
			titleText, context = "Answer", m.formTextLabel()
			if kind := m.pathInputType(); kind != "" {
				switch kind {
				case form.InputFile:
					titleText = "File"
				case form.InputDirectory:
					titleText = "Directory"
				default:
					titleText = "Path"
				}
			}
		}
		title, closeRegion := modalTitle(titleText, innerWidth)
		path := contextBoxStyle.Width(innerWidth - 2).Render(context)
		textareaView := m.clipboardTextareaView()
		pathInput := m.pathInputType() != ""
		if pathInput {
			textareaView, _ = m.editorView()
		}
		before := title + "\n" + path + "\n\n"
		if pathInput {
			title, closeRegion = modalTitleRow(modalTitleStyle.MarginBottom(0).Render(titleText), innerWidth)
			before = title + "\n" + path + "\n"
		}
		prefix, textareaRegion := layoutModalTextarea(before, textareaView, innerWidth)
		regions = append(regions, textareaRegion)
		buttons, buttonRegions := layoutModalActions([]modalButtonSpec{
			{rendered: m.renderPrimaryModalButton("Save", "C-s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		regions = append(regions, closeRegion)
		modalContent = frameStyle.Width(modalWidth).Render(prefix + buttons)
		if notice, tint := m.pathNotice(); pathInput && notice != "" {
			status := lipgloss.NewStyle().Foreground(tint).Bold(true).
				Width(innerWidth).Align(lipgloss.Right).Render(notice)
			withNotice := frameStyle.Width(modalWidth).Render(prefix + buttons + "\n" + status)
			modalNoticeHeight = lipgloss.Height(withNotice) - lipgloss.Height(modalContent)
			modalContent = withNotice
		}

	case replyModal, editModal:
		titleText := "Edit Comment"
		if m.modal == replyModal {
			titleText = "Add Reply"
		} else if m.editingReplyID != "" {
			titleText = "Edit Reply"
		}
		title, closeRegion := modalTitle(titleText, innerWidth)
		if m.hasModalResolution() {
			comment, _ := m.driftedComment(m.editingID)
			checkbox := renderResolution(comment.Resolved, "Resolved", false, false) + " " + modalBtnNormalKey.Render("M-r")
			checkboxWidth := lipgloss.Width(checkbox)
			left := max(0, innerWidth-checkboxWidth-3)
			title = ansi.Truncate(modalTitleStyle.MarginBottom(0).Render(titleText), max(0, left-1), "…")
			title = lipgloss.NewStyle().Width(left).Render(title) + checkbox + "  " + closeButtonStyle.Render("x") + "\n"
			regions = append(regions, modalMouseRegion{
				rect:   mouseRect{left: left, right: left + checkboxWidth, bottom: 1},
				action: modalMouseAction{resolve: true},
			})
		}
		var referenceContent string
		var thread threadLayout
		var threadStart int
		for _, c := range m.tabs[m.activeTab].state.Comments {
			if c.ID == m.editingID {
				if commentDrifted(m.tab(), c) {
					referenceContent = driftedContext(c)
				} else if isGeneralCommentScope(c.Scope) {
					referenceContent = m.tab().displayPath()
				} else {
					start := c.StartLine
					end := c.EndAt()
					referenceContent = m.renderContextPreview(c.Side, start, end, innerWidth-4, 0)
				}
				if m.modal == replyModal || m.editingReplyID != "" {
					thread = m.layoutThread(c.Author, c.Body, c.Replies, m.editingReplyID, innerWidth-4)
					if referenceContent != "" {
						referenceContent = lipgloss.Wrap(referenceContent, max(1, innerWidth-4), "")
						threadStart = strings.Count(referenceContent, "\n") + 2
						referenceContent += "\n\n"
					}
					referenceContent += strings.Join(thread.lines, "\n")
				}
				break
			}
		}
		buttonSpecs := []modalButtonSpec{
			{rendered: m.renderPrimaryModalButton("Save", "C-s", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Close", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}
		if m.canSuggest() {
			buttonSpecs = append(buttonSpecs, modalButtonSpec{
				rendered: m.renderModalButton("Suggest", "M-s", m.modalFocus == 3),
				action:   modalMouseAction{focus: 3},
			})
		}

		buildContent := func(referenceSection string, scrollOffset, scrollMaxOffset int) (string, []modalMouseRegion) {
			content := title + "\n"
			var contentRegions []modalMouseRegion
			if referenceSection != "" {
				referenceTop := strings.Count(content, "\n")
				content += referenceSection + "\n\n"
				contentRegions = append(contentRegions, modalMouseRegion{
					rect: mouseRect{
						left: 0, top: referenceTop,
						right: lipgloss.Width(referenceSection), bottom: referenceTop + lipgloss.Height(referenceSection),
					},
					action: modalMouseAction{
						scrollable: true, scrollOffset: scrollOffset, scrollMaxOffset: scrollMaxOffset,
					},
				})
			}
			content, textareaRegion := layoutModalTextarea(content, m.clipboardTextareaView(), innerWidth)
			contentRegions = append(contentRegions, textareaRegion)
			buttonY := strings.Count(content, "\n")
			buttonRow, buttonRegions := layoutModalActions(buttonSpecs, innerWidth, buttonY)
			content += buttonRow
			contentRegions = append(contentRegions, buttonRegions...)
			buttonY += lipgloss.Height(buttonRow)
			deleteStart := m.modalDeleteStartFocus()
			for i, target := range m.modalDeleteTargets() {
				content += "\n"
				deleteRow, deleteRegions := layoutModalActions([]modalButtonSpec{{
					rendered: m.renderDangerModalButton(target.label, "", m.modalFocus == i+deleteStart),
					action:   modalMouseAction{focus: i + deleteStart, deleteIndex: i},
				}}, innerWidth, buttonY)
				content += deleteRow
				contentRegions = append(contentRegions, deleteRegions...)
				buttonY += lipgloss.Height(deleteRow)
			}
			return content, contentRegions
		}

		fixedContent, _ := buildContent("", 0, 0)
		fixedHeight := lipgloss.Height(frameStyle.Width(modalWidth).Render(fixedContent))
		referenceHeight := max(3, min(18, bgH-fixedHeight-3))
		initialOffset := -1
		if len(thread.starts) > 0 {
			initialOffset = threadStart + thread.initialOffset(referenceHeight-2)
		}
		if _, drifted := m.driftedComment(m.editingID); drifted && len(thread.starts) == 0 {
			initialOffset = 0
		}
		referenceSection, scrollOffset, scrollMaxOffset := renderModalBoxAt(
			referenceContent, innerWidth-2, referenceHeight, m.modalReferenceOffset, initialOffset)
		content, contentRegions := buildContent(referenceSection, scrollOffset, scrollMaxOffset)
		modalContent = frameStyle.Width(modalWidth).Render(content)
		regions = append(regions, contentRegions...)
		regions = append(regions, closeRegion)

	case discardChangesModal:
		title, closeRegion := modalTitle("Discard changes?", innerWidth)
		info := "Your unsaved comment changes will be lost."
		prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", innerWidth, "")
		buttons, buttonRegions := layoutModalActions([]modalButtonSpec{
			{rendered: m.renderDangerModalButton("Discard", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
			{rendered: m.renderModalButton("Keep Editing", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		regions = append(regions, closeRegion)
		modalContent = frameStyle.Width(modalWidth).Render(prefix + buttons)

	case deleteConfirmModal:
		titleText := "Delete comment?"
		if targets := m.modalDeleteTargets(); m.pendingDelete >= 0 && m.pendingDelete < len(targets) && targets[m.pendingDelete].replyID != "" {
			titleText = "Delete reply?"
		}
		title, closeRegion := modalTitle(titleText, innerWidth)
		info := "This cannot be undone."
		prefix := lipgloss.Wrap(title+"\n"+info+"\n\n", innerWidth, "")
		buttons, buttonRegions := layoutModalActions([]modalButtonSpec{
			{rendered: m.renderDangerModalButton("Delete", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
			{rendered: m.renderModalButton("Keep", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
		}, innerWidth, strings.Count(prefix, "\n"))
		regions = append(regions, buttonRegions...)
		regions = append(regions, closeRegion)
		modalContent = frameStyle.Width(modalWidth).Render(prefix + buttons)

	case finishAbortModal:
		frameStyle = frameStyle.BorderForeground(finishColor(review.VerdictReject))
		content, abortRegions := m.finishAbortContent(innerWidth)
		regions = append(regions, abortRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)
	case finishResolveModal:
		frameStyle = frameStyle.BorderForeground(success)
		content, resolveRegions := m.finishResolveContent(innerWidth)
		regions = append(regions, resolveRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)
	case finishFormsModal:
		frameStyle = frameStyle.BorderForeground(lipgloss.Yellow)
		content, formRegions := m.finishFormsContent(innerWidth)
		regions = append(regions, formRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)
	case finishModal:
		frameStyle = frameStyle.BorderForeground(m.finishAttentionColor())
		content, finishRegions := m.finishModalContent(innerWidth)
		regions = append(regions, finishRegions...)
		modalContent = frameStyle.Width(modalWidth).Render(content)
	case lightboxModal:
		content, width, lightboxRegions := m.lightboxContent()
		regions = append(regions, lightboxRegions...)
		modalContent = frameStyle.Width(width).Render(content)
	}

	modalW := lipgloss.Width(modalContent)
	modalH := lipgloss.Height(modalContent)

	mx := (bgW - modalW) / 2
	my := (bgH - modalH) / 2
	if modalNoticeHeight > 0 {
		// Grow a notice below the actions without recentering the controls.
		my = min((bgH-(modalH-modalNoticeHeight))/2, bgH-modalH)
	}
	if mx < 0 {
		mx = 0
	}
	if modalH > bgH && (m.modal == commentModal || m.modal == replyModal || m.modal == editModal) {
		my = bgH - modalH
	} else if my < 0 {
		my = 0
	}
	contentX := mx + frameStyle.GetBorderLeftSize() + frameStyle.GetPaddingLeft()
	contentY := my + frameStyle.GetBorderTopSize() + frameStyle.GetPaddingTop()
	for i := range regions {
		regions[i].rect.left += contentX
		regions[i].rect.right += contentX
		regions[i].rect.top += contentY
		regions[i].rect.bottom += contentY
	}
	regions = append(regions, modalMouseRegion{
		rect:   mouseRect{left: mx, top: my, right: mx + modalW, bottom: my + modalH},
		action: modalMouseAction{dialog: true},
	})

	background = dimRendered(background, bgW, bgH)

	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(modalContent).X(mx).Y(my).Z(1),
	}
	if menu := m.completionLayer(regions, bgW, bgH); menu != nil {
		layers = append(layers, menu)
	}
	return lipgloss.NewCompositor(layers...).Render(), regions
}

func (m AppModel) modalWidth() int {
	width := m.width * 2 / 3
	if m.modal == helpModal {
		width = m.width - 4
	}
	return max(10, min(max(50, width), m.width-4))
}

func (m AppModel) modalInnerWidth() int {
	return m.modalWidth() - 6
}

func dimRendered(s string, w, h int) string {
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewLayer(s))

	// Backgrounds are dimmed too, so text on a gray background stays
	// distinguishable from the dimmed foreground.  Light terminals dim
	// toward white so the dialog still stands out from a pale page.
	ld := lipgloss.LightDark(terminalIsDark)
	dim := ld(lipgloss.Color("#aaaaaa"), lipgloss.Color("#555555"))
	dimBg := ld(lipgloss.Color("#dddddd"), lipgloss.Color("#2a2a2a"))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			cell.Style.Fg = dim
			if cell.Style.Bg != nil {
				cell.Style.Bg = dimBg
			}
		}
	}
	return canvas.Render()
}

// countNoun formats n with the noun in the matching number.
func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func reviewHost() string {
	return os.Getenv("CRITTY_HOST")
}
