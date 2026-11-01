package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLineEditorMouseSelectionAndEdits(t *testing.T) {
	e := newLineEditor("hello world", 0)
	e.Focus()
	e.click(0, 40)
	e.drag(5, 40)
	e.release()
	if e.selectedText() != "hello" || e.Position() != 5 {
		t.Fatalf("selection = %q at %d", e.selectedText(), e.Position())
	}
	view, cursorX := e.view(40, "")
	if ansi.Strip(view) != "hello world" || view == "hello world" || cursorX != 5 {
		t.Fatalf("view = %q cursor %d; want the selection highlighted", view, cursorX)
	}
	e.edit(tea.KeyPressMsg{Code: 'x', Text: "x"}, &killRing{})
	if e.Value() != "x world" || e.hasSelection() {
		t.Fatalf("typing over the selection gave %q (selected %v)", e.Value(), e.hasSelection())
	}
	e.click(2, 40)
	e.drag(7, 40)
	e.release()
	e.edit(tea.KeyPressMsg{Code: tea.KeyBackspace}, &killRing{})
	if e.Value() != "x " {
		t.Fatalf("backspace over the selection gave %q", e.Value())
	}
	e.click(1, 40)
	e.release()
	if e.hasSelection() || e.Position() != 1 {
		t.Fatalf("a plain click left selection %v at %d", e.hasSelection(), e.Position())
	}
}

func TestLineEditorWideCharactersAndScrolling(t *testing.T) {
	e := newLineEditor("日本語", 0)
	e.Focus()
	e.click(3, 20)
	if e.Position() != 1 || e.cursor(20).X != 2 {
		t.Fatalf("click on the second column of 本 gave position %d cursor %d", e.Position(), e.cursor(20).X)
	}
	e.SetValue(strings.Repeat("abcdefghij", 3))
	start, cursorX := e.window(10)
	view, _ := e.view(10, "")
	if start != 21 || cursorX != 9 || ansi.Strip(view) != "vwxyz"[0:0]+"bcdefghij" {
		t.Fatalf("window start %d cursor %d view %q", start, cursorX, ansi.Strip(view))
	}
	if e.indexAt(0, 10) != 21 {
		t.Fatalf("click at the scrolled start mapped to %d", e.indexAt(0, 10))
	}
	empty := newLineEditor("", 0)
	if view, _ := empty.view(10, "file name"); ansi.Strip(view) != "file name" {
		t.Fatalf("placeholder view = %q", view)
	}
}

func TestGotoLineDialogSelectsAndCopiesWithMouse(t *testing.T) {
	app := setupAppWithDoc(t, "one\ntwo\nthree")
	app.width, app.height = 80, 24
	app.recalculateLayout()
	app.openGotoLine()
	app.lineInput.SetValue("12345")
	inputRect := func() mouseRect {
		for _, region := range app.modalMouseRegions() {
			if region.action.lineInput {
				return region.rect
			}
		}
		t.Fatal("input mouse region missing")
		return mouseRect{}
	}
	rect := inputRect()
	left := rect.left + len(gotoLinePrompt)
	app = pressMouse(app, left+1, rect.top)
	app = moveMouse(app, left+4, rect.top)
	app = releaseMouse(app, left+4, rect.top)
	if app.lineInput.selectedText() != "234" {
		t.Fatalf("drag selected %q", app.lineInput.selectedText())
	}
	if footer := ansi.Strip(app.renderFooter()); !strings.Contains(footer, "Copy text") {
		t.Fatalf("footer = %q", footer)
	}
	_, layout := app.renderReviewScreen()
	if !layout.hasCopyTextButton {
		t.Fatal("copy button missing")
	}
	app, cmd := clickMouseCmd(app, layout.copyTextButton.left, layout.copyTextButton.top)
	if cmd == nil || app.killRing.current() != "234" || app.lineInput.hasSelection() || app.modal != gotoLineModal {
		t.Fatalf("copy button: cmd %v ring %q selected %v modal %v", cmd != nil, app.killRing.current(), app.lineInput.hasSelection(), app.modal)
	}

	rect = inputRect() // the copy notice grows the dialog
	app = pressMouse(app, left, rect.top)
	app = moveMouse(app, left+2, rect.top)
	app = releaseMouse(app, left+2, rect.top)
	app = pressKey(app, tea.KeyEscape)
	if app.modal != gotoLineModal || app.lineInput.hasSelection() {
		t.Fatalf("esc with a selection: modal %v selected %v", app.modal, app.lineInput.hasSelection())
	}
	app = pressMouse(app, left, rect.top)
	app = moveMouse(app, left+2, rect.top)
	app = releaseMouse(app, left+2, rect.top)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
	if app.killRing.current() != "12" {
		t.Fatalf("M-w copied %q", app.killRing.current())
	}
	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatal("esc without a selection did not close the dialog")
	}
}

