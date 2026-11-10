package tui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestMarkdownFrontmatter(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			previous := terminalIsDark
			terminalIsDark = dark
			t.Cleanup(func() { terminalIsDark = previous })
			for _, closing := range []string{"---", "..."} {
				source := "---\ntitle: '**literal** &amp;'\nenabled: true\ncount: 42\n# Metadata\ndescription: |\n  ## Literal heading\n  [label](https://example.org)\ntags: [one, two]\n" + closing + "\n# Body\n\n**bold**"
				parsed := parseMarkdownSource(strings.Split(source, "\n"), nil)
				wantHeadings := []outlineHeading{{line: 11, level: 1, label: "Body"}}
				if !reflect.DeepEqual(parsed.headings, wantHeadings) {
					t.Fatalf("headings = %#v, want %#v", parsed.headings, wantHeadings)
				}
				for i := range 10 {
					line := &parsed.lines[i]
					rendered := line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
					if got := ansi.Strip(rendered); got != line.source {
						t.Fatalf("line %d = %q, want %q", i+1, got, line.source)
					}
					if i == 1 && rendered == line.source {
						t.Fatal("YAML key and string have no syntax highlighting")
					}
					if got := line.sourceOffset(len(line.source), true); got != len(line.source) {
						t.Fatalf("line %d source end = %d", i+1, got)
					}
				}
				if got := ansi.Strip(parsed.lines[12].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)); got != "bold" {
					t.Fatalf("body rendering = %q", got)
				}
				inline := []gitpkg.InlineSegment{{Content: "title: "}, {Content: "'**literal** &amp;'", Changed: true}}
				got := parsed.lines[1].render(inline, diffCommonTextBg, diffAddedTextBg, nil)
				if ansi.Strip(got) != parsed.lines[1].source || !strings.Contains(got, bgToAnsi(diffAddedTextBg.GetBackground())) {
					t.Fatalf("word diff lost source or changed background: %q", got)
				}
			}
		})
	}
}

func TestMarkdownFrontmatterBoundaries(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, source string
		known        map[int]bool
		want         int
	}{
		{"empty", "", nil, 0},
		{"unclosed", "---\ntitle: value", nil, 0},
		{"body separator", "Intro\n---\ntitle: value\n---", nil, 0},
		{"indented", " ---\ntitle: value\n---", nil, 0},
		{"patch gap", "---\ntitle: value\n---", map[int]bool{1: true, 3: true}, 0},
		{"unknown start", "---\ntitle: value\n---", map[int]bool{2: true, 3: true}, 0},
		{"complete patch", "---\ntitle: value\n---", map[int]bool{1: true, 2: true, 3: true}, 3},
		{"empty metadata", "---\n---", nil, 2},
		{"CRLF and trailing space", "---\r\ntitle: value\r\n--- \t\r", nil, 3},
		{"BOM", "\ufeff---\ntitle: value\n---", nil, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.source, "\n")
			parsed := &markdownSource{source: tt.source, lines: make([]markdownLine, len(lines)), offsets: make([]int, len(lines))}
			for i, line := range lines {
				parsed.lines[i].source = line
				if i > 0 {
					parsed.offsets[i] = parsed.offsets[i-1] + len(lines[i-1]) + 1
				}
			}
			if got := parsed.parseFrontmatter(tt.known); got != tt.want {
				t.Fatalf("frontmatter end = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMarkdownFrontmatterOutlineView(t *testing.T) {
	const source = "---\ntitle: Example\n# Metadata\n---\n# Body\n\nText\n"
	app := outlineTestApp(t, source)
	if want := []outlineHeading{{line: 5, level: 1, label: "Body"}}; !reflect.DeepEqual(app.outline.rows, want) {
		t.Fatalf("outline rows = %#v, want %#v", app.outline.rows, want)
	}
	view := ansi.Strip(app.contentViewport.GetContent())
	for _, text := range []string{"title: Example", "# Metadata", "# Body"} {
		if !strings.Contains(view, text) {
			t.Fatalf("document view lost %q: %q", text, view)
		}
	}
}
