package tui

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"

	"github.com/crittylabs/critty/internal/icons"
)

type calloutKind uint8

const (
	calloutNote calloutKind = iota + 1
	calloutAbstract
	calloutInfo
	calloutTodo
	calloutTip
	calloutImportant
	calloutSuccess
	calloutQuestion
	calloutWarning
	calloutCaution
	calloutFailure
	calloutDanger
	calloutBug
	calloutExample
	calloutQuote
)

func calloutType(name string) calloutKind {
	switch strings.ToLower(name) {
	case "abstract", "summary", "tldr":
		return calloutAbstract
	case "info":
		return calloutInfo
	case "todo":
		return calloutTodo
	case "tip", "hint":
		return calloutTip
	case "important":
		return calloutImportant
	case "success", "check", "done":
		return calloutSuccess
	case "question", "help", "faq":
		return calloutQuestion
	case "warning", "attention":
		return calloutWarning
	case "caution":
		return calloutCaution
	case "failure", "fail", "missing":
		return calloutFailure
	case "danger", "error":
		return calloutDanger
	case "bug":
		return calloutBug
	case "example":
		return calloutExample
	case "quote", "cite":
		return calloutQuote
	default:
		return calloutNote
	}
}

func calloutTitleBackground(kind calloutKind) color.Color {
	return tintedBackground(calloutColor(kind), 12)
}

func calloutIcon(kind calloutKind, symbols icons.Set) string {
	icon, fallback := icons.Edit, "✎"
	switch kind {
	case calloutAbstract:
		icon, fallback = icons.Clipboard, "≡"
	case calloutInfo:
		icon, fallback = icons.Info, "i"
	case calloutTodo:
		icon, fallback = icons.Unchecked, "☐"
	case calloutTip:
		icon, fallback = icons.Flame, "*"
	case calloutImportant:
		icon, fallback = icons.Report, "!"
	case calloutSuccess:
		icon, fallback = icons.Check, "✓"
	case calloutQuestion:
		icon, fallback = icons.Question, "?"
	case calloutWarning:
		icon, fallback = icons.Warning, "!"
	case calloutCaution:
		icon, fallback = icons.Stop, "!"
	case calloutFailure:
		icon, fallback = icons.Error, "×"
	case calloutDanger:
		icon, fallback = icons.Bolt, "!"
	case calloutBug:
		icon, fallback = icons.Bug, "!"
	case calloutExample:
		icon, fallback = icons.List, "≡"
	case calloutQuote:
		icon, fallback = icons.Quote, "❝"
	}
	return symbols.Join(symbols.Glyph(icon, fallback), "")
}

