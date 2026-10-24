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
	visible := ansi.Cut(text, v.offset, v.offset+v.width)
	return visible + strings.Repeat(" ", max(0, v.width-ansi.StringWidth(visible)))
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
