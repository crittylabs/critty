package git

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func alignmentPatch(t testing.TB, before, after []string) []*gitdiff.File {
	t.Helper()
	var patch strings.Builder
	fmt.Fprintf(&patch, "diff --git a/f.txt b/f.txt\n--- a/f.txt\n+++ b/f.txt\n@@ -1,%d +1,%d @@\n", len(before), len(after))
	for _, line := range before {
		fmt.Fprintf(&patch, "-%s\n", line)
	}
	for _, line := range after {
		fmt.Fprintf(&patch, "+%s\n", line)
	}
	files, _, err := gitdiff.Parse(strings.NewReader(patch.String()))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestReplacementAlignment(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after []string
		pairs         map[int]int
	}{
		{"paragraph and additions", []string{"alpha beta"}, []string{"inserted heading", "alpha changed beta", "", "new paragraph"}, map[int]int{1: 0, 2: 1, 3: 0, 4: 0}},
		{"deletion before replacement", []string{"removed heading", "alpha old beta"}, []string{"alpha new beta"}, map[int]int{1: 2}},
		{"equal counts with shifted lines", []string{"removed heading", "alpha old beta"}, []string{"alpha new beta", "added footer"}, map[int]int{1: 2, 2: 0}},
		{"unrelated", []string{"alpha beta"}, []string{"gamma delta"}, map[int]int{1: 0}},
		{"Unicode", []string{"前置き é 👩‍💻 old 後置き"}, []string{"挿入行", "前置き é 👩‍💻 new 後置き"}, map[int]int{1: 0, 2: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := diffInfo(alignmentPatch(t, tc.before, tc.after))
			if !reflect.DeepEqual(info.PairedLines, tc.pairs) {
				t.Fatalf("pairs=%v, want %v", info.PairedLines, tc.pairs)
			}
			for next, old := range tc.pairs {
				segments := info.InlineChanges[next]
				if old == 0 {
					if len(segments) != 0 {
						t.Fatalf("unpaired line %d has inline segments", next)
					}
					continue
				}
				if len(segments) == 0 {
					t.Fatalf("paired line %d has no word diff", next)
				}
				var text strings.Builder
				for _, segment := range segments {
					text.WriteString(segment.Content)
				}
				if text.String() != tc.after[next-1] {
					t.Fatalf("new source changed: %q", text.String())
				}
				found := false
				for _, lines := range info.DeletedAfter {
					for _, line := range lines {
						if line.OldLineNum != old {
							continue
						}
						text.Reset()
						for _, segment := range line.Inline {
							text.WriteString(segment.Content)
						}
						found = text.String() == tc.before[old-1]
					}
				}
				if !found {
					t.Fatalf("old source %d changed or lost its inline segments", old)
				}
			}
		})
	}
}

func TestReplacementAlignmentLimits(t *testing.T) {
	for _, text := range []string{strings.Repeat("alpha ", 4000), strings.Repeat("x", maxInlineBytes+1)} {
		info := diffInfo(alignmentPatch(t, []string{text + "old"}, []string{"heading", text + "new"}))
		if info.PairedLines[2] != 0 || len(info.InlineChanges[2]) != 0 || len(info.DeletedAfter[0]) != 1 || !info.ChangedLines[2] {
			t.Fatal("expensive matching must retain full-line changes without word highlights")
		}
	}
}

func BenchmarkDiffAlignment(b *testing.B) {
	for _, name := range []string{"Paragraph", "InsertedCode", "Unrelated", "LongLines"} {
		b.Run(name, func(b *testing.B) {
			before, after := []string{"alpha old beta"}, []string{"inserted heading", "alpha new beta", "", "new paragraph"}
			switch name {
			case "InsertedCode":
				before, after = nil, []string{"inserted heading"}
				for i := range 1000 {
					before = append(before, fmt.Sprintf("return value_%d + old; // keep this code", i))
					after = append(after, fmt.Sprintf("return value_%d + new; // keep this code", i))
				}
			case "Unrelated":
				before, after = nil, nil
				for i := range 100 {
					before = append(before, fmt.Sprintf("old_%d alpha beta gamma delta", i))
					after = append(after, fmt.Sprintf("new_%d epsilon zeta eta theta", i))
				}
			case "LongLines":
				before = []string{strings.Repeat("alpha ", 4000) + "old beta"}
				after = []string{"inserted heading", strings.Repeat("alpha ", 4000) + "new beta"}
			}
			files := alignmentPatch(b, before, after)
			b.ReportAllocs()
			for b.Loop() {
				diffInfo(files)
			}
		})
	}
}
