package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestMarkdownKeyboard(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, source, want string }{
		{"single", "Press <kbd>X</kbd>.", "Press ⟦X⟧."},
		{"word", "Press <kbd>Enter</kbd>.", "Press ⟦Enter⟧."},
		{"combination", "<kbd>Ctrl</kbd>+<kbd>S</kbd>", "⟦Ctrl⟧+⟦S⟧"},
		{"nested", "<kbd><kbd>Ctrl</kbd>+<kbd>S</kbd></kbd>", "⟦Ctrl⟧+⟦S⟧"},
		{"formatting", "<kbd>**Ctrl**</kbd>", "⟦Ctrl⟧"},
		{"entity", "<kbd>&lt;</kbd>", "⟦<⟧"},
		{"Unicode", "<kbd>界 é</kbd>", "⟦界 é⟧"},
		{"attributes", "<KBD title=\"</kbd>\">X</KBD>", "⟦X⟧"},
		{"escaped label", "<kbd>\\*</kbd>", "⟦*⟧"},
		{"empty", "<kbd></kbd>", ""},
		{"unclosed", "Press <kbd>X", "Press <kbd>X"},
		{"closing only", "X</kbd>", "X</kbd>"},
		{"self closing", "<kbd/>X", "<kbd/>X"},
		{"code", "`<kbd>X</kbd>`", " <kbd>X</kbd> "},
		{"escaped", "\\<kbd>X</kbd>", "\\<kbd>X</kbd>"},
		{"encoded", "&lt;kbd&gt;X&lt;/kbd&gt;", "&lt;kbd&gt;X&lt;/kbd&gt;"},
		{"comment", "<!-- <kbd>X</kbd> -->", "<!-- <kbd>X</kbd> -->"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseMarkdownSource([]string{tt.source}, nil)
			if got := ansi.Strip(parsed.lines[0].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)); got != tt.want {
				t.Fatalf("document = %q, want %q", got, tt.want)
			}
			if got := ansi.Strip(highlightInline(tt.source)); got != tt.want {
				t.Fatalf("inline = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeyboardComments(t *testing.T) {
	app := NewApp("notes.md", AppConfig{})
	app.images.hidden = true
	const source = "Press <kbd>Ctrl</kbd>+<kbd>S</kbd>."
	if got := ansi.Strip(app.renderCommentContent("Reviewer", source, "", 60)); got != "Press ⟦Ctrl⟧+⟦S⟧." {
		t.Fatalf("comment = %q", got)
	}
	app.modal = commentModal
	app.modalTextarea.SetValue(source)
	app.modalTextarea.SetWidth(60)
	app.modalTextarea.SetHeight(3)
	view, _ := app.editorView()
	if !strings.Contains(ansi.Strip(view), source) {
		t.Fatal("editing must retain literal HTML tags")
	}
}

func TestCopyKeyboardSource(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, source, selected, want string }{
		{"whole", "<kbd>Enter</kbd>", "⟦Enter⟧", "<kbd>Enter</kbd>"},
		{"label", "Press <kbd>Enter</kbd>.", "Enter", "<kbd>Enter</kbd>"},
		{"opening only", "<kbd>X</kbd>", "⟦", ""},
		{"closing only", "<kbd>X</kbd>", "⟧", ""},
		{"prefix", "<kbd>Enter</kbd>", "⟦En", "<kbd>En</kbd>"},
		{"middle", "<kbd>Enter</kbd>", "nte", "nte"},
		{"suffix", "<kbd>Enter</kbd>", "ter⟧", "ter"},
		{"entity", "<kbd>&lt;</kbd>", "⟦<⟧", "<kbd>&lt;</kbd>"},
		{"source brackets", "<kbd>⟦X⟧</kbd>", "⟦⟦X⟧⟧", "<kbd>⟦X⟧</kbd>"},
		{"combination", "<kbd>Ctrl</kbd>+<kbd>S</kbd>", "⟦Ctrl⟧+⟦S⟧", "<kbd>Ctrl</kbd>+<kbd>S</kbd>"},
		{"nested", "<kbd><kbd>Ctrl</kbd>+<kbd>S</kbd></kbd>", "⟦Ctrl⟧+⟦S⟧", "<kbd><kbd>Ctrl</kbd>+<kbd>S</kbd></kbd>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseMarkdownSource([]string{tt.source}, nil)
			line := &parsed.lines[0]
			plain := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
			start := strings.Index(plain, tt.selected)
			if start < 0 {
				t.Fatalf("selection %q missing in %q", tt.selected, plain)
			}
			a, b := line.sourceOffset(start, false), line.sourceOffset(start+len(tt.selected), true)
			if got := parsed.copySourceRange(a, b); got != tt.want {
				t.Fatalf("copy = %q, want %q (range %d:%d)", got, tt.want, a, b)
			}
		})
	}
}

