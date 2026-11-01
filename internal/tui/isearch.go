package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/dlclark/regexp2/v2"

	"github.com/crittylabs/critty/internal/searchhistory"
)

// isearchMatchTimeout bounds one match attempt.  Backtracking lets queries
// use backreferences and lookaround, but a catastrophic pattern must not
// freeze the TUI.
var isearchMatchTimeout = 100 * time.Millisecond

// searchRegexp is a compiled query.  Once an attempt times out, matching
// stops for the rest of that query, so the delay is paid once per edit
// rather than on every line.
type searchRegexp struct {
	re       *regexp2.Regexp
	timedOut atomic.Bool
}

// compileSearch compiles a pattern in RE2-compatible syntax extended with
// backreferences and lookaround.
func compileSearch(pattern string) (*searchRegexp, error) {
	re, err := regexp2.Compile(pattern, regexp2.RE2)
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = isearchMatchTimeout
	return &searchRegexp{re: re}, nil
}

// find returns the first match in s as byte offsets.
func (r *searchRegexp) find(s string) []int {
	if r.timedOut.Load() {
		return nil
	}
	m, err := r.re.FindStringMatch(s)
	if err != nil {
		r.timedOut.Store(true)
		return nil
	}
	if m == nil {
		return nil
	}
	start, length := m.ByteRange()
	return []int{start, start + length}
}

// findAll returns the nonempty matches in s as byte offsets; empty matches
// cannot be shown or stepped over.
func (r *searchRegexp) findAll(s string) [][]int {
	if r.timedOut.Load() {
		return nil
	}
	var locs [][]int
	m, err := r.re.FindStringMatch(s)
	for m != nil && err == nil {
		if start, length := m.ByteRange(); length > 0 {
			locs = append(locs, []int{start, start + length})
		}
		m, err = r.re.FindNextMatch(m)
	}
	if err != nil {
		r.timedOut.Store(true)
		return nil
	}
	return locs
}

