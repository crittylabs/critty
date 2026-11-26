package tui

import "slices"

// editSnapshot stores the text and cursor before an editing command.
type editSnapshot struct {
	value     string
	line, col int
}

type editHistory struct {
	undo, redo []editSnapshot
}

func (h *editHistory) record(before editSnapshot, after string) {
	if before.value == after {
		return
	}
	// Bound the number of full text copies retained while an editor is open.
	const limit = 100
	if len(h.undo) == limit {
		h.undo = slices.Delete(h.undo, 0, 1)
	}
	h.undo = append(h.undo, before)
	h.redo = nil
}

func (h *editHistory) step(current editSnapshot, redo bool) (editSnapshot, bool) {
	from, to := &h.undo, &h.redo
	if redo {
		from, to = to, from
	}
	n := len(*from)
	if n == 0 {
		return editSnapshot{}, false
	}
	snap := (*from)[n-1]
	*from = slices.Delete(*from, n-1, n)
	*to = append(*to, current)
	return snap, true
}

func (m *AppModel) resetTextarea() {
	m.editorHistory = editHistory{}
	m.modalTextarea.Reset()
}

func (m *AppModel) textareaSnapshot() editSnapshot {
	return editSnapshot{value: m.modalTextarea.Value(), line: m.modalTextarea.Line(), col: m.modalTextarea.Column()}
}

func (m *AppModel) recordTextareaEdit(before editSnapshot) {
	m.editorHistory.record(before, m.modalTextarea.Value())
}

// replaceTextareaValue sets the editor text as an undoable change.
func (m *AppModel) replaceTextareaValue(text string) {
	before := m.textareaSnapshot()
	m.modalTextarea.SetValue(text)
	m.recordTextareaEdit(before)
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

func (m *AppModel) restoreTextarea(redo bool) {
	if snap, ok := m.editorHistory.step(m.textareaSnapshot(), redo); ok {
		m.setTextareaValueAt(snap.value, snap.line, snap.col)
		if m.pathInputType() != "" {
			m.resizePathInput()
		}
	}
	m.killRing.interrupt()
}
