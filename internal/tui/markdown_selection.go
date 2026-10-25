package tui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

type markdownMarkup struct {
	start, contentStart, contentEnd, end int
	link                                 bool
	autoURI                              string
	emphasis                             bool
}

var markdownSourceRanges = parser.NewContextKey()

// Keep the parser's consumed source range, including link destinations and
// titles. This avoids parsing Markdown punctuation a second time for copying.
type sourceInlineParser struct{ parser.InlineParser }

func (p sourceInlineParser) Parse(parent ast.Node, reader text.Reader, context parser.Context) ast.Node {
	_, before := reader.Position()
	n := p.InlineParser.Parse(parent, reader, context)
	ranges, ok := context.Get(markdownSourceRanges).(map[ast.Node]markdownMarkup)
	if !ok || n == nil {
		return n
	}
	_, after := reader.Position()
	switch node := n.(type) {
	case *ast.CodeSpan:
		indices := node.Value.Indices()
		if len(indices) > 0 {
			a, b := indices[0].Start, indices[len(indices)-1].Stop
			source := reader.Source()
			if a < b && source[a] == ' ' && source[b-1] == ' ' && strings.Trim(string(source[a:b]), " \n") != "" {
				a++
				b--
			}
			ranges[n] = markdownMarkup{start: before.Start, contentStart: a, contentEnd: b, end: after.Start}
		}
	case *ast.Link, *ast.Image:
		start := n.Pos()
		labelStart := start + 1
		if _, image := n.(*ast.Image); image {
			labelStart++
		}
		labelEnd := before.Start
		if start == before.Start {
			// The parser's fast path accepts only a single plain label.
			if child, ok := n.FirstChild().(*ast.Text); ok {
				labelEnd = child.Value.Index().Stop
			}
		}
		autoURI := ""
		if !markdownHasLabel(n, reader.Source()) {
			var destination text.SingleLineValue
			switch node := n.(type) {
			case *ast.Link:
				destination = node.Destination
			case *ast.Image:
				destination = node.Destination
			}
			i := destination.Index()
			if i.Start >= 0 && i.Stop > i.Start {
				labelStart, labelEnd = i.Start, i.Stop
				autoURI = destination.Str(reader.Source())
			}
		}
		ranges[n] = markdownMarkup{start: start, contentStart: labelStart, contentEnd: labelEnd, end: after.Start, link: true, autoURI: autoURI}
	}
	return n
}

func collectMarkdownMarkup(n ast.Node, source []byte, ranges map[ast.Node]markdownMarkup) (int, int) {
	first, last := len(source), 0
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		a, b := collectMarkdownMarkup(child, source, ranges)
		first, last = min(first, a), max(last, b)
	}
	if r, ok := ranges[n]; ok {
		return r.start, r.end
	}
	switch node := n.(type) {
	case *ast.Text:
		i := node.Value.Index()
		return i.Start, i.Stop
	case *ast.AutoLink:
		i := node.Text.Index()
		label := node.Label.Index()
		ranges[n] = markdownMarkup{start: i.Start, contentStart: label.Start, contentEnd: label.Stop, end: i.Stop, link: true, autoURI: node.Destination.Str(source)}
		return i.Start, i.Stop
	case *ast.RawHTML:
		i := node.Value.Indices()
		if len(i) > 0 {
			return i[0].Start, i[len(i)-1].Stop
		}
	case *ast.Strong, *ast.Emphasis:
		size := 1
		if _, strong := n.(*ast.Strong); strong {
			size = 2
		}
		if first >= size && last+size <= len(source) {
			open, close := string(source[first-size:first]), string(source[last:last+size])
			if open == close && (open == strings.Repeat("*", size) || open == strings.Repeat("_", size)) {
				r := markdownMarkup{start: first - size, contentStart: first, contentEnd: last, end: last + size, emphasis: true}
				ranges[n] = r
				return r.start, r.end
			}
		}
	}
	return first, last
}

