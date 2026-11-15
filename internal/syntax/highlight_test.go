package syntax

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

func TestCodeHighlighterKeepsInputsIndependent(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		path   string
		inputs []string
	}{
		{"test.mm", []string{"@implementation Service\n", "- (void)run { /* https://example.test */ }\n"}},
		{"test.go", []string{"package main\n/* unfinished comment", "var name = `日本語`", "", "// comment\n"}},
		{"test.py", []string{"value = \"\"\"unfinished string", "print('日本語')\n", "\t# comment"}},
		{"Makefile", []string{"all:\n\techo hello\n", "\techo goodbye"}},
		{"test.unknown-extension", []string{"plain text\n", "\t日本語", ""}},
	} {
		t.Run(tt.path, func(t *testing.T) {
			highlight := NewHighlighter(lexers.Match(tt.path), styles.Fallback)
			for _, input := range tt.inputs {
				want := NewHighlighter(lexers.Match(tt.path), styles.Fallback)(input)
				if got := highlight(input); !reflect.DeepEqual(got, want) {
					t.Fatalf("ANSI output for %q depends on previous input:\ngot  %q\nwant %q", input, got, want)
				}
			}
		})
	}
}

func TestHighlighterObserversAndFallback(t *testing.T) {
	lexer := chroma.MustNewLexer(&chroma.Config{Name: "observer-test"}, func() chroma.Rules {
		return chroma.Rules{
			"root": {{Pattern: `[a-z]+`, Type: chroma.Name}, {Pattern: `\n`, Type: chroma.Text}},
		}
	})
	for _, tt := range []struct {
		name      string
		lexer     chroma.Lexer
		source    string
		want      []string
		wantError bool
	}{
		{"errors", lexer, "word!\n", []string{"word!"}, true},
		{"nil lexer", nil, "plain\n", []string{"plain"}, false},
		{"bare CR", lexers.Get("ini"), "[core]\nvalue = a\rb\n", []string{"[core]", "value = a\rb"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var observed strings.Builder
			sawError, sawEOF := false, false
			got := NewHighlighter(tt.lexer, styles.Fallback, func(token chroma.Token) {
				observed.WriteString(token.Value)
				sawError = sawError || token.Type == chroma.Error
				sawEOF = sawEOF || token == chroma.EOF
			})(tt.source)
			for i := range got {
				got[i] = ansi.Strip(got[i])
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("lines = %q, want %q", got, tt.want)
			}
			if observed.String() != tt.source || sawError != tt.wantError || !sawEOF {
				t.Fatalf("observed %q, error=%v, EOF=%v", observed.String(), sawError, sawEOF)
			}
		})
	}
}
