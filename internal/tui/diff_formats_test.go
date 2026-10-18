package tui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/review"
)

const formatsPatch = "diff --git a/missing.go b/missing.go\n--- a/missing.go\n+++ b/missing.go\n" +
	"@@ -10,4 +20,3 @@\n before\n-old one\n-old two\n+new one\n after\n" +
	"@@ -40 +50 @@\n-tail\n+final\n"

func TestDiffFormatsPartialPatch(t *testing.T) {
	m := patchApp(t, formatsPatch)
	source := m.tab().doc.Content
	wantRefs := m.visualLines(m.tab())
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			m.split = split
			m.rebuildContent()
			got := ansi.Strip(m.contentViewport.GetContent())
			for _, text := range []string{"before", "old one", "old two", "new one", "after", "tail", "final"} {
				if !strings.Contains(got, text) {
					t.Errorf("missing %q in %s", text, got)
				}
			}

			for _, ref := range wantRefs {
				if _, ok := m.contentRenderedRange(ref.side, ref.line, ref.line); !ok {
					t.Errorf("missing source range %+v", ref)
				}
			}
			for _, ref := range m.visualLines(m.tab()) {
				found := false
				for _, want := range wantRefs {
					found = found || ref == want
				}
				if !found {
					t.Errorf("invented source line %+v", ref)
				}
			}
			if m.tab().doc.Content != source {
				t.Fatal("source changed")
			}
		})
	}
}

func TestSplitDiffShowsAllAvailableSource(t *testing.T) {
	var patch strings.Builder
	patch.WriteString("diff --git a/large.txt b/large.txt\n--- a/large.txt\n+++ b/large.txt\n@@ -1,80 +1,80 @@\n")
	for line := 1; line <= 80; line++ {
		if line == 20 || line == 70 {
			fmt.Fprintf(&patch, "-old %d\n+new %d\n", line, line)
		} else {
			fmt.Fprintf(&patch, " row %d\n", line)
		}
	}
	m := patchApp(t, patch.String())
	m.tab().doc.Known = nil // the complete source is available, as in a Git review
	m.tab().cursorLine, m.tab().cursorSide = 20, "old"
	m.split = true
	m.rebuildContent()
	content := ansi.Strip(m.contentViewport.GetContent())
	if hunks := diffHunks(m.tab()); len(hunks) != 1 || len(hunks[0]) != 80 {
		t.Fatalf("available source is missing: %+v", hunks)
	}
	for _, line := range []int{20, 70} {
		if m.contentLayout.oldRanges[line].start != m.contentLayout.lineRanges[line].start {
			t.Fatalf("old and new line %d are not paired", line)
		}
	}
	for _, text := range []string{"row 16", "row 24", "row 40", "row 66", "row 74"} {
		if !strings.Contains(content, text) {
			t.Fatalf("available context %q is missing", text)
		}
	}
	if _, ok := m.contentLayout.lineRanges[40]; !ok {
		t.Fatal("unchanged line has no display range")
	}
	source := m.tab().doc.Content
	m.navigateSource(sourceLocation{path: m.tab().path, line: 40})
	if _, ok := m.contentLayout.lineRanges[40]; !ok || m.tab().doc.Content != source {
		t.Fatal("explicit focus did not retain unchanged source without modifying it")
	}
	m = pressKey(m, 'n')
	if m.tab().cursorLine != 70 {
		t.Fatal("change navigation stopped on unchanged context")
	}
}