// Map displayed bytes back to raw bytes, retaining tabs, escapes and entities.
// Goldmark's decoder determines which source units represent one visible unit.
func rawTextOffset(raw, rendered string, offset int, end bool) int {
	decoder := text.NewDecoder()
	decode := string(decoder.Decode([]byte(raw))) == rendered
	pos := 0
	for i := 0; i < len(raw); {
		_, size := utf8.DecodeRuneInString(raw[i:])
		unit := raw[i : i+size]
		if decode && raw[i] == '\\' && i+1 < len(raw) {
			candidate := raw[i : i+2]
			if decoded := string(decoder.Decode([]byte(candidate))); decoded != candidate {
				unit, size = decoded, 2
			}
		} else if decode && raw[i] == '&' {
			stop := i + 1
			for stop < len(raw) && (raw[stop] == '#' || raw[stop] >= '0' && raw[stop] <= '9' || raw[stop] >= 'a' && raw[stop] <= 'z' || raw[stop] >= 'A' && raw[stop] <= 'Z') {
				stop++
			}
			if stop < len(raw) && raw[stop] == ';' {
				candidate := raw[i : stop+1]
				if decoded := string(decoder.Decode([]byte(candidate))); decoded != candidate {
					unit, size = decoded, len(candidate)
				}
			}
		}
		width := len(displaySourceText(unit))
		if offset <= pos || offset < pos+width && !end {
			return i
		}
		if offset < pos+width {
			return i + size
		}
		pos += width
		i += size
	}
	return len(raw)
}

func (line *markdownLine) sourceOffset(offset int, end bool) int {
	if offset == 0 && !end {
		return 0
	}
	pos := 0
	for i, token := range line.tokens {
		value := line.displayTokenText(i)
		stop := pos + len(value)
		if offset < stop || end && offset == stop && stop > pos {
			sourceEnd := token.sourceEnd
			if sourceEnd == 0 {
				sourceEnd = token.end
			}
			return token.start + rawTextOffset(line.source[token.start:sourceEnd], token.text, offset-pos, end)
		}
		pos = stop
	}
	return len(line.source)
}

func (line *markdownLine) displayTokenText(i int) string {
	token := line.tokens[i]
	value := displaySourceText(token.text)
	if !line.literal && i == 0 && token.style == 0 {
		var rest strings.Builder
		for _, remaining := range line.tokens[1:] {
			rest.WriteString(displaySourceText(remaining.text))
		}
		actual := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
		if suffix := rest.String(); strings.HasSuffix(actual, suffix) {
			value = actual[:len(actual)-len(suffix)]
		}
	}
	return value
}

func (m *markdownSource) copyRange(startLine, startOffset, endLine, endOffset int) string {
	a := m.offsets[startLine-1] + m.lines[startLine-1].sourceOffset(startOffset, false)
	b := m.offsets[endLine-1] + m.lines[endLine-1].sourceOffset(endOffset, true)
	return m.copySourceRange(a, b)
}

