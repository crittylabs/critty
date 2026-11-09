package tui

import (
	"image"
	"image/draw"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func (line *markdownLine) displayTokens() []markdownToken {
	if line.previewTokens != nil {
		return line.previewTokens
	}
	return line.tokens
}

func (m AppModel) markdownPreviewSize(ref markdownImage, img *inlineImage) (int, int) {
	if ref.renderBody == "" {
		return m.markdownImageSize(img)
	}
	width := max(1, m.contentViewport.Width()-gutterWidth-1)
	if m.alternateDiff(m.tab()) {
		width = max(1, (m.contentViewport.Width()-3)/2-gutterWidth-1)
	}
	if ref.renderKind == renderedMermaid {
		return img.fit(width, max(5, m.contentViewport.Height()-2), m.images.cell)
	}
	if ref.renderInline {
		return img.mathFit(width, 5, m.images.cell)
	}
	if m.tab().doc != nil && !ref.before {
		cols, rows := img.mathNativeCells(m.images.cell)
		return min(cols, maxPlaceholderCells), min(rows, maxPlaceholderCells)
	}
	return img.mathFit(width, min(19, max(5, m.contentViewport.Height())), m.images.cell)
}

// A 32-pixel math em occupies two thirds of a text line.
const mathLineHeight = 48

func mathCell(cell cellSize) cellSize {
	if !cell.known() {
		return cellSize{width: 24, height: mathLineHeight}
	}
	return cell
}

// Keep the terminal aspect ratio exact.  Rounding a reference cell's width
// makes long formulas and their placement rectangles diverge, adding margins.
func (img inlineImage) mathGeometry(cell cellSize) (inlineImage, cellSize) {
	cell = mathCell(cell)
	img.width *= cell.height
	img.height *= cell.height
	return img, cellSize{width: mathLineHeight * cell.width, height: mathLineHeight * cell.height}
}

func (img inlineImage) mathNativeCells(cell cellSize) (int, int) {
	img, grid := img.mathGeometry(cell)
	return img.nativeCells(grid)
}

func (img inlineImage) mathFit(width, height int, cell cellSize) (int, int) {
	img, grid := img.mathGeometry(cell)
	cols, rows := img.nativeCells(grid)
	if cols <= width && rows <= height {
		return cols, rows
	}
	return img.fit(width, height, grid)
}

// An odd number of complete cell rows gives inline math a center text row.
// Center the pixels too, so a fraction does not sit half a row below prose.
func centerMath(img image.Image, cell cellSize, inline bool) image.Image {
	bounds := img.Bounds()
	cell = mathCell(cell)
	geometry := inlineImage{width: bounds.Dx(), height: bounds.Dy()}
	cols, rows := geometry.mathNativeCells(cell)
	if inline {
		rows |= 1
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, cols*mathLineHeight*cell.width/cell.height, rows*mathLineHeight))
	offset := image.Pt((canvas.Bounds().Dx()-bounds.Dx())/2, (canvas.Bounds().Dy()-bounds.Dy())/2)
	draw.Draw(canvas, bounds.Sub(bounds.Min).Add(offset), img, bounds.Min, draw.Src)
	return canvas
}

type previewDisplaySpan struct {
	start, end int
	token      markdownToken
	offset     int
}

type previewDisplayBand struct{ start, height int }

func (t *FileTab) sourceRowGutter(line int, before bool, row int, marker, number string) string {
	if t.markdown != nil {
		source := t.markdown.after
		if before {
			source = t.markdown.before
		}
		if source != nil && line > 0 && line <= len(source.lines) {
			parsed := &source.lines[line-1]
			if parsed.previewTokens != nil {
				for _, band := range parsed.previewBands {
					if row < band.start || row >= band.start+band.height {
						continue
					}
					position := row - band.start
					if position == band.height/2 {
						row = band.start
						break
					}
					bracket := "⎜"
					if position == 0 {
						bracket = "⎛"
					} else if position == band.height-1 {
						bracket = "⎝"
					}
					return " " + lineNumStyle.Render(bracket) + " "
				}
			}
		}
	}
	if row == 0 {
		return marker + number + " "
	}
	return " " + continuationGutter + " "
}

