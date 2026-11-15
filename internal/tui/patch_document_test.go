package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/v2/internal/document"
	"github.com/crittylabs/critty/v2/internal/review"
)

func patchDocumentApp(t *testing.T, attachment bool) AppModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.patch")
	if err := os.WriteFile(path, []byte(formatsPatch), 0644); err != nil {
		t.Fatal(err)
	}
	var m AppModel
	if attachment {
		sess, err := review.OpenSession(t.TempDir(), "diff")
		if err != nil {
			t.Fatal(err)
		}
		docs, err := review.ReadAttachedDocuments([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		m = NewCodeReviewApp(nil, "", AppConfig{Session: sess, Documents: docs, Author: "Tester"})
	} else {
		m = NewApp(path, AppConfig{ReadOnly: true})
	}
	m, _ = updateApp(m, docRenderedMsg{})
	m, _ = updateApp(m, tea.WindowSizeMsg{Width: 160, Height: 45})
	m.focused = contentPane
	return m
}

func TestPatchDocumentSplit(t *testing.T) {
	for _, attachment := range []bool{false, true} {
		t.Run(map[bool]string{false: "view", true: "attachment"}[attachment], func(t *testing.T) {
			m := patchDocumentApp(t, attachment)
			if m.split || m.documentPatch(m.tab()) == nil || m.splitButton() == "" {
				t.Fatal("patch document must offer split without enabling it by default")
			}
			original := m.tab().doc.Content
			m = pressKey(m, 'S')
			if !m.split || !m.splitPatchDocument(m.tab()) {
				t.Fatal("S did not enable patch document splitting")
			}
			old := m.contentLayout.lineRanges[6].start
			new := m.contentLayout.lineRanges[8].start
			if old != new || m.contentLayout.columns[old].split == 0 {
				t.Fatalf("raw deleted/added lines were not paired: %d/%d", old, new)
			}
			for _, n := range []int{1, 4, 6, 7, 8, 11, 12} {
				if _, ok := m.contentRenderedRange("", n, n); !ok {
					t.Fatalf("source line %d lost its range", n)
				}
			}
			m.tab().cursorLine = 6
			m, _ = updateApp(m, tea.KeyPressMsg{Code: tea.KeyRight})
			if m.tab().cursorLine != 8 || m.tab().cursorSide != "" {
				t.Fatal("right arrow lost the paired patch source line")
			}
			m, _ = updateApp(m, tea.KeyPressMsg{Code: tea.KeyLeft})
			if m.tab().cursorLine != 6 || m.tab().cursorSide != "" {
				t.Fatal("left arrow lost the paired patch source line")
			}
			m = pressKey(m, 'S')
			if m.split || m.tab().cursorLine != 6 || m.tab().doc.Content != original {
				t.Fatal("toggle changed source or focus")
			}
			if len(m.contentLayout.columns) != 0 {
				t.Fatal("unified display retained split mouse columns")
			}
		})
	}
}

func TestPatchDocumentSourceInteractions(t *testing.T) {
	m := patchDocumentApp(t, true)
	m = pressKey(m, 'S')
	row := m.contentLayout.lineRanges[6].start
	columns := m.contentLayout.columns[row]
	for _, target := range []struct{ x, line int }{{0, 6}, {columns.split + 3, 8}} {
		ref, ok := m.contentMouseTargetAt(target.x, row)
		if !ok || ref.side != "" || ref.line != target.line {
			t.Fatalf("mouse target x=%d: %+v, %v", target.x, ref, ok)
		}
	}
	// Comment on the deletion using the real content gutter, not the old side
	// of the file changed by the patch.
	x, y := contentScreenPoint(m, 0, row)
	m = clickMouse(m, x, y)
	if m.modal != commentModal {
		t.Fatal("gutter did not open a comment")
	}
	m.modalTextarea.SetValue("Check this removal.")
	m, _ = updateApp(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	comments := m.tab().state.Comments
	if len(comments) != 1 || comments[0].Side != "" || comments[0].StartLine != 6 || comments[0].Anchor != "-old one" {
		t.Fatalf("wrong patch comment anchor: %+v", comments)
	}
	for range 2 {
		m = pressKey(m, 'S')
		if !reflect.DeepEqual(comments, m.tab().state.Comments) || !strings.Contains(ansi.Strip(m.contentViewport.GetContent()), "Check this removal.") {
			t.Fatal("toggle lost comment content or anchors")
		}
	}
	m.tab().cursorLine, m.tab().cursorOnAnnotation = 6, false
	m = pressKey(m, 'v')
	m.tab().cursorLine = 8
	m, cmd := updateApp(m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
	assertCopiedText(t, m, cmd, "-old one\n-old two\n+new one")
	m.tab().selecting = false
	m.tab().cursorLine = 8
	m, cmd = pressKeyCmd(m, '@')
	assertCopiedText(t, m, cmd, "@"+escapeReferencePath(m.tab().path)+" L8")
	for _, n := range []int{6, 8} {
		m.clearTextSelection()
		m.rebuildContent()
		found := false
		for _, span := range m.contentLayout.textSpans {
			if span.ref.line != n {
				continue
			}
			left, top, _, _ := m.contentBounds()
			y := top + span.row - m.contentViewport.YOffset()
			m = pressMouse(m, left+span.left, y)
			m = releaseMouse(m, left+span.left+ansi.StringWidth(span.text), y)
			if got := m.selectedText(); got != m.tab().doc.LineAt(n) {
				t.Fatalf("drag on line %d copied %q", n, got)
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("source line %d has no selectable text", n)
		}
	}
	m.clearTextSelection()

	// The same document can reload without turning into a multi-file patch.
	m.tab().doc = document.FromContent(m.tab().path, []byte(strings.ReplaceAll(formatsPatch, "+new one", "+updated one")))
	m.tab().resetHighlightCache()
	m.tab().ensureHighlightCache()
	m.rebuildContent()
	if !m.splitPatchDocument(m.tab()) || !strings.Contains(ansi.Strip(m.contentViewport.GetContent()), "+updated one") {
		t.Fatal("patch projection did not refresh with source")
	}
}

func TestPatchDocumentNarrowAndOuterDiff(t *testing.T) {
	m := patchDocumentApp(t, false)
	m = pressKey(m, 'S')
	m, _ = updateApp(m, tea.WindowSizeMsg{Width: 30, Height: 24})
	if !m.split || m.splitPatchDocument(m.tab()) || len(m.contentLayout.columns) != 0 {
		t.Fatal("narrow display must fall back without forgetting split preference")
	}
	m, _ = updateApp(m, tea.WindowSizeMsg{Width: 160, Height: 45})
	if !m.splitPatchDocument(m.tab()) || len(m.contentLayout.columns) == 0 {
		t.Fatal("wide display did not restore split")
	}
	m.multiFile = true
	m.tab().status = "untracked"
	if m.documentPatch(m.tab()) != nil {
		t.Fatal("an untracked patch under Git review enabled nested split")
	}
	outer := patchApp(t, strings.ReplaceAll(formatsPatch, "missing.go", "sample.patch"))
	outer = pressKey(outer, 'S')
	if outer.documentPatch(outer.tab()) != nil || !outer.alternateDiff(outer.tab()) {
		t.Fatal("Git changes to a patch must keep the outer diff renderer")
	}
}

func TestPatchDocumentDragColumns(t *testing.T) {
	for _, tc := range []struct {
		start, end int
		want       string
	}{
		{6, 7, "-old one\n-old two"},
		{8, 9, "+new one\n after"},
	} {
		m := patchDocumentApp(t, false)
		m = pressKey(m, 'S')
		start, end := -1, -1
		for i, span := range m.contentLayout.textSpans {
			if span.ref.line == tc.start {
				start = i
			}
			if start >= 0 && span.ref.line == tc.end && span.column == m.contentLayout.textSpans[start].column {
				end = i
			}
		}
		if start < 0 || end < 0 {
			t.Fatal("missing selectable patch cells")
		}
		a, b := m.contentLayout.textSpans[start], m.contentLayout.textSpans[end]
		left, top, _, _ := m.contentBounds()
		m = pressMouse(m, left+a.left, top+a.row)
		m = moveMouse(m, left+b.left+ansi.StringWidth(b.text), top+b.row)
		m = releaseMouse(m, left+b.left+ansi.StringWidth(b.text), top+b.row)
		if got := m.selectedText(); got != tc.want {
			t.Fatalf("drag from %d to %d copied %q, want %q", tc.start, tc.end, got, tc.want)
		}
	}
}

func TestPatchDocumentRows(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"git", formatsPatch, true},
		{"unified", "--- a.txt\n+++ a.txt\n@@ -1 +1 @@\n-old\n+new\n", true},
		{"mail", "Subject: Update\n\n" + formatsPatch + "-- \n2.0\n", true},
		{"files", formatsPatch + strings.ReplaceAll(formatsPatch, "missing.go", "other.go"), true},
		{"no newline", "--- a.txt\n+++ a.txt\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n", true},
		{"empty context", "--- a.txt\n+++ a.txt\n@@ -1,2 +1,2 @@\n\n-old\n+new\n", true},
		{"invalid", "--- a.txt\n+++ a.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n", false},
		{"ambiguous", "@@ -1,1 +1,1 @@\nPreamble\n" + formatsPatch, false},
		{"plain", "Text with no patch.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.source, "\n")
			patch := parsePatchDocument(lines, tc.source)
			if (patch != nil) != tc.valid {
				t.Fatalf("valid=%v, patch=%+v", tc.valid, patch)
			}
			if !tc.valid {
				return
			}
			seen := map[int]bool{}
			for _, row := range patch.rows {
				for _, n := range []int{row.left, row.right} {
					if n > 0 {
						seen[n] = true
					}
				}
			}
			if len(seen) != len(lines) {
				t.Fatalf("lost raw source lines: %v of %d", seen, len(lines))
			}
		})
	}
}