// isearchMatch locates a match among a tab's visual lines.  Matches stay
// within one line.
type isearchMatch struct {
	tab     int
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

type isearchMouseAction int

const (
	isearchStart isearchMouseAction = iota
	isearchPrevious
	isearchNext
	isearchKeep
	isearchCancel
	isearchInput
	isearchCaseFold
	isearchFiles
	isearchEdit
)

type isearchMouseRegion struct {
	rect   mouseRect
	action isearchMouseAction
}

func (m AppModel) searchHint() string {
	if len(m.tabs) == 0 || m.tab().doc == nil {
		return ""
	}
	return footerKeyStyle.Render("/") + " " + footerDescStyle.Render("search")
}

func (m AppModel) isearchControls(top int) (string, []isearchMouseRegion) {
	var b strings.Builder
	var regions []isearchMouseRegion
	x, y := 0, top
	caseLabel := "Case:smart"
	if m.isearch.caseFold {
		caseLabel = "Case:ignore"
	}
	filesLabel := "Files:current"
	if m.isearchAllFiles() {
		filesLabel = "Files:all"
	}
	editLabel := "Edit"
	if m.isearch.editing {
		editLabel = "Done"
	}
	for _, spec := range []struct {
		label, hint string
		action      isearchMouseAction
	}{
		{"Prev", "C-r", isearchPrevious},
		{"Next", "C-s", isearchNext},
		{"Keep", "RET", isearchKeep},
		{"Cancel", "ESC", isearchCancel},
		{editLabel, "M-e", isearchEdit},
		{caseLabel, "M-s i", isearchCaseFold},
		{filesLabel, "M-s m", isearchFiles},
	} {
		hint := spec.hint
		if m.width < 60 {
			hint = ""
		}
		button := renderBarButton(spec.label, hint)
		width := lipgloss.Width(button)
		if x > 0 && x+1+width > m.width {
			b.WriteByte('\n')
			x, y = 0, y+1
		}
		if x > 0 {
			b.WriteByte(' ')
			x++
		}
		b.WriteString(button)
		regions = append(regions, isearchMouseRegion{
			rect: mouseRect{left: x, right: x + width, top: y, bottom: y + 1}, action: spec.action,
		})
		x += width
	}
	return b.String(), regions
}

func (m *AppModel) handleISearchMouse(mouse tea.Mouse) (bool, tea.Cmd) {
	if m.modal != noModal || len(m.tabs) == 0 || mouse.Button != tea.MouseLeft {
		return false, nil
	}
	_, layout := m.renderReviewScreen()
	for _, region := range layout.isearchRegions {
		if !region.rect.contains(mouse) {
			continue
		}
		var cmd tea.Cmd
		if m.isearch.editing && region.action != isearchInput {
			m.finishISearchEdit(false)
			if region.action == isearchEdit {
				return true, nil
			}
		}
		switch region.action {
		case isearchStart:
			if !m.isearch.active {
				m.startISearch(true)
			}
		case isearchInput:
			if m.isearch.editing {
				m.isearchInput.SetCursor(m.isearchEditIndex(mouse.X))
			}
		case isearchEdit:
			cmd = m.startISearchEdit()
		case isearchPrevious:
			m.isearchRepeat(false)
		case isearchNext:
			m.isearchRepeat(true)
		case isearchKeep:
			cmd = m.exitISearch(false)
		case isearchCancel:
			cmd = m.exitISearch(true)
		case isearchCaseFold:
			m.isearch.prefix = false
			m.toggleISearch("i")
		case isearchFiles:
			m.isearch.prefix = false
			m.toggleISearch("m")
		}
		return true, cmd
	}
	return false, nil
}

// isearchOptions persist from one search to the next during a run.
type isearchOptions struct {
	mode     isearchMode
	caseFold bool // ignore case always instead of smart case
	allFiles bool // search every tab, as Emacs multi-file isearch does
}

var isearchModeNames = map[isearchMode]string{isearchRegexp: "regexp", isearchLiteral: "literal", isearchWord: "word"}

func (mode isearchMode) String() string { return isearchModeNames[mode] }

// parseISearchMode reads a saved mode name; unknown names mean regexp.
func parseISearchMode(name string) isearchMode {
	for mode, known := range isearchModeNames {
		if known == name {
			return mode
		}
	}
	return isearchRegexp
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
	history  []string      // queries before each typed or captured unit
	ring     int           // 1-based position from the newest recalled history entry, or 0
	editing  bool          // the query is being edited in a line editor
	re       *searchRegexp // nil while the query is empty or invalid
	invalid  bool
	failing  bool
	wrapped  bool
	found    bool
	match    isearchMatch
	count    int // matches in the searched tabs
	position int // 1-based position of the current match, or 0
	files    int // tabs with matches
	file     int // 1-based position of the current match's tab among them

	originTab          int
	origin             lineRef
	originOnAnnotation bool
	originOnForm       bool
	originAnnoIdx      int
	originOffset       int
	markdownRE         *searchRegexp
	markdownHighlights map[*markdownSource]map[int][]markdownSearchHighlight
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
func (o isearchOptions) compile(query string) (*searchRegexp, error) {
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
	return compileSearch(pattern)
}

// wordPattern matches the words of a query separated by any nonword
// characters, as Emacs word search does.  The last word may be incomplete
// until the query ends with a nonword character.
func wordPattern(query string) string {
	words := strings.FieldsFunc(query, func(r rune) bool { return !isSearchWordRune(r) })
	if len(words) == 0 {
		return `[^\x00-\x{10FFFF}]` // matches nothing
	}
	for i, word := range words {
		words[i] = regexp.QuoteMeta(word)
	}
	return strings.Join(words, `[^\pL\pN\pM_]+`)
}

func isSearchWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
}

// findMatches applies Unicode word boundaries outside the regexp engine,
// whose word boundaries only recognize ASCII characters.
func (s isearchState) findMatches(text string) [][]int {
	if s.re == nil {
		return nil
	}
	if s.mode != isearchWord {
		return s.re.findAll(text)
	}
	first, _ := utf8.DecodeRuneInString(s.query)
	last, _ := utf8.DecodeLastRuneInString(s.query)
	var locs [][]int
	for offset := 0; offset < len(text); {
		loc := s.re.find(text[offset:])
		if loc == nil {
			break
		}
		start, end := offset+loc[0], offset+loc[1]
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if end > start && (!isSearchWordRune(first) || !isSearchWordRune(before)) &&
			(isSearchWordRune(last) || !isSearchWordRune(after)) {
			locs = append(locs, []int{start, end})
			offset = end
		} else {
			// A rejected match may overlap a later valid one.
			_, size := utf8.DecodeRuneInString(text[start:])
			offset = start + size
		}
	}
	return locs
}

// isearchAllFiles reports whether the search covers every tab.  A selection
// cannot leave its file, so selecting confines the search to the origin.
func (m *AppModel) isearchAllFiles() bool {
	s := &m.isearch
	return s.allFiles && len(m.tabs) > 1 && !m.tabs[s.originTab].selecting
}

// isearchMatches lists every match in display order: in the origin tab, or
// in every tab in tab order when searching all files.  While lines are
// selected, only lines on the selected side are searched.
func (m *AppModel) isearchMatches() []isearchMatch {
	if m.isearch.re == nil {
		return nil
	}
	tabs := []int{m.isearch.originTab}
	if m.isearchAllFiles() {
		tabs = make([]int, len(m.tabs))
		for i := range tabs {
			tabs[i] = i
		}
	}
	var matches []isearchMatch
	for _, tab := range tabs {
		t := &m.tabs[tab]
		if t.doc == nil {
			continue
		}
		text := isearchLineText(t)
		for i, ref := range m.visualLines(t) {
			if t.selecting && ref.side != t.selectSide {
				continue
			}
			for ordinal, loc := range m.isearch.findMatches(text(ref)) {
				matches = append(matches, isearchMatch{tab: tab, ref: ref, index: i, start: loc[0], end: loc[1], ordinal: ordinal})
			}
		}
	}
	if m.isearch.re.timedOut.Load() {
		return nil
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
			return displaySourceText(t.doc.LineAt(ref.line))
		}
		return displaySourceText(deleted[ref.line])
	}
}

