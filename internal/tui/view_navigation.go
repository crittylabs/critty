package tui

import (
	"bytes"
	"maps"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/icons"
)

const viewHistoryLimit = 64

type viewVisit struct {
	tab FileTab
}

type viewNavigation struct {
	back, forward []viewVisit
	request       uint64
	watchPath     string
}

type viewLinkLoadedMsg struct {
	request     uint64
	path        string
	fragment    string
	doc         *document.Document
	unsupported bool
	reload      bool
	err         error
}

func (m *AppModel) currentViewVisit() viewVisit {
	m.saveTabViewport()
	tab := *m.tab()
	tab.horizontalOffsets = maps.Clone(tab.horizontalOffsets)
	tab.preview.before.image, tab.preview.after.image = nil, nil
	tab.preview.before.attempted, tab.preview.after.attempted = false, false
	tab.markdownImages = slices.Clone(tab.markdownImages)
	for i := range tab.markdownImages {
		ref := &tab.markdownImages[i]
		if ref.image != nil {
			ref.columns, ref.rows = ref.image.columns, ref.image.rows
		}
		ref.image, ref.attempted = nil, false
	}
	tab.markdownImagesPending = false
	return viewVisit{tab: tab}
}

func appendViewVisit(history []viewVisit, visit viewVisit) []viewVisit {
	if len(history) == viewHistoryLimit {
		copy(history, history[1:])
		history = history[:len(history)-1]
	}
	return append(history, visit)
}

func (m *AppModel) followViewLink(destination string) (tea.Cmd, bool) {
	if _, ok := webImageURL(destination); ok {
		return m.openLink(destination, false), true
	}
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || (u.Path == "" && u.Fragment == "") {
		return nil, false
	}
	path := m.sourcePath(m.tab().path)
	if u.Path != "" {
		var ok bool
		path, ok = m.localLinkPath(destination, false)
		if !ok {
			return nil, false
		}
	}
	for i := range m.tabs {
		tab := &m.tabs[i]
		if m.sourcePath(tab.path) == path || tab.attachment != nil && m.sourcePath(tab.attachment.Source) == path {
			line, ok := viewFragmentLine(tab, u.Fragment)
			if !ok {
				m.locationError = "Heading not found: " + u.Fragment
				return nil, true
			}
			origin := m.currentViewVisit()
			m.activateTab(i)
			m.browsing.back = appendViewVisit(m.browsing.back, origin)
			m.browsing.forward = nil
			m.browsing.request++
			m.refreshViewDocument(false)
			if u.Fragment != "" {
				m.tab().cursorLine, m.tab().cursorSide = line, ""
				m.tab().cursorOnAnnotation, m.tab().cursorOnForm = false, false
				m.scrollToCursor()
			}
			return m.restartViewWatch(), true
		}
	}
	if !regularFile(path) {
		m.locationError = "File does not exist on disk."
		return nil, true
	}
	return m.loadViewLink(path, u.Fragment, false), true
}

func (m *AppModel) loadViewLink(path, fragment string, reload bool) tea.Cmd {
	m.browsing.request++
	request := m.browsing.request
	return func() tea.Msg {
		msg := viewLinkLoadedMsg{request: request, path: path, fragment: fragment, reload: reload}
		data, err := os.ReadFile(path)
		msg.err = err
		if err == nil {
			msg.unsupported = !imagePath(path) && (!utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || bytes.HasPrefix(data, []byte("%PDF-")))
			if !msg.unsupported {
				msg.doc = document.FromContent(path, data)
			}
		}
		return msg
	}
}

