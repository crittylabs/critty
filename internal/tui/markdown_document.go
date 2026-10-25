package tui

import (
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

var (
	markdownBlockParser  = parser.New(parser.WithExtensions(extension.TableParser))
	markdownInlineParser = paragraphParser()
)

type markdownStyle uint8

const (
	markdownBold markdownStyle = 1 << iota
	markdownItalic
	markdownCode
	markdownLink
	markdownCodePadding
)

// Tokens retain source byte ranges so word-diff colors can be applied after
// parsing, without interpreting each changed fragment as separate Markdown.
type markdownToken struct {
	start, end int
	text       string
	style      markdownStyle
	uri        string
	sourceEnd  int
}

type markdownLine struct {
	source       string
	tokens       []markdownToken
	cells        [][2]int
	tableSep     bool
	literal      bool
	rendered     string
	renderedDark bool
	renderedSet  bool
}

type markdownEdit struct {
	start, end int
	tokens     []markdownToken
}

type markdownSource struct {
	lines   []markdownLine
	source  string
	offsets []int
	markup  []markdownMarkup
	fences  []markdownFence
}

func parseMarkdownSource(lines []string, known map[int]bool) *markdownSource {
	result := &markdownSource{lines: make([]markdownLine, len(lines)), source: strings.Join(lines, "\n"), offsets: make([]int, len(lines))}
	for i, line := range lines {
		if i > 0 {
			result.offsets[i] = result.offsets[i-1] + len(lines[i-1]) + 1
		}
		result.lines[i] = markdownLine{source: line, literal: true,
			tokens: []markdownToken{{end: len(line), text: line}}}
	}
	for start := 0; start < len(lines); {
		if known != nil && !known[start+1] {
			start++
			continue
		}
		end := start + 1
		for end < len(lines) && (known == nil || known[end+1]) {
			end++
		}
		result.parseRun(start, end, known != nil)
		start = end
	}
	return result
}

func (m *markdownSource) parseRun(start, end int, incomplete bool) {
	var source strings.Builder
	offsets := make([]int, end-start)
	for i := start; i < end; i++ {
		offsets[i-start] = source.Len()
		source.WriteString(m.lines[i].source)
		if i+1 < end {
			source.WriteByte('\n')
		}
	}
	data := []byte(source.String())
	root := markdownBlockParser.Parse(data)
	edits := make([][]markdownEdit, len(offsets))
	lineAt := func(offset int) int {
		return max(0, sort.Search(len(offsets), func(i int) bool { return offsets[i] > offset })-1)
	}
	var visit func(ast.Node)
	visit = func(n ast.Node) {
		switch n.(type) {
		case *ast.Paragraph, *ast.Heading, *extast.TableCell:
			segments := n.(ast.BlockNode).Source()
			if len(segments) == 0 {
				return
			}
			first, last := lineAt(segments[0].Start), lineAt(segments[len(segments)-1].Stop-1)
			// A patch edge may cut through a paragraph or a surrounding block.
			// Do not invent an opening/closing context outside the supplied text.
			if incomplete && (first == 0 && start > 0 || last == len(offsets)-1) {
				return
			}
			if _, cell := n.(*extast.TableCell); cell {
				m.lines[start+first].cells = append(m.lines[start+first].cells, [2]int{segments[0].Start - offsets[first], segments[len(segments)-1].Stop - offsets[first]})
				if _, header := n.Parent().(*extast.TableHeader); header && first+1 < len(offsets) {
					m.lines[start+first+1].tableSep = true
				}
			}
			m.parseBlock(data, segments, offsets, start, edits, lineAt)
			return
		case *ast.CodeBlock:
			if block := n.(*ast.CodeBlock); block.CodeBlockKind == ast.CodeBlockKindFenced && (!incomplete || start == 0 || lineAt(block.Pos()) > 0) {
				m.addCodeFence(block, data, m.offsets[start])
			}
			return
		case *ast.HTMLBlock:
			return
		case *ast.ThematicBreak:
			for _, seg := range n.(ast.BlockNode).Source() {
				m.lines[start+lineAt(seg.Start)].literal = false
			}
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(root)
	for i, replacements := range edits {
		if len(replacements) == 0 {
			continue
		}
		line := &m.lines[start+i]
		sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
		line.tokens = nil
		pos := 0
		for _, edit := range replacements {
			if edit.start < pos {
				continue
			}
			line.tokens = append(line.tokens, markdownToken{start: pos, end: edit.start, text: line.source[pos:edit.start]})
			line.tokens = append(line.tokens, edit.tokens...)
			pos = edit.end
		}
		line.tokens = append(line.tokens, markdownToken{start: pos, end: len(line.source), text: line.source[pos:]})
		line.literal = false
	}
}

// Parse only paragraph content here. Reference definitions from another block
// are deliberately unavailable, and a list/quote prefix cannot start a new
// block after it has already been classified by the document parser.
func paragraphParser() parser.Parser {
	return parser.New(parser.WithDefaultParsers(false),
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(
			util.Prioritized[parser.InlineParser](sourceInlineParser{parser.NewCodeSpanParser()}, 100),
			util.Prioritized[parser.InlineParser](sourceInlineParser{parser.NewLinkParser()}, 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			util.Prioritized(parser.NewRawHTMLParser(), 400),
			util.Prioritized(parser.NewEmphasisParser(), 500)),
		parser.WithExtensions(extension.LinkifyParser))
}

func (m *markdownSource) parseBlock(data []byte, segments []text.Segment, offsets []int, base int, edits [][]markdownEdit, lineAt func(int) int) {
	type projection struct{ local, source, length int }
	var body strings.Builder
	var projections []projection
	for _, seg := range segments {
		projections = append(projections, projection{body.Len(), seg.Start, seg.Stop - seg.Start})
		body.Write(data[seg.Start:seg.Stop])
	}
	input := []byte(body.String())
	output := make(map[int][]markdownToken)
	emit := func(from, to int, value string, style markdownStyle, uri string) {
		for from < to {
			pi := max(0, sort.Search(len(projections), func(i int) bool { return projections[i].local > from })-1)
			p := projections[pi]
			stop := min(to, p.local+p.length)
			if stop <= from {
				break
			}
			global := p.source + from - p.local
			line := lineAt(global)
			column := global - offsets[line]
			part := string(input[from:stop])
			if value != "" && to-from == len(value) {
				part = value[:stop-from]
			}
			part = strings.TrimSuffix(part, "\n")
			if part != "" {
				output[line] = append(output[line], markdownToken{start: column, end: column + len(part), text: part, style: style, uri: uri, sourceEnd: min(column+stop-from, len(m.lines[base+line].source))})
			}
			if value != "" && len(value) >= stop-from {
				value = value[stop-from:]
			}
			from = stop
		}
	}
	var render func(ast.Node, markdownStyle, string)
	render = func(n ast.Node, style markdownStyle, uri string) {
		switch node := n.(type) {
		case *ast.Text:
			i := node.Value.Index()
			emit(i.Start, i.Stop, node.Value.Str(input), style, uri)
			return
		case *ast.CodeSpan:
			indices := node.Value.Indices()
			if len(indices) == 0 {
				return
			}
			style |= markdownCode
			first, last := indices[0], indices[len(indices)-1]
			emit(first.Start-1, first.Start, " ", style|markdownCodePadding, uri)
			if first.Start < last.Stop && input[first.Start] == ' ' && input[last.Stop-1] == ' ' && strings.Trim(string(input[first.Start:last.Stop]), " \n") != "" {
				indices = slices.Clone(indices)
				indices[0].Start++
				indices[len(indices)-1].Stop--
			}
			for _, index := range indices {
				emit(index.Start, index.Stop, "", style, uri)
			}
			emit(last.Stop, last.Stop+1, " ", style|markdownCodePadding, uri)
			return
		case *ast.Strong:
			style |= markdownBold
		case *ast.Emphasis:
			style |= markdownItalic
		case *ast.Link:
			style |= markdownLink
			uri = node.Destination.Str(input)
		case *ast.AutoLink:
			i := node.Label.Index()
			emit(i.Start, i.Stop, node.Label.Str(input), style|markdownLink, node.Destination.Str(input))
			return
		case *ast.Image:
			emit(node.Pos(), node.Pos()+1, "!", style, uri)
			style |= markdownLink
		case *ast.RawHTML:
			for _, i := range node.Value.Indices() {
				emit(i.Start, i.Stop, "", style, uri)
			}
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			render(child, style, uri)
		}
	}
	ranges := make(map[ast.Node]markdownMarkup)
	context := parser.NewContext()
	context.Set(markdownSourceRanges, ranges)
	root := markdownInlineParser.Parse(input, parser.WithContext(context))
	collectMarkdownMarkup(root, input, ranges)
	project := func(offset int) int {
		pi := max(0, sort.Search(len(projections), func(i int) bool { return projections[i].local > offset })-1)
		p := projections[pi]
		return m.offsets[base] + p.source + offset - p.local
	}
	for _, r := range ranges {
		r.start, r.contentStart, r.contentEnd, r.end = project(r.start), project(r.contentStart), project(r.contentEnd), project(r.end)
		m.markup = append(m.markup, r)
	}
	render(root, 0, "")
	for _, seg := range segments {
		line := lineAt(seg.Start)
		from := seg.Start - offsets[line]
		to := min(seg.Stop-offsets[line], len(m.lines[base+line].source))
		to = from + len(strings.TrimRight(m.lines[base+line].source[from:to], " \t\r"))
		edits[line] = append(edits[line], markdownEdit{from, to, output[line]})
	}
}

func (line *markdownLine) render(inline []gitpkg.InlineSegment, base, changed lipgloss.Style, links []linkSpan) string {
	cacheable := len(inline) == 0 && len(links) == 0
	if cacheable && line.renderedSet && line.renderedDark == terminalIsDark {
		return line.rendered
	}
	var b strings.Builder
	segmentIndex, segmentStart := 0, 0
	for _, token := range linkedMarkdownTokens(line.tokens, links) {
		if token.text == "" {
			continue
		}
		style := lipgloss.NewStyle()
		if token.style&markdownBold != 0 {
			style = mdBoldStyle.Inherit(style)
		}
		if token.style&markdownItalic != 0 {
			style = mdItalicStyle.Inherit(style)
		}
		if token.style&markdownLink != 0 {
			style = mdLinkStyle.Inherit(style)
		}
		if token.style&markdownCode != 0 {
			style = mdCodeStyle.Inherit(style)
		}
		uri := token.uri
		write := func(value string, background lipgloss.Style) {
			tokenStyle := style
			if len(inline) > 0 {
				tokenStyle = tokenStyle.Background(background.GetBackground())
			}
			value = tokenStyle.Render(displaySourceText(value))
			if len(inline) > 0 {
				value = inlineBackground(background, value)
			}
			b.WriteString(linkedText(uri, value))
		}
		if len(inline) == 0 {
			write(token.text, base)
			continue
		}
		for segmentIndex < len(inline) && segmentStart+len(inline[segmentIndex].Content) <= token.start {
			segmentStart += len(inline[segmentIndex].Content)
			segmentIndex++
		}
		offset := segmentStart
		for i := segmentIndex; i < len(inline) && offset < token.end; i++ {
			segment := inline[i]
			end := offset + len(segment.Content)
			from, to := max(offset, token.start), min(end, token.end)
			if from < to {
				background := base
				if segment.Changed {
					background = changed
				}
				value := token.text
				if len(value) == token.end-token.start {
					value = value[from-token.start : to-token.start]
				}
				write(value, background)
			}
			offset = end
		}
	}
	result := b.String()
	if !line.literal {
		result = highlightMarkdownBlock(result)
	}
	if cacheable {
		line.rendered, line.renderedDark, line.renderedSet = result, terminalIsDark, true
	}
	return result
}

func (t *FileTab) markdownTableCells(line int, links []linkSpan) []string {
	return t.markdownTableCellsWithInline(line, links, nil)
}

func (t *FileTab) markdownTableCellsWithInline(line int, links []linkSpan, inline []gitpkg.InlineSegment) []string {
	t.ensureMarkdownCache()
	parsed := &t.markdown.after.lines[line-1]
	var cells []string
	for _, span := range parsed.cells {
		cell := markdownLine{literal: true}
		for _, token := range parsed.tokens {
			from, to := max(span[0], token.start), min(span[1], token.end)
			if from >= to {
				continue
			}
			part := token
			if len(token.text) == token.end-token.start {
				part.text = token.text[from-token.start : to-token.start]
			}
			part.start, part.end = from, to
			cell.tokens = append(cell.tokens, part)
		}
		cells = append(cells, cell.render(inline, diffCommonTextBg, diffAddedTextBg, links))
	}
	return cells
}

// Split unchanged source tokens at link boundaries before applying styles.
// Parser replacements (entities and escapes) retain their source range.
func linkedMarkdownTokens(tokens []markdownToken, links []linkSpan) []markdownToken {
	if len(links) == 0 {
		return tokens
	}
	var result []markdownToken
	for _, token := range tokens {
		cuts := []int{token.start, token.end}
		if len(token.text) == token.end-token.start {
			for _, link := range links {
				for _, boundary := range []int{link.start, link.end} {
					if boundary > token.start && boundary < token.end {
						cuts = append(cuts, boundary)
					}
				}
			}
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		for i := 1; i < len(cuts); i++ {
			part := token
			part.start, part.end = cuts[i-1], cuts[i]
			if len(token.text) == token.end-token.start {
				part.text = token.text[part.start-token.start : part.end-token.start]
			}
			for _, link := range links {
				if part.start >= link.start && part.end <= link.end {
					// Keep explicit Markdown destinations; image previews still
					// replace their outer link as before.
					if part.uri == "" || strings.HasPrefix(link.uri, "critty://image?") {
						part.uri = link.uri
						part.style |= markdownLink
					}
				}
			}
			result = append(result, part)
		}
	}
	return result
}

type markdownCache struct {
	doc     *document.Document
	diff    *gitpkg.DiffInfo
	content string
	partial bool
	after   *markdownSource
	before  *markdownSource
}

func (t *FileTab) ensureMarkdownCache() {
	if t.doc == nil || !t.isMarkdown || t.markdown != nil && t.markdown.doc == t.doc && t.markdown.diff == t.diff && t.markdown.content == t.doc.Content && t.markdown.partial == (t.doc.Known != nil) {
		return
	}
	cache := &markdownCache{doc: t.doc, diff: t.diff, content: t.doc.Content, partial: t.doc.Known != nil, after: parseMarkdownSource(t.doc.Lines, t.doc.Known)}
	if t.diff != nil {
		oldLines := make(map[int]string)
		old := 1
		last := 0
		for n := 0; n <= t.doc.LineCount(); n++ {
			for _, del := range t.diff.DeletedAfter[n] {
				oldLines[del.OldLineNum] = del.Content
				old = del.OldLineNum + 1
				last = max(last, del.OldLineNum)
			}
			next := n + 1
			if !t.doc.HasLine(next) || t.diff.ChangedLines[next] {
				continue
			}
			if mapped := t.diff.CommonLines[next]; mapped > 0 {
				old = mapped
			}
			oldLines[old] = t.doc.Lines[n]
			last = max(last, old)
			old++
		}
		lines := make([]string, last)
		var known map[int]bool
		if t.doc.Known != nil {
			known = make(map[int]bool)
		}
		for n, line := range oldLines {
			lines[n-1] = line
			if known != nil {
				known[n] = true
			}
		}
		cache.before = parseMarkdownSource(lines, known)
	}
	t.markdown = cache
}

func (t *FileTab) markdownDisplayLine(line int, before bool, fallback string, inline []gitpkg.InlineSegment, images []linkSpan) string {
	t.ensureMarkdownCache()
	var source *markdownSource
	if t.markdown != nil {
		source = t.markdown.after
		if before {
			source = t.markdown.before
		}
	}
	base, changed := diffCommonTextBg, diffAddedTextBg
	if before {
		changed = diffDeletedTextBg
	}
	if source != nil && line > 0 && line <= len(source.lines) && source.lines[line-1].source == fallback {
		return source.lines[line-1].render(inline, base, changed, images)
	}
	// Standalone deleted lines and test fixtures may have no document cache.
	source = parseMarkdownSource([]string{fallback}, nil)
	return source.lines[0].render(inline, base, changed, images)
}

func markdownDisplayRows(content string, width int, decorate func(string) (string, bool)) []string {
	if decorate != nil {
		content, _ = decorate(content)
	}
	return strings.Split(lipgloss.Wrap(content, width, ""), "\n")
}
