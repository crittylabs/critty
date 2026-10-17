package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newKillRingApp(value string) AppModel {
	m := NewApp("test.go", AppConfig{})
	m.modal = commentModal
	m.modalTextarea.SetWidth(20)
	m.modalTextarea.SetHeight(6)
	m.modalTextarea.SetValue(value)
	m.modalTextarea.MoveToBegin()
	m.modalTextarea.Focus()
	return m
}

func editorMessage(m *AppModel, msg tea.Msg) {
	*m, _ = updateApp(*m, msg)
}

func editorControl(m *AppModel, code rune) {
	editorMessage(m, tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
}

func editorMeta(m *AppModel, code rune) {
	editorMessage(m, tea.KeyPressMsg{Code: code, Mod: tea.ModAlt})
}

func TestKillRingYankAndRotate(t *testing.T) {
	m := newKillRingApp("日本語🙂\nsecond\nlast")
	editorControl(&m, 'k')
	editorControl(&m, 'k') // Append the newline to the first kill.
	editorControl(&m, 'f') // A cursor command ends the kill sequence.
	editorControl(&m, 'a')
	editorControl(&m, 'k')
	if len(m.killRing.entries) != 2 || m.killRing.entries[1] != "日本語🙂\n" {
		t.Fatalf("ring = %#v", m.killRing.entries)
	}
	editorControl(&m, 'y')
	if got := m.modalTextarea.Value(); got != "second\nlast" {
		t.Fatalf("yank = %q", got)
	}
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "日本語🙂\n\nlast" {
		t.Fatalf("yank-pop = %q", got)
	}
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "second\nlast" {
		t.Fatalf("wrapped yank-pop = %q", got)
	}
}

func TestKillRingBackwardAndWordKills(t *testing.T) {
	for _, test := range []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{"line", []tea.KeyPressMsg{{Code: 'u', Mod: tea.ModCtrl}, {Code: 'u', Mod: tea.ModCtrl}, {Code: 'u', Mod: tea.ModCtrl}}},
		{"word", []tea.KeyPressMsg{{Code: 'w', Mod: tea.ModCtrl}, {Code: 'w', Mod: tea.ModCtrl}, {Code: 'w', Mod: tea.ModCtrl}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := newKillRingApp("first\n日本語🙂")
			m.modalTextarea.MoveToEnd()
			for _, key := range test.keys {
				editorMessage(&m, key)
			}
			if m.modalTextarea.Value() != "" || len(m.killRing.entries) != 1 || m.killRing.entries[0] != "first\n日本語🙂" {
				t.Fatalf("value=%q ring=%#v", m.modalTextarea.Value(), m.killRing.entries)
			}
			editorControl(&m, 'y')
			if m.modalTextarea.Value() != "first\n日本語🙂" {
				t.Fatal("backward kills were not restored in text order")
			}
		})
	}
	m := newKillRingApp("one two")
	editorMeta(&m, 'd')
	editorMeta(&m, 'd')
	editorControl(&m, 'y')
	if m.modalTextarea.Value() != "one two" {
		t.Fatal("forward word kills were not restored")
	}
}

func TestYankPopRequiresConsecutiveYankCommands(t *testing.T) {
	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Code: tea.KeyLeft},
		tea.KeyPressMsg{Code: 'x', Text: "x"},
		tea.KeyPressMsg{Code: tea.KeyBackspace},
		tea.KeyPressMsg{Code: tea.KeyTab},
		tea.PasteMsg{Content: "pasted"},
		tea.MouseMotionMsg{},
	} {
		t.Run(fmt.Sprintf("%T/%v", msg, msg), func(t *testing.T) {
			m := newKillRingApp("prefix ")
			m.modalTextarea.MoveToEnd()
			m.killRing.entries = []string{"new", "old"}
			editorControl(&m, 'y')
			editorMessage(&m, msg)
			before := m.modalTextarea.Value()
			editorMeta(&m, 'y')
			if m.modalTextarea.Value() != before || m.killRing.yank != nil {
				t.Fatal("yank-pop remained active after another action")
			}
		})
	}
}