// Compose text and image boxes around the same center row.  Each image keeps
// its rows and columns contiguous instead of shrinking tall formulas to one
// terminal cell or letting surrounding prose shift the image's later rows.
func (line *markdownLine) renderPreview(inline []gitpkg.InlineSegment, base, changed lipgloss.Style, links []linkSpan) string {
	type box struct {
		rows          []string
		width, offset int
		token         markdownToken
	}
	var band []box
	used := 0
	width := line.previewWidth
	if width < 1 {
		width = 1 << 20
	}
	line.previewMap = nil
	line.previewBands = nil
	displayRow := 0
	var output strings.Builder
	offset := 0
	flush := func() {
		if len(band) == 0 {
			return
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
			offset++
		}
		height := 1
		for _, box := range band {
			height = max(height, len(box.rows))
		}
		height |= 1
		line.previewBands = append(line.previewBands, previewDisplayBand{displayRow, height})
		displayRow += height
		for row := range height {
			for _, box := range band {
				i := row - (height-len(box.rows))/2
				var value string
				if i >= 0 && i < len(box.rows) {
					value = box.rows[i]
					plain := ansi.Strip(value)
					line.previewMap = append(line.previewMap, previewDisplaySpan{offset, offset + len(plain), box.token, box.offset})
					value += strings.Repeat(" ", max(0, box.width-ansi.StringWidth(value)))
				} else {
					value = strings.Repeat(" ", box.width)
				}
				output.WriteString(value)
				offset += len(ansi.Strip(value))
			}
			if row+1 < height {
				output.WriteByte('\n')
				offset++
			}
		}
		band = nil
		used = 0
	}
	for _, token := range line.previewTokens {
		if token.text == "" {
			continue
		}
		if token.style&markdownRendered != 0 {
			size := lipgloss.Width(token.text)
			if used+size > width && used > 0 {
				flush()
			}
			band = append(band, box{rows: strings.Split(token.text, "\n"), width: size, token: token})
			used += size
			continue
		}
		part := markdownLine{source: line.source, literal: true, tokens: []markdownToken{token}}
		content := part.render(inline, base, changed, links)
		consumed := 0
		for ansi.StringWidth(content) > 0 {
			if used >= width {
				flush()
			}
			fragment := ansi.Truncate(content, width-used, "")
			size := ansi.StringWidth(fragment)
			if size == 0 {
				if used > 0 {
					flush()
					continue
				}
				fragment = ansi.Truncate(content, 2, "")
				size = ansi.StringWidth(fragment)
				if size == 0 {
					break
				}
			}
			band = append(band, box{rows: []string{fragment}, width: size, offset: consumed, token: token})
			used += size
			consumed += len(ansi.Strip(fragment))
			content = ansi.Cut(content, size, ansi.StringWidth(content))
			if ansi.StringWidth(content) > 0 {
				flush()
			}
		}
	}
	flush()
	return output.String()
}

func (line *markdownLine) previewSourceOffset(offset int, end bool) int {
	if offset == 0 && !end {
		start := 0
		for _, span := range line.previewMap {
			start = min(start, span.token.start)
		}
		return start
	}
	last := 0
	for _, span := range line.previewMap {
		last = max(last, span.token.end)
		if offset < span.end || end && offset == span.end && span.end > span.start {
			if span.token.style&markdownRendered != 0 {
				if end {
					return span.token.end
				}
				return span.token.start
			}
			stop := span.token.sourceEnd
			if stop == 0 {
				stop = span.token.end
			}
			return span.token.start + rawTextOffset(line.source[span.token.start:stop], span.token.text, span.offset+max(0, offset-span.start), end)
		}
	}
	return last
}

func (line *markdownLine) previewSourceRange(start, end int) (int, int, bool) {
	from, to, found := len(line.source), 0, false
	for _, span := range line.previewMap {
		if span.end <= start || span.start >= end {
			continue
		}
		a, b := span.token.start, span.token.end
		if span.token.style&markdownRendered == 0 {
			stop := span.token.sourceEnd
			if stop == 0 {
				stop = b
			}
			raw := line.source[a:stop]
			a += rawTextOffset(raw, span.token.text, span.offset+max(0, start-span.start), false)
			b = span.token.start + rawTextOffset(raw, span.token.text, span.offset+min(span.end-span.start, end-span.start), true)
		}
		from, to, found = min(from, a), max(to, b), true
	}
	return from, to, found
}

func previewPlaceholder(img *inlineImage) string {
	rows := strings.Split(img.placeholder(), "\n")
	for i := range rows {
		rows[i] = strings.TrimPrefix(rows[i], "  ")
	}
	return strings.Join(rows, "\n")
}

type previewDisplayEdit struct {
	start, end int
	token      *markdownToken
}

func (t *FileTab) previewLineHidden(line int, before bool) bool {
	if t.markdown == nil {
		return false
	}
	source := t.markdown.after
	if before {
		source = t.markdown.before
	}
	return source != nil && line > 0 && line <= len(source.lines) && source.lines[line-1].previewHidden
}