func (a isearchMatch) before(b isearchMatch) bool {
	if a.tab != b.tab {
		return a.tab < b.tab
	}
	return a.index < b.index || a.index == b.index && a.start < b.start
}

// originIndex returns the visual position of the search origin in its tab,
// or -1 before the first line.
func (m *AppModel) originIndex() int {
	for i, ref := range m.visualLines(&m.tabs[m.isearch.originTab]) {
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
		originTab:          m.activeTab,
		origin:             lineRef{side: t.cursorSide, line: t.cursorLine},
		originOnAnnotation: t.cursorOnAnnotation,
		originOnForm:       t.cursorOnForm,
		originAnnoIdx:      t.cursorAnnoIdx,
		originOffset:       m.contentViewport.YOffset(),
	}
	m.focused = contentPane
	m.recalculateLayout()
	m.rebuildContent()
}

// handleISearchKey handles a key in search mode.  It reports false after
// leaving the mode for a key that the review should handle instead.
func (m *AppModel) handleISearchKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.isearch.editing {
		return m.handleISearchEditKey(msg)
	}
	if m.isearch.prefix {
		m.isearch.prefix = false
		m.toggleISearch(msg.String())
		return true, nil
	}
	switch msg.String() {
	case "ctrl+s", "ctrl+r":
		m.isearchRepeat(msg.String() == "ctrl+s")
	case "alt+s":
		m.isearch.prefix = true
	case "alt+e":
		return true, m.startISearchEdit()
	case "alt+p", "alt+n":
		m.isearchRecall(msg.String() == "alt+p")
	case "enter":
		return true, m.exitISearch(false)
	case "ctrl+g", "ctrl+c", "esc":
		return true, m.exitISearch(true)
	case "backspace", "ctrl+h":
		m.isearchDeleteUnit()
	case "ctrl+d":
		m.isearchCapture(false)
	case "ctrl+w":
		m.isearchCapture(true)
	case "ctrl+y":
		m.isearchPaste(m.killRing.current())
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
			return false, m.exitISearch(false)
		}
		m.isearchAppend(msg.Text)
	}
	return true, nil
}

// isearchRecall replaces the query with an older or newer entry from the
// project's search history, restoring its mode and case folding, as Emacs
// does for `M-p` and `M-n`.  Both directions wrap around the ring.
func (m *AppModel) isearchRecall(older bool) {
	entry, ok := m.isearchRingStep(older)
	if !ok {
		return
	}
	m.isearch.history = append(m.isearch.history, m.isearch.query)
	m.applyISearchEntry(entry)
}

// isearchRingStep moves the ring position one entry older or newer and
// returns the entry there.
func (m *AppModel) isearchRingStep(older bool) (searchhistory.Entry, bool) {
	s := &m.isearch
	n := len(m.searchRing)
	if n == 0 {
		return searchhistory.Entry{}, false
	}
	switch {
	case older:
		s.ring = s.ring%n + 1
	case s.ring <= 1:
		s.ring = n
	default:
		s.ring--
	}
	return m.searchRing[n-s.ring], true
}

