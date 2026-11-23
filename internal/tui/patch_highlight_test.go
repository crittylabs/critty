package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/v2/internal/document"
	"github.com/crittylabs/critty/v2/internal/syntax"
)

func TestPatchSourceHighlight(t *testing.T) {
	const source = "--- a/sample.go\n+++ b/sample.go\n@@ -1,3 +1,3 @@\n package sample\n-var message = \"old\" // previous label\n+var message = \"new\" // current label\n func Read() {}\n"
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		tab := FileTab{path: "sample.patch", doc: document.FromContent("sample.patch", []byte(source))}
		tab.ensureHighlightCache()
		patch := tab.documentPatch()
		if patch == nil {
			t.Fatal("missing parsed patch")
		}
		for _, n := range []int{4, 5, 6, 7} {
			raw := tab.doc.LineAt(n)
			got := tab.patchDisplayLine(n, documentDisplayLine(&tab, n-1, raw))
			if ansi.Strip(got) != raw {
				t.Fatalf("line %d changed source: %q", n, got)
			}
			assertPatchSourceForeground(t, got, "sample.go", raw[1:])
			_, cells := renderedCells(got)
			if raw[0] == '-' && !sameColor(cells[1].Bg, diffDeletedLineBg.GetBackground()) ||
				raw[0] == '+' && !sameColor(cells[1].Bg, diffChangedLineBg.GetBackground()) {
				t.Fatalf("line %d lost its diff background", n)
			}
		}
		plainPatch := newCodeHighlighter(codeLexer(tab.path, source))(source)
		for _, n := range []int{1, 2, 3} {
			if tab.chromaLines[n-1] != plainPatch[n-1] {
				t.Fatal("source highlighting changed patch metadata colors")
			}
		}
	}
}

func assertPatchSourceForeground(t *testing.T, got, path, source string) {
	t.Helper()
	expected := newCodeHighlighter(codeLexer(path, source))(source + "\n")
	_, want := renderedCells(expected[0])
	_, cells := renderedCells(got)
	if len(cells) != len(want)+1 {
		t.Fatalf("source cells=%d, want %d plus prefix", len(cells), len(want))
	}
	for i := range want {
		got := cells[i+1]
		if got.Bg != nil && want[i].Fg != nil && contrastRatio(relativeLuminance(want[i].Fg), relativeLuminance(got.Bg)) < 4.5 {
			if got.Fg == nil || contrastRatio(relativeLuminance(got.Fg), relativeLuminance(got.Bg)) < 4.5 {
				t.Fatalf("source color at cell %d remains hard to read for %q", i, source)
			}
		} else if !sameColor(got.Fg, want[i].Fg) {
			t.Fatalf("source color at cell %d differs for %q", i, source)
		}
	}
}

func TestPatchSourceStateAndLanguages(t *testing.T) {
	const source = "--- a/source.go\n+++ b/source.go\n@@ -1,3 +1,3 @@\n-var text = `old\n+// updated\n continued\n end`\n" +
		"@@ -20 +20 @@\n-var next = 1\n+var next = 2\n" +
		"--- a/old.py\n+++ b/new.demo\n@@ -1 +1 @@\n-return \"old\"\n+return \"new\"\n" +
		"--- a/plain.unknown\n+++ b/plain.unknown\n@@ -1 +1 @@\n-previous\n+updated\n"
	rules, err := syntax.Compile(syntax.Config{Files: []syntax.FileType{{Pattern: "*.demo", Language: "ruby"}}})
	if err != nil {
		t.Fatal(err)
	}
	tab := FileTab{path: "sample.patch", syntax: rules, doc: document.FromContent("sample.patch", []byte(source))}
	tab.ensureHighlightCache()
	patch := tab.documentPatch()
	if patch == nil || len(patch.hunks) != 4 {
		t.Fatal("missing mixed-language patch hunks")
	}
	// Context belongs to both sides. The old-side raw string must not change
	// the new-side comment or the next hunk's independent lexer state.
	old := newCodeHighlighter(codeLexer("source.go", ""))("var text = `old\ncontinued\nend`\n")
	if patch.before[6] != " "+old[1] {
		t.Fatal("old-side context lost its multiline string color")
	}
	assertPatchSourceForeground(t, tab.chromaLines[5], "source.go", "continued")
	assertPatchSourceForeground(t, tab.chromaLines[9], "source.go", "var next = 2")
	assertPatchSourceForeground(t, tab.chromaLines[13], "old.py", "return \"old\"")
	assertPatchSourceForeground(t, tab.chromaLines[14], "new.rb", "return \"new\"")
	plain := newCodeHighlighter(codeLexer(tab.path, source))(source)
	for _, n := range []int{19, 20} {
		_, got := renderedCells(tab.chromaLines[n-1])
		_, want := renderedCells(plain[n-1])
		if !sameColor(got[1].Fg, want[1].Fg) || ansi.Strip(tab.chromaLines[n-1]) != tab.doc.LineAt(n) {
			t.Fatal("unknown language did not retain patch colors and source")
		}
	}
	if strings.Join(tab.doc.Lines, "\n") != source {
		t.Fatal("highlighting mutated the patch")
	}
}