func (m *AppModel) finishViewLink(msg viewLinkLoadedMsg) tea.Cmd {
	if !m.readOnly || msg.request != m.browsing.request {
		return nil
	}
	if msg.reload {
		m.reload.busy = false
	}
	if msg.err != nil {
		m.locationError = "Cannot open file: " + msg.err.Error()
		return nil
	}
	if msg.unsupported {
		m.pendingLocation = sourceLocation{path: msg.path}
		m.modal, m.modalFocus = openExternalModal, 1
		return nil
	}
	tab := FileTab{path: msg.path, doc: msg.doc, state: &fileReview{}, cursorLine: 1, viewDocument: true,
		isBinary: imagePath(msg.path) && !strings.HasSuffix(strings.ToLower(msg.path), ".svg")}
	if !tab.isBinary {
		tab.ensureHighlightCache()
	}
	line, ok := viewFragmentLine(&tab, msg.fragment)
	if !ok {
		m.locationError = "Heading not found: " + msg.fragment
		return nil
	}
	origin := m.currentViewVisit()
	if !msg.reload {
		m.browsing.back = appendViewVisit(m.browsing.back, origin)
		m.browsing.forward = nil
	} else {
		tab.cursorLine = min(origin.tab.cursorLine, max(1, len(tab.doc.Lines)))
		tab.contentOffset, tab.commentOffset = origin.tab.contentOffset, origin.tab.commentOffset
	}
	if msg.fragment != "" {
		tab.cursorLine = line
	}
	old := *m.tab()
	m.tabs[m.activeTab] = tab
	m.refreshViewDocument(!msg.reload)
	if msg.fragment != "" {
		m.scrollToCursor()
	}
	return tea.Batch(m.releaseRetiredMarkdownImages([]FileTab{old}), m.restartViewWatch())
}

func (m *AppModel) stepViewHistory(forward bool) tea.Cmd {
	from, to := &m.browsing.back, &m.browsing.forward
	if forward {
		from, to = to, from
	}
	if !m.readOnly || len(*from) == 0 {
		return nil
	}
	visit := (*from)[len(*from)-1]
	(*from)[len(*from)-1] = viewVisit{}
	*from = (*from)[:len(*from)-1]
	*to = appendViewVisit(*to, m.currentViewVisit())
	m.browsing.request++
	index := m.activeTab
	for i := range m.tabs {
		if m.tabs[i].path == visit.tab.path {
			index = i
			break
		}
	}
	old := m.tabs[index]
	m.activateTab(index)
	m.tabs[index] = visit.tab
	m.refreshViewDocument(false)
	return tea.Batch(m.releaseRetiredMarkdownImages([]FileTab{old}), m.restartViewWatch())
}

func (m *AppModel) refreshViewDocument(top bool) {
	m.locationError = ""
	m.clearTextSelection()
	m.tab().selecting = false
	m.focused = contentPane
	m.paths = newPathDisplay()
	m.recalculateLayout()
	m.rebuildContent()
	m.updateCommentSidebar()
	step := 0
	if top {
		step = 1
	}
	m.positionTabViewport(step)
}

func (m *AppModel) restartViewWatch() tea.Cmd {
	m.browsing.watchPath = m.tab().path
	m.reload.request++
	m.reload.busy, m.reload.err = false, nil
	m.reload.pending, m.reload.dirty = false, false
	m.reload.epoch++
	if m.reload.watcher != nil {
		m.reload.watcher.Close()
		m.reload.watcher = nil
	}
	if m.reload.watch {
		return m.startWatch()
	}
	return nil
}

func (m *AppModel) syncViewWatch() tea.Cmd {
	if m.readOnly && len(m.tabs) > 0 && m.browsing.watchPath != "" && m.tab().path != m.browsing.watchPath {
		return m.restartViewWatch()
	}
	return nil
}

func (m AppModel) viewHistoryButton(forward bool) string {
	if !m.readOnly {
		return ""
	}
	label, key, count := "Back", "M-←", len(m.browsing.back)
	kind := icons.Back
	if forward {
		label, key, count = "Forward", "M-→", len(m.browsing.forward)
		kind = icons.Forward
	}
	if count == 0 {
		return ""
	}
	return renderBarButton(m.icons.Label(kind, label), key)
}

func viewFragmentLine(tab *FileTab, fragment string) (int, bool) {
	if fragment == "" {
		return 0, true
	}
	tab.ensureHighlightCache()
	if tab.isMarkdown && tab.doc != nil {
		tab.ensureMarkdownCache()
		anchors := tab.markdown.after.headingAnchors
		if line, ok := anchors[fragment]; ok {
			return line, true
		}
		if line, ok := anchors[strings.TrimPrefix(fragment, "user-content-")]; ok {
			return line, true
		}
	}
	line, err := strconv.Atoi(strings.TrimPrefix(fragment, "L"))
	if strings.HasPrefix(fragment, "L") && err == nil && tab.doc != nil && tab.doc.HasLine(line) {
		return line, true
	}
	return 0, false
}
