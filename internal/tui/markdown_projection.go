package tui

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark/v2/text"
)

// markdownProjection joins parser segments while retaining their source byte
// positions. Container prefixes remain outside the joined text. The borrowed
// segments must remain unchanged while the projection is in use.
type markdownProjection struct {
	text     string
	segments []text.Segment
	starts   []int
}

type markdownProjectionSpan struct {
	local, end, source int
}

func projectMarkdownSegments(data []byte, segments []text.Segment) markdownProjection {
	size := 0
	for _, segment := range segments {
		size += segment.Stop - segment.Start
	}
	var body strings.Builder
	body.Grow(size)
	var starts []int
	if len(segments) > 1 {
		starts = make([]int, len(segments))
	}
	for i, segment := range segments {
		if starts != nil {
			starts[i] = body.Len()
		}
		body.Write(data[segment.Start:segment.Stop])
	}
	return markdownProjection{text: body.String(), segments: segments, starts: starts}
}

// spanAt requires at least one segment. At a shared boundary it chooses the
// rightmost segment, including empty segments; EOF uses the last segment.
func (p *markdownProjection) spanAt(offset int) markdownProjectionSpan {
	i, start := 0, 0
	if len(p.starts) > 0 {
		i = max(0, sort.Search(len(p.starts), func(i int) bool { return p.starts[i] > offset })-1)
		start = p.starts[i]
	}
	segment := p.segments[i]
	return markdownProjectionSpan{local: start, end: start + segment.Stop - segment.Start, source: segment.Start}
}

func (p *markdownProjection) sourceOffset(offset int) int {
	span := p.spanAt(offset)
	return span.source + offset - span.local
}