func TestYankPopSelectionAndInputLimits(t *testing.T) {
	m := newKillRingApp("prefix\n日本語🙂 suffix")
	m.modalTextarea.MoveToEnd()
	m.modalTextarea.SetCursorColumn(4)
	for range 4 {
		editorMessage(&m, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	}
	m.killRing.entries = []string{"a", "長いテキスト\nnext"}
	editorControl(&m, 'y')
	if got := m.modalTextarea.Value(); got != "prefix\na suffix" {
		t.Fatalf("selection replacement = %q", got)
	}
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "prefix\n長いテキスト\nnext suffix" {
		t.Fatalf("multiline rotation = %q", got)
	}
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "prefix\na suffix" {
		t.Fatalf("rotated selection replacement = %q", got)
	}
	m = newKillRingApp("prefix")
	m.modalTextarea.MoveToEnd()
	m.modalTextarea.CharLimit = 8
	m.killRing.entries = []string{"long text", "短"}
	editorControl(&m, 'y')
	if got := m.modalTextarea.Value(); got != "prefixlo" {
		t.Fatalf("limited yank = %q", got)
	}
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "prefix短" {
		t.Fatalf("rotation after truncation = %q", got)
	}
}

func TestKillSelectionAndShareAcrossDialogs(t *testing.T) {
	m := newKillRingApp("selected\ntext")
	editorControl(&m, 'g')
	editorControl(&m, 'k')
	if len(m.killRing.entries) != 1 || m.killRing.entries[0] != "selected\ntext" {
		t.Fatalf("selection kill = %#v", m.killRing.entries)
	}
	editorMessage(&m, tea.KeyPressMsg{Code: tea.KeyEscape})
	for _, modal := range []modalType{fileCommentModal, replyModal, editModal} {
		m.modal, m.modalFocus = modal, 0
		m.modalTextarea.SetValue("")
		m.modalTextarea.Focus()
		editorMeta(&m, 'y')
		if m.modalTextarea.Value() != "" {
			t.Fatal("yank-pop should not carry over to a new dialog")
		}
		editorControl(&m, 'y')
		if m.modalTextarea.Value() != "selected\ntext" {
			t.Fatal("ring contents did not carry over to the next dialog")
		}
		editorMessage(&m, tea.KeyPressMsg{Code: tea.KeyEscape})
	}
}

func TestKillRingLimitAndEmptyKill(t *testing.T) {
	var ring killRing
	for i := range killRingLimit + 5 {
		ring.interrupt()
		ring.add(fmt.Sprint(i), false)
	}
	if len(ring.entries) != killRingLimit || ring.entries[killRingLimit-1] != "5" {
		t.Fatalf("bounded ring = %#v", ring.entries)
	}
	m := newKillRingApp("")
	m.killRing = ring
	editorControl(&m, 'k')
	editorControl(&m, 'y')
	if m.modalTextarea.Value() != "64" {
		t.Fatal("empty kill replaced the latest entry")
	}
}

func TestYankPopAfterWrappedLines(t *testing.T) {
	m := newKillRingApp("長い先頭行を幅の狭い入力欄で折り返します\n前半後半")
	m.modalTextarea.SetWidth(10)
	m.modalTextarea.MoveToEnd()
	m.modalTextarea.SetCursorColumn(2)
	m.killRing.entries = []string{"one", "日本語\nnext"}
	editorControl(&m, 'y')
	editorMeta(&m, 'y')
	if got := m.modalTextarea.Value(); got != "長い先頭行を幅の狭い入力欄で折り返します\n前半日本語\nnext後半" {
		t.Fatalf("wrapped multiline yank-pop = %q", got)
	}
	if m.modalTextarea.Line() != 2 || m.modalTextarea.Column() != 4 {
		t.Fatalf("cursor = %d:%d", m.modalTextarea.Line(), m.modalTextarea.Column())
	}
}

func TestKillRingIgnoresCharacterDeletionAndEmptyYank(t *testing.T) {
	m := newKillRingApp("abc")
	editorControl(&m, 'g')
	editorControl(&m, 'y')
	editorMeta(&m, 'y')
	if m.modalTextarea.Value() != "abc" || !m.modalTextarea.HasSelection() {
		t.Fatal("empty ring changed the text or selection")
	}
	editorMessage(&m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.modalTextarea.Value() != "" || len(m.killRing.entries) != 0 {
		t.Fatal("ordinary deletion should not add a kill")
	}
}

func TestAltWCopiesCurrentKillRingEntryToSystemClipboard(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app = pressKey(app, tea.KeyEnter)
	if app.modal != commentModal {
		t.Fatalf("modal = %v, want comment modal", app.modal)
	}
	app, _ = updateApp(app, tea.PasteMsg{Content: "older"})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	app, _ = updateApp(app, tea.PasteMsg{Content: "newer"})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})

	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
	if cmd == nil || fmt.Sprint(cmd()) != "newer" {
		t.Fatalf("alt+w did not set the clipboard to the newest kill; cmd = %v", cmd)
	}
	if !strings.Contains(app.clipboardStatus, "newer") {
		t.Fatalf("status = %q", app.clipboardStatus)
	}

	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
	if cmd == nil || fmt.Sprint(cmd()) != "older" {
		t.Fatalf("alt+w after yank-pop did not copy the rotated entry; cmd = %v", cmd)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	if got := app.modalTextarea.Value(); got != "newer" {
		t.Fatalf("alt+w broke the yank sequence; value = %q", got)
	}
}
