package tui

import (
	"errors"
	"fmt"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/clipboard"
)

type clipboardImageMsg struct {
	id   uint64
	data []byte
	err  error
	text tea.Msg
}

func (m *AppModel) pasteClipboardImage() tea.Cmd {
	m.clipboardID++
	id := m.clipboardID
	m.clipboardPending = true
	m.clipboardStatus = "Reading clipboard image… (esc to cancel)"
	return func() tea.Msg {
		data, err := clipboard.ReadImage()
		msg := clipboardImageMsg{id: id, data: data, err: err}
		if errors.Is(err, clipboard.ErrNoImage) {
			msg.text = textarea.Paste()
		}
		return msg
	}
}

func (m *AppModel) finishClipboardImage(msg clipboardImageMsg) tea.Cmd {
	if !m.clipboardPending || msg.id != m.clipboardID {
		return nil
	}
	m.clipboardPending = false
	m.clipboardStatus = ""
	if !m.isTextModal() {
		return nil
	}
	if errors.Is(msg.err, clipboard.ErrNoImage) {
		if _, ok := msg.text.(tea.PasteMsg); !ok {
			m.clipboardStatus = "Clipboard has no supported image or readable text"
			return nil
		}
		var cmd tea.Cmd
		m.modalTextarea, cmd = m.modalTextarea.Update(msg.text)
		return cmd
	}
	if msg.err != nil {
		m.clipboardStatus = msg.err.Error()
		return nil
	}
	if m.session == nil {
		m.clipboardStatus = "No review session for image attachment"
		return nil
	}
	path, err := m.session.SaveAttachment(msg.data)
	if err != nil {
		m.clipboardStatus = fmt.Sprintf("Saving image: %v", err)
		return nil
	}
	m.killRing.interrupt()
	var cmd tea.Cmd
	m.modalTextarea, cmd = m.modalTextarea.Update(tea.PasteMsg{Content: "![clipboard image](" + path + ")"})
	return cmd
}

func (m AppModel) clipboardTextareaView() string {
	view := m.modalTextarea.View()
	if m.clipboardStatus != "" {
		view += "\n" + footerStyle.Render(m.clipboardStatus)
	}
	return view
}
