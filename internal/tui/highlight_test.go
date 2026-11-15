package tui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pelletier/go-toml/v2"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestObjectiveCIsCode(t *testing.T) {
	if !isCodeLexer(codeLexer("test.mm", "")) {
		t.Fatal("source file was classified as text")
	}
}

func BenchmarkHighlightDeletedLines(b *testing.B) {
	deleted := make([]gitpkg.DeletedLine, 1000)
	for i := range deleted {
		deleted[i] = gitpkg.DeletedLine{OldLineNum: i + 1, Content: "\tfmt.Println(\"deleted line\")"}
	}
	doc := document.FromContent("test.go", []byte("package main\n"))
	b.ReportAllocs()
	for b.Loop() {
		tab := FileTab{path: doc.Path, doc: doc, deletedAfter: map[int][]gitpkg.DeletedLine{0: deleted}}
		tab.ensureHighlightCache()
	}
}

func TestCodeHighlighterKeepsLineCountWithBareCR(t *testing.T) {
	t.Parallel()
	content := "[alias]\n\tfix = sed \"s/\r$//\"\n[core]\n\teditor = vim\n"
	got := newCodeHighlighter(codeLexer("config", content))(content)
	want := []string{"[alias]", "\tfix = sed \"s/\r$//\"", "[core]", "\teditor = vim"}
	if len(got) != len(want) {
		t.Fatalf("line count %d, want %d: %q", len(got), len(want), got)
	}
	for i, line := range got {
		if plain := ansi.Strip(line); plain != want[i] {
			t.Errorf("line %d: %q, want %q", i+1, plain, want[i])
		}
	}
}

func TestLexerErrorsUsePlainTextWithoutBackground(t *testing.T) {
	const source = "[tasks.sample]\nrun = '''\nprintf 'sample: %s\\n' \"$value\"\n'''\n"
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(source), &parsed); err != nil {
		t.Fatal(err)
	}
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		lexer := codeLexer("sample.toml", source)
		iterator, err := lexer.Tokenise(nil, source)
		if err != nil {
			t.Fatal(err)
		}
		errors := ""
		for token := iterator(); token != chroma.EOF; token = iterator() {
			if token.Type == chroma.Error {
				errors += token.Value
			}
		}
		lines := newCodeHighlighter(lexer)(source)
		if got := strings.Join(lines, "\n") + "\n"; ansi.Strip(got) != source {
			t.Fatalf("highlighting changed source: %q", got)
		}
		_, plain := renderedCells(newCodeHighlighter(nil)("x")[0])
		runes, styles := renderedCells(lines[2])
		for i, r := range runes {
			if r == ':' || r == '%' {
				if strings.ContainsRune(errors, r) && !sameColor(styles[i].Fg, plain[0].Fg) {
					t.Errorf("dark=%t: lexer error %c does not use plain text color", dark, r)
				}
				if styles[i].Bg != nil {
					t.Errorf("dark=%t: lexer error %c has background %v", dark, r, styles[i].Bg)
				}
			}
		}
	}
}

func TestPatchSyntaxColors(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	const source = "--- a.txt\n+++ a.txt\n@@ -1,2 +1,2 @@\n context\n-old\n+new\n"
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		lines := newCodeHighlighter(codeLexer("sample.patch", source))(source)
		_, context := renderedCells(lines[3])
		for _, n := range []int{2, 4, 5} {
			_, cells := renderedCells(lines[n])
			if len(cells) == 0 || sameColor(cells[0].Fg, context[0].Fg) {
				t.Errorf("dark=%v: patch line %q has no distinct syntax color", dark, ansi.Strip(lines[n]))
			}
		}
	}
}
