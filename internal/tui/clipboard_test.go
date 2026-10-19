package tui

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	t.Run("text command", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("command fixture uses the macOS clipboard backend")
		}
		dir := t.TempDir()
		for _, name := range []string{"pbpaste", "osascript"} {
			script := "#!/bin/sh\ncase \"$*\" in *JSON.stringify*) printf '{\"text\":\"text \"}\n' ;; *AppKit*) exit 0 ;; *) printf 'text ' ;; esac\n"
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("PATH", dir)
		var cmd tea.Cmd
		m, cmd = updateApp(m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("C-v returned no clipboard command")
		}
		editorMessage(&m, cmd())
		if m.modalTextarea.Value() != "text original" {
			t.Fatalf("text fallback = %q", m.modalTextarea.Value())
		}
	})
}
