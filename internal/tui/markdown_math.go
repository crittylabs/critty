package tui

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

var mathKind = ast.NewNodeKind("Math")

type mathNode struct {
	ast.BaseInline
	start, end int
	formula    string
	inline     bool
}

func (n *mathNode) Kind() ast.NodeKind        { return mathKind }
func (n *mathNode) Dump([]byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

type mathParser struct{}

func (mathParser) Trigger() []byte { return []byte{'$'} }

func (mathParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, start := reader.PeekLine()
	if len(line) < 2 {
		return nil
	}
	delim := "$"
	if line[1] == '$' {
		delim = "$$"
	} else if line[1] == ' ' || line[1] == '\t' || line[1] == '\n' {
		return nil
	}
	row, pos := reader.Position()
	reader.Advance(len(delim))
	var body strings.Builder
	for body.Len() < 16<<10 {
		line, seg := reader.PeekLine()
		if len(line) == 0 {
			break
		}
		escaped, comment := false, false
		for i, c := range line {
			if c == '%' && !escaped {
				comment = true
			}
			if !escaped && !comment && strings.HasPrefix(string(line[i:]), delim) {
				if delim == "$" && (i == 0 && body.Len() == 0 || i > 0 && (line[i-1] == ' ' || line[i-1] == '\t' || line[i-1] == '$') || i+1 < len(line) && (line[i+1] == '$' || line[i+1] >= '0' && line[i+1] <= '9')) {
					reader.SetPosition(row, pos)
					return nil
				}
				body.Write(line[:i])
				if strings.TrimSpace(body.String()) == "" {
					break
				}
				reader.Advance(i + len(delim))
				return &mathNode{start: start.Start, end: seg.Start + i + len(delim), formula: strings.Trim(body.String(), "\r\n"), inline: delim == "$"}
			}
			if c == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		if delim == "$" {
			break
		}
		body.Write(line)
		reader.AdvanceLine()
	}
	reader.SetPosition(row, pos)
	return nil
}

func markdownMathRefs(source string) []markdownImage {
	var refs []markdownImage
	for _, node := range markdownMathSpans([]byte(source), 0) {
		// Anchor a multiline preview after its closing delimiter.
		first := strings.Count(source[:node.start], "\n") + 1
		column := node.start - strings.LastIndexByte(source[:node.start], '\n') - 1
		stop := node.end - strings.LastIndexByte(source[:node.end], '\n') - 1
		refs = append(refs, markdownImage{line: strings.Count(source[:node.end], "\n") + 1, math: node.formula, mathInline: node.inline, mathFirst: first, mathColumn: column, mathStop: stop, mathSource: source[node.start:node.end]})
	}
	return refs
}

type mathSpan struct {
	start, end int
	formula    string
	inline     bool
}

// Parse code contents as Markdown too, while projecting offsets back through
// stripped container prefixes.  Inline code remains literal at every level.
func markdownMathSpans(data []byte, depth int) []mathSpan {
	if depth >= 16 {
		return nil
	}
	root := parser.New(parser.WithInlineParsers(util.Prioritized[parser.InlineParser](mathParser{}, 50))).Parse(data)
	var spans []mathSpan
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if len(spans) >= 64 {
			return ast.WalkStop, nil
		}
		switch node := n.(type) {
		case *mathNode:
			spans = append(spans, mathSpan{node.start, node.end, node.formula, node.inline})
		case *ast.CodeBlock:
			type projection struct{ local, start int }
			var body strings.Builder
			var positions []projection
			for _, segment := range node.Value.Segments() {
				positions = append(positions, projection{body.Len(), segment.Start})
				body.Write(data[segment.Start:segment.Stop])
			}
			if body.Len() == 0 {
				return ast.WalkSkipChildren, nil
			}
			project := func(offset int) int {
				i := max(0, sort.Search(len(positions), func(i int) bool { return positions[i].local > offset })-1)
				return positions[i].start + offset - positions[i].local
			}
			for _, span := range markdownMathSpans([]byte(body.String()), depth+1) {
				if len(spans) >= 64 {
					break
				}
				span.start, span.end = project(span.start), project(span.end)
				spans = append(spans, span)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return spans
}
