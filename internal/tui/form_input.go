package tui

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/form"
)

func (m *AppModel) pathInputType() form.InputType {
	if m.modal == formTextModal {
		if _, item := m.editingFormItem(); item != nil {
			return item.Type
		}
	}
	return ""
}

// Path values have one logical line; long names can wrap within the editor.
func (m *AppModel) resizePathInput() {
	m.modalTextarea.SetHeight(min(6, max(1, m.modalTextarea.LineInfo().Height)))
}

func (m *AppModel) updateModalTextarea(msg tea.Msg) tea.Cmd {
	if m.pathInputType() != "" {
		if press, ok := msg.(tea.KeyPressMsg); ok && key.Matches(press, m.modalTextarea.KeyMap.InsertNewline) {
			m.modalSubmit()
			return nil
		}
		if paste, ok := msg.(tea.PasteMsg); ok {
			text, valid := singleLinePath(paste.Content)
			if !valid {
				m.clipboardStatus = "Path must be a single line"
				return nil
			}
			msg = tea.PasteMsg{Content: text}
		}
	}
	var cmd tea.Cmd
	m.modalTextarea, cmd = m.modalTextarea.Update(msg)
	if m.pathInputType() != "" {
		m.resizePathInput()
	}
	return cmd
}

// Clipboard and external editors commonly append a final line ending.
func singleLinePath(text string) (string, bool) {
	text = strings.TrimRight(text, "\r\n")
	return text, !strings.ContainsAny(text, "\r\n")
}

type formPathInfo struct {
	missing       bool
	parentMissing bool
	directory     bool
	err           error
}

type formPathState struct {
	value, cwd string
	checked    bool
	info       formPathInfo
}

func (m *AppModel) formPathLookup(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return m.sourcePath(path), nil
}

func (m *AppModel) inspectFormPath(path string) formPathInfo {
	lookup, err := m.formPathLookup(path)
	if err != nil {
		return formPathInfo{err: err}
	}
	info, err := os.Stat(lookup)
	if err == nil {
		return formPathInfo{directory: info.IsDir()}
	}
	if !os.IsNotExist(err) {
		return formPathInfo{err: err}
	}
	_, parentErr := os.Stat(filepath.Dir(lookup))
	return formPathInfo{missing: true, parentMissing: os.IsNotExist(parentErr)}
}

func (m *AppModel) refreshPathInfo() {
	if m.pathInputType() == "" {
		m.pathInfo = formPathState{}
		return
	}
	value := m.modalTextarea.Value()
	cwd := m.session.Meta.CWD
	if m.pathInfo.checked && m.pathInfo.value == value && m.pathInfo.cwd == cwd {
		return
	}
	m.pathInfo = formPathState{value: value, cwd: cwd, checked: true}
	if value != "" && !strings.ContainsAny(value, "\r\n") {
		m.pathInfo.info = m.inspectFormPath(value)
	}
}

func missingPathNotice(kind form.InputType, info formPathInfo) string {
	if info.parentMissing {
		return "Parent directory does not exist"
	}
	switch kind {
	case form.InputFile:
		return "File does not exist"
	case form.InputDirectory:
		return "Directory does not exist"
	default:
		return "Path does not exist"
	}
}

func (m AppModel) pathNotice() (string, color.Color) {
	if m.pathInputType() == "" {
		return "", nil
	}
	if m.clipboardStatus != "" {
		return m.clipboardStatus, lipgloss.Red
	}
	_, item := m.editingFormItem()
	if item == nil || item.Type == "" || !m.pathInfo.info.missing {
		return "", nil
	}
	tint := textColors.cyan
	if m.pathInfo.info.parentMissing {
		tint = warning
	}
	if item.Existing {
		tint = lipgloss.Red
	}
	return missingPathNotice(item.Type, m.pathInfo.info), tint
}

// New paths are valid unless the field explicitly requires an existing one.
func (m *AppModel) validateFormPath(item form.Item, path string) error {
	if strings.ContainsAny(path, "\r\n") {
		return fmt.Errorf("Path must be a single line")
	}
	if path == "" {
		return nil
	}
	if item.Type == form.InputFile && strings.HasSuffix(path, "/") {
		return fmt.Errorf("File path must not end with /")
	}
	info := m.inspectFormPath(path)
	if info.err != nil {
		return fmt.Errorf("Cannot check path: %w", info.err)
	}
	if info.missing {
		if item.Existing {
			return errors.New(missingPathNotice(item.Type, info))
		}
		return nil
	}
	if item.Type == form.InputFile && info.directory {
		return fmt.Errorf("File path points to a directory")
	}
	if item.Type == form.InputDirectory && !info.directory {
		return fmt.Errorf("Directory path points to a file")
	}
	return nil
}
