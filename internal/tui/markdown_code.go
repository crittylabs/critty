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
	lexer      chroma.Lexer
	start, end int
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
	return codeFenceInfo{lexer: lexer, start: start, end: end}
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

func (m *markdownSource) highlightCodeBlock(block *ast.CodeBlock, data []byte, base int, lexer chroma.Lexer) {
	if lexer == nil {
		return
	}
	projection := projectMarkdownSegments(data, block.Value.Segments())
	source := projection.text
	iterator, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, source)
	if err != nil {
		return
	}
	// Project tokens through the parser's segments so quote/list prefixes and
	// source columns remain intact.  Commit only a lossless tokenisation.
	output := make(map[int][]markdownToken)
	pos := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		if !strings.HasPrefix(source[pos:], token.Value) {
			return
		}
		end := pos + len(token.Value)
		for pos < end {
			span := projection.spanAt(pos)
			stop := min(end, span.end)
			offset := base + span.source + pos - span.local
			row := sort.Search(len(m.offsets), func(i int) bool { return m.offsets[i] > offset }) - 1
			column := offset - m.offsets[row]
			length := min(stop-pos, len(m.lines[row].source)-column)
			kind := token.Type
			if kind == chroma.Error {
				kind = chroma.Text
			}
			if length > 0 {
				output[row] = append(output[row], markdownToken{start: column, end: column + length, text: source[pos : pos+length], syntax: kind})
			}
			pos = stop
		}
	}
	if pos != len(source) {
		return
	}
	for row, tokens := range output {
		line := &m.lines[row]
		first, last := tokens[0].start, tokens[len(tokens)-1].end
		line.tokens = append([]markdownToken{{end: first, text: line.source[:first]}}, tokens...)
		line.tokens = append(line.tokens, markdownToken{start: last, end: len(line.source), text: line.source[last:]})
	}
}

func (line *markdownLine) hasSyntax() bool {
	for _, token := range line.tokens {
		if token.syntax != 0 {
			return true
		}
	}
	return false
}