func calloutColor(kind calloutKind) color.Color {
	light, dark := "#0969DA", "#58A6FF"
	switch kind {
	case calloutAbstract:
		light, dark = "#087F8C", "#56C9C9"
	case calloutTip, calloutSuccess:
		light, dark = "#1A7F37", "#3FB950"
	case calloutImportant, calloutExample:
		light, dark = "#8250DF", "#BC8CFF"
	case calloutQuestion, calloutWarning:
		light, dark = "#9A6700", "#D29922"
	case calloutCaution, calloutFailure, calloutDanger, calloutBug:
		light, dark = "#CF222E", "#F85149"
	case calloutQuote:
		light, dark = "#57606A", "#8B949E"
	}
	if terminalIsDark {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

type quotePrefix struct{ start, end int }

var calloutQuotes = parser.NewContextKey()

// Record the prefixes consumed by Goldmark, including nested and blank quote
// lines. Literal examples in code blocks never enter this parser.
type calloutQuoteParser struct{ parser.BlockParser }

func recordQuotePrefix(n ast.Node, reader text.Reader, context parser.Context, start int) {
	_, after := reader.Position()
	quotes, _ := context.Get(calloutQuotes).(map[ast.Node][]quotePrefix)
	if quotes == nil {
		quotes = make(map[ast.Node][]quotePrefix)
		context.Set(calloutQuotes, quotes)
	}
	quotes[n] = append(quotes[n], quotePrefix{start, after.Start})
}

func (p calloutQuoteParser) Open(parent ast.Node, reader text.Reader, context parser.Context) (ast.Node, parser.State) {
	_, before := reader.Position()
	n, state := p.BlockParser.Open(parent, reader, context)
	if n != nil {
		recordQuotePrefix(n, reader, context, before.Start)
	}
	return n, state
}

func (p calloutQuoteParser) Continue(n ast.Node, reader text.Reader, context parser.Context) parser.State {
	_, before := reader.Position()
	state := p.BlockParser.Continue(n, reader, context)
	if state&parser.Continue != 0 {
		recordQuotePrefix(n, reader, context, before.Start)
	}
	return state
}

type calloutHeader struct {
	kind  calloutKind
	end   int
	label string
}

func parseCalloutHeader(raw string) (calloutHeader, bool) {
	if !strings.HasPrefix(raw, "[!") {
		return calloutHeader{}, false
	}
	end := strings.IndexByte(raw, ']')
	if end <= 2 || !validCalloutName(raw[2:end]) {
		return calloutHeader{}, false
	}
	name := raw[2:end]
	stop := end + 1
	if stop < len(raw) && (raw[stop] == '+' || raw[stop] == '-') {
		stop++
	}
	title := strings.TrimLeft(raw[stop:], " \t")
	stop = len(raw) - len(title)
	label := ""
	if strings.TrimSpace(title) == "" {
		label = strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	}
	return calloutHeader{calloutType(name), stop, label}, true
}

func (m *markdownSource) applyCallouts(root ast.Node, data []byte, context parser.Context, base int, offsets []int, incomplete bool) {
	quotes, _ := context.Get(calloutQuotes).(map[ast.Node][]quotePrefix)
	lineAt := func(pos int) int {
		return max(0, sort.Search(len(offsets), func(i int) bool { return offsets[i] > pos })-1)
	}
	var visit func(ast.Node)
	visit = func(n ast.Node) {
		if prefixes := quotes[n]; len(prefixes) > 0 {
			m.applyCallout(n, prefixes, data, base, offsets, lineAt, incomplete)
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			visit(child)
		}
	}
	visit(root)
}

func (m *markdownSource) applyCallout(n ast.Node, prefixes []quotePrefix, data []byte, base int, offsets []int, lineAt func(int) int, incomplete bool) {
	paragraph, ok := n.FirstChild().(*ast.Paragraph)
	if !ok || len(paragraph.Source()) == 0 {
		return
	}
	first := paragraph.Source()[0]
	header, ok := parseCalloutHeader(string(data[first.Start:first.Stop]))
	if !ok {
		return
	}
	row := lineAt(first.Start)
	if incomplete && ((base > 0 && row == 0) || lineAt(prefixes[len(prefixes)-1].start) == len(offsets)-1) {
		return
	}
	for _, prefix := range prefixes {
		line := lineAt(prefix.start)
		raw := string(data[prefix.start:prefix.end])
		indent := raw[:strings.IndexByte(raw, '>')]
		m.lines[base+line].replaceTokenRange(prefix.start-offsets[line], prefix.end-offsets[line], indent+"▎ ", markdownCalloutBar, header.kind)
		m.lines[base+line].calloutBody = true
	}
	// CommonMark allows paragraph continuation lines without another '>'.
	// Give these the same display bar without adding source bytes.
	present := make(map[int]bool, len(prefixes))
	for _, prefix := range prefixes {
		present[lineAt(prefix.start)] = true
	}
	var continueParagraphs func(ast.Node)
	continueParagraphs = func(node ast.Node) {
		if paragraph, ok := node.(*ast.Paragraph); ok {
			for _, segment := range paragraph.Source() {
				row := lineAt(segment.Start)
				if !present[row] {
					at := segment.Start - offsets[row]
					m.lines[base+row].replaceTokenRange(at, at, "▎ ", markdownCalloutBar, header.kind)
					m.lines[base+row].calloutBody = true
					present[row] = true
				}
			}
		}
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			continueParagraphs(child)
		}
	}
	continueParagraphs(n)
	line := &m.lines[base+row]
	from := first.Start - offsets[row]
	line.replaceTokenRange(from, from+header.end, calloutIcon(header.kind, m.symbols)+header.label, markdownCalloutTitle, header.kind)
	line.wrapStart, line.wrapExtra = from, 2
	if header.label == "" {
		// Keep inline formatting and links in custom titles.
		for i := range line.tokens {
			if line.tokens[i].start >= from+header.end {
				line.tokens[i].style |= markdownCalloutTitle
				line.tokens[i].callout = header.kind
			}
		}
	}
}

func validCalloutName(name string) bool {
	for _, c := range name {
		if c != '-' && c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func (line *markdownLine) replaceTokenRange(from, to int, value string, style markdownStyle, kind calloutKind) {
	var result []markdownToken
	inserted := false
	for _, token := range line.tokens {
		if from == to && !inserted && token.start <= from && token.end > from {
			if token.start < from {
				part := token
				part.end, part.text = from, token.text[:from-token.start]
				result = append(result, part)
			}
			result = append(result, markdownToken{start: from, end: to, text: value, style: style, callout: kind, decoration: openingDecoration})
			token.text, token.start = token.text[from-token.start:], from
			result = append(result, token)
			inserted = true
			continue
		}
		if token.end <= from || token.start >= to {
			result = append(result, token)
			continue
		}
		if token.start < from {
			part := token
			part.end, part.text = from, token.text[:from-token.start]
			result = append(result, part)
		}
		if !inserted {
			result = append(result, markdownToken{start: from, end: to, text: value, style: style | markdownAtomic, callout: kind})
			inserted = true
		}
		if token.end > to {
			token.text, token.start = token.text[to-token.start:], to
			result = append(result, token)
		}
	}
	line.tokens = result
	line.literal = true
}
