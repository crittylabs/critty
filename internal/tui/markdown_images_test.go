package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestMarkdownImageRefs(t *testing.T) {
	source := "![one](a.png) ![two](b%20c.png)\n" +
		"`![code](no.png)` \\![escaped](no.png)\n" +
		"```html\n<img src='no.png'>\n```\n" +
		"![ref][picture]\n\n[picture]: <猫.png>\n\n" +
		"<img SRC=\"a&amp;b.png\"> <img src='two.png'/>\n\n" +
		"<img\n src=\"multi.png\">\n\n<!-- <img src='no.png'> -->\n" +
		"<script>var x = '<img src=\"no.png\">';</script>\n"
	want := []markdownImageRef{{1, "a.png"}, {1, "b%20c.png"}, {6, "猫.png"}, {10, "a&b.png"}, {10, "two.png"}, {12, "multi.png"}}
	if got := markdownImageRefs(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("refs = %#v, want %#v", got, want)
	}
	for _, tc := range []struct{ destination, want string }{
		{"../assets/a%20b.png", filepath.Join("assets", "a b.png")},
		{"猫.png?x=1#part", filepath.Join("docs", "猫.png")},
		{"https://example.org/a.png", ""}, {"//example.org/a.png", ""}, {"data:image/png;base64,AAA", ""}, {"#part", ""}, {"file:///a.png", ""},
	} {
		got, ok := localImagePath("docs/readme.md", tc.destination)
		if (tc.want != "") != ok || ok && got != tc.want {
			t.Errorf("%q: %q, %t", tc.destination, got, ok)
		}
	}
}

func TestMarkdownImagesLayoutAndComments(t *testing.T) {
	app := setupAppWithDoc(t, "![one](a.png) ![two](b.png) ![three](c.png)\nnext line\n")
	app.images.enabled = true
	app.tab().state.Comments = []review.Comment{{ID: "c_123456", StartLine: 1, Body: "below images"}}
	app.tab().markdownImagesPending = true
	for i := range 3 {
		app.tab().markdownImages = append(app.tab().markdownImages, markdownImage{line: 1, previewSide: previewSide{image: &inlineImage{id: i + 1, width: 10, height: 10, columns: 18, rows: 9, ready: true}}})
	}
	for _, width := range []int{100, 45} {
		app.contentViewport.SetWidth(width)
		preview := app.renderMarkdownImages(1, false)
		if lipgloss.Width(preview) > width {
			t.Fatalf("overflow: %d > %d", lipgloss.Width(preview), width)
		}
		wantHeight := 9
		if width == 45 {
			wantHeight = 27
		}
		if lipgloss.Height(preview) != wantHeight {
			t.Fatalf("height=%d want=%d", lipgloss.Height(preview), wantHeight)
		}
		app.rebuildContent()
		content := app.contentViewport.GetContent()
		imageEnd := strings.LastIndex(content, string(kitty.Placeholder))
		comment := strings.Index(ansi.Strip(content), "below images")
		plainImageEnd := strings.LastIndex(ansi.Strip(content), string(kitty.Placeholder))
		if imageEnd < 0 || comment < plainImageEnd || strings.Index(ansi.Strip(content), "next line") < comment {
			t.Fatal("images, comments, next line are out of order")
		}
		for i, row := range app.contentLayout.rows {
			if strings.ContainsRune(strings.Split(content, "\n")[i], kitty.Placeholder) && row.line != 1 {
				t.Fatalf("image row anchored to %+v", row)
			}
		}
	}
	app.images.hidden = true
	app.rebuildContent()
	if strings.ContainsRune(app.contentViewport.GetContent(), kitty.Placeholder) {
		t.Fatal("off retained images")
	}
}

