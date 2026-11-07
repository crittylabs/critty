package tui

import (
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/mathrender"
)

func TestInlineMathCenteredOnTextRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		height int
	}{
		{"one row", 10},
		{"fraction", 28},
		{"three rows", 48},
		{"nested fraction", 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			height := tc.height
			cell := cellSize{width: 8, height: 16}
			original := image.NewNRGBA(image.Rect(0, 0, 22, height))
			for y := range height {
				for x := range 22 {
					original.SetNRGBA(x, y, color.NRGBA{A: 255})
				}
			}
			canvas := centerInlineMath(original, cell)
			bounds := canvas.Bounds()
			rows := bounds.Dy() / cell.height
			if rows%2 == 0 || bounds.Dy()%cell.height != 0 || bounds.Dx()%cell.width != 0 {
				t.Fatalf("height %d: canvas does not fill odd cell rows: %v", height, bounds)
			}
			top, bottom := bounds.Dy(), -1
			for y := range bounds.Dy() {
				if _, _, _, a := canvas.At(bounds.Dx()/2, y).RGBA(); a > 0 {
					top, bottom = min(top, y), max(bottom, y)
				}
			}
			if top != bounds.Dy()-bottom-1 {
				t.Fatalf("height %d: unequal padding above and below ink: %d, %d", height, top, bounds.Dy()-bottom-1)
			}
			img := inlineImage{id: 1, width: bounds.Dx(), height: bounds.Dy(), columns: bounds.Dx() / cell.width, rows: rows}
			line := markdownLine{mathTokens: []markdownToken{
				{text: "before "},
				{text: mathPlaceholder(&img), style: markdownMath},
				{text: " after"},
			}}
			got := strings.Split(ansi.Strip(line.renderMath(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)), "\n")
			if len(got) != rows || !strings.Contains(got[rows/2], "before ") || !strings.Contains(got[rows/2], " after") {
				t.Fatalf("height %d: text is not on the image's center row: %q", height, got)
			}
		})
	}
}

func TestMathGuttersGroupTallRows(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		width, rows  int
		gutters      []string
	}{
		{"one row", "Text before ", 100, 1, []string{">    1 "}},
		{"tall first row", "Text before ", 100, 3, []string{"     ⎛ ", ">    1 ", "     ⎝ "}},
		{"five rows", "Text before ", 100, 5, []string{"     ⎛ ", "     ⎜ ", ">    1 ", "     ⎜ ", "     ⎝ "}},
		{"tall continuation", strings.Repeat("t", 19) + " ", 27, 3, []string{">    1 ", "     ⎛ ", "     ↪ ", "     ⎝ "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.prefix + `$\frac{a}{b}$ after.`
			app := setupAppWithDoc(t, source)
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 140, Height: 50})
			app.contentViewport.SetWidth(tc.width)
			app.tab().cursorLine = 1
			app.images.enabled = true
			refs := markdownMathRefs(source)
			refs[0].previewSide = previewSide{attempted: true, image: &inlineImage{id: 1, width: 24, height: 16 * tc.rows, columns: 3, rows: tc.rows, ready: true}}
			app.tab().markdownImages, app.tab().markdownImagesPending = refs, true
			app.rebuildContent()
			content := strings.Split(app.contentViewport.GetContent(), "\n")
			r := app.contentLayout.lineRanges[1]
			var got []string
			for _, row := range content[r.start:r.end] {
				got = append(got, ansi.Truncate(ansi.Strip(row), gutterWidth, ""))
			}
			if !reflect.DeepEqual(got, tc.gutters) {
				t.Fatalf("gutters=%q, want=%q", got, tc.gutters)
			}
			for row := r.start; row < r.end; row++ {
				if target := app.contentLayout.rows[row]; target.line != 1 || target.annotation {
					t.Fatalf("row %d lost source-line target: %+v", row, target)
				}
			}
			app.tab().markdown.before = app.tab().markdown.after
			for _, side := range []string{"", "old"} {
				app.tab().cursorSide = side
				line := &diffLine{ref: lineRef{line: 1, side: side}, text: source, inline: []gitpkg.InlineSegment{{Content: source}}}
				cells, target := app.diffCell(line, tc.width, side == "old", app.diffGutter(app.tab()))
				got = nil
				for _, cell := range cells {
					got = append(got, ansi.Truncate(ansi.Strip(cell.text), gutterWidth, ""))
				}
				if !reflect.DeepEqual(got, tc.gutters) || target.line != 1 || target.side != side {
					t.Fatalf("diff side %q: gutters=%q, target=%+v", side, got, target)
				}
			}
		})
	}
}