func TestISearchEditorSelectsAndCopiesWithMouse(t *testing.T) {
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "alpha")
	app = altKey(app, 'e')
	_, layout := app.renderReviewScreen()
	var row int
	for _, region := range layout.isearchRegions {
		if region.action == isearchInput {
			row = region.rect.top
		}
	}
	left, _ := app.isearchEditColumns()
	app = pressMouse(app, left+1, row)
	app = moveMouse(app, left+4, row)
	app = releaseMouse(app, left+4, row)
	if app.isearchInput.selectedText() != "lph" {
		t.Fatalf("drag selected %q", app.isearchInput.selectedText())
	}
	if controls, _ := app.isearchControls(0); !strings.Contains(ansi.Strip(controls), "Copy") {
		t.Fatalf("controls = %q", ansi.Strip(controls))
	}
	_, layout = app.renderReviewScreen()
	for _, region := range layout.isearchRegions {
		if region.action == isearchCopy {
			app, _ = clickMouseCmd(app, region.rect.left, region.rect.top)
		}
	}
	if app.killRing.current() != "lph" || app.isearchInput.hasSelection() || !app.isearch.editing {
		t.Fatalf("copy: ring %q selected %v editing %v", app.killRing.current(), app.isearchInput.hasSelection(), app.isearch.editing)
	}
	app = ctrlKey(app, 'a')
	app = ctrlKey(app, 'y')
	if app.isearchInput.Value() != "lphalpha" {
		t.Fatalf("C-y gave %q", app.isearchInput.Value())
	}
}

func TestFileSelectInputUsesKillRing(t *testing.T) {
	app := setupAppWithDoc(t, "one")
	app.width, app.height = 80, 24
	app.recalculateLayout()
	app.openFileSelect()
	app = typeSearch(app, "abc")
	app = ctrlKey(app, 'a')
	app = ctrlKey(app, 'k')
	if app.fileSelect.input.Value() != "" || app.killRing.current() != "abc" {
		t.Fatalf("C-k left %q with ring %q", app.fileSelect.input.Value(), app.killRing.current())
	}
	app = ctrlKey(app, 'y')
	if app.fileSelect.input.Value() != "abc" || app.fileSelect.query != "abc" {
		t.Fatalf("C-y gave %q (query %q)", app.fileSelect.input.Value(), app.fileSelect.query)
	}
}

func TestLineEditorPastesClipboardTextWithCtrlV(t *testing.T) {
	app := setupAppWithDoc(t, "one")
	app.width, app.height = 80, 24
	app.recalculateLayout()
	app.openFileSelect()
	app = typeSearch(app, "ab")
	app = ctrlKey(app, 'a')
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd == nil || app.fileSelect.input.Value() != "ab" {
		t.Fatalf("C-v: cmd %v value %q", cmd != nil, app.fileSelect.input.Value())
	}
	app, _ = updateApp(app, lineEditorPasteMsg{id: app.clipboardID, text: "x\ty\n"})
	if app.fileSelect.input.Value() != "x y ab" || app.fileSelect.query != "x y ab" {
		t.Fatalf("paste gave %q (query %q)", app.fileSelect.input.Value(), app.fileSelect.query)
	}
	app, _ = updateApp(app, lineEditorPasteMsg{id: app.clipboardID - 1, text: "stale"})
	if app.fileSelect.input.Value() != "x y ab" {
		t.Fatalf("a stale clipboard result changed the value to %q", app.fileSelect.input.Value())
	}
}

func TestCommentEditorSelectsAndCopiesWithMouse(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\nthird\n")
	app.modal = commentModal
	app.modalTextarea.SetValue("hello world")
	app.modalTextarea.Focus()
	app.width, app.height = 100, 30
	app.recalculateLayout()
	// Status lines move the editor, so locate it before every gesture.
	drag := func(from, to int, release bool) {
		rect, ok := app.textareaRegion()
		if !ok {
			t.Fatal("textarea region missing")
		}
		left := rect.left + lipgloss.Width(app.modalTextarea.Prompt)
		app = pressMouse(app, left+from, rect.top)
		app = moveMouse(app, left+to, rect.top)
		if release {
			app = releaseMouse(app, left+to, rect.top)
		}
	}
	before, _ := app.textareaRegion()
	drag(6, 11, false)
	if !app.editorDragging || app.modalTextarea.SelectedText() != "world" {
		t.Fatalf("drag: dragging %v selected %q", app.editorDragging, app.modalTextarea.SelectedText())
	}
	rect, _ := app.textareaRegion()
	if rect != before {
		t.Fatalf("the editor moved from %+v to %+v when the selection appeared", before, rect)
	}
	app = releaseMouse(app, rect.left+lipgloss.Width(app.modalTextarea.Prompt)+11, rect.top)
	if app.editorDragging || app.modalTextarea.SelectedText() != "world" {
		t.Fatalf("release: dragging %v selected %q", app.editorDragging, app.modalTextarea.SelectedText())
	}
	if footer := ansi.Strip(app.renderFooter()); !strings.Contains(footer, "Copy text") {
		t.Fatalf("footer = %q", footer)
	}
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
	if cmd == nil || app.killRing.current() != "world" || app.modalTextarea.HasSelection() || app.modal != commentModal {
		t.Fatalf("M-w: cmd %v ring %q selected %v modal %v", cmd != nil, app.killRing.current(), app.modalTextarea.HasSelection(), app.modal)
	}

	drag(0, 5, true)
	app = pressKey(app, tea.KeyEscape)
	if app.modal != commentModal || app.modalTextarea.HasSelection() {
		t.Fatalf("esc with a selection: modal %v selected %v", app.modal, app.modalTextarea.HasSelection())
	}
	drag(0, 5, true)
	app = typeSearch(app, "bye")
	if app.modalTextarea.Value() != "bye world" {
		t.Fatalf("typing over the selection gave %q", app.modalTextarea.Value())
	}
	drag(1, 1, true)
	if app.modalTextarea.HasSelection() || app.modalTextarea.Column() != 1 {
		t.Fatalf("plain click: selected %v column %d", app.modalTextarea.HasSelection(), app.modalTextarea.Column())
	}
}
