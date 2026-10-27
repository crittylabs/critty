package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestMarkdownParagraphSourceLines(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   []string
		attr   uint8
		uri    string
	}{
		{"**日本語 é\ncontinued**", []string{"日本語 é", "continued"}, uv.AttrBold, ""},
		{"*first\nsecond*", []string{"first", "second"}, uv.AttrItalic, ""},
		{"`first\nsecond`", []string{" first", "second "}, 0, ""},
		{"` foo `", []string{" foo "}, 0, ""},
		{"`` `tick` ``", []string{" `tick` "}, 0, ""},
		{"[first\nsecond](https://example.org/a_(b))", []string{"first", "second"}, 0, "https://example.org/a_(b)"},
		{"- **first\n  second**", []string{"- first", "  second"}, 0, ""},
		{"> *first\n> second*", []string{"▎ first", "▎ second"}, 0, ""},
		{"`first\n\nsecond`", []string{"`first", "", "second`"}, 0, ""},
		{"- **first\n- second**", []string{"- **first", "- second**"}, 0, ""},
		{"**first\n# second**", []string{"**first", "# second**"}, 0, ""},
		{"```md\n**literal**\n```", []string{"```md", "**literal**", "```"}, 0, ""},
		{"| **first | second** |\n| --- | --- |", []string{"| **first | second** |", "| --- | --- |"}, 0, ""},
		{"[label][ref]\n\n[ref]: https://example.org", []string{"[label][ref]", "", "[ref]: https://example.org"}, 0, ""},
	} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%v", tt.source, dark), func(t *testing.T) {
				initAdaptiveStyles(dark)
				parsed := parseMarkdownSource(strings.Split(tt.source, "\n"), nil)
				for i, line := range parsed.lines {
					rendered := line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
					if got := ansi.Strip(rendered); got != tt.want[i] {
						t.Fatalf("line %d = %q, want %q", i+1, got, tt.want[i])
					}
					if strings.Contains(rendered, "\n") {
						t.Fatal("rendering changed source line boundaries")
					}
					for _, row := range uv.NewStyledString(rendered).Lines(ansi.GraphemeWidth) {
						for _, cell := range row {
							if strings.TrimSpace(cell.Content) != "" && (cell.Style.Attrs&tt.attr != tt.attr || cell.Link.URL != tt.uri) {
								t.Fatalf("line %d lost attributes/link: %+v", i+1, cell)
							}
						}
					}
				}
			})
		}
	}
}

func TestMarkdownIncompleteContextIsLiteral(t *testing.T) {
	lines := []string{"", "**cut", "middle**", "", "**complete", "paragraph**", "", "`cut"}
	known := map[int]bool{2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true}
	parsed := parseMarkdownSource(lines, known)
	for i, want := range map[int]string{1: "**cut", 2: "middle**", 4: "complete", 5: "paragraph", 7: "`cut"} {
		if got := ansi.Strip(parsed.lines[i].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)); got != want {
			t.Fatalf("line %d = %q, want %q", i+1, got, want)
		}
	}
}

func TestMarkdownTableInlineRendering(t *testing.T) {
	const uri = "https://github.com/knu/avail/pull/54"
	const content = "| PR | Notes |\n| --- | --- |\n| [avail #54](" + uri + ") | **bold** and `code` |\n"
	for _, attachment := range []bool{false, true} {
		t.Run(fmt.Sprintf("attachment=%t", attachment), func(t *testing.T) {
			app := setupAppWithDoc(t, content)
			if attachment {
				app, _ = formTestAppWithContent(t, content)
			}
			app.width, app.height = 100, 30
			app.recalculateLayout()
			app.rebuildContent()
			view := app.contentViewport.GetContent()
			if !strings.Contains(view, ansi.SetHyperlink(uri)) {
				t.Fatalf("table link is not rendered: %q", ansi.Strip(view))
			}
			if strings.Contains(ansi.Strip(view), "**bold**") || strings.Contains(ansi.Strip(view), "`code`") {
				t.Fatalf("table markup is visible: %q", ansi.Strip(view))
			}
		})
	}
}

func TestMarkdownTableCells(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         []string
	}{
		{"nested link destination", "| plain | [label](https://example.org/a_(b)) |", []string{"plain", "label"}},
		{"empty cell", "| | **bold** |", []string{"", "bold"}},
		{"unicode", "| 日本語 | é |", []string{"日本語", "é"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "| First | Second |\n| --- | --- |\n"+tt.source)
			cells := app.tab().markdownTableCells(3, nil)
			if len(cells) != len(tt.want) {
				t.Fatalf("cells = %q, want %q", cells, tt.want)
			}
			for i, cell := range cells {
				if got := ansi.Strip(cell); got != tt.want[i] {
					t.Fatalf("cell %d = %q, want %q", i, got, tt.want[i])
				}
			}
		})
	}
}

