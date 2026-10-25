package tui

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

type reloadState struct {
	context     context.Context
	watcher     *sourceWatcher
	watch       bool
	epoch       uint64
	request     uint64
	busy        bool
	pending     bool
	dirty       bool
	fingerprint string
	err         error
}

type reloadMsg struct {
	request     uint64
	epoch       uint64
	automatic   bool
	fingerprint string
	model       *AppModel
	err         error
}

func (m AppModel) frozenDocument() bool {
	return m.session != nil && m.session.Meta.Mode == "plan"
}

func (m AppModel) canReload() bool {
	if m.patch == nil && !m.frozenDocument() {
		return true
	}
	for _, d := range m.attachments {
		if !d.Removed && d.Source != "" {
			return true
		}
	}
	return false
}

// CanWatch reports whether at least one review input has a live source.
func (m AppModel) CanWatch() bool {
	if m.patch == nil && !m.frozenDocument() && (m.source == nil || m.source.Scope != "range") {
		return true
	}
	for _, d := range m.attachments {
		if !d.Removed && d.Source != "" {
			return true
		}
	}
	return false
}

func (m AppModel) reloadBlocked() bool {
	return m.modal != noModal || m.isearch.active || m.textSelection.dragging || m.hasTextSelection() ||
		len(m.tabs) > 0 && (m.tab().state == nil || m.tab().selecting)
}

func (m *AppModel) toggleWatch() tea.Cmd {
	if !m.CanWatch() {
		return nil
	}
	m.reload.watch = !m.reload.watch
	m.reload.epoch++
	m.reload.dirty = false
	if m.reload.watcher != nil {
		m.reload.watcher.Close()
		m.reload.watcher = nil
	}
	if m.reload.watch {
		return m.startWatch()
	}
	return nil
}

func (m *AppModel) resumeReload() tea.Cmd {
	if (!m.reload.pending && (!m.reload.watch || !m.reload.dirty)) || m.reloadBlocked() || m.reload.busy {
		return nil
	}
	force := m.reload.pending
	m.reload.pending, m.reload.dirty = false, false
	return m.requestReload(force)
}

func (m *AppModel) requestReload(force bool) tea.Cmd {
	if !m.canReload() || m.err != nil {
		return nil
	}
	if m.reloadBlocked() || m.reload.busy {
		m.reload.pending = m.reload.pending || force
		return nil
	}
	m.reload.busy = true
	m.reload.request++
	request, epoch := m.reload.request, m.reload.epoch
	previous := m.reload.fingerprint
	filePath, frozen, patch := m.filePath, m.frozenDocument(), m.patch
	var source *gitpkg.ReviewSource
	if m.source != nil {
		copy := *m.source
		source = &copy
	} else if filePath == "" && patch == nil {
		scope := "all"
		if m.staged {
			scope = "staged"
		}
		source = &gitpkg.ReviewSource{Scope: scope, Base: m.baseRef}
	}
	docs := make([]review.AttachedDocument, 0, len(m.attachments))
	for _, tab := range m.tabs {
		if tab.attachment != nil {
			docs = append(docs, *tab.attachment)
		}
	}
	branch := ""
	if source != nil && m.session != nil {
		branch = m.session.Meta.Branch
	}
	return func() tea.Msg {
		msg := reloadMsg{request: request, epoch: epoch, automatic: !force}
		next, fingerprint, err := readReloadInputs(source, filePath, frozen, patch, docs, branch, previous, force)
		msg.model, msg.fingerprint, msg.err = next, fingerprint, err
		return msg
	}
}

