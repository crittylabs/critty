package tui

import (
	"slices"
	"strings"
)

// editSnapshot is the text of an editor and its cursor before a change
// that removed text or pasted some in.  One level is kept; undoing swaps
// it with the current text, so undoing again restores the change.
type editSnapshot struct {
	value     string
	line, col int
	modal     modalType
}

// undoKeys are the keys that restore the last snapshot in any editor.
// `C-/` reaches most terminals as `ctrl+_`.
func isUndoKey(key string) bool {
	return key == "ctrl+z" || key == "ctrl+/" || key == "ctrl+_"
}

// destructiveEdit reports whether an edit removed or replaced text, as
// opposed to inserting at the cursor, by checking that the text before and
// after the old cursor survived unchanged.
func destructiveEdit(before, after string, cursor int) bool {
	if before == after {
		return false
	}
	b, a := []rune(before), []rune(after)
	cursor = min(max(cursor, 0), len(b))
	if len(a) < len(b) {
		return true
	}
	return !slices.Equal(a[:cursor], b[:cursor]) || !slices.Equal(a[len(a)-(len(b)-cursor):], b[cursor:])
}

// runeOffset converts a line and column to a rune index in text.
func runeOffset(text string, line, col int) int {
	offset := 0
	for i, l := range strings.Split(text, "\n") {
		if i == line {
			return offset + col
		}
		offset += len([]rune(l)) + 1
	}
	return offset
}

func (m *AppModel) textareaSnapshot() editSnapshot {
	return editSnapshot{value: m.modalTextarea.Value(), line: m.modalTextarea.Line(), col: m.modalTextarea.Column(), modal: m.modal}
}

// recordTextareaEdit keeps the snapshot taken before an edit when the edit
// removed or replaced text, or when it pasted text, which is as easy to
// trigger by mistake as a deletion.
func (m *AppModel) recordTextareaEdit(before editSnapshot, pasted bool) {
	after := m.modalTextarea.Value()
	if pasted && after != before.value || destructiveEdit(before.value, after, runeOffset(before.value, before.line, before.col)) {
		m.editorUndo = &before
	}
}

// replaceTextareaValue sets the editor text as an undoable change.
func (m *AppModel) replaceTextareaValue(text string) {
	before := m.textareaSnapshot()
	m.modalTextarea.SetValue(text)
	m.recordTextareaEdit(before, true)
}

// setTextareaValueAt sets the editor text and places the cursor.
func (m *AppModel) setTextareaValueAt(value string, line, col int) {
	m.modalTextarea.SetValue(value)
	m.modalTextarea.MoveToBegin()
	for m.modalTextarea.Line() < line {
		m.modalTextarea.CursorEnd()
		m.modalTextarea.CursorDown()
	}
	m.modalTextarea.SetCursorColumn(col)
}

// undoTextarea restores the text before the last deletion, overwrite, or
// paste in the comment editor and keeps the current text so the undo can
// be undone.
func (m *AppModel) undoTextarea() bool {
	snap := m.editorUndo
	if snap == nil || snap.modal != m.modal {
		return false
	}
	current := m.textareaSnapshot()
	m.setTextareaValueAt(snap.value, snap.line, snap.col)
	m.editorUndo = &current
	m.killRing.interrupt()
	if m.pathInputType() != "" {
		m.resizePathInput()
	}
	return true
}