func TestMarkdownCachedInlineCompatibilityAndImages(t *testing.T) {
	for _, source := range []string{
		`plain \*literal\* &amp; &#x1F600;`,
		"**bold**  \t ",
		"[**label**](https://example.org/a_(b))  ",
		"before ` code ` after",
		"https://example.org/plain.",
	} {
		t.Run(source, func(t *testing.T) {
			parsed := parseMarkdownSource([]string{source}, nil)
			got := parsed.lines[0].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			if ansi.Strip(got) != ansi.Strip(highlightInline(displaySourceText(source))) {
				t.Fatalf("inline text changed: got %q, previous %q", ansi.Strip(got), ansi.Strip(highlightInline(displaySourceText(source))))
			}
		})
	}
	source := "![picture](https://example.org/a**b**.png)"
	parsed := parseMarkdownSource([]string{source}, nil)
	uri := imageTarget{url: "https://example.org/a**b**.png"}.uri()
	rendered := parsed.lines[0].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), []linkSpan{{start: 0, end: len(source), uri: uri}})
	if !strings.Contains(rendered, ansi.SetHyperlink(uri)) || !strings.Contains(ansi.Strip(rendered), "picture") {
		t.Fatalf("image link changed: %q", rendered)
	}
}

func TestMarkdownEmptyLabels(t *testing.T) {
	for _, tc := range []struct{ source, want, uri string }{
		{"![](./clip.mp4)", "!./clip.mp4", ""},
		{"[](./guide.md)", "./guide.md", "./guide.md"},
		{"before [ ](https://example.org) after", "before https://example.org after", "https://example.org"},
		{"[](https://example.org/a**b**)", "https://example.org/a**b**", "https://example.org/a**b**"},
		{"before ![ ](./chart.png) after", "before !./chart.png after", ""},
		{"![](https://example.org/a**b**.png)", "!https://example.org/a**b**.png", ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			parsed := parseMarkdownSource([]string{tc.source}, nil)
			cached := parsed.lines[0].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			for name, got := range map[string]string{"cached": cached, "inline": highlightInline(tc.source)} {
				if plain := ansi.Strip(got); plain != tc.want {
					t.Errorf("%s label = %q, want %q", name, plain, tc.want)
				}
				if tc.uri != "" && !strings.Contains(got, ansi.SetHyperlink(tc.uri)) {
					t.Errorf("%s link lost its target: %q", name, got)
				}
			}
			if parsed.lines[0].source != tc.source {
				t.Fatal("rendering changed the source")
			}
			start := parsed.lines[0].sourceOffset(0, false)
			end := parsed.lines[0].sourceOffset(len(tc.want), true)
			if got := parsed.copySourceRange(start, end); got != tc.source {
				t.Fatalf("copied source = %q, want %q", got, tc.source)
			}
		})
	}
}

func TestMarkdownWordDiffKeepsParagraphContext(t *testing.T) {
	initAdaptiveStyles(true)
	parsed := parseMarkdownSource([]string{"**old first", "common changed**"}, nil)
	inline := []gitpkg.InlineSegment{{Content: "common "}, {Content: "changed", Changed: true}, {Content: "**"}}
	rendered := parsed.lines[1].render(inline, diffCommonTextBg, diffAddedTextBg, nil)
	if ansi.Strip(rendered) != "common changed" {
		t.Fatalf("word diff lost text: %q", rendered)
	}
	for _, row := range uv.NewStyledString(rendered).Lines(ansi.GraphemeWidth) {
		for i, cell := range row {
			if cell.Style.Attrs&uv.AttrBold == 0 {
				t.Fatalf("bold lost across changed segment: %+v", cell)
			}
			want := diffCommonTextBg.GetBackground()
			if i >= len("common ") {
				want = diffAddedTextBg.GetBackground()
			}
			if !sameColor(cell.Style.Bg, want) {
				t.Fatalf("wrong diff background at %d: %+v", i, cell)
			}
		}
	}
}

func TestMarkdownCacheOldNewAndRedisplay(t *testing.T) {
	patch := "diff --git a/review.md b/review.md\n--- a/review.md\n+++ b/review.md\n@@ -1,4 +1,4 @@\n-**old\n+`new\n common\n-tail**\n+tail`\n \n"
	app := patchApp(t, patch)
	app.tab().doc.Known = nil // complete blobs are available in ordinary Git reviews
	app.tab().ensureMarkdownCache()
	cache := app.tab().markdown
	before := app.tab().markdownDisplayLine(2, true, "common", nil, nil)
	after := app.tab().markdownDisplayLine(2, false, "common", nil, nil)
	if !strings.Contains(before, "\x1b[1;") && !strings.Contains(before, "\x1b[1m") {
		t.Fatalf("before side lost bold: %q", before)
	}
	if before == after {
		t.Fatal("before and after shared their inline context")
	}
	for _, split := range []bool{false, true} {
		app.split = split
		app.rebuildContent()
		for _, ref := range []lineRef{{line: 1}, {line: 2}, {line: 3}, {side: "old", line: 1}, {side: "old", line: 3}} {
			if _, ok := app.contentRenderedRange(ref.side, ref.line, ref.line); !ok {
				t.Fatalf("source line lost after rendering: %+v", ref)
			}
		}
		app, _ = updateApp(app, tea.WindowSizeMsg{Width: 54, Height: 16})
		app, _ = updateApp(app, tea.BackgroundColorMsg{Color: lipgloss.White})
		if app.tab().markdown != cache {
			t.Fatal("resize/recolor reparsed Markdown")
		}
	}
	app.tab().doc = document.FromContent("review.md", []byte("**replacement**"))
	app.tab().ensureMarkdownCache()
	if app.tab().markdown == cache {
		t.Fatal("new content reused the old parse")
	}
}