// readReloadInputs only reads sources.  Session changes stay on the UI loop.
func readReloadInputs(source *gitpkg.ReviewSource, filePath string, frozen bool, patch *gitpkg.Patch, docs []review.AttachedDocument, branch, previous string, force bool) (*AppModel, string, error) {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	var files []gitpkg.FileChange
	if source != nil {
		if branch != "" && source.Scope != "range" {
			current, err := gitpkg.CurrentBranch()
			if err != nil {
				return nil, "", err
			}
			if current != branch {
				return nil, "", fmt.Errorf("review belongs to branch %s", branch)
			}
		}
		var err error
		files, err = source.Files()
		if err != nil {
			return nil, "", err
		}
		fingerprint, err := source.Fingerprint(files)
		if err != nil {
			return nil, "", err
		}
		if err := encoder.Encode(fingerprint); err != nil {
			return nil, "", err
		}
	}
	paths := []string{}
	if filePath != "" && !frozen {
		paths = append(paths, filePath)
	}
	for _, d := range docs {
		if !d.Removed && d.Source != "" {
			paths = append(paths, d.Source)
		}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, "", err
		}
		if err := encoder.Encode(struct {
			Path     string
			Size     int64
			Modified int64
			Mode     os.FileMode
		}{path, info.Size(), info.ModTime().UnixNano(), info.Mode()}); err != nil {
			return nil, "", err
		}
	}
	fingerprint := fmt.Sprintf("%x", hash.Sum(nil))
	if !force && fingerprint == previous {
		return nil, fingerprint, nil
	}
	var attachmentPaths []string
	for _, d := range docs {
		if !d.Removed && d.Source != "" {
			attachmentPaths = append(attachmentPaths, d.Source)
		}
	}
	fresh, err := review.ReadAttachedDocuments(attachmentPaths)
	if err != nil {
		return nil, "", err
	}
	bySource := make(map[string]review.AttachedDocument, len(fresh))
	for _, d := range fresh {
		bySource[d.Source] = d
	}
	for i, d := range docs {
		if next, ok := bySource[d.Source]; ok && !d.Removed {
			docs[i] = next
		}
	}
	cfg := AppConfig{Documents: docs, Source: source, Patch: patch}
	var next AppModel
	if filePath != "" {
		next = NewApp(filePath, cfg)
	} else {
		if patch != nil {
			files = patch.Changes()
		}
		ref := ""
		if source != nil {
			ref = source.Base
		}
		next = NewCodeReviewApp(files, ref, cfg)
	}
	for i := range next.tabs {
		tab := &next.tabs[i]
		if tab.loadErr != nil {
			return nil, "", tab.loadErr
		}
		if tab.isBinary || tab.isDeleted {
			continue
		}
		doc, err := next.loadDocument(tab.path)
		if err != nil {
			return nil, "", err
		}
		tab.doc = doc
	}
	return &next, fingerprint, nil
}

func (m *AppModel) finishReload(msg reloadMsg) tea.Cmd {
	if msg.request != m.reload.request {
		return nil
	}
	m.reload.busy = false
	if msg.automatic && (!m.reload.watch || msg.epoch != m.reload.epoch) {
		return nil
	}
	if m.reloadBlocked() {
		m.reload.pending = m.reload.pending || !msg.automatic
		m.reload.dirty = m.reload.dirty || msg.automatic
		return nil
	}
	if msg.err != nil {
		m.reload.err = msg.err
		return nil
	}
	if msg.model == nil {
		m.reload.err = nil
		return nil
	}
	m.persist()
	if m.err != nil {
		return nil
	}
	active, offset := "", m.contentViewport.YOffset()
	var previous FileTab
	if len(m.tabs) > 0 {
		active = m.tab().path
		previous = *m.tab()
	}
	old := make(map[string]FileTab, len(m.tabs))
	for _, tab := range m.tabs {
		old[tab.path] = tab
	}
	next := msg.model
	hasCurrentChanges := false
	for _, tab := range next.tabs {
		if tab.attachment == nil && tab.path != overallCommentsPath && !tab.outsideChanges {
			hasCurrentChanges = true
		}
	}
	for i := range next.tabs {
		tab := &next.tabs[i]
		if previous, ok := old[tab.path]; ok {
			tab.unread = previous.unread || tabContentChanged(previous, *tab)
			tab.cursorLine, tab.cursorSide = previous.cursorLine, previous.cursorSide
			tab.sidebarCursor = previous.sidebarCursor
			delete(old, tab.path)
		} else {
			tab.unread = true
		}
	}
	for _, tab := range m.tabs {
		keepSnapshot := !hasCurrentChanges && tab.outsideChanges && tab.doc != nil
		if _, ok := old[tab.path]; ok && tab.state != nil && (len(tab.state.Comments) > 0 || keepSnapshot) {
			tab.unread = tab.unread || !tab.outsideChanges
			tab.outsideChanges = true
			next.tabs = append(next.tabs, tab)
		}
	}
	if m.session != nil {
		present := map[string]bool{}
		for _, tab := range next.tabs {
			present[tab.path] = true
		}
		for path, file := range m.session.CJ.Files {
			if present[path] || len(file.Comments) == 0 {
				continue
			}
			tab := newFileTab(path, nil)
			tab.outsideChanges = true
			if snapshot, ok := m.session.CJ.RoundState.Files[path]; ok {
				tab.doc = document.FromContent(path, []byte(snapshot.Content))
			}
			next.tabs = append(next.tabs, tab)
		}
	}
	m.tabs, m.source, m.baseRef = next.tabs, next.source, next.baseRef
	m.documentSnapshot = next.documentSnapshot
	m.attachments = next.attachments
	m.orderTabsByTree()
	m.activeTab = 0
	for i := range m.tabs {
		if m.tabs[i].path == active {
			m.activeTab = i
		}
	}
	if m.session != nil {
		var docs []review.AttachedDocument
		for _, tab := range m.tabs {
			if tab.attachment != nil && !tab.attachment.Removed {
				docs = append(docs, *tab.attachment)
			}
		}
		if err := m.session.ReplaceDocuments(docs); err != nil {
			m.err = err
			return nil
		}
		if err := m.restoreRound(); err != nil {
			m.err = err
			return nil
		}
	}
	for i := range m.tabs {
		tab := &m.tabs[i]
		tab.state = &fileReview{Comments: m.sessionComments(tab.path)}
		if tab.doc != nil {
			tab.cursorLine = min(max(1, tab.cursorLine), max(1, tab.doc.LineCount()))
			tab.ensureHighlightCache()
		}
	}
	m.ensureOverallCommentsTab()
	if m.ignoreWhitespace {
		m.ignoreWhitespace = false
		m.toggleWhitespace()
	}
	m.formViewed = nil
	m.viewAttention = nil
	m.commentImages, m.imageBodies = nil, nil
	m.fileTree.syncedTab = -1
	m.reload.fingerprint, m.reload.err = msg.fingerprint, nil
	m.recalculateLayout()
	m.rebuildContent()
	m.updateCommentSidebar()
	if len(m.tabs) > 0 && m.tab().path == active {
		m.contentViewport.SetYOffset(offset)
		m.scrollToCursor()
	}
	if active != "" {
		if len(m.tabs) == 0 || m.tab().path != active {
			return m.flashNotice("Review updated")
		}
		if tabContentChanged(previous, *m.tab()) {
			return m.flashNotice("Updated: " + m.tab().displayPath())
		}
	}
	return nil
}

