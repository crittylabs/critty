package tui

import (
	"testing"

	"github.com/yuin/goldmark/v2/text"
)

func TestMarkdownProjection(t *testing.T) {
	for _, tt := range []struct {
		name, source, want string
		segments           []text.Segment
		offsets            []int
		bounds             [][2]int
	}{
		{name: "no segments", source: "unused"},
		{name: "single", source: "prefix abc suffix", want: "abc", segments: []text.Segment{{Start: 7, Stop: 10}}, offsets: []int{7, 8, 9, 10}},
		{name: "containers and byte offsets", source: "> α\n> z", want: "α\nz", segments: []text.Segment{{Start: 2, Stop: 5}, {Start: 7, Stop: 8}}, offsets: []int{2, 3, 4, 7, 8}},
		{name: "empty boundaries", source: "> α\n> z  ", want: "α\nz", segments: []text.Segment{{Start: 0, Stop: 0}, {Start: 2, Stop: 5}, {Start: 6, Stop: 6}, {Start: 7, Stop: 8}, {Start: 10, Stop: 10}}, offsets: []int{2, 3, 4, 7, 10}, bounds: [][2]int{{0, 3}, {0, 3}, {0, 3}, {3, 4}, {4, 4}}},
		{name: "only empty segments", source: "abc", segments: []text.Segment{{Start: 1, Stop: 1}, {Start: 3, Stop: 3}}, offsets: []int{3}, bounds: [][2]int{{0, 0}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			projection := projectMarkdownSegments([]byte(tt.source), tt.segments)
			if projection.text != tt.want {
				t.Fatalf("text=%q, want %q", projection.text, tt.want)
			}
			for offset, want := range tt.offsets {
				if got := projection.sourceOffset(offset); got != want {
					t.Errorf("offset %d maps to %d, want %d", offset, got, want)
				}
			}
			for offset, want := range tt.bounds {
				span := projection.spanAt(offset)
				if got := [2]int{span.local, span.end}; got != want {
					t.Errorf("offset %d has bounds %v, want %v", offset, got, want)
				}
			}
		})
	}
}
