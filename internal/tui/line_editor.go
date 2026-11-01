package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/clipboard"
)

// lineEditor is the single-line text input shared by the search query
// editor, the go-to-line dialog, and the file selector.  textinput supplies
// the editing keys; the editor adds the kill ring, mouse selection, and a
// renderer that keeps the terminal cursor and clicks aligned with wide
// characters, which textinput's rune-based cursor does not.
type lineEditor struct {
	input     textinput.Model
	anchor    int  // rune index where a mouse selection started
	selecting bool // the text between anchor and the cursor is selected
	dragging  bool // the mouse button is still held
}

func newLineEditor(value string, limit int) lineEditor {
	input := textinput.New()
	input.SetVirtualCursor(false)
	input.Prompt = ""
	input.CharLimit = limit
	input.KeyMap.Paste = key.NewBinding() // `C-v` reads the clipboard through editLineEditor
	input.SetValue(value)
	input.CursorEnd()
	return lineEditor{input: input}
}

func (e *lineEditor) Focus() tea.Cmd { return e.input.Focus() }

func (e *lineEditor) Blur() {
	e.input.Blur()
	e.clearSelection()
}

func (e lineEditor) Focused() bool { return e.input.Focused() }
func (e lineEditor) Value() string { return e.input.Value() }
func (e lineEditor) Position() int { return e.input.Position() }

// SetValue replaces the text and moves the cursor to its end.
func (e *lineEditor) SetValue(value string) {
	e.input.SetValue(value)
	e.input.CursorEnd()
	e.clearSelection()
}

func (e *lineEditor) SetCursor(pos int) {
	e.input.SetCursor(pos)
	e.clearSelection()
}

func (e lineEditor) hasSelection() bool {
	return e.selecting && e.anchor != e.input.Position()
}

// bounds returns the selected rune range in order.
func (e lineEditor) bounds() (int, int) {
	a, b := e.anchor, e.input.Position()
	if a > b {
		a, b = b, a
	}
	return a, b
}

func (e lineEditor) selectedText() string {
	if !e.hasSelection() {
		return ""
	}
	a, b := e.bounds()
	return string([]rune(e.input.Value())[a:b])
}

func (e *lineEditor) clearSelection() {
	e.selecting, e.dragging = false, false
}

// deleteSelection removes the selected text and leaves the cursor there.
func (e *lineEditor) deleteSelection() {
	if !e.hasSelection() {
		return
	}
	a, b := e.bounds()
	value := []rune(e.input.Value())
	e.input.SetValue(string(value[:a]) + string(value[b:]))
	e.input.SetCursor(a)
	e.clearSelection()
}

// insert places text at the cursor, replacing any selection.
func (e *lineEditor) insert(text string) {
	e.deleteSelection()
	e.clearSelection()
	if text == "" {
		return
	}
	value, pos := []rune(e.input.Value()), e.input.Position()
	e.input.SetValue(string(value[:pos]) + text + string(value[pos:]))
	e.input.SetCursor(pos + utf8.RuneCountInString(text))
}

// paste inserts pasted text with line breaks and tabs folded into spaces.
func (e *lineEditor) paste(text string) {
	e.insert(sanitizeLineText(text))
}

// sanitizeLineText folds line breaks and tabs into spaces and drops other
// control characters and styling.
func sanitizeLineText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
}

// edit applies an editing key.  Typing or deleting replaces or removes the
// selection, kills record their text in ring, and `C-y` yanks from it.
func (e *lineEditor) edit(msg tea.KeyPressMsg, ring *killRing) tea.Cmd {
	bindings := e.input.KeyMap
	if msg.String() == "ctrl+y" {
		e.insert(sanitizeLineText(ring.current()))
		return nil
	}
	if e.hasSelection() {
		switch {
		case key.Matches(msg, bindings.DeleteCharacterBackward, bindings.DeleteCharacterForward):
			e.deleteSelection()
			return nil
		case msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0:
			e.deleteSelection()
		}
	}
	e.clearSelection()
	kill, backward := lineEditorKill(msg, bindings)
	before := []rune(e.input.Value())
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	if kill {
		start := e.input.Position()
		if removed := len(before) - utf8.RuneCountInString(e.input.Value()); removed > 0 {
			ring.add(string(before[start:start+removed]), backward)
		} else {
			ring.yank = nil
		}
	}
	return cmd
}

