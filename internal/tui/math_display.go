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
	if line.mathTokens != nil {
		return line.mathTokens
	}
	return line.tokens
}

func (m AppModel) markdownPreviewSize(ref markdownImage, img *inlineImage) (int, int) {
	if ref.math == "" {
		return m.markdownImageSize(img)
	}
	width := max(1, m.contentViewport.Width()-gutterWidth-1)
	if m.alternateDiff(m.tab()) {
		width = max(1, (m.contentViewport.Width()-3)/2-gutterWidth-1)
	}
	if ref.mathInline {
		return img.fit(min(44, width), 5, m.images.cell)
	}
	return img.fit(min(44, width), min(19, max(5, m.contentViewport.Height())), m.images.cell)
}

// An odd number of complete cell rows gives inline math a center text row.
// Center the pixels too, so a fraction does not sit half a row below prose.
func centerInlineMath(img image.Image, cell cellSize) image.Image {
	if !cell.known() {
		cell = cellSize{width: 8, height: 16}
	}
	bounds := img.Bounds()
	cols := (bounds.Dx() + cell.width - 1) / cell.width
	rows := (bounds.Dy() + cell.height - 1) / cell.height
	rows |= 1
	canvas := image.NewNRGBA(image.Rect(0, 0, cols*cell.width, rows*cell.height))
	offset := image.Pt((canvas.Bounds().Dx()-bounds.Dx())/2, (canvas.Bounds().Dy()-bounds.Dy())/2)
	draw.Draw(canvas, bounds.Sub(bounds.Min).Add(offset), img, bounds.Min, draw.Src)
	return canvas
}

type mathDisplaySpan struct {
	start, end int
	token      markdownToken
	offset     int
}

type mathDisplayBand struct{ start, height int }

