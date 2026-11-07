package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/review"
)

type svgStub func(context.Context, []byte) ([]byte, error)

func (f svgStub) Render(ctx context.Context, source []byte) ([]byte, error) { return f(ctx, source) }

func TestSVGPreviewSurfaces(t *testing.T) {
	source := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40"/></svg>`)
	path := filepath.Join(t.TempDir(), "image.svg")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	app := setupAppWithDoc(t, "![Preview]("+path+")")
	app.images.enabled = true
	app.svg = svgStub(func(_ context.Context, data []byte) ([]byte, error) {
		if !bytes.Equal(data, source) {
			t.Fatalf("source changed: %q", data)
		}
		return previewPNG(t, 80, 40), nil
	})
	refs := app.findMarkdownImages()().(markdownImagesMsg)
	if len(refs.images) != 1 || refs.images[0].path != path {
		t.Fatal(refs)
	}
	app.tab().markdownImages = refs.images
	loaded := app.loadMarkdownImage(0)().(imageLoadedMsg)
	if loaded.err != nil || loaded.width != 80 || loaded.info.Format != "SVG" {
		t.Fatal(loaded.err, loaded.info)
	}
	key := commentImageKey{path: path, width: 60}
	if msg := app.loadCommentImage(key)().(imageLoadedMsg); msg.err != nil || msg.width != 80 {
		t.Fatal(msg.err)
	}
	if msg := app.loadLightboxImage(imageTarget{path: path})().(lightboxLoadedMsg); msg.err != nil || msg.width != 80 {
		t.Fatal(msg.err)
	}
	app.modal = commentModal
	app.applyEditorImages("![Preview](" + path + ")")
	if len(app.editorImages.refs) != 1 || app.editorImages.refs[0].key.path != path {
		t.Fatal(app.editorImages.refs)
	}
}

func TestSVGFilePreviewPreservesSourceAndToggle(t *testing.T) {
	source := "<svg width=\"80\" height=\"40\">\n<rect width=\"80\" height=\"40\"/>\n</svg>"
	path := filepath.Join(t.TempDir(), "image.svg")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(path, AppConfig{})
	app.tab().doc = &document.Document{Path: path, Content: source, Lines: strings.Split(source, "\n")}
	app.tab().state = &fileReview{}
	app.images.enabled = true
	app.width, app.height = 100, 40
	app.recalculateLayout()
	if app.tab().isBinary {
		t.Fatal("SVG lost its text source")
	}
	app.tab().preview.after.image = &inlineImage{id: 1, width: 80, height: 40, columns: 10, rows: 3, ready: true}
	app.rebuildContent()
	if got := ansi.Strip(app.contentViewport.GetContent()); !strings.ContainsRune(got, kitty.Placeholder) || !strings.Contains(got, "<rect") {
		t.Fatalf("preview or source missing: %q", got)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'i', Mod: tea.ModAlt})
	if got := ansi.Strip(app.contentViewport.GetContent()); strings.ContainsRune(got, kitty.Placeholder) || !strings.Contains(got, "<rect") {
		t.Fatalf("toggle changed source: %q", got)
	}
	app.images.hidden = false
	app.tab().preview.after = previewSide{failed: true, attempted: true}
	app.rebuildContent()
	if got := ansi.Strip(app.contentViewport.GetContent()); !strings.Contains(got, "<rect") {
		t.Fatalf("failure lost source: %q", got)
	}
	app.svg = svgStub(func(context.Context, []byte) ([]byte, error) { return nil, fmt.Errorf("unavailable") })
	if msg := app.loadImage(false)().(imageLoadedMsg); msg.err == nil {
		t.Fatal("failed conversion accepted")
	}
}

func TestSVGAttachmentUsesTextSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.svg")
	source := `<svg width="80" height="40"/>`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := review.ReadAttachedDocuments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if docs[0].Content != source || docs[0].Image != "" {
		t.Fatal("SVG snapshot lost original text")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(nil, "", AppConfig{Documents: docs})
	app.svg = svgStub(func(_ context.Context, data []byte) ([]byte, error) {
		if string(data) != source {
			t.Fatal("snapshot changed")
		}
		return previewPNG(t, 80, 40), nil
	})
	if msg := app.loadImage(false)().(imageLoadedMsg); msg.err != nil || msg.width != 80 {
		t.Fatal(msg.err)
	}
}
