package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type sourceLocation struct {
	path string
	line int // zero opens the file without a line argument
}

type sourceEditorReadyMsg struct {
	cmd *exec.Cmd
	err error
}

type sourceEditorFinishedMsg struct{ err error }

func startupLocation(cfg AppConfig) *sourceLocation {
	if cfg.FocusPath == "" {
		return nil
	}
	return &sourceLocation{path: cfg.FocusPath, line: cfg.FocusLine}
}

func (m *AppModel) focusStartupLocation() {
	if m.initialFocus == nil || m.width <= 0 || m.height <= 0 || len(m.tabs) == 0 || m.tab().state == nil {
		return
	}
	location := *m.initialFocus
	m.initialFocus = nil
	m.previousReplyIDs = nil
	for i := range m.tabs {
		t := &m.tabs[i]
		if m.sourcePath(t.path) != m.sourcePath(location.path) {
			continue
		}
		m.activeTab = i
		m.focused = contentPane
		if location.line > 0 && t.doc != nil && t.doc.HasLine(location.line) {
			t.cursorLine, t.cursorSide = location.line, ""
		} else if location.line > 0 {
			m.locationError = fmt.Sprintf("Line %d is not in the review.", location.line)
		}
		m.rebuildContent()
		m.updateCommentSidebar()
		m.scrollToCursor()
		return
	}
	m.locationError = "Focus file is not in the review."
}

