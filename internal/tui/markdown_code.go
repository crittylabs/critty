package tui

import (
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark/v2/ast"
)

func (m *markdownSource) highlightCodeBlock(block *ast.CodeBlock, data []byte, base int) {
	language, _ := block.Language(data)
	lexer := lexers.Get(language)
	if language == "suggestion" {
		lexer = m.suggestionLexer
	}
	if lexer == nil {
		return
	}
	segments := block.Value.Segments()
	var body strings.Builder
	starts := make([]int, len(segments))
	for i, segment := range segments {
		starts[i] = body.Len()
		body.Write(data[segment.Start:segment.Stop])
	}
	source := body.String()
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
			i := sort.Search(len(starts), func(i int) bool { return starts[i] > pos }) - 1
			segment := segments[i]
			stop := min(end, starts[i]+segment.Stop-segment.Start)
			offset := base + segment.Start + pos - starts[i]
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
