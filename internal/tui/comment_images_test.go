package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/crittylabs/critty/internal/review"
)

func TestCommentImagesPathsAndReplies(t *testing.T) {
	cwd, outside, sessionDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Chdir(cwd)
	path := filepath.Join(outside, "画像.png")
	if err := os.WriteFile(path, previewPNG(t, 8, 4), 0o644); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "source\n")
	app.images.enabled = true
	app.session = &review.Session{Dir: sessionDir}
	app.session.Meta.CWD = filepath.Join(t.TempDir(), "removed-worktree")
	app.session.Meta.Workdir = cwd
	attachment, err := app.session.SaveAttachment(previewPNG(t, 4, 8))
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ destination, path string }{{path, path}, {filepath.ToSlash(relative), path}, {attachment, filepath.Join(sessionDir, attachment)}} {
		got, ok := app.commentImagePath(tc.destination)
		if !ok || got != tc.path {
			t.Fatalf("%s resolved to %s", tc.destination, got)
		}
	}
	body := "![outside](" + filepath.ToSlash(relative) + ")\nafter image"
	replies := []review.Reply{{ID: "rp_1", Author: "Codex", Body: "![pasted](" + attachment + ")"}}
	for _, width := range []int{60, 24} {
		app.layoutThread("Reviewer", body, replies, "", width)
	}
	if len(app.commentImages) != 4 {
		t.Fatalf("image slots=%d want=4", len(app.commentImages))
	}
	for key, side := range app.commentImages {
		msg := app.loadCommentImage(key)().(imageLoadedMsg)
		if msg.err != nil || msg.comment == nil || !strings.Contains(msg.sequence, "f=100") {
			t.Fatalf("load=%+v", msg)
		}
		model, _ := app.Update(imageTransmittedMsg{msg})
		app = model.(AppModel)
		if side.image == nil {
			t.Fatal("image transmission did not populate cache")
		}
		if side.image.columns == 0 {
			app.prepareCommentImages()
		}
		model, _ = app.Update(imageReadyMsg{comment: &key, columns: side.image.columns, rows: side.image.rows})
		app = model.(AppModel)
	}
	for _, width := range []int{60, 24} {
		layout := app.layoutThread("Reviewer", body, replies, "", width)
		content := strings.Join(layout.lines, "\n")
		if strings.Count(content, string(kitty.Placeholder)) == 0 {
			t.Fatal("comment/reply preview missing")
		}
		first := strings.Index(content, string(kitty.Placeholder))
		if first > strings.Index(content, "after image") {
			t.Fatal("image appeared below following line")
		}
		for _, line := range layout.lines {
			if lipgloss.Width(line) > width {
				t.Fatalf("comment image overflow=%d want<=%d", lipgloss.Width(line), width)
			}
		}
	}
	app.images.hidden = true
	if got := app.renderCommentContent("Reviewer", body, "", 60); strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatal("off retained comment images")
	}
	app.images.hidden = false
	app.modal = commentModal
	if got := app.renderCommentContent("Reviewer", body, "", 60); strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatal("display preview entered editor")
	}
}

func TestCommentImagesFailuresAndUnsupportedTerminal(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.renderCommentContent("Reviewer", "![missing](missing.png)", "", 30)
	if app.prepareCommentImages() == nil || !app.images.checking {
		t.Fatal("comment image did not request capability detection")
	}
	for _, side := range app.commentImages {
		if side.attempted {
			t.Fatal("unsupported terminal loaded images")
		}
	}
	app.images.enabled = true
	app.renderCommentContent("Reviewer", "![missing](missing.png)", "", 30)
	for key := range app.commentImages {
		msg := app.loadCommentImage(key)().(imageLoadedMsg)
		if msg.err == nil {
			t.Fatal("missing image succeeded")
		}
		model, _ := app.finishImageLoad(msg)
		app = model.(AppModel)
	}
	if got := app.renderCommentContent("Reviewer", "![missing](missing.png)", "", 30); !strings.Contains(got, "Image preview unavailable") {
		t.Fatal("missing image failure hidden")
	}
}

func TestCommentImageColorsAndModal(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	app.images.enabled = true
	app.width, app.height = 120, 40
	app.recalculateLayout()
	app.tab().state.Comments = []review.Comment{{ID: "c_123456", StartLine: 1, Body: "![pic](a.png)"}}
	app.rebuildContent()
	app.updateCommentSidebar()
	for key, side := range app.commentImages {
		img := &inlineImage{id: 0x123456, width: 8, height: 4, ready: true}
		img.columns, img.rows = img.size(min(44, key.width), 11, cellSize{})
		side.image = img
	}
	app.rebuildContent()
	app.updateCommentSidebar()
	count := 0
	for _, line := range uv.NewStyledString(app.View().Content).Lines(ansi.GraphemeWidth) {
		for _, cell := range line {
			if !strings.ContainsRune(cell.Content, kitty.Placeholder) {
				continue
			}
			count++
			if cell.Style.Fg == nil {
				t.Fatal("placeholder lost color")
			}
			r, g, b, _ := cell.Style.Fg.RGBA()
			if r>>8 != 0x12 || g>>8 != 0x34 || b>>8 != 0x56 {
				t.Fatal("comment styling corrupted the image ID")
			}
		}
	}
	if count == 0 {
		t.Fatal("comment image missing from actual screen")
	}
	app.modal = helpModal
	if strings.ContainsRune(app.View().Content, kitty.Placeholder) {
		t.Fatal("comment image visible beneath modal")
	}
}