func (m *AppModel) sourcePath(path string) string {
	if !filepath.IsAbs(path) && m.session != nil {
		path = filepath.Join(m.session.Meta.WorkingDirectory(), path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (m *AppModel) openSourceEditor(location sourceLocation) tea.Cmd {
	if m.readOnly {
		m.locationError = "View: editing is disabled"
		return nil
	}
	for _, tab := range m.tabs {
		if tab.path == location.path {
			if tab.attachment != nil {
				location.path = tab.attachment.Source
			}
			if tab.textconv.Command != "" {
				location.line = 0
			}
			break
		}
	}
	location.path = m.sourcePath(location.path)
	if !regularFile(location.path) {
		return nil
	}
	return func() tea.Msg {
		cmd, err := sourceEditorCommand(location.path, location.line)
		return sourceEditorReadyMsg{cmd: cmd, err: err}
	}
}

// navigateSource uses the new-side coordinates, matching the file on disk.
// Complete documents include unchanged lines; partial patches may lack them.
func (m *AppModel) navigateSource(location sourceLocation) tea.Cmd {
	path := m.sourcePath(location.path)
	for i := range m.tabs {
		t := &m.tabs[i]
		if m.sourcePath(t.path) != path || t.outsideChanges {
			continue
		}
		if location.line > 0 && (t.doc == nil || !t.doc.HasLine(location.line)) {
			continue
		}
		m.activeTab = i
		m.focused = contentPane
		t.selecting, t.cursorOnAnnotation = false, false
		t.cursorOnForm = false
		t.cursorSide = ""
		t.cursorLine = max(1, location.line)
		if location.line == 0 {
			if lines := m.visualLines(t); len(lines) > 0 {
				t.cursorLine, t.cursorSide = lines[0].line, lines[0].side
			}
		}
		m.rebuildContent()
		m.updateCommentSidebar()
		m.scrollToCursor()
		return nil
	}
	if regularFile(path) {
		m.pendingLocation = location
		m.modal, m.modalFocus = openSourceModal, 1
		m.locationError = ""
	} else {
		m.locationError = "File does not exist on disk."
	}
	return nil
}

func (m *AppModel) openGotoLine() tea.Cmd {
	m.lineInput = textinput.New()
	m.lineInput.SetStyles(textinput.DefaultStyles(terminalIsDark))
	m.lineInput.SetVirtualCursor(false)
	m.lineInput.Prompt = "Line: "
	m.lineInput.Placeholder = strconv.Itoa(max(1, m.tab().cursorLine))
	m.lineInput.CharLimit = 20
	m.lineInput.SetWidth(max(1, min(24, m.width-16)))
	m.modal, m.modalFocus, m.locationError = gotoLineModal, 0, ""
	return m.lineInput.Focus()
}

func (m *AppModel) handleLocationModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" || msg.String() == "ctrl+c" {
		m.modal, m.locationError = noModal, ""
		m.lineInput.Blur()
		return m, nil
	}
	if m.modal == gotoLineModal {
		switch msg.String() {
		case "enter":
			if m.modalFocus == 2 {
				m.modal = noModal
				return m, nil
			}
			return m, m.submitGotoLine()
		case "tab", "shift+tab":
			m.cycleModalFocus([]int{0, 2, 1}, msg.String() == "shift+tab")
			if m.modalFocus == 0 {
				return m, m.lineInput.Focus()
			}
			m.lineInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.lineInput, cmd = m.lineInput.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "y", "Y":
		return m, m.confirmSourceEditor()
	case "n", "N":
		m.modal = noModal
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.modalFocus = 1 - m.modalFocus
	case "enter":
		if m.modalFocus == 0 {
			return m, m.confirmSourceEditor()
		}
		m.modal = noModal
	}
	return m, nil
}

func (m *AppModel) submitGotoLine() tea.Cmd {
	line, err := strconv.Atoi(strings.TrimSpace(m.lineInput.Value()))
	if err != nil || line < 1 {
		m.locationError = "Enter a positive line number."
		m.modalFocus = 0
		return m.lineInput.Focus()
	}
	m.modal, m.locationError = noModal, ""
	m.lineInput.Blur()
	return m.navigateSource(sourceLocation{path: m.tab().path, line: line})
}

func (m *AppModel) handleLocationModalMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if m.modal == gotoLineModal {
			switch region.action.focus {
			case 0:
				m.modalFocus = 0
				cmd := m.lineInput.Focus()
				cursor := m.lineInput.Cursor()
				m.lineInput.SetCursor(m.lineInput.Position() + mouse.X - region.rect.left - cursor.X)
				return m, cmd
			case 1:
				return m, m.submitGotoLine()
			case 2:
				m.modal = noModal
				m.lineInput.Blur()
			}
		} else if region.action.focus == 0 {
			return m, m.confirmSourceEditor()
		} else {
			m.modal = noModal
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) confirmSourceEditor() tea.Cmd {
	m.modal = noModal
	return m.openSourceEditor(m.pendingLocation)
}

func (m AppModel) locationModalContent(width int) (string, []modalMouseRegion) {
	if m.modal == gotoLineModal {
		title, closeRegion := modalTitle("Go to line", width)
		prefix := title + "\n"
		content := prefix + m.lineInput.View() + "\n\n"
		if m.locationError != "" {
			content += lipgloss.Wrap(m.locationError, max(1, width), "") + "\n\n"
		}
		regions := []modalMouseRegion{{
			rect:   mouseRect{top: strings.Count(prefix, "\n"), bottom: strings.Count(prefix, "\n") + 1, right: width},
			action: modalMouseAction{lineInput: true},
		}}
		buttons, buttonRegions := layoutModalActions([]modalButtonSpec{
			{rendered: m.renderPrimaryModalButton("Go", "enter", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
			{rendered: m.renderModalButton("Cancel", "esc", m.modalFocus == 2), action: modalMouseAction{focus: 2}},
		}, width, strings.Count(content, "\n"))
		return content + buttons, append(append(regions, buttonRegions...), closeRegion)
	}
	target := m.pendingLocation.path
	if m.pendingLocation.line > 0 {
		target += fmt.Sprintf(" L%d", m.pendingLocation.line)
	}
	title, closeRegion := modalTitle("Open in editor?", width)
	prefix := lipgloss.Wrap(title+"\n"+
		target+"\nThis location is not in the review.\n\n", max(1, width), "")
	buttons, regions := layoutModalActions([]modalButtonSpec{
		{rendered: m.renderPrimaryModalButton("Open", "y", m.modalFocus == 0), action: modalMouseAction{focus: 0}},
		{rendered: m.renderModalButton("Cancel", "n / esc", m.modalFocus == 1), action: modalMouseAction{focus: 1}},
	}, width, strings.Count(prefix, "\n"))
	return prefix + buttons, append(regions, closeRegion)
}
