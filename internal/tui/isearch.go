package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// isearchMatch locates a match among the active tab's visual lines.  Matches
// stay within one line.
type isearchMatch struct {
	ref     lineRef
	index   int // position in visualLines
	start   int // byte offset in the line text with tabs expanded
	ordinal int // position among the matches on the same line
}

// isearchState holds an incremental search modeled on Emacs isearch: each
// edit searches again from the origin, and ctrl+s/ctrl+r step from the
// current match.
type isearchState struct {
	active    bool
	forward   bool
	query     string
	lastQuery string
	re        *regexp.Regexp // nil while the query is empty or invalid
	invalid   bool
	failing   bool
	wrapped   bool
	found     bool
	match     isearchMatch
	count     int // matches in the tab
	position  int // 1-based position of the current match, or 0

	origin             lineRef
	originOnAnnotation bool
	originAnnoIdx      int
	originOffset       int
}

// compileISearch compiles a query with smart case: it matches case
// sensitively only when the query contains an uppercase letter outside an
// escape sequence.
func compileISearch(query string) (*regexp.Regexp, error) {
	if !hasUnescapedUpper(query) {
		query = "(?i)" + query
	}
	return regexp.Compile(query)
}

func hasUnescapedUpper(s string) bool {
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case unicode.IsUpper(r):
			return true
		}
	}
	return false
}

// findNonEmpty returns the nonempty matches of re in s; empty matches cannot
// be shown or stepped over.
func findNonEmpty(re *regexp.Regexp, s string) [][]int {
	var locs [][]int
	for _, loc := range re.FindAllStringIndex(s, -1) {
		if loc[1] > loc[0] {
			locs = append(locs, loc)
		}
	}
	return locs
}

// isearchMatches lists every match in the active tab in display order.
// While lines are selected, only lines on the selected side are searched.
func (m *AppModel) isearchMatches() []isearchMatch {
	t := m.tab()
	if m.isearch.re == nil || t.doc == nil {
		return nil
	}
	deleted := make(map[int]string)
	for _, dels := range t.deletedAfter {
		for _, del := range dels {
			deleted[del.OldLineNum] = del.Content
		}
	}
	var matches []isearchMatch
	for i, ref := range m.visualLines(t) {
		if t.selecting && ref.side != t.selectSide {
			continue
		}
		text := deleted[ref.line]
		if ref.side == "" {
			text = t.doc.LineAt(ref.line)
		}
		for ordinal, loc := range findNonEmpty(m.isearch.re, expandDisplayTabs(text)) {
			matches = append(matches, isearchMatch{ref: ref, index: i, start: loc[0], ordinal: ordinal})
		}
	}
	return matches
}

func (a isearchMatch) before(b isearchMatch) bool {
	return a.index < b.index || a.index == b.index && a.start < b.start
}

// originIndex returns the visual position of the search origin, or -1 before
// the first line.
func (m *AppModel) originIndex() int {
	for i, ref := range m.visualLines(m.tab()) {
		if ref == m.isearch.origin {
			return i
		}
	}
	return -1
}

// startISearch enters search mode at the content cursor.
func (m *AppModel) startISearch(forward bool) {
	t := m.tab()
	m.isearch = isearchState{
		active:             true,
		forward:            forward,
		lastQuery:          m.isearch.lastQuery,
		origin:             lineRef{side: t.cursorSide, line: t.cursorLine},
		originOnAnnotation: t.cursorOnAnnotation,
		originAnnoIdx:      t.cursorAnnoIdx,
		originOffset:       m.contentViewport.YOffset(),
	}
	m.focused = contentPane
	m.recalculateLayout()
	m.rebuildContent()
}

// handleISearchKey handles a key in search mode.  It reports false after
// leaving the mode for a key that the review should handle instead.
func (m *AppModel) handleISearchKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "ctrl+s", "ctrl+r":
		m.isearchRepeat(msg.String() == "ctrl+s")
	case "enter":
		m.exitISearch(false)
	case "ctrl+g", "ctrl+c":
		m.exitISearch(true)
	case "backspace", "ctrl+h":
		if m.isearch.query != "" {
			_, size := utf8.DecodeLastRuneInString(m.isearch.query)
			m.isearch.query = m.isearch.query[:len(m.isearch.query)-size]
			m.isearchFromOrigin()
		}
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
			m.exitISearch(false)
			return false
		}
		m.isearchAppend(msg.Text)
	}
	return true
}

func (m *AppModel) isearchAppend(text string) {
	m.isearch.query += text
	m.isearchFromOrigin()
}

// isearchFromOrigin searches for the edited query from the origin.
func (m *AppModel) isearchFromOrigin() {
	s := &m.isearch
	s.re, s.invalid, s.failing, s.wrapped, s.found = nil, false, false, false, false
	if s.query == "" {
		m.restoreISearchOrigin()
		return
	}
	re, err := compileISearch(s.query)
	if err != nil {
		s.invalid = true
		m.rebuildContent()
		return
	}
	s.re = re
	m.isearchLand(firstFromOrigin(m.isearchMatches(), m.originIndex(), s.forward))
}

// firstFromOrigin finds the first match from the origin line in the search
// direction, including matches on that line.
func firstFromOrigin(matches []isearchMatch, origin int, forward bool) (isearchMatch, bool) {
	var match isearchMatch
	ok := false
	for _, candidate := range matches {
		if forward && candidate.index >= origin {
			return candidate, true
		}
		if !forward && candidate.index <= origin {
			match, ok = candidate, true
		}
	}
	return match, ok
}

