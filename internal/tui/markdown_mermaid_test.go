package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/crittylabs/critty/internal/mermaidrender"
)

func enableMermaid(t *testing.T, app *AppModel) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	app.mermaid = mermaidrender.New(exe, "off", nil)
}

func TestMermaidFenceSourceCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name, source, body string
		first, last        int
	}{
		{"fence", "Before\n```mermaid\nflowchart LR\n A-->B\n```\nAfter", "flowchart LR\n A-->B\n", 2, 5},
		{"quote", "> ~~~mermaid\n> flowchart LR\n>  A-->B\n> ~~~", "flowchart LR\n A-->B\n", 1, 4},
		{"list", "- ```mermaid\n  flowchart LR\n   A-->B\n  ```", "flowchart LR\n A-->B\n", 1, 4},
		{"CRLF", "```mermaid\r\nflowchart LR\r\n A-->B\r\n```\r\n", "flowchart LR\r\n A-->B\r\n", 1, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := markdownMermaidRefs(tc.source)
			if len(refs) != 1 {
				t.Fatalf("refs=%+v", refs)
			}
			ref := refs[0]
			if ref.renderKind != renderedMermaid || ref.renderFirst != tc.first || ref.line != tc.last || ref.renderBody != tc.body {
				t.Fatalf("ref=%+v", ref)
			}
			lines := strings.Split(tc.source, "\n")
			span := strings.Join(lines[ref.renderFirst-1:ref.line], "\n")
			span = span[ref.renderColumn : len(span)-(len(lines[ref.line-1])-ref.renderStop)]
			if ref.renderSource != span {
				t.Fatalf("source=%q want=%q", ref.renderSource, span)
			}
		})
	}
	for _, source := range []string{"`mermaid`", "```text\nflowchart LR\n```", "```mermaid\nflowchart LR", "```mermaid\n```", "> ```mermaid\n> flowchart LR\n```", "- ```mermaid\n  flowchart LR\n```"} {
		if refs := markdownMermaidRefs(source); len(refs) != 0 {
			t.Fatalf("unexpected refs for %q: %+v", source, refs)
		}
	}
}

func TestMermaidDisplayCopyAndToggle(t *testing.T) {
	source := "Before\n```mermaid\nflowchart LR\n A-->B\n```\nAfter"
	app := setupAppWithDoc(t, source)
	enableMermaid(t, &app)
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.images.enabled = true
	refs := app.renderedRefs(source)
	if len(refs) != 1 {
		t.Fatal(refs)
	}
	refs[0].previewSide = previewSide{attempted: true, image: &inlineImage{id: 1, width: 120, height: 64, columns: 15, rows: 4, ready: true}}
	app.tab().markdownImages, app.tab().markdownImagesPending = refs, true
	app.rebuildContent()
	got := ansi.Strip(app.contentViewport.GetContent())
	if strings.Contains(got, "flowchart") || strings.Contains(got, "```") || !strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("preview=%q", got)
	}
	for line := 2; line <= 5; line++ {
		if _, ok := app.contentLayout.lineRanges[line]; !ok {
			t.Fatalf("line %d lost navigation", line)
		}
	}
	spans := app.contentLayout.textSpans
	first, last := -1, -1
	for i, span := range spans {
		if span.ref.line == 5 {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		t.Fatal("image spans missing")
	}
	app.textSelection = contentTextSelection{active: true, tab: app.activeTab, spans: spans, anchor: textPoint{first, 0}, cursor: textPoint{last, len(spans[last].text)}}
	if got := app.selectedText(); got != "```mermaid\nflowchart LR\n A-->B\n```" {
		t.Fatalf("copy=%q", got)
	}
	app.textSelection = contentTextSelection{}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'I', Text: "I"})
	if got := ansi.Strip(app.contentViewport.GetContent()); !app.images.hidden || !strings.Contains(got, "flowchart") || strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("hidden=%t source=%q", app.images.hidden, got)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'I', Text: "I"})
	app.tab().markdownImages[0].image = &inlineImage{id: 2, width: 120, height: 64, columns: 15, rows: 4, ready: true}
	app.rebuildContent()
	if got := ansi.Strip(app.contentViewport.GetContent()); app.images.hidden || !strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("restored=%q", got)
	}
}

func TestMermaidCommentsEditorAndMathLabels(t *testing.T) {
	body := "```mermaid\nflowchart LR\n A[\"$$x^2$$\"]-->B\n```"
	app := setupAppWithDoc(t, "document")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 50})
	enableMermaid(t, &app)
	if refs := app.renderedRefs(body); len(refs) != 1 || refs[0].renderKind != renderedMermaid {
		t.Fatalf("diagram math parsed separately: %+v", refs)
	}
	if refs := markdownMathRefs(body); len(refs) != 0 {
		t.Fatalf("math inside diagram: %+v", refs)
	}
	app.images.enabled = true
	app.commentRenderedDisplay(body, 60, true)
	if len(app.commentImages) != 1 {
		t.Fatal(app.commentImages)
	}
	for key, side := range app.commentImages {
		if key.renderKind != renderedMermaid {
			t.Fatal(key)
		}
		side.image = &inlineImage{id: 1, width: 120, height: 64, columns: 15, rows: 4, ready: true}
	}
	if got := app.renderCommentContent("Author", body, "", 60); strings.Contains(got, "flowchart") || !strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("comment=%q", got)
	}
	app.modal = commentModal
	app.modalTextarea.SetValue(body)
	app.modalTextarea.Focus()
	app.editorImages.active = true
	app.applyEditorImages(body)
	for key, side := range app.commentImages {
		if key.editor {
			side.image = &inlineImage{id: 2, width: 120, height: 64, columns: 15, rows: 4, ready: true}
		}
	}
	if view, _ := app.editorView(); !strings.Contains(view, "flowchart") || !strings.ContainsRune(view, kitty.Placeholder) {
		t.Fatalf("editor=%q", view)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'i', Mod: tea.ModAlt})
	if app.modalTextarea.Value() != body || !app.images.hidden || !app.modalTextarea.Focused() {
		t.Fatal("editor toggle changed input or focus")
	}
	if view, _ := app.editorView(); strings.ContainsRune(view, kitty.Placeholder) {
		t.Fatalf("hidden editor=%q", view)
	}
	app.width, app.height = 100, 40
	app.recalculateLayout()
	var button *modalMouseRegion
	for _, region := range app.modalMouseRegions() {
		if region.action.toggleImages {
			copy := region
			button = &copy
			break
		}
	}
	if button == nil {
		t.Fatal("missing editor preview button")
	}
	app, _ = updateApp(app, tea.MouseClickMsg{X: button.rect.left, Y: button.rect.top, Button: tea.MouseLeft})
	if app.images.hidden || app.modalTextarea.Value() != body {
		t.Fatal("click did not restore preview")
	}
}

func TestNativeRenderersInDocumentAndGitReview(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := AppConfig{MathRaTeX: "/missing/ratex", MermaidMerman: exe, MermaidMmdr: "off"}
	for _, app := range []AppModel{NewApp("notes.md", cfg), NewCodeReviewApp(nil, "HEAD", cfg)} {
		if !app.math.Available() || !app.mermaid.Available() {
			t.Fatal("native renderers not initialized")
		}
	}
}
