package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/v2/internal/document"
	"github.com/crittylabs/critty/v2/internal/review"
	"github.com/crittylabs/critty/v2/internal/syntax"
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

func TestMarkdownLinkParsesAreIndependent(t *testing.T) {
	for _, tc := range []struct{ source, destination string }{
		{"[one][ref]\n\n[ref]: https://example.org/one", "https://example.org/one"},
		{"[two][ref]\n\n[ref]: https://example.org/two", "https://example.org/two"},
		{"[missing][ref] ![image](image.png)", ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			for range 2 {
				refs := markdownLinkRefs(tc.source)
				if tc.destination == "" {
					if len(refs) != 0 {
						t.Fatalf("unexpected references: %+v", refs)
					}
				} else if len(refs) != 1 || refs[0].destination != tc.destination {
					t.Fatalf("references=%+v, want %s", refs, tc.destination)
				}
			}
		})
	}
}

func TestReferenceDistance(t *testing.T) {
	const source = "αe\u0301\n\n界 end\n"
	for _, tc := range []struct {
		name         string
		line, column int
		ref          markdownImageRef
		want         int
	}{
		{"before", 1, 0, markdownImageRef{line: 3, start: 0, end: 3}, 5},
		{"at start", 3, 0, markdownImageRef{line: 3, start: 0, end: 3}, 0},
		{"at end", 3, 1, markdownImageRef{line: 3, start: 0, end: 3}, 0},
		{"after", 3, 2, markdownImageRef{line: 3, start: 0, end: 3}, 1},
		{"combining", 1, 2, markdownImageRef{line: 1, start: 0, end: 5}, 0},
		{"trailing line", 4, 0, markdownImageRef{line: 3, start: 0, end: 3}, 5},
		{"clipped end", 4, 0, markdownImageRef{line: 3, start: 0, end: 99}, 1},
		{"empty line", 2, 0, markdownImageRef{line: 2}, 0},
		{"cursor before source", 0, 0, markdownImageRef{line: 3, end: 3}, 5},
		{"cursor past source", 9, 2, markdownImageRef{line: 3, end: 3}, 8},
		{"missing line", 1, 0, markdownImageRef{}, int(^uint(0) >> 1)},
		{"past last line", 1, 0, markdownImageRef{line: 5}, int(^uint(0) >> 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			distance := referenceDistance(source, tc.line, tc.column)
			for range 2 {
				if got := distance(tc.ref); got != tc.want {
					t.Fatalf("distance=%d, want %d", got, tc.want)
				}
			}
		})
	}
}

func BenchmarkReferenceDistance(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := strings.Repeat("α [link](https://example.org) ![pic](image.png)\n", count)
			refs := append(markdownLinkRefs(source), markdownImageRefs(source)...)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				distance := referenceDistance(source, 2, 3)
				for _, ref := range refs {
					distance(ref)
				}
			}
		})
	}
}

func TestEditorReferencePriorityAndSiblings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, marked, image, link string
		imageOnly                 bool
		siblings                  int
	}{
		{"wrapped image", "[![|one](a.png)](https://example.org)", "a.png", "", false, 1},
		{"equal images", "![a](a.png) | ![b](b.png)", "a.png", "", false, 2},
		{"second image", "![a](a.png) ![|b](b.png)", "b.png", "", false, 2},
		{"equal links", "[a](https://example.org/a) | [b](https://example.org/b)", "", "https://example.org/a", false, 0},
		{"non-image tie", "![](clip.mp4) | [b](https://example.org/b)", "", "https://example.org/b", false, 0},
		{"images only", "![a](a.png) [|b](https://example.org)", "a.png", "", true, 1},
		{"no references", "plain |text", "", "", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := stubOpener(t, false)
			app := setupAppWithDoc(t, "source\n")
			app.session.Meta.CWD = dir
			app.images.enabled = true
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
			app.openLineComment()
			before, after, _ := strings.Cut(tc.marked, "|")
			app.modalTextarea.SetValue(before + after)
			app.modalTextarea.MoveToBegin()
			app.modalTextarea.SetCursorColumn(utf8.RuneCountInString(before))
			var cmd tea.Cmd
			if tc.imageOnly {
				cmd = app.openImageAtCursor()
			} else {
				cmd = app.openReferenceAtCursor()
			}
			switch {
			case tc.image != "":
				if app.modal != lightboxModal || filepath.Base(app.lightbox.target.path) != tc.image || len(app.lightbox.siblings) != tc.siblings || len(*opened) != 0 {
					t.Fatalf("image selection: modal=%v target=%+v siblings=%v opened=%v", app.modal, app.lightbox.target, app.lightbox.siblings, *opened)
				}
			case tc.link != "":
				if got := openStatus(cmd); got != "Opened "+tc.link {
					t.Fatalf("link selection: %s", got)
				}
			default:
				if cmd != nil || app.modal != commentModal {
					t.Fatal("no reference must leave the editor open")
				}
			}
		})
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

