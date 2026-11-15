package tui

import (
	"strings"
	"testing"
)

func TestSourceLineCursorPositions(t *testing.T) {
	for _, source := range []string{"", "\n\n", "a\r\nb\rc\n", "α e\u0301\n界\nlast"} {
		t.Run(source, func(t *testing.T) {
			lines := sourceLineCursor{text: source}
			var offsets []int
			for offset := 0; offset <= len(source); offset++ {
				offsets = append(offsets, offset, offset)
			}
			// Reverse order also covers callers that visit nested spans.
			for offset := len(source); offset >= 0; offset-- {
				offsets = append(offsets, offset)
			}
			if len(source) > 1 {
				offsets = append(offsets, 1, len(source))
			}
			for _, offset := range offsets {
				line, column := lines.position(offset)
				wantLine := strings.Count(source[:offset], "\n") + 1
				wantColumn := offset - strings.LastIndexByte(source[:offset], '\n') - 1
				if line != wantLine || column != wantColumn {
					t.Fatalf("offset %d: (%d, %d), want (%d, %d)", offset, line, column, wantLine, wantColumn)
				}
			}
		})
	}
}
