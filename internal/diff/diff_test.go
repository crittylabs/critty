package diff

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestComputeLineDiff(t *testing.T) {
	for _, tt := range []struct {
		name, old, new string
		want           []DiffEntry
	}{
		{"mixed changes", "a\nb\nc", "a\nx\nc\nd", []DiffEntry{
			{Type: "unchanged", OldLine: 1, NewLine: 1, Text: "a"},
			{Type: "removed", OldLine: 2, Text: "b"},
			{Type: "added", NewLine: 2, Text: "x"},
			{Type: "unchanged", OldLine: 3, NewLine: 3, Text: "c"},
			{Type: "added", NewLine: 4, Text: "d"},
		}},
		{"both empty", "", "", nil},
		{"all added", "", "a\nb", []DiffEntry{
			{Type: "added", NewLine: 1, Text: "a"},
			{Type: "added", NewLine: 2, Text: "b"},
		}},
		{"all removed", "a\nb", "", []DiffEntry{
			{Type: "removed", OldLine: 1, Text: "a"},
			{Type: "removed", OldLine: 2, Text: "b"},
		}},
		{"whitespace is significant", "x\n  y", "x\n    y", []DiffEntry{
			{Type: "unchanged", OldLine: 1, NewLine: 1, Text: "x"},
			{Type: "removed", OldLine: 2, Text: "  y"},
			{Type: "added", NewLine: 2, Text: "    y"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeLineDiff(tt.old, tt.new); len(got)+len(tt.want) > 0 && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ComputeLineDiff = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMapOldLineToNew(t *testing.T) {
	for _, tt := range []struct {
		name, old, new string
		want           map[int]int
	}{
		{"unchanged", "a\nb\nc", "a\nb\nc", map[int]int{1: 1, 2: 2, 3: 3}},
		{"insertion shifts later lines", "a\nb\nc", "a\nx\nb\nc", map[int]int{1: 1, 2: 3, 3: 4}},
		{"removed line maps to the next surviving line", "a\nb\nc", "a\nc", map[int]int{1: 1, 2: 2, 3: 2}},
		{"replacements map to their successors", "a\nb\nc\nd\ne", "a\nx\nc\ny\ne", map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5}},
		{"everything removed", "a\nb\nc", "", map[int]int{1: 0, 2: 0, 3: 0}},
		{"everything added", "", "a\nb\nc", map[int]int{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapOldLineToNew(ComputeLineDiff(tt.old, tt.new)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapOldLineToNew = %v, want %v", got, tt.want)
			}
		})
	}
}

// generateCodeLines builds n lines of code-like content (function signatures,
// assignments, comments) to produce realistic benchmark inputs.
func generateCodeLines(n int, seed string) []string {
	lines := make([]string, n)
	for i := range lines {
		switch i % 5 {
		case 0:
			lines[i] = fmt.Sprintf("func %s_%d() {", seed, i)
		case 1:
			lines[i] = fmt.Sprintf("\tx := %d // initialize", i)
		case 2:
			lines[i] = fmt.Sprintf("\tfmt.Println(%q, x)", seed)
		case 3:
			lines[i] = fmt.Sprintf("\treturn // %s line %d", seed, i)
		default:
			lines[i] = "}"
		}
	}
	return lines
}

// mutateLines returns a copy of lines with ~20% changed: a mix of
// modifications, insertions, and deletions spread evenly across the input.
func mutateLines(lines []string) []string {
	out := make([]string, 0, len(lines)+len(lines)/10)
	for i, line := range lines {
		switch i % 15 {
		case 3: // ~7% deleted
			continue
		case 7: // ~7% modified
			out = append(out, line+" // modified")
		case 11: // ~7% inserted (original kept + new line)
			out = append(out, line)
			out = append(out, fmt.Sprintf("\t// inserted after line %d", i))
		default:
			out = append(out, line)
		}
	}
	return out
}

func BenchmarkComputeLineDiff(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		oldLines := generateCodeLines(n, "old")
		oldContent := strings.Join(oldLines, "\n")
		newContent := strings.Join(mutateLines(oldLines), "\n")

		b.Run(fmt.Sprintf("%d_lines", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ComputeLineDiff(oldContent, newContent)
			}
		})
	}
}