// isearchRepeat steps to the next or previous match, or resumes the last
// query when the current one is empty.  Stepping past the last match fails
// once, then wraps around.
func (m *AppModel) isearchRepeat(forward bool) {
	s := &m.isearch
	if s.query == "" {
		s.forward = forward
		if s.lastQuery == "" {
			return
		}
		s.query = s.lastQuery
		m.isearchFromOrigin()
		return
	}
	if s.re == nil {
		s.forward = forward
		return
	}
	matches := m.isearchMatches()
	if len(matches) == 0 {
		s.forward = forward
		return
	}
	wrap := s.failing && s.forward == forward
	s.forward = forward
	var match isearchMatch
	ok := false
	switch {
	case wrap && forward:
		match, ok = matches[0], true
	case wrap:
		match, ok = matches[len(matches)-1], true
	case !s.found:
		// Turning around after a failure searches from the origin again.
		match, ok = firstFromOrigin(matches, m.originIndex(), forward)
	case forward:
		for _, candidate := range matches {
			if s.match.before(candidate) {
				match, ok = candidate, true
				break
			}
		}
	default:
		for _, candidate := range matches {
			if !candidate.before(s.match) {
				break
			}
			match, ok = candidate, true
		}
	}
	if !ok {
		s.failing = true
		m.countISearch()
		m.rebuildContent()
		return
	}
	s.wrapped = s.wrapped || wrap
	m.isearchLand(match, true)
}

// isearchLand moves the cursor to a match, or marks the search as failing
// and leaves the cursor where it is.
func (m *AppModel) isearchLand(match isearchMatch, ok bool) {
	s := &m.isearch
	s.found, s.failing = ok, !ok
	if ok {
		s.match = match
	}
	m.countISearch()
	if !ok {
		m.rebuildContent()
		return
	}
	t := m.tab()
	t.cursorLine, t.cursorSide = match.ref.line, match.ref.side
	t.cursorOnAnnotation, t.cursorAnnoIdx = false, 0
	m.rebuildContent()
	m.scrollToCursor()
}

// countISearch records the number of matches and the current match's
// position for the prompt.
func (m *AppModel) countISearch() {
	s := &m.isearch
	matches := m.isearchMatches()
	s.count, s.position = len(matches), 0
	for i, match := range matches {
		if s.found && match == s.match {
			s.position = i + 1
		}
	}
}

func (m *AppModel) restoreISearchOrigin() {
	s := &m.isearch
	t := m.tab()
	t.cursorLine, t.cursorSide = s.origin.line, s.origin.side
	t.cursorOnAnnotation, t.cursorAnnoIdx = s.originOnAnnotation, s.originAnnoIdx
	m.rebuildContent()
	m.contentViewport.SetYOffset(s.originOffset)
}

// exitISearch leaves search mode, keeping the cursor at the match or, when
// cancelled, restoring it to the origin.
func (m *AppModel) exitISearch(cancel bool) {
	s := &m.isearch
	if s.query != "" {
		s.lastQuery = s.query
	}
	s.active, s.re = false, nil
	m.recalculateLayout()
	if cancel {
		m.restoreISearchOrigin()
		return
	}
	m.rebuildContent()
	m.scrollToCursor()
}

// isearchDecorator returns a function that highlights search matches in a
// rendered line, or nil when no search is highlighting.
func (m *AppModel) isearchDecorator(ref lineRef) func(string) (string, bool) {
	s := &m.isearch
	if !s.active || s.re == nil {
		return nil
	}
	current := -1
	if s.found && s.match.ref == ref {
		current = s.match.ordinal
	}
	return func(styled string) (string, bool) {
		return highlightISearch(styled, s.re, current)
	}
}

// highlightISearch highlights the matches of re in the visible text of a
// styled line, keeping the surrounding styles.  The match with the given
// ordinal is the current one.
func highlightISearch(styled string, re *regexp.Regexp, current int) (string, bool) {
	plain := ansi.Strip(styled)
	locs := findNonEmpty(re, plain)
	if len(locs) == 0 {
		return styled, false
	}
	var b strings.Builder
	prev := 0
	for i, loc := range locs {
		start, end := ansi.StringWidth(plain[:loc[0]]), ansi.StringWidth(plain[:loc[1]])
		b.WriteString(ansi.Cut(styled, prev, start))
		style := isearchMatchStyle
		if i == current {
			style = isearchCurrentStyle
		}
		b.WriteString(style.Render(plain[loc[0]:loc[1]]))
		prev = end
	}
	b.WriteString(ansi.Cut(styled, prev, ansi.StringWidth(plain)))
	return b.String(), true
}

// renderISearchPrompt renders the search prompt shown in place of the footer.
func (m AppModel) renderISearchPrompt() string {
	s := m.isearch
	label := "I-search"
	if !s.forward {
		label += " backward"
	}
	switch {
	case s.failing && s.re != nil:
		label = "Failing " + label
	case s.wrapped:
		label = "Wrapped " + label
	}
	labelStyle := isearchPromptStyle
	if s.failing && s.re != nil {
		labelStyle = isearchFailingStyle
	}
	var status string
	switch {
	case s.invalid:
		status = " [incomplete regexp]"
	case s.re != nil:
		status = fmt.Sprintf(" (%d/%d)", s.position, s.count)
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	// Keep the prompt on one row; the layout reserves exactly one.
	return ansi.Truncate(labelStyle.Render(label+":")+" "+s.query+cursor+footerStyle.Render(status), m.width, "…")
}
