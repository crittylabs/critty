package tui

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	extast "github.com/yuin/goldmark/v2/extension/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"

	"github.com/crittylabs/critty/internal/icons"
	"github.com/crittylabs/critty/internal/syntax"
)

var (
	markdownBlockParser  = parser.New(parser.WithExtensions(extension.TableParser), parser.WithBlockParsers(util.Prioritized[parser.BlockParser](calloutQuoteParser{parser.NewBlockquoteParser()}, 799)))
	markdownInlineParser = paragraphParser()
)

type markdownStyle uint16

type markdownDecoration uint8

const (
	noDecoration markdownDecoration = iota
	openingDecoration
	closingDecoration
)

const (
	markdownBold markdownStyle = 1 << iota
	markdownItalic
	markdownCode
	markdownLink
	markdownCodePadding
	markdownRendered
	markdownDelimiter
	markdownKeyboard
	markdownFenceLanguage
	markdownFenceAttribute
	markdownCalloutTitle
	markdownCalloutBar
	markdownAtomic
	markdownQuoteBar
)

// Tokens retain source byte ranges so word-diff colors can be applied after
// parsing, without interpreting each changed fragment as separate Markdown.
type markdownToken struct {
	start, end int
	text       string
	style      markdownStyle
	uri        string
	sourceEnd  int
	syntax     chroma.TokenType
	decoration markdownDecoration
	callout    calloutKind
}

type markdownLine struct {
	source        string
	tokens        []markdownToken
	cells         [][2]int
	tableSep      bool
	thematicBreak bool
	ruleStart     int
	literal       bool
	codeBlock     bool
	quoted        bool
	calloutBody   bool
	wrapStart     int
	wrapExtra     int
	wrapIndent    int
	previewTokens []markdownToken
	previewHidden bool
	previewMap    []previewDisplaySpan
	previewBands  []previewDisplayBand
	previewWidth  int
	previewScroll *horizontalBlockViewport
	// plainRender caches the rendering without word-diff segments, and
	// inlineRender the one for the segments it was last rendered with.
	plainRender, inlineRender markdownRender
}

type markdownEdit struct {
	start, end int
	tokens     []markdownToken
}

type markdownSource struct {
	lines           []markdownLine
	source          string
	offsets         []int
	markup          []markdownMarkup
	fences          []markdownFence
	headings        []outlineSymbol
	headingText     []string
	headingAnchors  map[string]int
	suggestionLexer chroma.Lexer
	syntax          *syntax.Rules
	symbols         icons.Set
	hasCodeBlocks   bool
}

func parseMarkdownSource(lines []string, known map[int]bool, suggestion ...chroma.Lexer) *markdownSource {
	return parseMarkdownWithSyntax(lines, known, nil, icons.Plain, suggestion...)
}

