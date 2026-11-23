package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Offsets and widths use terminal columns, not bytes or rune counts.
type horizontalViewport struct {
	offset, width, contentWidth int
}

func (v horizontalViewport) maxOffset() int {
	return max(0, v.contentWidth-v.width)
}

func (v horizontalViewport) clamp(offset int) int {
	return max(0, min(offset, v.maxOffset()))
}

func (v horizontalViewport) render(text string) string {
	if v.width <= 0 {
		return ""
	}
	visible := text
	leftPad := 0
	if v.offset > 0 {
		visible = ansi.TruncateLeft(text, v.offset, "")
		// TruncateLeft keeps a whole grapheme when the cut falls inside it.
		// Reserve only its visible cells so later columns stay in place.
		if excess := ansi.StringWidth(visible) - max(0, ansi.StringWidth(text)-v.offset); excess > 0 {
			_, width := ansi.FirstGraphemeCluster(ansi.Strip(visible), ansi.GraphemeWidth)
			leftPad = min(v.width, width-excess)
			visible = ansi.TruncateLeft(visible, width, "")
		}
	}
	visible = ansi.Truncate(visible, v.width-leftPad, "")
	rightPad := max(0, v.width-leftPad-ansi.StringWidth(visible))
	return strings.Repeat(" ", leftPad) + visible + strings.Repeat(" ", rightPad)
}

func (v horizontalViewport) reveal(start, end int) int {
	if start < v.offset || end-start > v.width {
		return v.clamp(start)
	}
	if end > v.offset+v.width {
		return v.clamp(end - v.width)
	}
	return v.offset
}