func TestDiffFormatMouseAndNavigation(t *testing.T) {
	m := patchApp(t, formatsPatch)
	m.split = true
	m.rebuildContent()
	row := m.contentLayout.oldRanges[11].start
	columns := m.contentLayout.columns[row]
	plainRows := strings.Split(ansi.Strip(m.contentViewport.GetContent()), "\n")
	commonRow := m.contentLayout.lineRanges[20].start
	if text := plainRows[commonRow]; !strings.HasPrefix(text, "    10 before") {
		t.Fatalf("unchanged gutter is misaligned: %q", text)
	}
	for _, tc := range []struct {
		x    int
		side string
		line int
		ok   bool
	}{
		{0, "old", 11, true}, {columns.split + 3, "", 21, true}, {columns.split + 1, "", 0, false},
	} {
		got, ok := m.contentMouseTargetAt(tc.x, row)
		if ok != tc.ok || ok && (got.side != tc.side || got.line != tc.line) {
			t.Fatalf("x=%d: %+v %v", tc.x, got, ok)
		}
	}
	unpaired := m.contentLayout.oldRanges[12].start
	if _, ok := m.contentMouseTargetAt(columns.split+3, unpaired); ok {
		t.Fatal("blank cell became a comment anchor")
	}
	m.tab().cursorLine, m.tab().cursorSide = 11, "old"
	m.moveDiffSide(false)
	if m.tab().cursorLine != 21 || m.tab().cursorSide != "" {
		t.Fatal("right arrow lost paired new line")
	}
	m.moveDiffSide(true)
	if m.tab().cursorLine != 11 || m.tab().cursorSide != "old" {
		t.Fatal("left arrow lost paired old line")
	}
	if next, ok := m.adjacentLine(m.tab(), 1); !ok || next != (lineRef{side: "old", line: 12}) {
		t.Fatalf("next=%+v %v", next, ok)
	}
	left, top, _, _ := m.contentBounds()
	m, _ = updateApp(m, tea.MouseClickMsg{X: left + columns.split + 4, Y: top + row - m.contentViewport.YOffset(), Button: tea.MouseLeft})
	if m.tab().cursorLine != 21 || m.tab().cursorSide != "" {
		t.Fatal("mouse selected wrong side")
	}
}

func TestDiffFormatSwitchKeepsAttentionAndComments(t *testing.T) {
	var patch strings.Builder
	patch.WriteString("diff --git a/long.txt b/long.txt\n--- a/long.txt\n+++ b/long.txt\n@@ -1,80 +1,80 @@\n")
	for i := 1; i <= 80; i++ {
		if i == 40 {
			patch.WriteString("-old focused\n+new focused\n")
		} else {
			fmt.Fprintf(&patch, " context %d\n", i)
		}
	}
	for _, mode := range []string{"source", "new", "thread", "new-thread", "scrolled"} {
		t.Run(mode, func(t *testing.T) {
			m := patchApp(t, patch.String())
			m.contentViewport.SetHeight(12)
			m.tab().state.Comments = []review.Comment{{ID: "old", Side: "old", StartLine: 40, EndLine: 40, Body: "old comment"}, {ID: "new", StartLine: 40, EndLine: 40, Body: "new comment"}}
			m.tab().cursorLine, m.tab().cursorSide = 40, "old"
			if strings.HasPrefix(mode, "new") {
				m.tab().cursorSide = ""
			}
			m.tab().selecting, m.tab().selectSide, m.tab().selectAnchor = true, "old", 40
			m.tab().selectSide = m.tab().cursorSide
			if mode == "thread" || mode == "new-thread" {
				m.tab().selecting = false
				m.tab().cursorOnAnnotation = true
			}
			m.rebuildContent()
			anchor := contentMouseTarget{line: 40, side: m.tab().cursorSide, annotation: m.tab().cursorOnAnnotation}
			row, _ := m.diffAnchorRow(anchor)
			m.contentViewport.SetYOffset(row - 4)
			if mode == "scrolled" {
				m.contentViewport.SetYOffset(8)
				anchor = m.contentLayout.rows[8]
				row = 8
			}
			wantY := row - m.contentViewport.YOffset()
			before, err := json.Marshal(m.tab().state.Comments)
			if err != nil {
				t.Fatal(err)
			}
			for range 4 {
				m.toggleSplit()
				row, ok := m.diffAnchorRow(anchor)
				if !ok || row-m.contentViewport.YOffset() != wantY {
					t.Fatalf("split=%v moved focus: row %d offset %d want screen y %d", m.split, row, m.contentViewport.YOffset(), wantY)
				}
				if m.tab().cursorLine != 40 || m.tab().cursorSide != anchor.side && mode != "scrolled" {
					t.Fatal("cursor moved")
				}
				after, err := json.Marshal(m.tab().state.Comments)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("comments changed")
				}
				if (mode == "source" || mode == "new") && (!m.tab().selecting || m.tab().selectAnchor != 40 || m.tab().selectSide != anchor.side) {
					t.Fatal("selection lost")
				}
				if (mode == "thread" || mode == "new-thread") && !m.tab().cursorOnAnnotation {
					t.Fatal("thread focus lost")
				}
			}
		})
	}
}

