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
	end     int
	ordinal int // position among the matches on the same line
}

// isearchMode selects how a query is matched.  The zero value is regexp.
type isearchMode int

const (
	isearchRegexp isearchMode = iota
	isearchLiteral
	isearchWord
)

// isearchOptions persist from one search to the next during a run.
type isearchOptions struct {
	lastQuery string
	mode      isearchMode
	caseFold  bool // ignore case always instead of smart case
}

// isearchState holds an incremental search modeled on Emacs isearch: each
// edit searches again from the origin, and ctrl+s/ctrl+r step from the
// current match.
type isearchState struct {
	isearchOptions
	active   bool
	forward  bool
	prefix   bool // M-s was pressed and awaits a toggle key
	query    string
	history  []string       // queries before each typed or captured unit
	re       *regexp.Regexp // nil while the query is empty or invalid
	invalid  bool
	failing  bool
	wrapped  bool
	found    bool
	match    isearchMatch
	count    int // matches in the tab
	position int // 1-based position of the current match, or 0

	origin             lineRef
	originOnAnnotation bool
	originAnnoIdx      int
	originOffset       int
}

// caseSensitive reports whether a query matches case sensitively: with smart
// case, only when it contains an uppercase letter outside a regexp escape.
func (o isearchOptions) caseSensitive(query string) bool {
	if o.caseFold {
		return false
	}
	escaped := false
	for _, r := range query {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && o.mode == isearchRegexp:
			escaped = true
		case unicode.IsUpper(r):
			return true
		}
	}
	return false
}