func TestKeyboardLinksAndWordDiff(t *testing.T) {
	const uri = "https://example.org"
	const source = "[<kbd>Enter</kbd>](" + uri + ")"
	parsed := parseMarkdownSource([]string{source}, nil)
	for _, links := range [][]linkSpan{nil, {{start: 0, end: len(source), uri: uri}}} {
		inline := []gitpkg.InlineSegment{{Content: "[<kbd>"}, {Content: "Enter", Changed: true}, {Content: "</kbd>](" + uri + ")"}}
		view := parsed.lines[0].render(inline, diffCommonTextBg, diffAddedTextBg, links)
		if ansi.Strip(view) != "⟦Enter⟧" || !strings.Contains(view, ansi.SetHyperlink(uri)) {
			t.Fatalf("keyboard lost decoration, label or link: %q", view)
		}
		if !strings.Contains(view, bgToAnsi(diffAddedTextBg.GetBackground())) {
			t.Fatalf("keyboard lost word diff background: %q", view)
		}
	}
}

func TestKeyboardSearchOffsets(t *testing.T) {
	t.Parallel()
	const source = "Press <kbd>Enter</kbd> then <kbd>X</kbd>."
	parsed := parseMarkdownSource([]string{source}, nil)
	line := &parsed.lines[0]
	plain := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
	for _, label := range []string{"Enter", "X"} {
		from := strings.Index(source, label)
		a, b := line.displayOffset(from), line.displayOffset(from+len(label))
		if got := plain[a:b]; got != label {
			t.Fatalf("search display = %q, want %q", got, label)
		}
	}
}

func TestKeyboardWrappedSelection(t *testing.T) {
	t.Parallel()
	const source = "<kbd>Enter</kbd> " + "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima " + "<kbd>界 é</kbd>"
	for _, table := range []bool{false, true} {
		content := source
		if table {
			content = "| Keys |\n| --- |\n| " + source + " |"
		}
		app := setupPrivateAppWithDoc(t, content)
		app.width, app.height, app.viewMode = 70, 40, true
		app.recalculateLayout()
		app.rebuildContent()
		var first, last contentTextSpan
		start, end := -1, -1
		for _, span := range app.contentLayout.textSpans {
			if at := strings.Index(span.text, "⟦Enter⟧"); at >= 0 {
				first, start = span, at
			}
			if at := strings.Index(span.text, "界 é⟧"); at >= 0 {
				last, end = span, at+len("界 é⟧")
			}
		}
		if start < 0 || end < 0 || first.row == last.row {
			t.Fatalf("table=%t: test did not find wrapped keyboard labels: %#v", table, app.contentLayout.textSpans)
		}
		left, top, _, _ := app.contentBounds()
		app = pressMouse(app, left+first.left+ansi.StringWidth(first.text[:start]), top+first.row)
		app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text[:end]), top+last.row)
		if got := app.selectedText(); got != source {
			t.Fatalf("table=%t: copied = %q, want %q", table, got, source)
		}
	}
}