// lineEditorPasteMsg carries clipboard text read for a `C-v` in an input.
type lineEditorPasteMsg struct {
	id   uint64
	text string
	err  error
}

// editLineEditor applies a key to an input.  `C-v` reads the system
// clipboard as text, since these fields take no images.
func (m *AppModel) editLineEditor(editor *lineEditor, msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() != "ctrl+v" {
		return editor.edit(msg, &m.killRing)
	}
	m.clipboardID++
	id := m.clipboardID
	return func() tea.Msg {
		text, err := clipboard.ReadText()
		return lineEditorPasteMsg{id: id, text: text, err: err}
	}
}

// finishLineEditorPaste inserts clipboard text into the input still focused.
func (m *AppModel) finishLineEditorPaste(msg lineEditorPasteMsg) tea.Cmd {
	editor := m.focusedLineEditor()
	if msg.id != m.clipboardID || editor == nil {
		return nil
	}
	if msg.err != nil {
		return m.flashNotice("Clipboard: " + msg.err.Error())
	}
	editor.paste(msg.text)
	if m.modal == fileSelectModal {
		m.refreshFileSelect()
	}
	m.recalculateLayout()
	return nil
}

// lineEditorKill reports whether a key kills text and whether it kills
// backward, so the kill ring can record the removed text.
func lineEditorKill(msg tea.KeyPressMsg, bindings textinput.KeyMap) (kill, backward bool) {
	if key.Matches(msg, bindings.DeleteBeforeCursor, bindings.DeleteWordBackward) {
		return true, true
	}
	return key.Matches(msg, bindings.DeleteAfterCursor, bindings.DeleteWordForward), false
}

// window returns the first visible rune and the cursor column for the
// width, scrolling so the cursor stays visible with one cell reserved
// for it.
func (e lineEditor) window(width int) (start, cursorX int) {
	value := []rune(e.input.Value())
	start = min(e.input.Position(), len(value))
	for start > 0 {
		w := ansi.StringWidth(string(value[start-1]))
		if cursorX+w > width-1 {
			break
		}
		start--
		cursorX += w
	}
	return start, cursorX
}

// view renders the visible text with the selection highlighted and returns
// the cursor column.  An empty value shows the placeholder.
func (e lineEditor) view(width int, placeholder string) (string, int) {
	value := []rune(e.input.Value())
	if len(value) == 0 {
		return lineEditorPlaceholderStyle.Render(ansi.Truncate(placeholder, max(0, width), "")), 0
	}
	start, cursorX := e.window(width)
	text := ansi.Truncate(string(value[start:]), max(0, width), "")
	if e.hasSelection() {
		a, b := e.bounds()
		x := ansi.StringWidth(string(value[start:max(a, start)]))
		y := min(ansi.StringWidth(string(value[start:max(b, start)])), lipgloss.Width(text))
		if x < y {
			text = ansi.Cut(text, 0, x) + inlineBackground(textSelectionBg, ansi.Cut(text, x, y)) + ansi.Cut(text, y, lipgloss.Width(text))
		}
	}
	return text, cursorX
}

// cursor returns the terminal cursor within the rendered text, or nil when
// the editor is not focused.
func (e lineEditor) cursor(width int) *tea.Cursor {
	if !e.input.Focused() {
		return nil
	}
	_, x := e.window(width)
	return tea.NewCursor(x, 0)
}

// indexAt maps a column within the rendered text to a rune index.
func (e lineEditor) indexAt(column, width int) int {
	value := []rune(e.input.Value())
	start, _ := e.window(width)
	x := 0
	for start < len(value) {
		w := ansi.StringWidth(string(value[start]))
		if x+w > column {
			break
		}
		x += w
		start++
	}
	return start
}

// click moves the cursor to a column and starts a mouse selection there.
func (e *lineEditor) click(column, width int) {
	index := e.indexAt(column, width)
	e.input.SetCursor(index)
	e.anchor, e.selecting, e.dragging = index, true, true
}

// drag extends the mouse selection to a column while the button is held.
func (e *lineEditor) drag(column, width int) {
	if !e.dragging {
		return
	}
	e.input.SetCursor(e.indexAt(column, width))
}

// release ends the drag, keeping the selection when it covers any text.
func (e *lineEditor) release() {
	e.dragging = false
	e.selecting = e.hasSelection()
}