func TestOpenNonImageMarkdownTarget(t *testing.T) {
	for _, markup := range []string{"[Video](./clip%20one.mp4)\n", "![](./clip%20one.mp4)\n"} {
		t.Run(markup, func(t *testing.T) {
			opened := stubOpener(t, false)
			dir := t.TempDir()
			path := filepath.Join(dir, "clip one.mp4")
			if err := os.WriteFile(path, []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), 0o600); err != nil {
				t.Fatal(err)
			}
			app, _ := formTestAppWithContent(t, markup)
			app.tab().attachment.Source = filepath.Join(dir, "media.md")
			app.width, app.height = 100, 35
			app.recalculateLayout()
			app.rebuildContent()
			check := func(cmd tea.Cmd, count int) {
				t.Helper()
				if status := openStatus(cmd); status != "Opened ./clip%20one.mp4" || len(*opened) != count || !strings.HasSuffix((*opened)[count-1], " "+path) {
					t.Fatalf("video link should open externally: status=%q opened=%v", status, *opened)
				}
			}
			check(app.openReferenceAtCursor(), 1)
			cells, _ := linkCells(app.contentViewport.GetContent(), "./clip%20one.mp4")
			if len(cells) == 0 {
				t.Fatal("video target has no clickable cells")
			}
			left, top, _, _ := app.contentBounds()
			point := cells[len(cells)-1]
			app = pressMouse(app, left+point.X, top+point.Y)
			app, cmd := updateApp(app, tea.MouseReleaseMsg{X: left + point.X, Y: top + point.Y, Button: tea.MouseLeft})
			check(cmd, 2)
			app.session.Meta.CWD = dir
			app.openLineComment()
			app.modalTextarea.SetValue(markup)
			app.modalTextarea.MoveToEnd()
			check(app.openReferenceAtCursor(), 3)
			remoteSession = func() bool { return true }
			cmd, _ = app.followLink("./clip%20one.mp4")
			if status := openStatus(cmd); !strings.HasPrefix(status, "Not opened:") || len(*opened) != 3 {
				t.Fatalf("remote media link: %q opened=%v", status, *opened)
			}
		})
	}
}