func TestDiffFormatKeyAndResize(t *testing.T) {
	m := patchApp(t, formatsPatch)
	m = pressKey(m, 'S')
	if !m.split {
		t.Fatal("S did not enable split")
	}
	m.openLineComment()
	m, _ = updateApp(m, tea.KeyPressMsg{Code: 'S', Text: "S"})
	if !m.split || m.modalTextarea.Value() != "S" {
		t.Fatal("S in comment input changed split")
	}
	m.modal = noModal
	m.split = true
	m.tab().cursorLine, m.tab().cursorSide = 11, "old"
	m.rebuildContent()
	anchor := contentMouseTarget{line: 11, side: "old"}
	for _, width := range []int{55, 120} {
		row, _ := m.diffAnchorRow(anchor)
		m.contentViewport.SetYOffset(max(0, row-2))
		y := row - m.contentViewport.YOffset()
		m, _ = updateApp(m, tea.WindowSizeMsg{Width: width, Height: 20})
		row, ok := m.diffAnchorRow(anchor)
		maxOffset := max(0, len(strings.Split(m.contentViewport.GetContent(), "\n"))-m.contentViewport.Height())
		wantOffset := min(max(0, row-y), maxOffset)
		if !ok || m.contentViewport.YOffset() != wantOffset {
			t.Fatalf("resize %d moved attention: row=%d offset=%d, wanted offset=%d", width, row, m.contentViewport.YOffset(), wantOffset)
		}
		if !m.split {
			t.Fatal("resize replaced requested format")
		}
	}
}

func TestDiffFormatWrappingAndNarrowFallback(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-"+strings.Repeat("旧é", 30)+"\n+"+strings.Repeat("新\t", 30)+"\n")
	m.split = true
	m.rebuildContent()
	if m.contentLayout.oldRanges[1].end-m.contentLayout.oldRanges[1].start < 2 {
		t.Fatal("old text did not wrap")
	}
	for _, row := range strings.Split(m.contentViewport.GetContent(), "\n") {
		if ansi.StringWidth(row) > m.contentViewport.Width() {
			t.Fatalf("row overflow: %d > %d", ansi.StringWidth(row), m.contentViewport.Width())
		}
	}
	before := m.tab().cursorLine
	m.contentViewport.SetWidth(35)
	m.rebuildContent()
	if m.alternateDiff(m.tab()) || !strings.Contains(ansi.Strip(m.splitButton()), "Split: on") {
		t.Fatal("narrow fallback lost split preference")
	}
	m.contentViewport.SetWidth(95)
	m.rebuildContent()
	if !m.alternateDiff(m.tab()) || m.tab().cursorLine != before {
		t.Fatal("resize lost requested format or cursor")
	}
}

func TestSplitWhitespaceAndEmptySides(t *testing.T) {
	for _, tc := range []struct{ name, patch, side string }{
		{"added", "diff --git a/a b/a\nnew file mode 100644\n--- /dev/null\n+++ b/a\n@@ -0,0 +1,2 @@\n+one\n+two\n", ""},
		{"deleted", "diff --git a/a b/a\ndeleted file mode 100644\n--- a/a\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-one\n-two\n", "old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := patchApp(t, tc.patch)
			m.split = true
			m.rebuildContent()
			refs := m.visualLines(m.tab())
			if len(refs) != 2 {
				t.Fatal("lost source lines")
			}
			for i, ref := range refs {
				if ref.side != tc.side || ref.line != i+1 {
					t.Fatalf("wrong source anchor: %+v", ref)
				}
			}
		})
	}
	m := patchApp(t, whitespacePatch)
	m.split = true
	m.toggleWhitespace()
	for _, hunk := range diffHunks(m.tab()) {
		for _, row := range hunk {
			if row.before != nil && row.before.ref.side == "" && row.before.oldLine != row.before.ref.line {
				t.Fatal("whitespace filtering lost original coordinates")
			}
		}
	}
	original := m.tab().doc.Content
	m.toggleSplit()
	m.toggleWhitespace()
	if !reflect.DeepEqual(m.tab().changedLines, m.tab().diff.ChangedLines) || m.tab().doc.Content != original {
		t.Fatal("switching format lost diff source")
	}
}