func TestPatchSourceLateBackground(t *testing.T) {
	wasDark, previousBackground := terminalIsDark, terminalBackground
	t.Cleanup(func() { initAdaptiveStylesWithBackground(wasDark, previousBackground) })
	initAdaptiveStyles(true)
	app := patchDocumentApp(t, false)
	for _, tc := range []struct {
		background, added, deleted string
	}{
		{"#020203", "#29392C", "#43251F"},
		{"#202830", "#384C42", "#523835"},
		{"#FFFFFF", "#DAFBE1", "#FFEBE9"},
		{"#F5F6F8", "#D5F7DE", "#FAE7E6"},
	} {
		app, _ = updateApp(app, tea.BackgroundColorMsg{Color: lipgloss.Color(tc.background)})
		rows := strings.Split(app.contentViewport.GetContent(), "\n")
		for _, side := range []struct {
			line int
			want string
		}{{6, tc.deleted}, {8, tc.added}} {
			_, cells := renderedCells(rows[app.contentLayout.lineRanges[side.line].start])
			if got := cells[app.sourceGutterWidth()+1].Bg; !sameColor(got, lipgloss.Color(side.want)) {
				t.Errorf("terminal %s, line %d: background = %v, want %s", tc.background, side.line, got, side.want)
			}
		}
	}
}

func TestPatchSourceSelectionAndSearch(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	initAdaptiveStyles(wasDark)
	for _, split := range []bool{false, true} {
		m := patchDocumentApp(t, false)
		m.split = split
		m.hideLineNumbers = false
		m.recalculateLayout()
		m.tab().cursorLine, m.tab().selectAnchor = 6, 6
		m.tab().selecting = true
		m.rebuildContent()
		row := m.contentLayout.lineRanges[6].start
		text := strings.Split(m.contentViewport.GetContent(), "\n")[row]
		runes, cells := renderedCells(text)
		if !sameColor(cells[m.sourceGutterWidth()-2].Bg, buttonBackground) ||
			!sameColor(cells[m.sourceGutterWidth()+1].Bg, diffDeletedLineBg.GetBackground()) {
			t.Fatalf("split=%v: expected the current-line gutter and the deletion background", split)
		}
		if runes[gutterWidth-1] != '┃' || !sameColor(cells[gutterWidth-1].Fg, commentFocusedBorderColor) {
			t.Fatalf("split=%v: selected deletion is missing the blue selection bar", split)
		}
		if split && strings.Count(text, "┃") != 1 {
			t.Fatal("selection bar should appear only on the selected side")
		}
		m.tab().selecting = false
		m = ctrlKey(m, 's')
		m = typeSearch(m, "old one")
		assertISearchHighlight(t, m.contentViewport.GetContent(), "old one", true)
	}
}
