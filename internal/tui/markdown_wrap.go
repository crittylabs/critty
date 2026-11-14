package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

func (m *markdownSource) markParagraphWrapping(n ast.Node, segments []text.Segment, base int, offsets []int, lineAt func(int) int) {
	container := false
	list := false
	quoted := false
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.(type) {
		case *ast.ListItem:
			container, list = true, true
		case *ast.Blockquote:
			container = true
			quoted = true
		}
	}
	if !container {
		return
	}
	for _, segment := range segments {
		row := lineAt(segment.Start)
		line := &m.lines[base+row]
		if len(line.cells) > 0 {
			continue
		}
		line.wrapStart = segment.Start - offsets[row]
		body := line.source[line.wrapStart:]
		if list && !quoted && (strings.HasPrefix(body, "[ ] ") || strings.HasPrefix(body, "[x] ") || strings.HasPrefix(body, "[X] ")) {
			line.wrapStart += 4
			line.wrapExtra = -4 // A rendered checkbox replaces the list and task markers.
		}
	}
}

func (m *markdownSource) applyQuotePrefixes(root ast.Node, data []byte, context parser.Context, base int, offsets []int, lineAt func(int) int, incomplete bool) {
	quotes, _ := context.Get(calloutQuotes).(map[ast.Node][]quotePrefix)
	var visit func(ast.Node)
	visit = func(n ast.Node) {
		prefixes := quotes[n]
		if incomplete && len(prefixes) > 0 && (base > 0 && lineAt(prefixes[0].start) == 0 || lineAt(prefixes[len(prefixes)-1].start) == len(offsets)-1) {
			prefixes = nil
		}
		for _, prefix := range prefixes {
			row := lineAt(prefix.start)
			line := &m.lines[base+row]
			raw := string(data[prefix.start:prefix.end])
			indent := raw[:strings.IndexByte(raw, '>')]
			line.replaceTokenRange(prefix.start-offsets[row], prefix.end-offsets[row], indent+"▎ ", markdownQuoteBar, 0)
			line.quoted = true
			line.wrapStart = max(line.wrapStart, prefix.end-offsets[row])
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(root)
}

func (m *markdownSource) prepareWrapping() {
	for i := range m.lines {
		line := &m.lines[i]
		if line.wrapStart < 0 || len(line.cells) > 0 {
			continue
		}
		end := line.displayOffset(line.wrapStart)
		var plain strings.Builder
		for token := range line.tokens {
			plain.WriteString(line.displayTokenText(token))
		}
		prefix := plain.String()[:min(end, plain.Len())]
		line.wrapIndent = max(0, ansi.StringWidth(prefix)+line.wrapExtra)
	}
}

// Only quote bars repeat. List markers, checkbox glyphs and title icons become
// spaces, retaining their column widths without duplicating source content.
func markdownContinuationPrefix(prefix string) string {
	var result strings.Builder
	column := 0
	for _, r := range ansi.Strip(prefix) {
		width := ansi.StringWidth(string(r))
		if r == '▎' {
			result.WriteString(ansi.Cut(prefix, column, column+width))
		} else {
			result.WriteString(strings.Repeat(" ", width))
		}
		column += width
	}
	return result.String()
}

func (line *markdownLine) continuationWidth(width int) int {
	if line == nil || line.previewTokens != nil || line.wrapIndent >= width {
		return 0
	}
	return line.wrapIndent
}

func (line *markdownLine) wrapRows(content string, width, initialPrefix int) []string {
	if !strings.ContainsRune(content, '\n') && ansi.StringWidth(content) <= width {
		return []string{content}
	}
	indent := line.continuationWidth(width - initialPrefix)
	if indent == 0 {
		return strings.Split(lipgloss.Wrap(content, width, ""), "\n")
	}
	indent += initialPrefix
	prefix := ansi.Cut(content, 0, indent)
	body := ansi.Cut(content, indent, ansi.StringWidth(content))
	rows := strings.Split(lipgloss.Wrap(body, width-indent, ""), "\n")
	rows[0] = prefix + rows[0]
	continuation := strings.Repeat(" ", initialPrefix) + markdownContinuationPrefix(ansi.Cut(prefix, initialPrefix, indent))
	for row := 1; row < len(rows); row++ {
		rows[row] = continuation + rows[row]
	}
	return rows
}

func (t *FileTab) markdownParsedLine(line int, before bool) *markdownLine {
	if t.markdown == nil {
		return nil
	}
	source := t.markdown.after
	if before {
		source = t.markdown.before
	}
	if source == nil || line <= 0 || line > len(source.lines) {
		return nil
	}
	return &source.lines[line-1]
}

func (t *FileTab) markdownWrapRows(line int, before bool, content string, width int, decorate func(string) (string, bool)) []string {
	if decorate != nil {
		content, _ = decorate(content)
	}
	return t.markdownParsedLine(line, before).wrapRows(content, width, 0)
}

func (t *FileTab) markdownHanging(line int, before bool, row, width int) int {
	if row == 0 {
		return 0
	}
	return t.markdownParsedLine(line, before).continuationWidth(width)
}