// Build a display-only token projection.  Parsed tokens and source offsets
// remain unchanged; an image is an atomic selection of its original source.
func (m *AppModel) prepareRenderedDisplay() {
	t := m.tab()
	t.ensureMarkdownCache()
	if t.markdown == nil {
		return
	}
	for _, source := range []*markdownSource{t.markdown.after, t.markdown.before} {
		if source == nil {
			continue
		}
		for i := range source.lines {
			source.lines[i].previewTokens = nil
			source.lines[i].previewHidden = false
			source.lines[i].previewBands = nil
			source.lines[i].previewScroll = nil
			source.lines[i].previewWidth = max(1, m.contentViewport.Width()-gutterWidth-1)
			if m.alternateDiff(t) {
				source.lines[i].previewWidth = max(1, (m.contentViewport.Width()-3)/2-gutterWidth-1)
			}
		}
		if !m.images.enabled || m.images.hidden || m.isearch.active || t.selecting {
			continue
		}
		var refs []markdownImage
		for _, ref := range t.markdownImages {
			if ref.before == (source == t.markdown.before) {
				ref.renderOffset = t.horizontalOffsets[ref.renderFirst]
				refs = append(refs, ref)
			}
		}
		projectRenderedDisplay(source, refs)
	}
}

func projectRenderedDisplay(source *markdownSource, refs []markdownImage) {
	edits := make(map[int][]previewDisplayEdit)
	for _, ref := range refs {
		if ref.renderBody == "" || ref.image == nil || !ref.image.ready || ref.renderFirst < 1 || ref.line > len(source.lines) {
			continue
		}
		blockMath := !ref.renderInline && ref.renderKind != renderedMermaid
		if width := source.lines[ref.line-1].previewWidth; width > 0 && ref.image.columns > width && !blockMath {
			continue
		}
		start := source.offsets[ref.renderFirst-1] + ref.renderColumn
		end := source.offsets[ref.line-1] + ref.renderStop
		if start < 0 || end > len(source.source) || start >= end || source.source[start:end] != ref.renderSource {
			continue
		}
		for line := ref.renderFirst; line <= ref.line; line++ {
			from, to := 0, len(source.lines[line-1].source)
			if line == ref.renderFirst {
				from = ref.renderColumn
			}
			var token *markdownToken
			if line == ref.line {
				to = ref.renderStop
				preview := previewPlaceholder(ref.image)
				width := source.lines[line-1].previewWidth
				if blockMath && width > 2 && ref.image.columns > width {
					viewport := &horizontalBlockViewport{startLine: ref.renderFirst, endLine: ref.line, left: gutterWidth, compactEdges: true,
						window: horizontalViewport{width: width, contentWidth: ref.image.columns}}
					viewport.window.offset = max(0, min(ref.renderOffset, ref.image.columns-width+1))
					if viewport.window.offset > 0 {
						viewport.left++
						viewport.window.width--
					}
					if viewport.window.offset+viewport.window.width < ref.image.columns {
						viewport.window.width--
					}
					rows := strings.Split(preview, "\n")
					for i := range rows {
						rows[i] = viewport.render(rows[i])
					}
					preview = strings.Join(rows, "\n")
					source.lines[line-1].previewScroll = viewport
				}
				token = &markdownToken{start: start - source.offsets[line-1], end: to, text: preview, style: markdownRendered}
			}
			edits[line-1] = append(edits[line-1], previewDisplayEdit{from, to, token})
		}
	}
	for index, replacements := range edits {
		line := &source.lines[index]
		sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
		line.previewTokens = make([]markdownToken, 0, len(line.tokens))
		appendSource := func(from, to int) {
			for _, token := range line.tokens {
				a, b := max(from, token.start), min(to, token.end)
				if a >= b {
					continue
				}
				part := token
				if len(token.text) == token.end-token.start {
					part.text = token.text[a-token.start : b-token.start]
				}
				part.start, part.end = a, b
				line.previewTokens = append(line.previewTokens, part)
			}
		}
		pos := 0
		for _, edit := range replacements {
			if edit.start < pos {
				continue
			}
			appendSource(pos, edit.start)
			if edit.token != nil {
				line.previewTokens = append(line.previewTokens, *edit.token)
			}
			pos = edit.end
		}
		appendSource(pos, len(line.source))
		line.previewHidden = strings.TrimSpace(ansi.Strip(line.render(nil, diffCommonTextBg, diffAddedTextBg, nil))) == ""
	}
}

// Hidden source lines still navigate to the replacement image.  Keep any
// annotations already laid out for a hidden line in its reachable range.
func (m AppModel) mapMathRanges(layout *renderedContentLayout) {
	t := m.tab()
	if t.markdown == nil {
		return
	}
	for _, ref := range t.markdownImages {
		if ref.renderBody == "" {
			continue
		}
		source, ranges := t.markdown.after, layout.lineRanges
		if ref.before {
			source, ranges = t.markdown.before, layout.oldRanges
		}
		if source == nil || ref.line > len(source.lines) || source.lines[ref.line-1].previewTokens == nil {
			continue
		}
		anchor, ok := ranges[ref.line]
		if !ok {
			continue
		}
		for line := ref.renderFirst; line < ref.line; line++ {
			if !source.lines[line-1].previewHidden {
				continue
			}
			r := anchor
			if old, ok := ranges[line]; ok {
				r.start = min(old.start, r.start)
				r.end = max(old.end, r.end)
			}
			ranges[line] = r
		}
	}
}
