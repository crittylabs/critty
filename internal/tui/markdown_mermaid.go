package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

type renderedKind uint8

const (
	renderedMath renderedKind = iota
	renderedMermaid
)

type fenceBounds struct{ start, end int }

// Retain positions from Goldmark's own fence parser, including whether a
// closing fence was consumed before a surrounding quote or list ended.
type previewFenceParser struct {
	parser.BlockParser
	bounds map[ast.Node]fenceBounds
}

func (p *previewFenceParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	_, segment := reader.PeekLine()
	start := segment.Start + pc.BlockOffset()
	node, state := p.BlockParser.Open(parent, reader, pc)
	if node != nil {
		p.bounds[node] = fenceBounds{start: start}
	}
	return node, state
}

func (p *previewFenceParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	state := p.BlockParser.Continue(node, reader, pc)
	if state&parser.Close != 0 {
		bounds := p.bounds[node]
		bounds.end = segment.Start + len(strings.TrimRight(string(line), "\r\n"))
		p.bounds[node] = bounds
	}
	return state
}

func markdownMermaidRefs(source string) []markdownImage {
	data := []byte(source)
	fences := &previewFenceParser{BlockParser: parser.NewFencedCodeBlockParser(), bounds: make(map[ast.Node]fenceBounds)}
	root := parser.New(parser.WithBlockParsers(util.Prioritized[parser.BlockParser](fences, 699))).Parse(data)
	var refs []markdownImage
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		block, ok := n.(*ast.CodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		language, _ := block.Language(data)
		if block.CodeBlockKind != ast.CodeBlockKindFenced || language != "mermaid" || len(refs) >= 64 {
			return ast.WalkSkipChildren, nil
		}
		segments := block.Value.Segments()
		if len(segments) == 0 {
			return ast.WalkSkipChildren, nil
		}
		bounds := fences.bounds[block]
		start, end := bounds.start, bounds.end
		if end <= start || end > len(source) {
			return ast.WalkSkipChildren, nil
		}
		var body strings.Builder
		for _, segment := range segments {
			body.Write(segment.Bytes(data))
		}
		if body.Len() > 64<<10 || strings.TrimSpace(body.String()) == "" {
			return ast.WalkSkipChildren, nil
		}
		refs = append(refs, markdownImage{
			line:       strings.Count(source[:end], "\n") + 1,
			renderKind: renderedMermaid, renderBody: body.String(),
			renderFirst:  strings.Count(source[:start], "\n") + 1,
			renderColumn: start - strings.LastIndexByte(source[:start], '\n') - 1,
			renderStop:   end - strings.LastIndexByte(source[:end], '\n') - 1,
			renderSource: source[start:end],
		})
		return ast.WalkSkipChildren, nil
	})
	return refs
}

func (m AppModel) renderedRefs(source string) []markdownImage {
	var refs []markdownImage
	if m.mermaid.Available() {
		refs = append(refs, markdownMermaidRefs(source)...)
	}
	if m.math.Available() {
		refs = append(refs, markdownMathRefs(source)...)
	}

	sort.SliceStable(refs, func(i, j int) bool { return refs[i].line < refs[j].line })
	return refs[:min(64, len(refs))]
}

func (m AppModel) renderSource(ctx context.Context, kind renderedKind, body string, inline, dark bool) ([]byte, error) {
	if kind == renderedMermaid {
		return m.mermaid.Render(ctx, body, dark)
	}
	return m.math.Render(ctx, body, inline, dark)
}
