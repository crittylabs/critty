package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func TestMarkdownLinkRefs(t *testing.T) {
	source := "日本語 [site](https://example.org/a_(b)) and https://example.org/plain.\n" +
		"[reference][id] <https://example.org/auto>\n\n" +
		"[id]: https://example.org/ref\n\n" +
		"`https://example.org/code`\n\n```\nhttps://example.org/fenced\n```\n\n" +
		"<a href=\"https://example.org/html?a=1&amp;b=2\">HTML</a>\n"
	refs := markdownLinkRefs(source)
	want := []string{"https://example.org/a_(b)", "https://example.org/plain", "https://example.org/ref", "https://example.org/auto", "https://example.org/html?a=1&b=2"}
	if len(refs) != len(want) {
		t.Fatalf("refs=%+v", refs)
	}
	for i, ref := range refs {
		if ref.destination != want[i] || ref.end <= ref.start {
			t.Fatalf("reference %d: %+v", i, ref)
		}
	}
}

func TestEditorOpenNearestImageOrLink(t *testing.T) {
	opened := stubOpener(t, false)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("a.png", previewPNG(t, 160, 80), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("notes.txt", []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "source\n")
	app.session.Meta.CWD = dir
	app.images.enabled = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.openLineComment()
	value := "日本語 é ![pic](a.png)    [site](https://example.org/page)\n[notes](notes.txt)"
	app.modalTextarea.SetValue(value)
	app.modalTextarea.MoveToBegin()
	column := utf8.RuneCountInString("日本語 é ![pic]")
	app.modalTextarea.SetCursorColumn(column)
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.modal != lightboxModal || cmd == nil || len(*opened) != 0 {
		t.Fatalf("unloaded image should open the lightbox: modal=%v opened=%v", app.modal, *opened)
	}
	loaded := app.loadLightboxImage(app.lightbox.target)().(lightboxLoadedMsg)
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	app, cmd = updateApp(app, loaded)
	if app.lightbox.image.id == 0 || len(rawMessages(cmd)) < 2 {
		t.Fatal("image must be transmitted before placement")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.modal != commentModal || !app.modalTextarea.Focused() || app.modalTextarea.Value() != value || app.modalTextarea.Column() != column {
		t.Fatal("lightbox did not preserve the draft and cursor")
	}
	app.modalTextarea.SetCursorColumn(utf8.RuneCountInString(strings.Split(value, "\n")[0]))
	app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if status := openStatus(cmd); status != "Opened https://example.org/page" || app.modal != commentModal {
		t.Fatalf("nearest URL: %q modal=%v", status, app.modal)
	}
	app.modalTextarea.MoveToEnd()
	_, cmd = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if status := openStatus(cmd); status != "Opened notes.txt" || !strings.HasSuffix((*opened)[1], " "+filepath.Join(dir, "notes.txt")) {
		t.Fatalf("local link: %q opened=%v", status, *opened)
	}
	before := app.modalTextarea.Value()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'o', Text: "o"})
	if app.modalTextarea.Value() != before+"o" {
		t.Fatal("plain o must remain text input")
	}
}

func TestOpenLinksRespectsRemoteAndSchemes(t *testing.T) {
	opened := stubOpener(t, true)
	app := setupAppWithDoc(t, "[site](https://example.org)\n")
	app = pressKey(app, 'j')
	_, cmd := updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if status := openStatus(cmd); !strings.HasPrefix(status, "Not opened:") || len(*opened) != 0 {
		t.Fatalf("remote open: %q %v", status, *opened)
	}
	for _, destination := range []string{"javascript:alert(1)", "file:///etc/passwd", "//example.org/a", "#anchor"} {
		if arg, ok := app.linkArgument(destination, false); ok {
			t.Fatalf("accepted %q as %q", destination, arg)
		}
	}
}