func TestUnreviewedLinkUsesSyntaxAndContent(t *testing.T) {
	rules, err := syntax.Compile(syntax.Config{Files: []syntax.FileType{
		{Pattern: "*.demo", Language: "go"},
		{Pattern: "plain.go", Language: "text"},
		{Pattern: "plain.md", Language: "text"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		editor        bool
	}{
		{"source.go", "package example\n", true},
		{"Gemfile", "source 'https://example.org'\n", true},
		{"tool", "#!/usr/bin/env python3\n", true},
		{"custom.demo", "package example\n", true},
		{"guide.md", "# Guide\n", true},
		{"guide.markdown", "# Guide\n", true},
		{"plain.md", "# Guide\n", true},
		{"plain.go", "package example\n", true},
		{"notes.txt", "Example notes.\n", true},
		{"LICENSE", "Permission is granted to use this example.\n", true},
		{"Examplefile", "Example configuration.\n", true},
		{"unknown", "Example notes.\n", true},
		{"unicode", "αβγ\n", true},
		{"empty", "", true},
		{"binary", "\x00\x01\x02", false},
		{"binary.txt", "\x00\x01\x02", false},
		{"report.pdf", "%PDF-1.7\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := stubOpener(t, false)
			t.Setenv("EDITOR", "vi")
			app := setupAppWithDoc(t, "Source\n")
			app.syntax = rules
			path := filepath.Join(filepath.Dir(app.tab().path), tc.name)
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd, ok := app.followLink(path)
			if !ok || cmd == nil {
				t.Fatal("local link did not produce an action")
			}
			if tc.editor {
				ready, ok := cmd().(sourceEditorReadyMsg)
				if !ok || ready.err != nil || ready.cmd.Args[len(ready.cmd.Args)-1] != path || len(*opened) != 0 {
					t.Fatalf("source link did not open in the editor: %+v opened=%v", ready, *opened)
				}
			} else if status := openStatus(cmd); status != "Opened "+path || len(*opened) != 1 {
				t.Fatalf("non-source link did not open externally: %q opened=%v", status, *opened)
			}
		})
	}
}

func TestFollowLocalMarkdownLinks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tab        bool
		disk       bool
		attachment bool
		outside    bool
		media      bool
	}{
		{name: "review snapshot", tab: true},
		{name: "existing file", tab: true, disk: true},
		{name: "attachment source", tab: true, attachment: true},
		{name: "retained tab", tab: true, outside: true},
		{name: "media snapshot", tab: true, media: true},
		{name: "editor fallback", disk: true},
		{name: "missing file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := stubOpener(t, false)
			t.Setenv("EDITOR", "vi")
			name, destination := "guide file.md", "guide%20file.md"
			if tc.media {
				name, destination = "clip.mp4", "clip.mp4"
			}
			app := setupAppWithDoc(t, "[Guide](../"+destination+")\n")
			dir := filepath.Dir(app.tab().path)
			app.session.Meta.CWD = dir
			// Resolve relative to the reviewed file, not the process directory.
			app.tab().path = "docs/start.md"
			path := filepath.Join(dir, name)
			if tc.disk {
				if err := os.WriteFile(path, []byte("# Guide\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.tab {
				target := FileTab{path: name, doc: document.FromContent(path, []byte("# Guide\n")), state: &fileReview{}, outsideChanges: tc.outside}
				if tc.media {
					target.isBinary, target.doc = true, nil
				}
				if tc.attachment {
					attachment := review.AttachedDocument{Source: path}
					target.path, target.attachment = "saved.md", &attachment
				}
				app.tabs = append(app.tabs, target)
			}
			app.width, app.height = 100, 30
			app.recalculateLayout()
			app.rebuildContent()
			app.tab().cursorLine = 1
			model, cmd := app.updateMessage(tea.KeyPressMsg{Code: 'o'})
			app = *model.(*AppModel)
			if len(*opened) != 0 {
				t.Fatal("local link must not invoke the system opener")
			}
			switch {
			case tc.tab:
				if cmd != nil || app.activeTab != 1 || app.modal != noModal || app.focused != contentPane {
					t.Fatalf("navigation: tab=%d modal=%v", app.activeTab, app.modal)
				}
			case tc.disk:
				if cmd == nil || app.modal != noModal || app.activeTab != 0 {
					t.Fatalf("editor fallback: modal=%v tab=%d", app.modal, app.activeTab)
				}
				if ready := cmd().(sourceEditorReadyMsg); ready.err != nil || ready.cmd.Args[len(ready.cmd.Args)-1] != path {
					t.Fatalf("editor command: %+v", ready)
				}
			default:
				if app.activeTab != 0 || app.modal != noModal || app.locationError == "" {
					t.Fatal("missing file must report an error without changing tabs")
				}
			}
		})
	}
}

func TestLocalMarkdownLinkClick(t *testing.T) {
	app := setupAppWithDoc(t, "[Guide](guide.md)\n")
	path := filepath.Join(filepath.Dir(app.tab().path), "guide.md")
	app.tabs = append(app.tabs, FileTab{path: path,
		doc: document.FromContent(path, []byte("# Guide\n")), state: &fileReview{}})
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.rebuildContent()
	cells, _ := linkCells(app.contentViewport.GetContent(), "guide.md")
	if len(cells) == 0 {
		t.Fatal("link has no clickable cells")
	}
	left, top, _, _ := app.contentBounds()
	point := cells[len(cells)-1]
	app = pressMouse(app, left+point.X, top+point.Y)
	if app.activeTab != 0 {
		t.Fatal("press must allow dragging to select the label")
	}
	app, _ = updateApp(app, tea.MouseReleaseMsg{X: left + point.X, Y: top + point.Y, Button: tea.MouseLeft})
	if app.activeTab != 1 || app.modal != noModal {
		t.Fatal("click did not follow the local link")
	}
}

func TestExternalLinkMarkClickAndSelection(t *testing.T) {
	opened := stubOpener(t, false)
	const uri = "https://example.org"
	app := setupAppWithDoc(t, "[Site]("+uri+") after\n")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app.rebuildContent()
	cells, _ := linkCells(app.contentViewport.GetContent(), uri)
	if len(cells) != len("Site ")+1 {
		t.Fatalf("link cells=%d, want label, space, and marker", len(cells))
	}
	left, top, _, _ := app.contentBounds()
	mark := cells[len(cells)-1]
	x, y := left+mark.X, top+mark.Y
	app = pressMouse(app, x, y)
	app, cmd := updateApp(app, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if status := openStatus(cmd); status != "Opened "+uri || len(*opened) != 1 {
		t.Fatalf("marker click=%q, opened=%v", status, *opened)
	}
	app = pressMouse(app, x, y)
	app = releaseMouse(app, x+1, y)
	if got := app.selectedText(); got != "" || len(*opened) != 1 {
		t.Fatalf("marker selection copied %q or opened the link", got)
	}
}