func (m *markdownSource) copySourceRange(a, b int) string {
	if a >= b {
		return ""
	}
	closingFence := ""
	for _, f := range m.fences {
		if a <= f.start && b >= f.markerEnd && (b < f.end || !f.closed) {
			if f.closed && b > f.closingStart {
				b = f.end
			} else {
				closingFence = f.closing
			}
		}
	}
	finish := func(value string) string {
		if closingFence != "" {
			return strings.TrimSuffix(value, "\n") + "\n" + closingFence
		}
		return value
	}
	markup := append([]markdownMarkup(nil), m.markup...)
	sort.Slice(markup, func(i, j int) bool {
		if markup[i].start == markup[j].start {
			return markup[i].end > markup[j].end
		}
		return markup[i].start < markup[j].start
	})
	for i := len(markup) - 1; i >= 0; i-- {
		r := markup[i]
		if a >= r.contentEnd || b <= r.contentStart {
			continue
		}
		if a > r.start && a <= r.contentStart {
			a = r.start
		}
		if b >= r.contentEnd && b < r.end {
			b = r.end
		}
	}
	var prefix, suffix string
	trimSuffixSpace := false
	type edit struct {
		start, end int
		value      string
	}
	var edits []edit
	for _, r := range markup {
		if a >= r.contentEnd || b <= r.contentStart {
			continue
		}
		if r.autoURI != "" && (a > r.start || b < r.end) {
			label := m.source[max(a, r.contentStart):min(b, r.contentEnd)]
			edits = append(edits, edit{max(a, r.start), min(b, r.end), partialAutoLink(label, r.autoURI)})
			continue
		}
		if a > r.start && a <= r.contentStart {
			a = r.start
		}
		if b >= r.contentEnd && b < r.end {
			b = r.end
		}
		if r.link && a > r.contentStart {
			prefix += m.source[r.start:r.contentStart]
		}
		if b < r.contentEnd && (a <= r.start || r.link) {
			suffix = m.source[r.contentEnd:r.end] + suffix
			trimSuffixSpace = trimSuffixSpace || r.emphasis
		}
		if !r.link && a > r.contentStart && b > r.contentEnd {
			edits = append(edits, edit{max(a, r.contentEnd), min(b, r.end), ""})
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var result strings.Builder
	result.WriteString(prefix)
	pos := a
	for _, edit := range edits {
		if edit.start < pos {
			continue
		}
		result.WriteString(m.source[pos:edit.start])
		result.WriteString(edit.value)
		pos = edit.end
	}
	result.WriteString(m.source[pos:b])
	value := result.String()
	if trimSuffixSpace {
		trimmed := strings.TrimRight(value, " \t\r\n")
		return finish(trimmed + suffix + value[len(trimmed):])
	}
	return finish(value + suffix)
}

type markdownFence struct {
	start, markerEnd, closingStart, end int
	closed                              bool
	closing                             string
}

func (m *markdownSource) addCodeFence(block *ast.CodeBlock, data []byte, base int) {
	source := string(data)
	start := block.Pos()
	if start < 0 || start >= len(source) {
		return
	}
	marker := source[start]
	length := 0
	for start+length < len(source) && source[start+length] == marker {
		length++
	}
	lineStart := strings.LastIndexByte(source[:start], '\n') + 1
	var continuation strings.Builder
	for _, r := range source[lineStart:start] {
		if r == '>' || r == '\t' {
			continuation.WriteRune(r)
		} else {
			continuation.WriteByte(' ')
		}
	}
	prefix := continuation.String()
	contentEnd := start + len(strings.SplitN(source[start:], "\n", 2)[0]) + 1
	if segments := block.Value.Segments(); len(segments) > 0 {
		contentEnd = segments[len(segments)-1].Stop
		lineStart = strings.LastIndexByte(source[:segments[0].Start], '\n') + 1
		prefix = source[lineStart:segments[0].Start]
	}
	fence := strings.Repeat(string(marker), length)
	end := len(source)
	closed := false
	if contentEnd < len(source) {
		tail := strings.SplitN(source[contentEnd:], "\n", 2)[0]
		closed = strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(tail, prefix)), fence)
		if closed {
			end = contentEnd + len(tail)
		}
	}
	m.fences = append(m.fences, markdownFence{start: base + start, markerEnd: base + start + length, closingStart: base + min(contentEnd, len(source)), end: base + end, closed: closed, closing: prefix + fence})
}

func partialAutoLink(label, uri string) string {
	var escaped strings.Builder
	for _, r := range label {
		if r < utf8.RuneSelf && util.IsPunct(byte(r)) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	// Angle destinations accept parentheses without balancing. Escape only
	// characters forbidden there, retaining existing percent escapes.
	destination := strings.NewReplacer("\\", "%5C", "<", "%3C", ">", "%3E", " ", "%20", "\t", "%09", "\r", "%0D", "\n", "%0A").Replace(uri)
	return "[" + escaped.String() + "](<" + destination + ">)"
}

// Table layout adds padding and replaces source pipes and separator runs.
// Match those display units without copying their decoration into the source.
func tableSourceOffset(source, displayed string, offset int, end bool) int {
	a, b := 0, 0
	// Wrapping can discard the space before a continuation row.
	if r, _ := utf8.DecodeRuneInString(displayed); r != ' ' && r != '\t' && r != '│' && r != '┼' && r != '─' {
		for a < len(source) && (source[a] == ' ' || source[a] == '\t') {
			a++
		}
	}
	for b < len(displayed) && b < offset {
		r, size := utf8.DecodeRuneInString(displayed[b:])
		if r != ' ' && r != '\t' {
			for a < len(source) && (source[a] == ' ' || source[a] == '\t') {
				a++
			}
		}
		start := a
		stop := b + size
		switch r {
		case ' ', '\t':
			for stop < len(displayed) && (displayed[stop] == ' ' || displayed[stop] == '\t') {
				stop++
			}
			for a < len(source) && (source[a] == ' ' || source[a] == '\t') {
				a++
			}
		case '│', '┼':
			if a < len(source) && source[a] == '|' {
				a++
			}
		case '─':
			for strings.HasPrefix(displayed[stop:], "─") {
				stop += len("─")
			}
			for a < len(source) && (source[a] == '-' || source[a] == ':') {
				a++
			}
		default:
			if a < len(source) {
				_, rawSize := utf8.DecodeRuneInString(source[a:])
				a += rawSize
			}
		}
		if offset < stop && !end {
			return start
		}
		b = stop
	}
	return a
}
