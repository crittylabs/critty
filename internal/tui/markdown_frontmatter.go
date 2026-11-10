package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Only a complete block at the document start is frontmatter.  Patch gaps
// cannot establish its boundary, so leave those runs to the Markdown parser.
func (m *markdownSource) parseFrontmatter(known map[int]bool) int {
	if len(m.lines) == 0 || strings.TrimRight(strings.TrimPrefix(m.lines[0].source, "\ufeff"), " \t\r") != "---" {
		return 0
	}
	end := 0
	for i, line := range m.lines {
		if known != nil && !known[i+1] {
			return 0
		}
		marker := strings.TrimRight(line.source, " \t\r")
		if i > 0 && (marker == "---" || marker == "...") {
			end = i + 1
			break
		}
	}
	if end == 0 {
		return 0
	}
	for _, i := range []int{0, end - 1} {
		line := &m.lines[i]
		line.tokens = []markdownToken{{end: len(line.source), text: line.source, style: markdownDelimiter}}
	}
	stop := m.offsets[end-1] + len(m.lines[end-1].source)
	iterator, err := lexers.Get("yaml").Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, m.source[:stop])
	if err != nil {
		return end
	}
	tokens := make([][]markdownToken, end)
	row, column := 0, 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		for i, part := range strings.Split(token.Value, "\n") {
			if i > 0 {
				row++
				column = 0
			}
			if row >= end || !strings.HasPrefix(m.lines[row].source[column:], part) {
				return end
			}
			kind := token.Type
			if kind == chroma.Error {
				kind = chroma.Text
			}
			if part != "" {
				tokens[row] = append(tokens[row], markdownToken{start: column, end: column + len(part), text: part, syntax: kind})
				column += len(part)
			}
		}
	}
	if row != end-1 || column != len(m.lines[row].source) {
		return end
	}
	for i := 1; i < end-1; i++ {
		m.lines[i].tokens = tokens[i]
	}
	return end
}
