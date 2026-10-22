package tui

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"

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
	return m.modal != noModal || m.isearch.active || len(m.tabs) > 0 && (m.tab().state == nil || m.tab().selecting)
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
		if branch != "" {
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

func (m *AppModel) finishReload(msg reloadMsg) {
	if msg.request != m.reload.request {
		return
	}
	m.reload.busy = false
	if msg.automatic && (!m.reload.watch || msg.epoch != m.reload.epoch) {
		return
	}
	if m.reloadBlocked() {
		m.reload.pending = m.reload.pending || !msg.automatic
		m.reload.dirty = m.reload.dirty || msg.automatic
		return
	}
	if msg.err != nil {
		m.reload.err = msg.err
		return
	}
	if msg.model == nil {
		m.reload.err = nil
		return
	}
	m.persist()
	if m.err != nil {
		return
	}
	active, offset := "", m.contentViewport.YOffset()
	if len(m.tabs) > 0 {
		active = m.tab().path
	}
	old := make(map[string]FileTab, len(m.tabs))
	for _, tab := range m.tabs {
		old[tab.path] = tab
	}
	next := msg.model
	for i := range next.tabs {
		tab := &next.tabs[i]
		if previous, ok := old[tab.path]; ok {
			tab.cursorLine, tab.cursorSide = previous.cursorLine, previous.cursorSide
			tab.sidebarCursor = previous.sidebarCursor
			delete(old, tab.path)
		}
	}
	for _, tab := range m.tabs {
		if _, ok := old[tab.path]; ok && tab.state != nil && len(tab.state.Comments) > 0 {
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
			return
		}
		if err := m.restoreRound(); err != nil {
			m.err = err
			return
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
