package tui

import (
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

type codeFenceInfo struct {
	lexer              chroma.Lexer
	start, end         int
	markdownSuggestion bool
}

func (m *markdownSource) codeFenceInfo(block *ast.CodeBlock, data []byte) codeFenceInfo {
	index := block.Info.Index()
	if index.IsEmpty() {
		return codeFenceInfo{}
	}
	raw := string(data[index.Start:index.Stop])
	language, start, end := "", 0, 0
	if strings.HasPrefix(raw, "{") {
		attrs, ok := parser.ParseAttributes(text.NewReader([]byte(raw), text.NewDecoder()))
		if !ok {
			return codeFenceInfo{}
		}
		for _, attr := range attrs {
			if attr.Name != "class" {
				continue
			}
			classes := strings.Fields(attr.Value.Str([]byte(raw)))
			if len(classes) == 0 {
				break
			}
			language = classes[0]
			if spans := attr.Value.Indices(); len(spans) > 0 {
				start, end = spans[0].Start, spans[0].Start+len(language)
			} else {
				start, end = fenceClassRange(raw, language)
			}
			break
		}
	} else {
		language, _ = block.Language(data)
		end = len(language)
	}
	lexer := m.codeFenceLexer(language)
	if start < end {
		start, end = index.Start+start, index.Start+end
	}
	markdownSuggestion := language == "suggestion" && !m.syntax.HasAlias(language) && lexer != nil && lexer.Config().Name == "markdown"
	return codeFenceInfo{lexer: lexer, start: start, end: end, markdownSuggestion: markdownSuggestion}
}

// Cache hits and misses for this parse; names without a direct Chroma match
// trigger a full filename-pattern scan.  Keep exact keys for case-sensitive matches.
func (m *markdownSource) codeFenceLexer(language string) chroma.Lexer {
	if language == "suggestion" && !m.syntax.HasAlias(language) {
		return m.suggestionLexer
	}
	if lexer, ok := m.fenceLexers[language]; ok {
		return lexer
	}
	lexer := m.syntax.Lexer(language)
	if m.fenceLexers == nil {
		m.fenceLexers = make(map[string]chroma.Lexer)
	}
	m.fenceLexers[language] = lexer
	return lexer
}

// Goldmark merges multiple classes into a string without source positions.
// Recover the first class span, skipping quoted attribute values.
func fenceClassRange(raw, language string) (int, int) {
	var quote byte
	for i := 1; i < len(raw); i++ {
		if quote != 0 {
			if raw[i] == quote {
				quote = 0
			}
			continue
		}
		if raw[i] == '\'' || raw[i] == '"' {
			quote = raw[i]
			continue
		}
		if raw[i] != '.' || raw[i-1] != ' ' && raw[i-1] != '\t' && raw[i-1] != '{' {
			continue
		}
		end := i + 1
		for end < len(raw) && raw[end] != ' ' && raw[end] != '\t' && raw[end] != '}' {
			end++
		}
		if raw[i+1:end] == language {
			return i + 1, end
		}
	}
	return 0, 0
}

func (m *markdownSource) styleCodeFence(fence markdownFence, info codeFenceInfo) {
	styleLine := func(start, end int, opening bool) {
		row := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > start }) - 1
		line := &m.lines[row]
		column := start - m.offsets[row]
		tokens := []markdownToken{{end: column, text: line.source[:column]}}
		appendToken := func(from, to int, style markdownStyle) {
			if from < to {
				tokens = append(tokens, markdownToken{start: from, end: to, text: line.source[from:to], style: style})
			}
		}
		appendToken(column, end-m.offsets[row], markdownDelimiter)
		if opening {
			from := end - m.offsets[row]
			if info.start < info.end {
				appendToken(from, info.start-m.offsets[row], markdownFenceAttribute)
				style := markdownDelimiter
				if info.lexer != nil {
					style = markdownFenceLanguage
				}
				appendToken(info.start-m.offsets[row], info.end-m.offsets[row], style)
				from = info.end - m.offsets[row]
			}
			appendToken(from, len(line.source), markdownFenceAttribute)
		} else {
			appendToken(end-m.offsets[row], len(line.source), markdownDelimiter)
		}
		line.tokens = tokens
	}
	styleLine(fence.start, fence.markerEnd, true)
	if fence.closed {
		row := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > fence.closingStart }) - 1
		line := m.lines[row].source
		openingRow := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > fence.start }) - 1
		marker := m.lines[openingRow].source[fence.start-m.offsets[openingRow]]
		column := strings.IndexByte(line, marker)
		if column >= 0 {
			styleLine(m.offsets[row]+column, fence.end, false)
		}
	}
}