// startISearchEdit opens the query in a line editor, as Emacs does for
// `M-e`, so the user can move within it instead of deleting back to a point.
func (m *AppModel) startISearchEdit() tea.Cmd {
	m.isearchInput = textinput.New()
	m.isearchInput.SetVirtualCursor(false)
	m.isearchInput.Prompt = ""
	m.isearchInput.SetValue(m.isearch.query)
	m.isearchInput.CursorEnd()
	m.isearch.editing, m.isearch.prefix = true, false
	m.recalculateLayout()
	return m.isearchInput.Focus()
}

// handleISearchEditKey handles a key while the query is being edited.
func (m *AppModel) handleISearchEditKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	s := &m.isearch
	switch msg.String() {
	case "enter", "alt+e":
		m.finishISearchEdit(false)
	case "ctrl+s", "ctrl+r":
		s.forward = msg.String() == "ctrl+s"
		m.finishISearchEdit(true)
	case "esc", "ctrl+g", "ctrl+c":
		s.editing = false
		m.isearchInput.Blur()
		m.recalculateLayout()
		m.rebuildContent()
	case "alt+p", "alt+n":
		if entry, ok := m.isearchRingStep(msg.String() == "alt+p"); ok {
			s.mode, s.caseFold = parseISearchMode(entry.Mode), entry.CaseFold
			m.isearchInput.SetValue(entry.Query)
			m.isearchInput.CursorEnd()
			m.recalculateLayout()
		}
	case "ctrl+y":
		m.isearchEditInsert(m.killRing.current())
	default:
		kill, backward := isearchEditKill(msg, m.isearchInput.KeyMap)
		before := []rune(m.isearchInput.Value())
		var cmd tea.Cmd
		m.isearchInput, cmd = m.isearchInput.Update(msg)
		if kill {
			start := m.isearchInput.Position()
			if removed := len(before) - utf8.RuneCountInString(m.isearchInput.Value()); removed > 0 {
				m.killRing.add(string(before[start:start+removed]), backward)
			} else {
				m.killRing.yank = nil
			}
		}
		return true, cmd
	}
	return true, nil
}

// isearchEditKill reports whether a key kills text in the query editor and
// whether it kills backward, so the kill ring can record the removed text.
func isearchEditKill(msg tea.KeyPressMsg, bindings textinput.KeyMap) (kill, backward bool) {
	if key.Matches(msg, bindings.DeleteBeforeCursor, bindings.DeleteWordBackward) {
		return true, true
	}
	return key.Matches(msg, bindings.DeleteAfterCursor, bindings.DeleteWordForward), false
}

// isearchEditInsert inserts text at the editor cursor.
func (m *AppModel) isearchEditInsert(text string) {
	text = sanitizeISearchText(text)
	if text == "" {
		return
	}
	value, pos := []rune(m.isearchInput.Value()), m.isearchInput.Position()
	m.isearchInput.SetValue(string(value[:pos]) + text + string(value[pos:]))
	m.isearchInput.SetCursor(pos + utf8.RuneCountInString(text))
}

// finishISearchEdit resumes the search with the edited query.  A changed
// query searches again from the origin; an unchanged one steps to the next
// match when repeat is set, as `C-s` and `C-r` do in the editor.
func (m *AppModel) finishISearchEdit(repeat bool) {
	s := &m.isearch
	s.editing = false
	m.isearchInput.Blur()
	text := m.isearchInput.Value()
	m.recalculateLayout()
	switch {
	case text != s.query:
		s.history = append(s.history, s.query)
		s.query = text
		m.isearchFromOrigin()
	case repeat:
		m.isearchRepeat(s.forward)
	default:
		m.rebuildContent()
	}
}

// isearchEditWindow returns the first visible rune of the edited query and
// the cursor column within the available width, keeping the cursor visible.
func isearchEditWindow(value []rune, pos, available int) (start, cursorX int) {
	start = min(pos, len(value))
	for start > 0 {
		width := ansi.StringWidth(string(value[start-1]))
		if cursorX+width > available-1 {
			break
		}
		start--
		cursorX += width
	}
	return start, cursorX
}

