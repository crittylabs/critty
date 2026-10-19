package tui

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const killRingLimit = 60

type killRing struct {
	entries []string
	killing bool
	yank    *yankState
}

type yankState struct {
	base        string
	row, column int
	index       int
}

// current returns the entry a yank would insert: the one selected by an
// ongoing yank-pop sequence, otherwise the newest.
func (r *killRing) current() string {
	if len(r.entries) == 0 {
		return ""
	}
	if r.yank != nil {
		return r.entries[r.yank.index]
	}
	return r.entries[0]
}

func (r *killRing) interrupt() {
	r.killing = false
	r.yank = nil
}

func (r *killRing) add(text string, backward bool) {
	r.yank = nil
	if text == "" {
		return
	}
	if r.killing && len(r.entries) > 0 {
		if backward {
			r.entries[0] = text + r.entries[0]
		} else {
			r.entries[0] += text
		}
	} else {
		r.entries = append([]string{text}, r.entries...)
		if len(r.entries) > killRingLimit {
			r.entries = r.entries[:killRingLimit]
		}
	}
	r.killing = true
}

func killDirection(msg tea.KeyPressMsg, bindings textarea.KeyMap) (kill, backward bool) {
	if key.Matches(msg, bindings.DeleteBeforeCursor, bindings.DeleteWordBackward) {
		return true, true
	}
	return key.Matches(msg, bindings.DeleteAfterCursor, bindings.DeleteWordForward), false
}

// updateKillRing delegates deletion boundaries and input limits to textarea.
func (m *AppModel) updateKillRing(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	r := &m.killRing
	if kill, backward := killDirection(msg, m.modalTextarea.KeyMap); kill {
		before := []rune(m.modalTextarea.Value())
		var cmd tea.Cmd
		cmd = m.updateModalTextarea(msg)
		after := m.modalTextarea.Value()
		removed := len(before) - utf8.RuneCountInString(after)
		if removed > 0 {
			lines := strings.Split(after, "\n")
			start := m.modalTextarea.Column()
			for _, line := range lines[:m.modalTextarea.Line()] {
				start += utf8.RuneCountInString(line) + 1
			}
			r.add(string(before[start:start+removed]), backward)
		} else {
			r.yank = nil
		}
		return true, cmd
	}
	if msg.String() != "ctrl+y" && msg.String() != "alt+y" {
		r.interrupt()
		return false, nil
	}
	r.killing = false
	if len(r.entries) == 0 || (msg.String() == "alt+y" && r.yank == nil) {
		return true, nil
	}
	if msg.String() == "ctrl+y" {
		m.modalTextarea.DeleteSelection()
		r.yank = &yankState{
			base: m.modalTextarea.Value(), row: m.modalTextarea.Line(), column: m.modalTextarea.Column(),
		}
	} else {
		r.yank.index = (r.yank.index + 1) % len(r.entries)
		m.modalTextarea.SetValue(r.yank.base)
		m.modalTextarea.MoveToBegin()
		for m.modalTextarea.Line() < r.yank.row {
			m.modalTextarea.CursorEnd()
			m.modalTextarea.CursorDown()
		}
		m.modalTextarea.SetCursorColumn(r.yank.column)
	}
	var cmd tea.Cmd
	cmd = m.updateModalTextarea(tea.PasteMsg{Content: r.entries[r.yank.index]})
	return true, cmd
}
