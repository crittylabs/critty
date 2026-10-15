package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestCommentEditorPreviewsImagesAfterTypingPause(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("a.png", previewPNG(t, 160, 80), 0o644); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "source\n")
	app.images.enabled = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.openLineComment()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !app.editorImages.active || len(app.editorImages.refs) != 0 {
		t.Fatalf("editor state=%+v", app.editorImages)
	}
	typeText(&app, "![p](a.png)")
	if len(app.editorImages.refs) != 0 || len(app.commentImages) != 0 {
		t.Fatal("references must wait for the typing pause")
	}
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("edits should schedule the delayed refresh")
	}
	typeText(&app, "below")
	app, _ = updateApp(app, editorImagesMsg{seq: app.editorImages.seq - 1})
	if len(app.editorImages.refs) != 0 {
		t.Fatal("a stale delay must not apply")
	}
	app, _ = updateApp(app, editorImagesMsg{seq: app.editorImages.seq})
	want, _ := filepath.Abs("a.png")
	if len(app.editorImages.refs) != 1 || app.editorImages.refs[0].line != 1 || app.editorImages.refs[0].path != want {
		t.Fatalf("refs=%+v", app.editorImages.refs)
	}
	key := commentImageKey{want, app.modalTextarea.Width()}
	side := app.commentImages[key]
	if side == nil {
		t.Fatal("editor image not registered for loading")
	}
	if !side.attempted {
		t.Fatal("editor image not loaded while the dialog is open")
	}
	img := &inlineImage{id: 0x123456, width: 160, height: 80}
	img.columns, img.rows = img.size(min(44, key.width), 11)
	img.ready = true
	side.image = img

	view := app.View().Content
	imageCells := strings.Count(view, string(kitty.Placeholder))
	if imageCells != img.columns*img.rows {
		t.Fatalf("editor placeholders=%d want=%d", imageCells, img.columns*img.rows)
	}
	cursor := app.editorCursor()
	if cursor == nil || cursor.Y != 1+img.rows {
		t.Fatalf("cursor=%+v want row %d", cursor, 1+img.rows)
	}
	textRows := app.editorTextRow(1 + img.rows)
	if textRows != 1 || app.editorTextRow(1) != -1 {
		t.Fatalf("text row mapping: %d %d", textRows, app.editorTextRow(1))
	}

	app.discardTextModal()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'j'})
	if app.editorImages.active {
		t.Fatal("closing the editor should clear its image state")
	}
}

func TestTextareaLineEndsMatchTextarea(t *testing.T) {
	ta := textarea.New()
	ta.SetWidth(20)
	ta.SetHeight(30)
	ta.SetValue("short\n\n" +
		"a line that is long enough to soft wrap twice over\n" +
		"supercalifragilisticexpialidocious-word\n" +
		"日本語の長い行はどこで折り返されるでしょうか\n" +
		"exactly twenty chars\n" +
		"trailing spaces     \nlast")
	ta.MoveToBegin()
	var want []int
	row := -1
	for line := 0; line < ta.LineCount(); line++ {
		row += ta.LineInfo().Height
		want = append(want, row)
		ta.CursorEnd()
		ta.CursorDown()
	}
	if got := textareaLineEnds(ta); len(got) != len(want) || strings.Join(strings.Fields(fmtInts(got)), " ") != strings.Join(strings.Fields(fmtInts(want)), " ") {
		t.Fatalf("line ends: got %v want %v", got, want)
	}
}

func fmtInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, " ")
}
