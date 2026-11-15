package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
)

func TestPatchMarkdownSharesDocumentStyles(t *testing.T) {
	lines := []string{
		"# Heading", "", "## Section", "", "### Detail", "", "#### Minor", "",
		"Text **bold** and *italic* and `code`.", "", "> Quoted", "", "- [ ] Task", "",
		"[Link](https://example.org) <kbd>Enter</kbd>", "", "```go", "var name = \"value\"", "```", "",
		"```text", "# literal", "```", "", "\\# escaped", "", "---", "", "A &amp; B and é.",
	}
	var patch strings.Builder
	fmt.Fprintf(&patch, "--- /dev/null\n+++ b/docs.md\n@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		fmt.Fprintf(&patch, "+%s\n", line)
	}
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		tab := FileTab{path: "sample.patch", doc: document.FromContent("sample.patch", []byte(patch.String()))}
		tab.ensureHighlightCache()
		parsed := parseMarkdownSource(lines, nil)
		for _, sample := range []struct {
			line int
			word string
		}{
			{0, "Heading"}, {2, "Section"}, {4, "Detail"}, {6, "Minor"},
			{8, "bold"}, {8, "italic"}, {8, "code"}, {10, "Quoted"}, {12, "Task"},
			{14, "Link"}, {14, "Enter"}, {17, "var"},
		} {
			normal := parsed.lines[sample.line].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			want := sourceWordStyle(t, normal, sample.word)
			got := sourceWordStyle(t, tab.chromaLines[sample.line+3], sample.word)
			if !sameColor(got.Fg, want.Fg) && (got.Fg != nil || want.Fg != nil) || got.Attrs != want.Attrs {
				t.Fatalf("dark=%v, %q: source style %+v differs from document %+v", dark, sample.word, got, want)
			}
		}
		for i, line := range lines {
			got := tab.chromaLines[i+3]
			if ansi.Strip(got) != "+"+line {
				t.Fatalf("line %d changed source: %q", i+1, got)
			}
		}
		for _, i := range []int{21, 24} {
			style := sourceWordStyle(t, tab.chromaLines[i+3], "#")
			if sameColor(style.Fg, mdH1Style.GetForeground()) || style.Attrs&uv.AttrBold != 0 {
				t.Fatalf("literal or escaped heading on line %d received heading styling", i+1)
			}
		}
	}
}

func sourceWordStyle(t *testing.T, rendered, word string) uv.Style {
	t.Helper()
	runes, styles := renderedCells(rendered)
	plain := string(runes)
	at := strings.Index(plain, word)
	if at < 0 {
		t.Fatalf("missing %q in %q", word, plain)
	}
	return styles[len([]rune(plain[:at]))]
}