func TestMarkdownImagesUseDocumentLocationAndScope(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("docs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("docs/image.png", previewPNG(t, 3, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("docs/old.md", []byte("![old](image.png)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "add", "docs/image.png", "docs/old.md")
	runGitIn(t, dir, "commit", "-qm", "base")
	if err := os.WriteFile("docs/image.png", previewPNG(t, 6, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "![image](image.png)\n")
	app.tab().path = "docs/readme.md"
	app.multiFile = true
	app.source = &gitpkg.ReviewSource{Scope: "staged"}
	refs := app.findMarkdownImages()().(markdownImagesMsg)
	if len(refs.images) != 1 || refs.images[0].path != "docs/image.png" {
		t.Fatalf("refs=%+v", refs)
	}
	app.tab().markdownImages = refs.images
	msg := app.loadMarkdownImage(0)().(imageLoadedMsg)
	if msg.err != nil || msg.width != 3 {
		t.Fatalf("wrong scope: %+v", msg)
	}
	app.tab().oldPath = "docs/old.md"
	app.tab().deletedAfter = map[int][]gitpkg.DeletedLine{0: {{OldLineNum: 1, Content: "![old](image.png)"}}}
	refs = app.findMarkdownImages()().(markdownImagesMsg)
	app.tab().markdownImages = refs.images
	if len(refs.images) != 2 || !refs.images[1].before {
		t.Fatalf("missing renamed old document reference: %+v", refs)
	}
	msg = app.loadMarkdownImage(1)().(imageLoadedMsg)
	if msg.err != nil || msg.width != 3 {
		t.Fatalf("wrong old snapshot: %+v", msg)
	}
	app.tab().attachment = &review.AttachedDocument{Source: filepath.Join(dir, "docs", "notice.md")}
	refs = app.findMarkdownImages()().(markdownImagesMsg)
	app.tab().markdownImages = refs.images
	msg = app.loadMarkdownImage(0)().(imageLoadedMsg)
	if msg.err != nil || msg.width != 6 {
		t.Fatalf("attachment location: %+v", msg)
	}
}

func TestImagesTogglePreservesTextInput(t *testing.T) {
	for _, modal := range []modalType{commentModal, fileCommentModal, replyModal, editModal} {
		app := setupAppWithDoc(t, "source\n")
		app.modal = modal
		app.modalTextarea.Focus()
		updated, _ := app.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
		app = *updated.(*AppModel)
		if app.images.hidden || app.modalTextarea.Value() != "I" {
			t.Fatal("I intercepted text input")
		}
	}
	app := setupAppWithDoc(t, "source\n")
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'i', Text: "i"})
	if app.images.hidden {
		t.Fatal("lowercase i toggled images")
	}
	updated, _ := app.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	app = *updated.(*AppModel)
	if !app.images.hidden {
		t.Fatal("I did not hide images")
	}
	app.width, app.height = 100, 40
	app.recalculateLayout()
	_, layout := app.renderReviewScreen()
	if !layout.hasImagesButton {
		t.Fatal("missing clickable image toggle")
	}
	app, _ = updateApp(app, tea.MouseClickMsg{X: layout.imagesButton.left, Y: layout.imagesButton.top, Button: tea.MouseLeft})
	if app.images.hidden {
		t.Fatal("click did not restore images")
	}
}

func TestMarkdownImagesRejectRepositorySymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	t.Chdir(root)
	if err := os.Symlink(outside, "images"); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "![image](images/a.png)\n")
	app.multiFile = true
	app.tab().path = "README.md"
	app.source = &gitpkg.ReviewSource{Scope: "all"}
	if refs := app.findMarkdownImages()().(markdownImagesMsg); len(refs.images) != 0 {
		t.Fatal("followed a repository directory symlink outside the repository")
	}
	app.source.Scope = "staged"
	if refs := app.findMarkdownImages()().(markdownImagesMsg); len(refs.images) != 1 {
		t.Fatal("filesystem symlink affected index references")
	}
	app.multiFile = false
	if refs := app.findMarkdownImages()().(markdownImagesMsg); len(refs.images) != 1 {
		t.Fatal("standalone documents lost ordinary local references")
	}
}