// compile builds the pattern for a query in the current mode.
func (o isearchOptions) compile(query string) (*regexp.Regexp, error) {
	pattern := query
	switch o.mode {
	case isearchLiteral:
		pattern = regexp.QuoteMeta(query)
	case isearchWord:
		pattern = wordPattern(query)
	}
	if !o.caseSensitive(query) {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// wordPattern matches the words of a query separated by any nonword
// characters, as Emacs word search does.  The last word may be incomplete
// until the query ends with a nonword character.
func wordPattern(query string) string {
	isWord := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	words := strings.FieldsFunc(query, func(r rune) bool { return !isWord(r) })
	if len(words) == 0 {
		return `[^\x00-\x{10FFFF}]` // matches nothing
	}
	for i, word := range words {
		words[i] = regexp.QuoteMeta(word)
	}
	pattern := strings.Join(words, `\W+`)
	if first, _ := utf8.DecodeRuneInString(query); isWord(first) {
		pattern = `\b` + pattern
	}
	if last, _ := utf8.DecodeLastRuneInString(query); !isWord(last) {
		pattern += `\b`
	}
	return pattern
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
	text := isearchLineText(t)
	var matches []isearchMatch
	for i, ref := range m.visualLines(t) {
		if t.selecting && ref.side != t.selectSide {
			continue
		}
		for ordinal, loc := range findNonEmpty(m.isearch.re, text(ref)) {
			matches = append(matches, isearchMatch{ref: ref, index: i, start: loc[0], end: loc[1], ordinal: ordinal})
		}
	}
	return matches
}

// isearchLineText returns a lookup for the searched text of a tab's lines,
// with tabs expanded as displayed.
func isearchLineText(t *FileTab) func(lineRef) string {
	deleted := make(map[int]string)
	for _, dels := range t.deletedAfter {
		for _, del := range dels {
			deleted[del.OldLineNum] = del.Content
		}
	}
	return func(ref lineRef) string {
		if ref.side == "" {
			return expandDisplayTabs(t.doc.LineAt(ref.line))
		}
		return expandDisplayTabs(deleted[ref.line])
	}
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
		isearchOptions:     m.isearch.isearchOptions,
		active:             true,
		forward:            forward,
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
	if m.isearch.prefix {
		m.isearch.prefix = false
		m.toggleISearch(msg.String())
		return true
	}
	switch msg.String() {
	case "ctrl+s", "ctrl+r":
		m.isearchRepeat(msg.String() == "ctrl+s")
	case "alt+s":
		m.isearch.prefix = true
	case "enter":
		m.exitISearch(false)
	case "ctrl+g", "ctrl+c", "esc":
		m.exitISearch(true)
	case "backspace", "ctrl+h":
		m.isearchDeleteUnit()
	case "ctrl+d":
		m.isearchCapture(false)
	case "ctrl+w":
		m.isearchCapture(true)
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
			m.exitISearch(false)
			return false
		}
		m.isearchAppend(msg.Text)
	}
	return true
}

// toggleISearch applies a toggle key typed after M-s and searches again.
// Word and regexp modes exclude each other, as in Emacs.
func (m *AppModel) toggleISearch(key string) {
	s := &m.isearch
	switch key {
	case "c":
		s.caseFold = !s.caseFold
	case "w":
		s.mode = toggledMode(s.mode, isearchWord)
	case "r":
		s.mode = toggledMode(s.mode, isearchRegexp)
	default:
		return
	}
	m.isearchFromOrigin()
}

// toggledMode turns mode on, or back to literal search when it is already on.
func toggledMode(current, mode isearchMode) isearchMode {
	if current == mode {
		return isearchLiteral
	}
	return mode
}

func (m *AppModel) isearchAppend(text string) {
	m.isearch.history = append(m.isearch.history, m.isearch.query)
	m.isearch.query += text
	m.isearchFromOrigin()
}

// isearchDeleteUnit removes the last typed or captured unit, as Emacs does
// on DEL, and searches again from the origin.
func (m *AppModel) isearchDeleteUnit() {
	s := &m.isearch
	switch {
	case len(s.history) > 0:
		s.query = s.history[len(s.history)-1]
		s.history = s.history[:len(s.history)-1]
	case s.query != "":
		_, size := utf8.DecodeLastRuneInString(s.query)
		s.query = s.query[:len(s.query)-size]
	default:
		return
	}
	m.isearchFromOrigin()
}

// isearchCapture appends the character or word following the current match
// to the query and keeps the cursor on that match.  Without a current match
// it does nothing.
func (m *AppModel) isearchCapture(word bool) {
	s := &m.isearch
	if !s.found || s.re == nil {
		return
	}
	rest := isearchLineText(m.tab())(s.match.ref)[s.match.end:]
	if rest == "" {
		return
	}
	_, size := utf8.DecodeRuneInString(rest)
	if word {
		size = len(nextWord(rest))
	}
	piece := rest[:size]
	if !s.caseSensitive(s.query) {
		// Captured text keeps a case-insensitive search insensitive.
		piece = strings.ToLower(piece)
	}
	if s.mode == isearchRegexp {
		piece = regexp.QuoteMeta(piece)
	}
	s.history = append(s.history, s.query)
	s.query += piece
	re, err := s.compile(s.query)
	if err != nil {
		m.isearchFromOrigin()
		return
	}
	s.re, s.invalid, s.failing = re, false, false
	for _, match := range m.isearchMatches() {
		if match.index == s.match.index && match.start == s.match.start {
			m.isearchLand(match, true)
			return
		}
	}
	m.isearchFromOrigin()
}

// isWordRune reports whether r forms words in a script that separates words
// with spaces.  CJKV ideographs and kana have no word separators, so they
// are taken one character at a time.
func isWordRune(r rune) bool {
	if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
		return false
	}
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// nextWord returns the word at the start of s, or its first character when
// that does not start a word.
func nextWord(s string) string {
	end := 0
	for end < len(s) {
		r, n := utf8.DecodeRuneInString(s[end:])
		if !isWordRune(r) {
			break
		}
		end += n
	}
	if end == 0 {
		_, end = utf8.DecodeRuneInString(s)
	}
	return s[:end]
}

// isearchFromOrigin searches for the edited query from the origin.
func (m *AppModel) isearchFromOrigin() {
	s := &m.isearch
	s.re, s.invalid, s.failing, s.wrapped, s.found = nil, false, false, false, false
	if s.query == "" {
		m.restoreISearchOrigin()
		return
	}
	re, err := s.compile(s.query)
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
		s.history = append(s.history, s.query)
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
	// Describe the search as Emacs isearch-message-prefix does.
	failing := s.failing && s.re != nil
	var words []string
	if failing {
		words = append(words, "failing")
	}
	if s.wrapped {
		words = append(words, "wrapped")
	}
	switch {
	case s.caseSensitive(s.query):
		words = append(words, "case-sensitive")
	case s.caseFold:
		words = append(words, "case-insensitive")
	}
	switch s.mode {
	case isearchRegexp:
		words = append(words, "regexp")
	case isearchWord:
		words = append(words, "word")
	}
	words = append(words, "I-search")
	if !s.forward {
		words = append(words, "backward")
	}
	label := strings.Join(words, " ")
	label = strings.ToUpper(label[:1]) + label[1:]
	labelStyle := isearchPromptStyle
	if failing {
		labelStyle = isearchFailingStyle
	}
	var status string
	switch {
	case s.invalid:
		status = " [incomplete input]"
	case s.re != nil:
		status = fmt.Sprintf(" (%d/%d)", s.position, s.count)
	}
	if s.prefix {
		status += " [M-s: c case-fold/smart-case, w word, r regexp]"
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	// Keep the prompt on one row; the layout reserves exactly one.
	return ansi.Truncate(labelStyle.Render(label+":")+" "+s.query+cursor+footerStyle.Render(status), m.width, "…")
}
