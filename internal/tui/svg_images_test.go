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
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/crittylabs/critty/v2/internal/document"
	gitpkg "github.com/crittylabs/critty/v2/internal/git"
	"github.com/crittylabs/critty/v2/internal/review"
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

func TestSVGPatchPreview(t *testing.T) {
	app := patchApp(t, "diff --git a/image.svg b/image.svg\nnew file mode 100644\n--- /dev/null\n+++ b/image.svg\n@@ -0,0 +1 @@\n+<svg width=\"80\" height=\"40\"/>\n")
	app.images.enabled = true
	app.svg = svgStub(func(_ context.Context, data []byte) ([]byte, error) {
		if string(data) != "<svg width=\"80\" height=\"40\"/>\n" {
			t.Fatalf("wrong snapshot: %q", data)
		}
		return previewPNG(t, 80, 40), nil
	})
	if app.prepareImage() == nil || !app.tab().preview.after.attempted {
		t.Fatal("SVG patch did not load its inline preview")
	}
	if msg := app.loadImage(false)().(imageLoadedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
}

func TestSVGPreviewSplitAlignment(t *testing.T) {
	app := patchApp(t, "diff --git a/image.svg b/image.svg\n--- a/image.svg\n+++ b/image.svg\n@@ -1 +1 @@\n-<svg/>\n+<svg width=\"80\"/>\n")
	app.images.enabled = true
	app.split = true
	app.tab().preview = imagePreview{
		before: previewSide{image: &inlineImage{id: 1, width: 80, height: 40, columns: 10, rows: 3, ready: true}},
		after:  previewSide{image: &inlineImage{id: 2, width: 80, height: 40, columns: 10, rows: 3, ready: true}},
	}
	for _, width := range []int{60, 81, 100} {
		app.contentViewport.SetWidth(width)
		app.rebuildContent()
		lines := strings.Split(ansi.Strip(app.contentViewport.GetContent()), "\n")
		var split int
		for _, row := range app.contentLayout.columns {
			if row.split > 0 {
				split = row.split
				break
			}
		}
		if split == 0 {
			t.Fatal("missing split diff")
		}
		for _, line := range lines[:4] {
			left, _, ok := strings.Cut(line, " │ ")
			if !ok || lipgloss.Width(left) != split {
				t.Fatalf("width %d: preview separator is not at %d: %q", width, split, line)
			}
		}
	}
	for _, status := range []string{"added", "deleted"} {
		app.tab().status = status
		app.tab().isDeleted = status == "deleted"
		lines := strings.Split(ansi.Strip(app.renderImagePreview()), "\n")
		left, right, ok := strings.Cut(lines[1], " │ ")
		if !ok || strings.ContainsRune(left, kitty.Placeholder) != app.tab().isDeleted || strings.ContainsRune(right, kitty.Placeholder) == app.tab().isDeleted {
			t.Fatalf("%s preview is on the wrong side: %q", status, lines[1])
		}
	}
}

func TestSVGPatchSnapshots(t *testing.T) {
	before := "<svg>\n<rect width=\"20\"/>\n</svg>\n"
	after := "<svg>\n<rect width=\"40\"/>\n</svg>\n"
	diff := &gitpkg.DiffInfo{
		ChangedLines: map[int]bool{2: true},
		DeletedAfter: map[int][]gitpkg.DeletedLine{1: {{OldLineNum: 2, Content: `<rect width="20"/>`}}},
	}
	for _, tc := range []struct {
		name, path, oldPath, content, want string
		before, partial                    bool
		diff                               *gitpkg.DiffInfo
	}{
		{name: "after", path: "image.svg", content: after, want: after, diff: diff},
		{name: "before", path: "image.svg", content: after, want: before, before: true, diff: diff},
		{name: "renamed", path: "renamed.svg", oldPath: "image.svg", content: after, want: before, before: true, diff: diff},
		{name: "deleted", path: "image.svg", before: true, want: before, diff: &gitpkg.DiffInfo{DeletedAfter: map[int][]gitpkg.DeletedLine{0: {{Content: "<svg>"}, {Content: `<rect width="20"/>`}, {Content: "</svg>"}}}}},
		{name: "partial", path: "image.svg", content: after, partial: true, diff: diff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := gitpkg.PatchFile{Change: gitpkg.FileChange{Path: tc.path, OldPath: tc.oldPath}, Content: tc.content, Diff: tc.diff}
			if tc.partial {
				file.Known = map[int]bool{2: true}
			}
			app := AppModel{patch: &gitpkg.Patch{Files: []gitpkg.PatchFile{file}}}
			if tc.oldPath != "" {
				// A rename can reuse its old path for a different added file.
				app.patch.Files = append(app.patch.Files, gitpkg.PatchFile{Change: gitpkg.FileChange{Path: tc.oldPath}, Content: "<svg/>"})
			}
			path := tc.path
			if tc.before && tc.oldPath != "" {
				path = tc.oldPath
			}
			got, err := app.imageContent(path, tc.before)
			if tc.partial {
				if err == nil {
					t.Fatal("partial snapshot accepted as a complete SVG")
				}
				return
			}
			if err != nil || string(got) != tc.want {
				t.Fatalf("snapshot = %q, %v; want %q", got, err, tc.want)
			}
		})
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