// isearchEditLayout returns the rendered prefix, the first visible rune of
// the edited query, and the available width for it.
func (m AppModel) isearchEditLayout(label string) (prefix string, start, available int) {
	value := []rune(m.isearchInput.Value())
	inputWidth := min(ansi.StringWidth(string(value)), max(2, m.width/2))
	prefix = ansi.Truncate(label, max(0, m.width-inputWidth-1), "…")
	available = max(0, m.width-ansi.StringWidth(prefix)-1)
	start, _ = isearchEditWindow(value, m.isearchInput.Position(), available)
	return prefix, start, available
}

// isearchEditIndex maps a footer column to a rune index in the edited query.
func (m AppModel) isearchEditIndex(column int) int {
	prefix, start, _ := m.isearchEditLayout(m.isearchPromptLabel())
	value := []rune(m.isearchInput.Value())
	x := ansi.StringWidth(prefix)
	for start < len(value) {
		width := ansi.StringWidth(string(value[start]))
		if x+width > column {
			break
		}
		x += width
		start++
	}
	return start
}

func (m *AppModel) applyISearchEntry(entry searchhistory.Entry) {
	s := &m.isearch
	s.query, s.mode, s.caseFold = entry.Query, parseISearchMode(entry.Mode), entry.CaseFold
	m.recalculateLayout()
	m.isearchFromOrigin()
}

// recordISearch adds the finished query to the project's search history and
// saves it unless the review is a demo.  It returns a notice for a failed save.
func (m *AppModel) recordISearch() tea.Cmd {
	s := m.isearch
	entry := searchhistory.Entry{Query: s.query, Mode: s.mode.String(), CaseFold: s.caseFold}
	m.searchRing = searchhistory.Push(m.searchRing, entry)
	if m.searchHistory == nil || (m.session != nil && m.session.InMemory()) {
		return nil
	}
	ring, err := m.searchHistory.Add(entry)
	if err != nil {
		return m.flashNotice("Search history not saved: " + err.Error())
	}
	m.searchRing = ring
	return nil
}

// loadSearchRing reads the saved history, starting empty when unavailable.
func loadSearchRing(store *searchhistory.Store) []searchhistory.Entry {
	if store == nil {
		return nil
	}
	ring, err := store.Load()
	if err != nil {
		return nil
	}
	return ring
}