func BenchmarkMarkdownParagraphs(b *testing.B) {
	for _, count := range []int{20, 20000} {
		lines := strings.Split("**first\n"+strings.Repeat("words ", count)+"\nlast**", "\n")
		b.Run(fmt.Sprintf("words=%d", count), func(b *testing.B) {
			b.Run("parse", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					parseMarkdownSource(lines, nil)
				}
			})
			b.Run("cached", func(b *testing.B) {
				parsed := parseMarkdownSource(lines, nil)
				b.ReportAllocs()
				for b.Loop() {
					for i := range parsed.lines {
						parsed.lines[i].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
					}
				}
			})
			b.Run("previous-line-parser", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					for _, line := range lines {
						highlightMarkdown(line)
					}
				}
			})
		})
	}
}

func FuzzMarkdownSourceLines(f *testing.F) {
	for _, source := range []string{
		"**first\nsecond**", "- `first\n  second`", "[日本語\né](https://example.org)",
		"| **first | second** |\n| --- | --- |", "\\*escaped\\*", "` \n `", "",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		parsed := parseMarkdownSource(strings.Split(source, "\n"), nil)
		for i := range parsed.lines {
			if rendered := parsed.lines[i].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil); strings.ContainsRune(rendered, '\n') {
				t.Fatalf("source line %d gained a newline: %q", i+1, rendered)
			}
		}
	})
}

func TestMarkdownCodeWordDiffHighlightsChangedText(t *testing.T) {
	defer initAdaptiveStyles(true)
	for _, pair := range []struct{ before, after, oldText, newText string }{
		{"Use `--dry-run` for inspection.", "Use `critty view` for inspection.", "--dry-run", "critty view"},
		{"Use `git status --short`.", "Use `git status --branch`.", "short", "branch"},
		{"表示 `日本語 é old` です。", "表示 `日本語 é new` です。", "old", "new"},
	} {
		patch, err := gitpkg.ParsePatch(strings.NewReader("diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-" + pair.before + "\n+" + pair.after + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		diff := patch.Files[0].Diff
		for _, dark := range []bool{false, true} {
			initAdaptiveStyles(dark)
			for _, tt := range []struct {
				name, source, changedText string
				segments                  []gitpkg.InlineSegment
				style                     lipgloss.Style
			}{
				{"old", pair.before, pair.oldText, diff.DeletedAfter[0][0].Inline, diffDeletedTextBg},
				{"new", pair.after, pair.newText, diff.InlineChanges[1], diffAddedTextBg},
			} {
				t.Run(fmt.Sprintf("%s/%s/dark=%t", tt.changedText, tt.name, dark), func(t *testing.T) {
					parsed := parseMarkdownSource([]string{tt.source}, nil)
					rendered := parsed.lines[0].render(tt.segments, diffCommonTextBg, tt.style, nil)
					runes, styles := renderedCells(rendered)
					text := string(runes)
					start := strings.Index(text, tt.changedText)
					if start < 0 {
						t.Fatalf("changed code missing: %q", text)
					}
					first := len([]rune(text[:start]))
					for i := first; i < first+len([]rune(tt.changedText)); i++ {
						if !sameColor(styles[i].Bg, tt.style.GetBackground()) {
							t.Fatalf("changed code %q at %d has background %v, want %v", runes[i], i, styles[i].Bg, tt.style.GetBackground())
						}
					}
					for _, width := range []int{12, 80} {
						rows := inlineDiffDisplayLines(tt.segments, true, width, diffCommonTextBg, tt.style, nil)
						var highlighted strings.Builder
						for _, row := range rows {
							chars, cells := renderedCells(row)
							for i, r := range chars {
								if r != ' ' && sameColor(cells[i].Bg, tt.style.GetBackground()) {
									highlighted.WriteRune(r)
								}
							}
						}
						if !strings.Contains(highlighted.String(), strings.ReplaceAll(tt.changedText, " ", "")) {
							t.Fatalf("width %d lost changed code: %q", width, highlighted.String())
						}
					}
				})
			}
		}
	}
}