func (m *markdownSource) highlightCodeBlock(block *ast.CodeBlock, data []byte, base int, info codeFenceInfo) {
	if info.lexer == nil {
		return
	}
	projection := projectMarkdownSegments(data, block.Value.Segments())
	if info.markdownSuggestion {
		m.highlightMarkdownSuggestion(projection, base)
		return
	}
	iterator, err := info.lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, projection.text)
	if err != nil {
		return
	}
	m.highlightSyntaxSegments(projection, base, iterator, 0)
}

func (m *markdownSource) highlightSyntaxSegments(projection markdownProjection, base int, iterator chroma.Iterator, style markdownStyle) {
	source := projection.text
	// Project tokens through the parser's segments so quote/list prefixes and
	// source columns remain intact.  Commit only a lossless tokenisation.
	output := make(map[int][]markdownToken)
	pos := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		if !strings.HasPrefix(source[pos:], token.Value) {
			return
		}
		end := pos + len(token.Value)
		kind := token.Type
		if kind == chroma.Error {
			kind = chroma.Text
		}
		m.projectSyntaxToken(projection, base, markdownToken{start: pos, end: end, syntax: kind, style: style}, output)
		pos = end
	}
	if pos != len(source) {
		return
	}
	m.setSyntaxTokens(output)
}

func (m *markdownSource) projectSyntaxToken(projection markdownProjection, base int, token markdownToken, output map[int][]markdownToken) {
	for pos := token.start; pos < token.end; {
		span := projection.spanAt(pos)
		stop := min(token.end, span.end)
		offset := base + span.source + pos - span.local
		row := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > offset }) - 1
		column := offset - m.offsets[row]
		length := min(stop-pos, len(m.lines[row].source)-column)
		if length > 0 {
			part := token
			part.start, part.end, part.text = column, column+length, projection.text[pos:pos+length]
			output[row] = append(output[row], part)
		}
		pos = stop
	}
}

func (m *markdownSource) setSyntaxTokens(output map[int][]markdownToken) {
	for row, tokens := range output {
		line := &m.lines[row]
		first, last := tokens[0].start, tokens[len(tokens)-1].end
		line.tokens = append([]markdownToken{{end: first, text: line.source[:first]}}, tokens...)
		line.tokens = append(line.tokens, markdownToken{start: last, end: len(line.source), text: line.source[last:]})
	}
}

func (m *markdownSource) highlightMarkdownSuggestion(projection markdownProjection, base int) {
	// The replacement is source code. Parse it on its own, without inheriting
	// suggestion syntax for any fences it contains.
	source := parseMarkdownWithSyntax(strings.Split(projection.text, "\n"), nil, m.syntax, m.symbols)
	output := make(map[int][]markdownToken)
	for i, line := range source.lines {
		if line.source == "" {
			continue
		}
		start := source.offsets[i]
		offset := base + projection.sourceOffset(start)
		row := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > offset }) - 1
		m.lines[row].markdownSuggestion = true
		m.lines[row].headingLevel = line.headingLevel
		for _, token := range line.sourceTokens() {
			token.start += start
			token.end += start
			token.sourceEnd = 0
			m.projectSyntaxToken(projection, base, token, output)
		}
	}
	m.setSyntaxTokens(output)
}
