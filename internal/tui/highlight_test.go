package tui

import (
	"reflect"
	"testing"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func TestCodeHighlighterKeepsInputsIndependent(t *testing.T) {
	for _, tt := range []struct {
		path   string
		inputs []string
	}{
		{"test.go", []string{"package main\n/* unfinished comment", "var name = `日本語`", "", "// comment\n"}},
		{"test.py", []string{"value = \"\"\"unfinished string", "print('日本語')\n", "\t# comment"}},
		{"Makefile", []string{"all:\n\techo hello\n", "\techo goodbye"}},
		{"test.unknown-extension", []string{"plain text\n", "\t日本語", ""}},
	} {
		t.Run(tt.path, func(t *testing.T) {
			highlight := newCodeHighlighter(tt.path)
			for _, input := range tt.inputs {
				want := newCodeHighlighter(tt.path)(input)
				if got := highlight(input); !reflect.DeepEqual(got, want) {
					t.Fatalf("ANSI output for %q depends on previous input:\ngot  %q\nwant %q", input, got, want)
				}
			}
		})
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
