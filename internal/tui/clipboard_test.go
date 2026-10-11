package tui

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/internal/clipboard"
	"github.com/crittylabs/critty/internal/review"
)

func TestClipboardImageInTextModals(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, modal := range []modalType{commentModal, fileCommentModal, replyModal, editModal} {
		m := newKillRingApp("after")
		m.modal = modal
		m.session = &review.Session{Dir: t.TempDir()}
		editorControl(&m, 'v')
		if !m.clipboardPending {
			t.Fatal("C-v did not start image paste")
		}
		editorMessage(&m, clipboardImageMsg{id: m.clipboardID, data: data.Bytes()})
		body := m.modalTextarea.Value()
		if !strings.HasPrefix(body, "![clipboard image](attachments/") || !strings.HasSuffix(body, ")after") {
			t.Fatalf("body = %q", body)
		}
		path := strings.TrimSuffix(strings.TrimPrefix(body, "![clipboard image]("), ")after")
		if _, err := os.Stat(filepath.Join(m.session.Dir, path)); err != nil {
			t.Fatal(err)
		}
		if m.clipboardPending || m.err != nil {
			t.Fatalf("paste state: %+v", m.err)
		}
	}
}

func TestClipboardCancelAndFailures(t *testing.T) {
	m := newKillRingApp("original")
	m.session = &review.Session{Dir: t.TempDir()}
	editorControl(&m, 'v')
	id := m.clipboardID
	editorMessage(&m, tea.KeyPressMsg{Code: tea.KeyEscape})
	editorMessage(&m, clipboardImageMsg{id: id, data: []byte("late result")})
	if m.modalTextarea.Value() != "original" || m.session.HasAttachments() {
		t.Fatal("cancelled paste changed draft")
	}
	editorControl(&m, 'v')
	editorMessage(&m, clipboardImageMsg{id: m.clipboardID, err: errors.New("clipboard failed")})
	if m.err != nil || m.clipboardStatus != "clipboard failed" || m.modalTextarea.Value() != "original" {
		t.Fatal("clipboard error was fatal or lost draft")
	}
	editorControl(&m, 'v')
	editorMessage(&m, clipboardImageMsg{id: m.clipboardID, err: clipboard.ErrNoImage, text: tea.PasteMsg{Content: "text "}})
	if m.modalTextarea.Value() != "text original" {
		t.Fatalf("text fallback = %q", m.modalTextarea.Value())
	}
}