func TestMarkdownMathRefs(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{"inline", `A $\frac{1}{2}$ and $x_1$.`, []string{`\frac{1}{2}`, "x_1"}},
		{"block", "$$\n\\frac{1}{2}\n$$", []string{`\frac{1}{2}`}},
		{"multiline", "$$\n\\begin{aligned}\na&=b\\\\\nc&=d\n\\end{aligned}\n$$", []string{"\\begin{aligned}\na&=b\\\\\nc&=d\n\\end{aligned}"}},
		{"inline code and escapes", "`$x$` \\$y$", nil},
		{"code blocks", "```tex\n$$z$$\n```\n\n    $q$\n", []string{"z", "q"}},
		{"suggestion", "```suggestion:-2+3\nResult: $x$.\n```", []string{"x"}},
		{"md", "```md\n`$literal$` and $x$.\n```", []string{"x"}},
		{"markdown", "~~~markdown\n$$\na\\\\\nb\n$$\n~~~", []string{"a\\\\\nb"}},
		{"other language", "```go\nvar label = \"$x$\"\n```", []string{"x"}},
		{"nested fences", "````markdown\n```suggestion\n$x$\n```\n````", []string{"x"}},
		{"currency", "Costs $5 and $10. Empty $$ and spaced $ x $.", nil},
		{"unclosed", "$$\nx + y", nil},
		{"comment", "$$x % $$ ignored\n+y$$", []string{"x % $$ ignored\n+y"}},
		{"escaped dollar", `$$\text{\$5}$$`, []string{`\text{\$5}`}},
		{"html", "<!-- $x$ -->\n\n<script>$y$</script>", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := markdownMathRefs(tc.source)
			var got []string
			for _, ref := range refs {
				got = append(got, ref.math)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestMathCodeBlockOffsetsAndCopy(t *testing.T) {
	quoted := "> ```markdown\n> Before $x$ after.\n> $$\n> a\\\\\n> b\n> $$\n> ```"
	refs := markdownMathRefs(quoted)
	if len(refs) != 2 || refs[0].mathFirst != 2 || refs[0].mathColumn != 9 || refs[0].mathSource != "$x$" || refs[1].mathFirst != 3 || refs[1].line != 6 || refs[1].math != "a\\\\\nb" || refs[1].mathSource != "$$\n> a\\\\\n> b\n> $$" {
		t.Fatalf("code block source projection: %+v", refs)
	}
	source := "```suggestion\nBefore $x_1$ after.\n```"
	app := setupAppWithDoc(t, source)
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.images.enabled = true
	refs = markdownMathRefs(source)
	if len(refs) != 1 {
		t.Fatalf("refs: %+v", refs)
	}
	refs[0].previewSide = previewSide{attempted: true, image: &inlineImage{id: 1, width: 32, height: 16, columns: 4, rows: 1, ready: true}}
	app.tab().markdownImages, app.tab().markdownImagesPending = refs, true
	app.rebuildContent()
	got := ansi.Strip(app.contentViewport.GetContent())
	if strings.Contains(got, "$x_1$") || !strings.Contains(got, "```suggestion") || !strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("fenced math replacement: %q", got)
	}
	spans := app.contentLayout.textSpans
	first, last := -1, -1
	for i, span := range spans {
		if span.ref.line == 1 && first < 0 {
			first = i
		}
		if span.ref.line == 3 {
			last = i
		}
	}
	if first < 0 || last < 0 {
		t.Fatalf("fence spans missing: %+v", spans)
	}
	app.textSelection = contentTextSelection{active: true, tab: app.activeTab, spans: spans, anchor: textPoint{first, 0}, cursor: textPoint{last, len(spans[last].text)}}
	if got := app.selectedText(); got != source {
		t.Fatalf("fenced math copy=%q, want=%q", got, source)
	}
}

func TestMathReplacementsCopyOriginalAndRetainAnchors(t *testing.T) {
	source := "Before $x_1$ after.\n\n$$\na\\\\\nb\n$$\nend"
	app := setupAppWithDoc(t, source)
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.images.enabled = true
	app.images.cell = cellSize{width: 8, height: 16}
	refs := markdownMathRefs(source)
	for i := range refs {
		refs[i].previewSide = previewSide{attempted: true, image: &inlineImage{id: i + 1, width: 48, height: 32, columns: 6, rows: 2, ready: true}}
	}
	app.tab().markdownImages, app.tab().markdownImagesPending = refs, true
	app.rebuildContent()
	got := ansi.Strip(app.contentViewport.GetContent())
	if strings.Contains(got, "$x_1$") || strings.Contains(got, "$$") || !strings.ContainsRune(got, kitty.Placeholder) || !strings.Contains(got, "Before ") || !strings.Contains(got, " after.") {
		t.Fatalf("not replaced in place: %q", got)
	}
	selectLine := func(line int) string {
		first, last := -1, -1
		for i, span := range app.contentLayout.textSpans {
			if span.ref.line == line {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			t.Fatalf("line %d has no spans", line)
		}
		spans := app.contentLayout.textSpans
		app.textSelection = contentTextSelection{active: true, tab: app.activeTab, spans: spans, anchor: textPoint{first, 0}, cursor: textPoint{last, len(spans[last].text)}}
		return app.selectedText()
	}
	if got := selectLine(1); got != "Before $x_1$ after." {
		t.Fatalf("inline copy=%q", got)
	}
	if got := selectLine(refs[1].line); got != "$$\na\\\\\nb\n$$" {
		t.Fatalf("block copy=%q", got)
	}
	for line := refs[1].mathFirst; line <= refs[1].line; line++ {
		if app.contentLayout.lineRanges[line] != app.contentLayout.lineRanges[refs[1].line] {
			t.Fatalf("line %d lost image anchor", line)
		}
	}
	app.textSelection = contentTextSelection{}
	app.contentViewport.SetWidth(24)
	app.rebuildContent()
	if got := selectLine(1); got != "Before $x_1$ after." {
		t.Fatalf("wrapped image copy=%q", got)
	}
	rows := strings.Split(app.contentViewport.GetContent(), "\n")
	column := -1
	imageRows := 0
	r := app.contentLayout.lineRanges[1]
	for _, row := range rows[r.start:r.end] {
		plain := ansi.Strip(row)
		if i := strings.IndexRune(plain, kitty.Placeholder); i >= 0 {
			imageRows++
			left := ansi.StringWidth(plain[:i])
			if column < 0 {
				column = left
			} else if left != column {
				t.Fatalf("image rows shifted during wrapping: %q", plain)
			}
		}
	}
	if imageRows != refs[0].image.rows {
		t.Fatalf("image split during wrapping: rows=%d, want=%d", imageRows, refs[0].image.rows)
	}
	app.textSelection = contentTextSelection{}
	app.images.hidden = true
	app.rebuildContent()
	got = ansi.Strip(app.contentViewport.GetContent())
	if !strings.Contains(got, "$x_1$") || !strings.Contains(got, "$$") || strings.ContainsRune(got, kitty.Placeholder) {
		t.Fatalf("Images off did not restore source: %q", got)
	}
}

func TestCommentMathThemeSwitchReleasesPendingLoads(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	initAdaptiveStyles(true)
	app := setupAppWithDoc(t, "document")
	app.math = mathrender.New("/missing/ratex")
	app.images.enabled = true
	body := "$x$ and $y$"
	app.renderCommentContent("Reviewer", body, "", 40)
	for _, side := range app.commentImages {
		side.attempted = true
	}
	model, _ := app.updateMessage(tea.BackgroundColorMsg{Color: color.White})
	app = model.(AppModel)
	for _, side := range app.commentImages {
		if side.attempted {
			t.Fatal("stale pending load blocks new theme")
		}
	}
	app.renderCommentContent("Reviewer", body, "", 40)
	app.prepareCommentImages()
	loading := 0
	for key, side := range app.commentImages {
		if key.math != "" && !key.dark && side.attempted {
			loading++
		}
	}
	if loading != 2 {
		t.Fatalf("new theme loads=%d, want 2", loading)
	}
}

func TestCommentMathReadingAndEditing(t *testing.T) {
	app := setupAppWithDoc(t, "document")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.math = mathrender.New("/missing/ratex")
	app.images.enabled = true
	body := "Before $x_1$ after."
	app.renderCommentContent("Reviewer", body, "", 40)
	key := commentImageKey{width: 40, math: "x_1", inline: true, dark: terminalIsDark}
	side := app.commentImages[key]
	if side == nil {
		t.Fatal("comment math was not registered")
	}
	side.image = &inlineImage{id: 1, width: 32, height: 16, columns: 4, rows: 1, ready: true}
	got := ansi.Strip(app.renderCommentContent("Reviewer", body, "", 40))
	if strings.Contains(got, "$x_1$") || !strings.ContainsRune(got, kitty.Placeholder) || !strings.HasPrefix(got, "Before ") || !strings.HasSuffix(got, " after.") {
		t.Fatalf("reading comment: %q", got)
	}
	app.openLineComment()
	app.modalTextarea.SetValue(body)
	app.editorImages.active = true
	app.applyEditorImages(body)
	if len(app.editorImages.refs) != 1 {
		t.Fatalf("editor refs: %+v", app.editorImages.refs)
	}
	ref := app.editorImages.refs[0]
	app.commentImages[ref.key].image = side.image
	view, _ := app.editorView()
	if app.modalTextarea.Value() != body || !strings.Contains(ansi.Strip(view), "$x_1$") || !strings.ContainsRune(view, kitty.Placeholder) {
		t.Fatalf("editor should retain source and show a preview: %q", view)
	}
}

func TestMathSourcePreserved(t *testing.T) {
	source := `Text $a_*b* + \frac{x}{y}$ end.`
	parsed := parseMarkdownSource([]string{source}, nil)
	got := ansi.Strip(parsed.lines[0].render(nil, diffCommonTextBg, diffAddedTextBg, nil))
	if got != source {
		t.Fatalf("math source changed: %q", got)
	}
	linked := parseMarkdownSource([]string{`[$x$](https://example.com)`}, nil)
	if got := ansi.Strip(linked.lines[0].render(nil, diffCommonTextBg, diffAddedTextBg, nil)); got != "$x$" {
		t.Fatalf("math link label=%q", got)
	}
	if refs := markdownImageRefs(`$\text{![code](not.png)}$`); len(refs) != 0 {
		t.Fatalf("parsed images inside math: %+v", refs)
	}
	refs := markdownMathRefs("$$\na\\\\\nb\n$$\nend")
	if len(refs) != 1 || refs[0].line != 4 || !strings.Contains(refs[0].math, "\n") {
		t.Fatalf("bad multiline anchor: %+v", refs)
	}
}

func TestMathPreviewKeepsSourceAndImageNavigation(t *testing.T) {
	source := "Math $x_1$.\n\n![image](example.png) $y$\n"
	app := setupAppWithDoc(t, source)
	app.math = mathrender.New("/missing/ratex-render")
	refs := app.findMarkdownImages()().(markdownImagesMsg)
	if len(refs.images) != 3 || refs.images[0].math != "x_1" || refs.images[1].math != "" || refs.images[2].math != "y" {
		t.Fatalf("unexpected references: %+v", refs.images)
	}
	app.tab().markdownImages, app.tab().markdownImagesPending = refs.images, true
	app.images.enabled = true
	app.tab().markdownImages[0].failed = true
	app.rebuildContent()
	if got := ansi.Strip(app.contentViewport.GetContent()); !strings.Contains(got, "$x_1$") || strings.Contains(got, "preview unavailable") {
		t.Fatalf("fallback lost math source: %q", got)
	}
	if got := app.lineImages(imageTarget{line: 3}); len(got) != 1 || !strings.HasSuffix(got[0].path, "example.png") {
		t.Fatalf("math entered file navigation: %+v", got)
	}
}
