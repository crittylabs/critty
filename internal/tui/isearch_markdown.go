package tui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type markdownSearchHighlight struct {
	start, end int
	line       int
	ordinal    int
}

type markdownSearchToken struct {
	sourceStart, sourceEnd int
	start, end             int
}

func (s *isearchState) markdownSearchHighlights(source *markdownSource) map[int][]markdownSearchHighlight {
	if s.markdownRE != s.re {
		s.markdownRE = s.re
		s.markdownHighlights = make(map[*markdownSource]map[int][]markdownSearchHighlight)
	}
	if highlights, ok := s.markdownHighlights[source]; ok {
		return highlights
	}
	lineAt := func(offset int) int {
		return max(0, sort.Search(len(source.offsets), func(i int) bool { return source.offsets[i] > offset })-1)
	}
	markup := make([][]markdownMarkup, len(source.lines))
	for _, r := range source.markup {
		for i := lineAt(r.start); i <= lineAt(r.end-1); i++ {
			markup[i] = append(markup[i], r)
		}
	}
	highlights := make(map[int][]markdownSearchHighlight)
	tokens := make([][]markdownSearchToken, len(source.lines))
	for i := range source.lines {
		line := &source.lines[i]
		searched := displaySourceText(line.source)
		for ordinal, loc := range s.findMatches(searched) {
			from, to := loc[0], loc[1]
			if searched != line.source {
				from = rawTextOffset(line.source, searched, from, false)
				to = rawTextOffset(line.source, searched, to, true)
			}
			start, end := source.offsets[i]+from, source.offsets[i]+to
			var ranges [][2]int
			hidden := false
			for _, r := range markup[i] {
				if start < r.contentStart && end > r.start || start < r.end && end > r.contentEnd {
					ranges = append(ranges, [2]int{r.contentStart, r.contentEnd})
					hidden = hidden || start >= r.start && end <= r.contentStart || start >= r.contentEnd && end <= r.end
				}
			}
			if !hidden {
				ranges = append(ranges, [2]int{start, end})
			}
			for _, r := range ranges {
				for row := lineAt(r[0]); row <= lineAt(max(r[0], r[1]-1)); row++ {
					if tokens[row] == nil {
						tokens[row] = source.lines[row].searchDisplayTokens()
					}
					for _, span := range searchDisplayRanges(tokens[row], r[0]-source.offsets[row], r[1]-source.offsets[row]) {
						highlights[row+1] = append(highlights[row+1], markdownSearchHighlight{span[0], span[1], i + 1, ordinal})
					}
				}
			}
		}
	}
	s.markdownHighlights[source] = highlights
	return highlights
}

func (line *markdownLine) searchDisplayTokens() []markdownSearchToken {
	var tokens []markdownSearchToken
	offset := 0
	for i, token := range line.tokens {
		value := line.displayTokenText(i)
		rawEnd := token.sourceEnd
		if rawEnd == 0 {
			rawEnd = token.end
		}
		if value != "" {
			tokens = append(tokens, markdownSearchToken{token.start, rawEnd, offset, offset + len(value)})
		}
		offset += len(value)
	}
	return tokens
}

func searchDisplayRanges(tokens []markdownSearchToken, start, end int) [][2]int {
	var ranges [][2]int
	first := sort.Search(len(tokens), func(i int) bool { return tokens[i].sourceEnd > start })
	for _, token := range tokens[first:] {
		if token.sourceStart >= end {
			break
		}
		from, to := token.start, token.end
		if token.sourceEnd-token.sourceStart == token.end-token.start {
			from += max(0, start-token.sourceStart)
			to -= max(0, token.sourceEnd-end)
		}
		if from < to {
			ranges = append(ranges, [2]int{from, to})
		}
	}
	return ranges
}

func (m *AppModel) markdownISearchDecorator(ref lineRef) func(string) (string, bool) {
	t := m.tab()
	if !t.isMarkdown {
		return nil
	}
	t.ensureMarkdownCache()
	source := t.markdown.after
	if ref.side == "old" {
		source = t.markdown.before
	}
	if source == nil || ref.line < 1 || ref.line > len(source.lines) {
		return nil
	}
	line := &source.lines[ref.line-1]
	plain := ansi.Strip(line.render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil))
	highlights := m.isearch.markdownSearchHighlights(source)[ref.line]
	return func(styled string) (string, bool) {
		actual := ansi.Strip(styled)
		if actual != plain && len(m.isearch.findMatches(actual)) > 0 {
			current := -1
			if m.isearch.found && m.isearch.match.tab == m.activeTab && m.isearch.match.ref == ref {
				current = m.isearch.match.ordinal
			}
			return highlightISearch(styled, m.isearch.findMatches(ansi.Strip(styled)), current)
		}
		spans := make([]isearchHighlight, 0, len(highlights))
		for _, h := range highlights {
			current := m.isearch.found && m.isearch.match.tab == m.activeTab &&
				m.isearch.match.ref.side == ref.side && m.isearch.match.ref.line == h.line && m.isearch.match.ordinal == h.ordinal
			start, end := h.start, h.end
			if actual != plain {
				// Form controls replace the source prefix and omit metadata.
				label := plain[start:end]
				if label == "" {
					continue
				}
				start = 0
				for occurrence := strings.Count(plain[:h.start], label); occurrence >= 0; occurrence-- {
					at := strings.Index(actual[start:], label)
					if at < 0 {
						start = -1
						break
					}
					start += at
					if occurrence > 0 {
						start += len(label)
					}
				}
				if start < 0 {
					continue
				}
				end = start + len(label)
			}
			spans = append(spans, isearchHighlight{start, end, current})
		}
		return highlightISearchSpans(styled, spans)
	}
}