func (t *FileTab) sourceRowGutter(line int, before bool, row int, marker, number string) string {
	if t.markdown != nil {
		source := t.markdown.after
		if before {
			source = t.markdown.before
		}
		if source != nil && line > 0 && line <= len(source.lines) {
			parsed := &source.lines[line-1]
			if parsed.mathTokens != nil {
				for _, band := range parsed.mathBands {
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
func (line *markdownLine) renderMath(inline []gitpkg.InlineSegment, base, changed lipgloss.Style, links []linkSpan) string {
	type box struct {
		rows          []string
		width, offset int
		token         markdownToken
	}
	var band []box
	used := 0
	width := line.mathWidth
	if width < 1 {
		width = 1 << 20
	}
	line.mathMap = nil
	line.mathBands = nil
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
		line.mathBands = append(line.mathBands, mathDisplayBand{displayRow, height})
		displayRow += height
		for row := range height {
			for _, box := range band {
				i := row - (height-len(box.rows))/2
				value := strings.Repeat(" ", box.width)
				if i >= 0 && i < len(box.rows) {
					value = box.rows[i]
					plain := ansi.Strip(value)
					line.mathMap = append(line.mathMap, mathDisplaySpan{offset, offset + len(plain), box.token, box.offset})
					value += strings.Repeat(" ", max(0, box.width-ansi.StringWidth(value)))
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
	for _, token := range line.mathTokens {
		if token.text == "" {
			continue
		}
		if token.style&markdownMath != 0 {
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

func (line *markdownLine) mathSourceOffset(offset int, end bool) int {
	if offset == 0 && !end {
		start := 0
		for _, span := range line.mathMap {
			start = min(start, span.token.start)
		}
		return start
	}
	last := 0
	for _, span := range line.mathMap {
		last = max(last, span.token.end)
		if offset < span.end || end && offset == span.end && span.end > span.start {
			if span.token.style&markdownMath != 0 {
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

func (line *markdownLine) mathSourceRange(start, end int) (int, int, bool) {
	from, to, found := len(line.source), 0, false
	for _, span := range line.mathMap {
		if span.end <= start || span.start >= end {
			continue
		}
		a, b := span.token.start, span.token.end
		if span.token.style&markdownMath == 0 {
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

func mathPlaceholder(img *inlineImage) string {
	rows := strings.Split(img.placeholder(), "\n")
	for i := range rows {
		rows[i] = strings.TrimPrefix(rows[i], "  ")
	}
	return strings.Join(rows, "\n")
}

type mathDisplayEdit struct {
	start, end int
	token      *markdownToken
}

func (t *FileTab) mathLineHidden(line int, before bool) bool {
	if t.markdown == nil {
		return false
	}
	source := t.markdown.after
	if before {
		source = t.markdown.before
	}
	return source != nil && line > 0 && line <= len(source.lines) && source.lines[line-1].mathHidden
}

// Build a display-only token projection.  Parsed tokens and source offsets
// remain unchanged; an image is an atomic selection of its original TeX.
func (m *AppModel) prepareMathDisplay() {
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
			source.lines[i].mathTokens = nil
			source.lines[i].mathHidden = false
			source.lines[i].mathBands = nil
			source.lines[i].mathWidth = max(1, m.contentViewport.Width()-gutterWidth-1)
			if m.alternateDiff(t) {
				source.lines[i].mathWidth = max(1, (m.contentViewport.Width()-3)/2-gutterWidth-1)
			}
		}
		if !m.images.enabled || m.images.hidden || m.isearch.active || t.selecting {
			continue
		}
		var refs []markdownImage
		for _, ref := range t.markdownImages {
			if ref.before == (source == t.markdown.before) {
				refs = append(refs, ref)
			}
		}
		projectMathDisplay(source, refs)
	}
}

func projectMathDisplay(source *markdownSource, refs []markdownImage) {
	edits := make(map[int][]mathDisplayEdit)
	for _, ref := range refs {
		if ref.math == "" || ref.image == nil || !ref.image.ready || ref.mathFirst < 1 || ref.line > len(source.lines) {
			continue
		}
		if width := source.lines[ref.line-1].mathWidth; width > 0 && ref.image.columns > width {
			continue
		}
		start := source.offsets[ref.mathFirst-1] + ref.mathColumn
		end := source.offsets[ref.line-1] + ref.mathStop
		if start < 0 || end > len(source.source) || start >= end || source.source[start:end] != ref.mathSource {
			continue
		}
		for line := ref.mathFirst; line <= ref.line; line++ {
			from, to := 0, len(source.lines[line-1].source)
			if line == ref.mathFirst {
				from = ref.mathColumn
			}
			var token *markdownToken
			if line == ref.line {
				to = ref.mathStop
				token = &markdownToken{start: start - source.offsets[line-1], end: to, text: mathPlaceholder(ref.image), style: markdownMath}
			}
			edits[line-1] = append(edits[line-1], mathDisplayEdit{from, to, token})
		}
	}
	for index, replacements := range edits {
		line := &source.lines[index]
		sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
		line.mathTokens = make([]markdownToken, 0, len(line.tokens))
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
				line.mathTokens = append(line.mathTokens, part)
			}
		}
		pos := 0
		for _, edit := range replacements {
			if edit.start < pos {
				continue
			}
			appendSource(pos, edit.start)
			if edit.token != nil {
				line.mathTokens = append(line.mathTokens, *edit.token)
			}
			pos = edit.end
		}
		appendSource(pos, len(line.source))
		line.mathHidden = strings.TrimSpace(ansi.Strip(line.render(nil, diffCommonTextBg, diffAddedTextBg, nil))) == ""
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
		if ref.math == "" {
			continue
		}
		source, ranges := t.markdown.after, layout.lineRanges
		if ref.before {
			source, ranges = t.markdown.before, layout.oldRanges
		}
		if source == nil || ref.line > len(source.lines) || source.lines[ref.line-1].mathTokens == nil {
			continue
		}
		anchor, ok := ranges[ref.line]
		if !ok {
			continue
		}
		for line := ref.mathFirst; line < ref.line; line++ {
			if !source.lines[line-1].mathHidden {
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