func parseMarkdownWithSyntax(lines []string, known map[int]bool, rules *syntax.Rules, symbols icons.Set, suggestion ...chroma.Lexer) *markdownSource {
	result := &markdownSource{syntax: rules, symbols: symbols, lines: make([]markdownLine, len(lines)), source: strings.Join(lines, "\n"), offsets: make([]int, len(lines))}
	if len(suggestion) > 0 {
		result.suggestionLexer = suggestion[0]
	}
	for i, line := range lines {
		if i > 0 {
			result.offsets[i] = result.offsets[i-1] + len(lines[i-1]) + 1
		}
		result.lines[i] = markdownLine{source: line, literal: true, wrapStart: -1,
			tokens: []markdownToken{{end: len(line), text: line}}}
	}
	for start := result.parseFrontmatter(known); start < len(lines); {
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
	result.prepareWrapping()
	ids := parser.NewIDs(parser.WithIDGenerator(markdownHeadingIDs{}))
	result.headingAnchors = make(map[string]int, len(result.headings))
	for i, heading := range result.headings {
		id := string(ids.Generate([]byte(result.headingText[i]), ast.KindHeading))
		result.headingAnchors[id] = heading.line
	}
	result.headingText = nil
	return result
}

func (m *markdownSource) parseRun(start, end int, incomplete bool) {
	base := m.offsets[start]
	source := m.source[base : m.offsets[end-1]+len(m.lines[end-1].source)]
	offsets := make([]int, end-start)
	for i := range offsets {
		offsets[i] = m.offsets[start+i] - base
	}
	data := []byte(source)
	context := parser.NewContext()
	root := markdownBlockParser.Parse(data, parser.WithContext(context))
	edits := make([][]markdownEdit, len(offsets))
	lineAt := func(offset int) int {
		return max(0, sort.Search(len(offsets), func(i int) bool { return offsets[i] > offset })-1)
	}
	var visit func(ast.Node)
	visit = func(n ast.Node) {
		switch n.(type) {
		case *ast.Paragraph, *ast.Heading, *extast.TableCell:
			if heading, ok := n.(*ast.Heading); ok {
				m.headingText = append(m.headingText, outlineHeadingText(heading, data))
				m.headings = append(m.headings, outlineSymbol{outlineHeading: outlineHeading{
					line: start + lineAt(heading.Pos()) + 1, level: heading.Level,
					label: outlineHeadingLabel(heading, data),
				}, kind: icons.Heading})
			}
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
			m.markParagraphWrapping(n, segments, start, offsets, lineAt)
			return
		case *ast.CodeBlock:
			if block := n.(*ast.CodeBlock); block.CodeBlockKind == ast.CodeBlockKindFenced && (!incomplete || start == 0 || lineAt(block.Pos()) > 0) {
				info := m.codeFenceInfo(block, data)
				if fence := m.addCodeFence(block, source, base); fence != nil {
					info.start, info.end = info.start+base, info.end+base
					m.styleCodeFence(*fence, info)
					m.markCodeRows(fence.start, fence.end)
				}
				m.highlightCodeBlock(block, data, base, info.lexer)
			} else if block.CodeBlockKind == ast.CodeBlockKindIndented && !incomplete {
				segments := block.Value.Segments()
				if len(segments) > 0 {
					m.markCodeRows(base+segments[0].Start, base+segments[len(segments)-1].Stop)
				}
			}
			return
		case *ast.HTMLBlock:
			return
		case *ast.ThematicBreak:
			row := lineAt(n.Pos())
			if incomplete && (row == 0 && start > 0 || row == len(offsets)-1) {
				return
			}
			line := &m.lines[start+row]
			line.thematicBreak = true
			line.ruleStart = max(0, n.Pos()-offsets[row])
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
	m.applyQuotePrefixes(root, data, context, start, offsets, lineAt, incomplete)
	m.applyCallouts(root, data, context, start, offsets, incomplete)
}

// Heading anchors retain Unicode text and use numbered suffixes for duplicates.
type markdownHeadingIDs struct{}

func (markdownHeadingIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	return []byte(strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
			return unicode.ToLower(r)
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, string(value)))
}

// Parse only paragraph content here. Reference definitions from another block
// are deliberately unavailable, and a list/quote prefix cannot start a new
// block after it has already been classified by the document parser.
func paragraphParser() parser.Parser {
	return parser.New(parser.WithDefaultParsers(false),
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(
			util.Prioritized[parser.InlineParser](mathParser{}, 50),
			util.Prioritized[parser.InlineParser](sourceInlineParser{parser.NewCodeSpanParser()}, 100),
			util.Prioritized[parser.InlineParser](sourceInlineParser{parser.NewLinkParser()}, 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
			util.Prioritized(parser.NewRawHTMLParser(), 400),
			util.Prioritized(parser.NewEmphasisParser(), 500)),
		parser.WithExtensions(extension.LinkifyParser))
}

func (m *markdownSource) parseBlock(data []byte, segments []text.Segment, offsets []int, base int, edits [][]markdownEdit, lineAt func(int) int) {
	projection := projectMarkdownSegments(data, segments)
	input := []byte(projection.text)
	output := make(map[int][]markdownToken)
	emit := func(from, to int, value string, style markdownStyle, uri string) {
		for from < to {
			p := projection.spanAt(from)
			stop := min(to, p.end)
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
				end := column + len(part)
				if style&markdownKeyboard != 0 && style&markdownCode == 0 {
					part = string(text.NewDecoder().Decode([]byte(part)))
				}
				output[line] = append(output[line], markdownToken{start: column, end: end, text: part, style: style, uri: uri, sourceEnd: min(column+stop-from, len(m.lines[base+line].source))})
			}
			if value != "" && len(value) >= stop-from {
				value = value[stop-from:]
			}
			from = stop
		}
	}
	ranges := make(map[ast.Node]markdownMarkup)
	var render func(ast.Node, markdownStyle, string)
	decorate := func(offset int, value string, style markdownStyle, uri string, kind markdownDecoration) {
		global := projection.sourceOffset(offset)
		row := lineAt(global)
		column := global - offsets[row]
		output[row] = append(output[row], markdownToken{start: column, end: column, text: value, style: style, uri: uri, decoration: kind})
	}
	render = func(n ast.Node, style markdownStyle, uri string) {
		switch node := n.(type) {
		case *keyboardNode:
			if containsKeyboard(node) {
				break
			}
			if node.FirstChild() == nil {
				return
			}
			style |= markdownKeyboard
			decorate(node.contentStart, "⟦", style, uri, openingDecoration)
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				render(child, style, uri)
			}
			decorate(node.contentEnd, "⟧", style, uri, closingDecoration)
			return
		case *mathNode:
			emit(node.start, node.end, "", style, uri)
			return
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
			if !markdownHasLabel(node, input) {
				i := node.Destination.Index()
				if i.Start >= 0 && i.Stop <= len(input) {
					emit(i.Start, i.Stop, uri, style, uri)
					if suffix := externalLinkSuffix(uri); suffix != "" {
						decorate(i.Stop, suffix, style, uri, closingDecoration)
					}
				}
				return
			}
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				render(child, style, uri)
			}
			if suffix := externalLinkSuffix(uri); suffix != "" {
				if r, ok := ranges[node]; ok {
					decorate(r.contentEnd, suffix, style, uri, closingDecoration)
				}
			}
			return
		case *ast.AutoLink:
			i := node.Label.Index()
			uri = node.Destination.Str(input)
			emit(i.Start, i.Stop, node.Label.Str(input), style|markdownLink, uri)
			if suffix := externalLinkSuffix(uri); suffix != "" {
				decorate(i.Stop, suffix, style|markdownLink, uri, closingDecoration)
			}
			return
		case *ast.Image:
			uri = node.Destination.Str(input)
			emit(node.Pos(), node.Pos()+1, "!", style, uri)
			style |= markdownLink
			if !markdownHasLabel(node, input) {
				i := node.Destination.Index()
				if i.Start >= 0 && i.Stop <= len(input) {
					emit(i.Start, i.Stop, node.Destination.Str(input), style, uri)
				}
				return
			}
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
	context := parser.NewContext()
	context.Set(markdownSourceRanges, ranges)
	root := markdownInlineParser.Parse(input, parser.WithContext(context))
	groupKeyboardNodes(root, input)
	collectMarkdownMarkup(root, input, ranges)
	project := func(offset int) int {
		return m.offsets[base] + projection.sourceOffset(offset)
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