func (m AppModel) watchIndicator() string {
	if !m.CanWatch() || !m.reload.watch {
		return ""
	}
	return modalBtnFocusedKey.Render("W") + modalBtnFocusedLabel.Bold(true).Padding(0).Render("atch: on")
}

func (m AppModel) renderReloadStatus() string {
	if m.reload.err != nil {
		return "Reload failed: " + headerLabel(m.reload.err.Error())
	}
	return ""
}

func tabContentChanged(before, after FileTab) bool {
	content := func(tab FileTab) string {
		if tab.doc == nil {
			return ""
		}
		return tab.doc.Content
	}
	return content(before) != content(after) || !reflect.DeepEqual(before.diff, after.diff) ||
		before.binaryRevision != after.binaryRevision || before.status != after.status || before.isBinary != after.isBinary || before.isDeleted != after.isDeleted ||
		before.outsideChanges != after.outsideChanges || before.oldPath != after.oldPath || before.textconv != after.textconv
}

// Binary previews have no document text or line diff to compare.  Keep a cheap
// source revision without reading their contents on each reload.
func (m *AppModel) loadBinaryRevisions() {
	var paths []string
	for _, tab := range m.tabs {
		if tab.isBinary && tab.attachment == nil && !tab.isDeleted && !tab.outsideChanges {
			paths = append(paths, tab.path)
		}
	}
	if len(paths) == 0 {
		return
	}
	var revisions map[string]string
	var err error
	source := m.source
	if source == nil && m.staged {
		source = &gitpkg.ReviewSource{Scope: "staged"}
	}
	if source != nil && (source.Scope == "staged" || source.Scope == "range") {
		revisions, err = source.ObjectIDs(paths)
	} else {
		revisions = make(map[string]string, len(paths))
		for _, path := range paths {
			info, statErr := os.Stat(path)
			if os.IsNotExist(statErr) {
				revisions[path] = "missing"
				continue
			}
			if statErr != nil {
				err = statErr
				break
			}
			revisions[path] = fmt.Sprintf("%d:%d:%v", info.Size(), info.ModTime().UnixNano(), info.Mode())
		}
	}
	for i := range m.tabs {
		tab := &m.tabs[i]
		if !tab.isBinary || tab.attachment != nil || tab.isDeleted || tab.outsideChanges {
			continue
		}
		tab.binaryRevision = revisions[tab.path]
		if err != nil && tab.loadErr == nil {
			tab.loadErr = err
		}
	}
}
