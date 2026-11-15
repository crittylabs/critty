package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/v2/internal/git"
	"github.com/crittylabs/critty/v2/internal/icons"
)

func TestMarkdownCallouts(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"GitHub", "> [!NOTE]\n> Example text.", "▎ ✎ Note\n▎ Example text."},
		{"Obsidian title", "> [!tip] **Custom title**\n> Example text.", "▎ * Custom title\n▎ Example text."},
		{"expanded", "> [!INFO]+ More information\n> Example text.", "▎ i More information\n▎ Example text."},
		{"collapsed source", "> [!faq]- Details\n> Example text.", "▎ ? Details\n▎ Example text."},
		{"unknown", "> [!custom-type]\n> Example text.", "▎ ✎ Custom-type\n▎ Example text."},
		{"lazy continuation", "> [!note]\nExample text.", "▎ ✎ Note\n▎ Example text."},
		{"nested", "> [!note]\n> Outer\n>\n> > [!warning] Inner\n> > Body", "▎ ✎ Note\n▎ Outer\n▎ \n▎ ▎ ! Inner\n▎ ▎ Body"},
		{"code example", "```md\n> [!NOTE]\n> Example\n```", "```md\n> [!NOTE]\n> Example\n```"},
		{"indented code", "    > [!NOTE]\n    > Example", "    > [!NOTE]\n    > Example"},
		{"inline example", "Text > [!NOTE]", "Text > [!NOTE]"},
		{"later marker", "> Ordinary quote\n> [!NOTE]", "▎ Ordinary quote\n▎ [!NOTE]"},
		{"escaped marker", "> \\[!NOTE]\n> Example", "▎ \\[!NOTE]\n▎ Example"},
		{"malformed", "> [!bad name]\n> Example", "▎ [!bad name]\n▎ Example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseMarkdownSource(strings.Split(tc.source, "\n"), nil)
			var lines []string
			for i := range parsed.lines {
				line := &parsed.lines[i]
				lines = append(lines, ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)))
			}
			if got := strings.Join(lines, "\n"); got != tc.want {
				t.Fatalf("display = %q, want %q", got, tc.want)
			}
			if got := parsed.copySourceRange(0, len(tc.source)); got != tc.source {
				t.Fatalf("copied = %q, want %q", got, tc.source)
			}
		})
	}
}

func TestCalloutTypes(t *testing.T) {
	for _, tc := range []struct {
		names string
		kind  calloutKind
	}{
		{"note custom-type", calloutNote}, {"abstract summary tldr", calloutAbstract},
		{"info", calloutInfo}, {"todo", calloutTodo}, {"tip hint", calloutTip},
		{"important", calloutImportant}, {"success check done", calloutSuccess},
		{"question help faq", calloutQuestion}, {"warning attention", calloutWarning},
		{"caution", calloutCaution}, {"failure fail missing", calloutFailure},
		{"danger error", calloutDanger}, {"bug", calloutBug}, {"example", calloutExample}, {"quote cite", calloutQuote},
	} {
		for _, name := range strings.Fields(tc.names) {
			t.Run(name, func(t *testing.T) {
				body := "> [!" + strings.ToUpper(name) + "]\n> Body"
				app := AppModel{}
				parsed := app.parsedComment(body, nil).source
				if tokens := parsed.lines[0].tokens; len(tokens) < 2 || tokens[0].callout != tc.kind {
					t.Fatalf("comment callout %q has wrong type: %+v", name, tokens)
				}
			})
		}
	}
}

func TestCalloutColorsAndCoordinates(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { terminalIsDark = previous })
	for _, dark := range []bool{false, true} {
		terminalIsDark = dark
		source := "> [!WARNING]"
		line := &parseMarkdownSource([]string{source, "> Body"}, nil).lines[0]
		inline := []gitpkg.InlineSegment{{Content: "> [!"}, {Content: "WARNING", Changed: true}, {Content: "]"}}
		rendered := line.render(inline, diffCommonTextBg, diffAddedTextBg, nil)
		_, styles := renderedCells(rendered)
		if !sameColor(styles[2].Fg, calloutColor(calloutWarning)) || !sameColor(styles[2].Bg, diffAddedTextBg.GetBackground()) {
			t.Fatalf("dark=%t: callout or word-diff color missing: %q", dark, rendered)
		}
		plain := ansi.Strip(rendered)
		if plain != "▎ ! Warning" || line.sourceOffset(len("▎ "), false) != 2 || line.sourceOffset(len(plain), true) != len(source) {
			t.Fatalf("display/source mapping lost: %q", plain)
		}
	}
}