// toggleISearch applies a toggle key typed after M-s and searches again.
// Word and regexp modes exclude each other, as in Emacs.
func (m *AppModel) toggleISearch(key string) {
	s := &m.isearch
	switch key {
	case "c", "i":
		s.caseFold = !s.caseFold
	case "w":
		s.mode = toggledMode(s.mode, isearchWord)
	case "r":
		s.mode = toggledMode(s.mode, isearchRegexp)
	case "m":
		s.allFiles = !s.allFiles
	default:
		return
	}
	m.recalculateLayout()
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

// isearchPaste keeps terminal pastes and kill-ring entries on one prompt row.
func (m *AppModel) isearchPaste(text string) {
	if text = sanitizeISearchText(text); text != "" {
		m.isearchAppend(text)
	}
}

// sanitizeISearchText folds pasted line breaks and tabs into spaces and
// drops other control characters and styling.
func sanitizeISearchText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
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
	rest := isearchLineText(&m.tabs[s.match.tab])(s.match.ref)[s.match.end:]
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
		if match.tab == s.match.tab && match.index == s.match.index && match.start == s.match.start {
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
	m.isearchLand(m.firstFromOrigin(m.isearchMatches(), s.forward))
}

// firstFromOrigin finds the first match from the origin line in the search
// direction, including matches on that line.
func (m *AppModel) firstFromOrigin(matches []isearchMatch, forward bool) (isearchMatch, bool) {
	origin := isearchMatch{tab: m.isearch.originTab, index: m.originIndex()}
	var match isearchMatch
	ok := false
	for _, candidate := range matches {
		atOrAfter := candidate.tab > origin.tab || candidate.tab == origin.tab && candidate.index >= origin.index
		atOrBefore := candidate.tab < origin.tab || candidate.tab == origin.tab && candidate.index <= origin.index
		if forward && atOrAfter {
			return candidate, true
		}
		if !forward && atOrBefore {
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
		if len(m.searchRing) == 0 {
			return
		}
		s.history = append(s.history, s.query)
		s.ring = 1
		m.applyISearchEntry(m.searchRing[len(m.searchRing)-1])
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
		match, ok = m.firstFromOrigin(matches, forward)
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
		if m.activeTab != s.originTab && !m.isearchAllFiles() {
			// The search no longer covers this file; fail at the origin.
			m.restoreISearchOrigin()
			return
		}
		m.rebuildContent()
		return
	}
	changed := m.activateTab(match.tab)
	t := m.tab()
	t.cursorLine, t.cursorSide = match.ref.line, match.ref.side
	t.cursorOnAnnotation, t.cursorAnnoIdx = false, 0
	t.cursorOnForm = false
	m.rebuildContent()
	if changed {
		m.updateCommentSidebar()
		step := -1
		if s.forward {
			step = 1
		}
		m.positionTabViewport(step)
	}
	m.scrollToCursor()
	m.revealTableMatch()
}

// countISearch records the number of matches and the current match's
// position for the prompt.
func (m *AppModel) countISearch() {
	s := &m.isearch
	matches := m.isearchMatches()
	s.count, s.position, s.files, s.file = len(matches), 0, 0, 0
	for i, match := range matches {
		if i == 0 || match.tab != matches[i-1].tab {
			s.files++
		}
		if s.found && match == s.match {
			s.position, s.file = i+1, s.files
		}
	}
}

func (m *AppModel) restoreISearchOrigin() {
	s := &m.isearch
	m.selectTab(s.originTab)
	t := m.tab()
	t.cursorLine, t.cursorSide = s.origin.line, s.origin.side
	t.cursorOnAnnotation, t.cursorAnnoIdx = s.originOnAnnotation, s.originAnnoIdx
	t.cursorOnForm = s.originOnForm
	m.rebuildContent()
	m.contentViewport.SetYOffset(s.originOffset)
}

// exitISearch leaves search mode, keeping the cursor at the match or, when
// cancelled, restoring it to the origin.
func (m *AppModel) exitISearch(cancel bool) tea.Cmd {
	s := &m.isearch
	if s.editing {
		m.finishISearchEdit(false)
	}
	var cmd tea.Cmd
	if s.query != "" {
		cmd = m.recordISearch()
	}
	s.active, s.re = false, nil
	m.recalculateLayout()
	if cancel {
		m.restoreISearchOrigin()
		return cmd
	}
	m.rebuildContent()
	m.scrollToCursor()
	return cmd
}

// isearchDecorator returns a function that highlights search matches in a
// rendered line, or nil when no search is highlighting.
func (m *AppModel) isearchDecorator(ref lineRef) func(string) (string, bool) {
	s := &m.isearch
	if !s.active || s.re == nil {
		return nil
	}
	if decorate := m.markdownISearchDecorator(ref); decorate != nil {
		return decorate
	}
	current := -1
	if s.found && s.match.tab == m.activeTab && s.match.ref == ref {
		current = s.match.ordinal
	}
	return func(styled string) (string, bool) {
		return highlightISearch(styled, s.findMatches(ansi.Strip(styled)), current)
	}
}

// highlightISearch highlights matches in the visible text of a
// styled line, keeping the surrounding styles.  The match with the given
// ordinal is the current one.
func highlightISearch(styled string, locs [][]int, current int) (string, bool) {
	spans := make([]isearchHighlight, 0, len(locs))
	for i, loc := range locs {
		spans = append(spans, isearchHighlight{loc[0], loc[1], i == current})
	}
	return highlightISearchSpans(styled, spans)
}

type isearchHighlight struct {
	start, end int
	current    bool
}

func highlightISearchSpans(styled string, spans []isearchHighlight) (string, bool) {
	if len(spans) == 0 {
		return styled, false
	}
	type event struct{ offset, count, current int }
	events := make([]event, 0, 2*len(spans))
	for _, span := range spans {
		current := 0
		if span.current {
			current = 1
		}
		events = append(events, event{span.start, 1, current}, event{span.end, -1, -current})
	}
	slices.SortFunc(events, func(a, b event) int { return a.offset - b.offset })
	var b, run strings.Builder
	var pen uv.Style
	var link uv.Link
	flush := func() {
		value := pen.Styled(run.String())
		if link.URL != "" {
			value = ansi.SetHyperlink(link.URL, link.Params) + value + ansi.ResetHyperlink()
		}
		b.WriteString(value)
		run.Reset()
	}
	offset, next, active, selected := 0, 0, 0, 0
	advance := func(end int) {
		for next < len(events) && events[next].offset <= end {
			active += events[next].count
			selected += events[next].current
			next++
		}
	}
	for rowIndex, row := range uv.NewStyledString(styled).Lines(ansi.GraphemeWidth) {
		if rowIndex > 0 {
			flush()
			b.WriteByte('\n')
			offset++
		}
		for _, cell := range row {
			if cell.Content == "" {
				continue
			}
			advance(offset)
			matched, current := active > 0, selected > 0
			end := offset + len(cell.Content)
			// A match inside a combining sequence highlights its whole grapheme.
			for next < len(events) && events[next].offset < end {
				advance(events[next].offset)
				matched, current = matched || active > 0, current || selected > 0
			}
			if matched {
				style := isearchMatchStyle
				if current {
					style = isearchCurrentStyle
				}
				cell.Style.Bg = style.GetBackground()
				cell.Style.Attrs |= uv.AttrBold
			}
			offset = end
			if run.Len() > 0 && (!pen.Equal(&cell.Style) || link != cell.Link) {
				flush()
			}
			pen, link = cell.Style, cell.Link
			run.WriteString(cell.Content)
		}
	}
	if run.Len() > 0 {
		flush()
	}
	return b.String(), true
}

// renderISearchPrompt renders the search prompt shown in place of the footer.
func (m AppModel) renderISearchPrompt() string {
	prompt, _ := m.isearchPrompt()
	return prompt
}

// isearchPrompt returns the prompt and the terminal cursor's display column.
// isearchLabel describes the search as Emacs isearch-message-prefix does
// and reports whether the search is failing.
func (m AppModel) isearchLabel() (string, bool) {
	s := m.isearch
	failing := s.failing && s.re != nil && !s.editing
	query := s.query
	if s.editing {
		query = m.isearchInput.Value()
	}
	var words []string
	if s.editing {
		words = append(words, "edit")
	}
	if failing {
		words = append(words, "failing")
	}
	if s.wrapped && !s.editing {
		words = append(words, "wrapped")
	}
	switch {
	case s.caseSensitive(query):
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
	if m.isearchAllFiles() {
		words = append(words, "multi-file")
	}
	words = append(words, "I-search")
	if !s.forward {
		words = append(words, "backward")
	}
	label := strings.Join(words, " ")
	return strings.ToUpper(label[:1]) + label[1:], failing
}

// isearchPromptLabel renders the styled label that precedes the query.
func (m AppModel) isearchPromptLabel() string {
	label, failing := m.isearchLabel()
	style := isearchPromptStyle
	if failing {
		style = isearchFailingStyle
	}
	return style.Render(label+":") + " "
}

func (m AppModel) isearchPrompt() (string, int) {
	if m.width <= 0 {
		return "", 0
	}
	s := m.isearch
	rendered := m.isearchPromptLabel()
	if s.editing {
		prefix, start, available := m.isearchEditLayout(rendered)
		value := []rune(m.isearchInput.Value())
		_, cursorX := isearchEditWindow(value, m.isearchInput.Position(), available)
		text := ansi.Truncate(string(value[start:]), available, "")
		return prefix + text, ansi.StringWidth(prefix) + cursorX
	}
	allFiles := m.isearchAllFiles()
	var status string
	switch {
	case s.invalid:
		status = " [incomplete input]"
	case s.re != nil && s.re.timedOut.Load():
		status = " [match timed out]"
	case s.re != nil && allFiles:
		status = fmt.Sprintf(" (%d/%d, file %d/%d)", s.position, s.count, s.file, s.files)
	case s.re != nil:
		status = fmt.Sprintf(" (%d/%d)", s.position, s.count)
	}
	if s.prefix {
		status += " [M-s: i/c case-fold/smart-case, w word, r regexp, m multi-file]"
	}
	// Keep the input end visible and reserve a cell for the real terminal cursor.
	queryWidth := ansi.StringWidth(s.query)
	inputWidth := min(queryWidth, max(2, m.width/2))
	prefix := ansi.Truncate(rendered, max(0, m.width-inputWidth-1), "…")
	available := max(0, m.width-ansi.StringWidth(prefix)-1)
	query := s.query
	for queryWidth > available && query != "" {
		cluster, width := ansi.FirstGraphemeCluster(query, ansi.GraphemeWidth)
		query = query[len(cluster):]
		queryWidth -= width
	}
	x := ansi.StringWidth(prefix + query)
	status = ansi.Truncate(footerStyle.Render(status), max(0, m.width-x-1), "…")
	return prefix + query + " " + status, x
}