func TestCalloutPartialPatch(t *testing.T) {
	parsed := parseMarkdownSource([]string{"missing", "> [!NOTE]", "> Body", "missing"}, map[int]bool{2: true, 3: true})
	for _, line := range parsed.lines {
		for _, token := range line.tokens {
			if token.callout != 0 {
				t.Fatal("partial patch invented callout context")
			}
		}
	}
}

func TestCalloutWrappedSelection(t *testing.T) {
	const source = "> [!NOTE]\n> First **bold** " + "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima " + "last."
	app := setupPrivateAppWithDoc(t, source)
	app.width, app.height, app.viewMode = 60, 35, true
	app.recalculateLayout()
	app.rebuildContent()
	spans := app.contentLayout.textSpans
	first, last := spans[0], spans[len(spans)-1]
	if first.row == last.row {
		t.Fatal("test did not wrap")
	}
	left, top, _, _ := app.contentBounds()
	app = pressMouse(app, left+first.left, top+first.row)
	app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
	if got := app.selectedText(); got != source {
		t.Fatalf("copied = %q, want %q", got, source)
	}
}

func TestCalloutCommentColors(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previous) })
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := NewApp("notes.md", AppConfig{Author: "Self"})
		app.images.hidden = true
		view := app.renderCommentContent("Reviewer", "> [!WARNING] Custom title\n> Body", "", 80)
		runes, styles := renderedCells(view)
		plain := string(runes)
		for _, label := range []string{"Custom title", "Body"} {
			start := strings.Index(plain, label)
			if start < 0 {
				t.Fatalf("label missing: %s", label)
			}
			want := calloutColor(calloutWarning)
			if label == "Body" {
				want = app.commentAuthorStyle("Reviewer").GetForeground()
			}
			at := len([]rune(plain[:start]))
			if !sameColor(styles[at].Fg, want) {
				t.Fatalf("dark=%t: %s color = %v, want %v", dark, label, styles[at].Fg, want)
			}
		}
	}
}

func TestCalloutNerdFontIcons(t *testing.T) {
	const body = "> [!NOTE] A title\n> Body"
	for _, nerd := range []bool{false, true} {
		app := NewApp("notes.md", AppConfig{NerdFonts: nerd})
		parsed := app.parsedComment(body, nil).source
		line := &parsed.lines[0]
		view := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
		glyph := "✎ "
		if nerd {
			glyph = icons.Nerd.Glyph(icons.Edit, "")
		}
		if view != "▎ "+glyph+"A title" || ansi.StringWidth(glyph) != 2 {
			t.Fatalf("nerd=%t: icon spacing = %q", nerd, view)
		}
		if got := parsed.copySourceRange(0, len(body)); got != body {
			t.Fatalf("nerd=%t: copied decorative glyph: %q", nerd, got)
		}
	}
}

func TestCalloutTitleTint(t *testing.T) {
	previousDark, previousBackground := terminalIsDark, terminalBackground
	t.Cleanup(func() {
		initAdaptiveStyles(previousDark)
		terminalBackground = previousBackground
	})
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, name := range []string{"note", "success", "danger"} {
			line := &parseMarkdownSource([]string{"> [!" + name + "]"}, nil).lines[0]
			_, cells := renderedCells(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
			red, green, blue, _ := cells[2].Bg.RGBA()
			valid := name == "note" && blue > red || name == "success" && green > red || name == "danger" && red > green
			if !valid {
				t.Fatalf("dark=%t: %s title has unrelated background %v", dark, name, cells[2].Bg)
			}
		}
	}
}
