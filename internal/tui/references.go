package tui

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Match crit's bare @path syntax, also allowing extensionless files and
// unescaped letters, marks and digits from any script. Only the optional
// Ldigits suffix has no trailing boundary, so L40に and L40-45 work.
var fileReferencePattern = regexp.MustCompile(`(^|[\s\p{Z}\x60])@((?:[\p{L}\p{M}\p{N}._/-]|\\[^\r\n])+)(?: +L([0-9]+))?`)

var commentReferencePattern = regexp.MustCompile(`\bc_[a-f0-9]{6,}\b`)

func (m *AppModel) copyReference() tea.Cmd {
	text := m.selectedCommentID()
	if text == "" {
		if len(m.tabs) == 0 {
			return nil
		}
		t := m.tab()
		if m.focused != contentPane || t.path == "" || strings.ContainsAny(t.path, "\r\n") {
			return nil
		}
		text = "@" + escapeReferencePath(t.path)
		if t.cursorSide != "old" && t.doc != nil && t.doc.HasLine(t.cursorLine) {
			text += " L" + strconv.Itoa(t.cursorLine)
		}
	}
	m.killRing.interrupt()
	m.killRing.add(text, false)
	m.killRing.interrupt()
	return m.flashNotice("Copied to kill ring: " + text)
}

func (m *AppModel) commentReferenceTab(id string) (int, bool) {
	index := -1
	for i, t := range m.tabs {
		if t.state == nil {
			continue
		}
		for _, c := range t.state.Comments {
			if c.ID == id {
				if index >= 0 {
					return 0, false
				}
				index = i
			}
		}
	}
	return index, index >= 0
}

func (m *AppModel) navigateCommentReference(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "critty" || u.Host != "comment" {
		return false
	}
	id := u.Query().Get("id")
	i, ok := m.commentReferenceTab(id)
	if !ok {
		return false
	}
	if m.hideComments {
		m.hideComments = false
		m.recalculateLayout()
	}
	m.tab().selecting = false
	m.tabs[i].selecting = false
	for _, target := range m.commentTargets(i) {
		if target.id == id {
			m.selectComment(i, target)
			return true
		}
	}
	return false
}

func referenceWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r)
}

// escapeReferencePath writes path in the @path notation. Letters, marks and
// digits of any script pass through, as does an underscore between two of
// them; everything else is escaped so it neither ends the reference nor acts
// as Markdown punctuation in other renderers.
func escapeReferencePath(path string) string {
	runes := []rune(path)
	var out strings.Builder
	for i, r := range runes {
		plain := referenceWordRune(r) || strings.ContainsRune("./-", r)
		if r == '_' {
			plain = i > 0 && i+1 < len(runes) && referenceWordRune(runes[i-1]) && referenceWordRune(runes[i+1])
		}
		if !plain {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// resolveReferencePath returns the longest prefix of the escaped reference
// text naming a regular file, so prose in scripts without word spacing can
// follow a path directly (@main.goを参照). Only unescaped non-ASCII runes are
// dropped; ASCII terminators are explicit in the notation.
func (m *AppModel) resolveReferencePath(raw string) (string, int) {
	path := unescapeReferencePath(raw)
	if m.referencePathExists(path) {
		return path, len(raw)
	}
	var cuts []int
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[i:])
		if r == '\\' {
			_, next := utf8.DecodeRuneInString(raw[i+size:])
			i += size + next
			continue
		}
		if r >= utf8.RuneSelf {
			cuts = append(cuts, i)
		}
		i += size
	}
	for j := len(cuts) - 1; j >= 0; j-- {
		if cuts[j] == 0 {
			break
		}
		path := unescapeReferencePath(raw[:cuts[j]])
		if m.referencePathExists(path) {
			return path, cuts[j]
		}
	}
	return "", 0
}

func (m *AppModel) referencePathExists(path string) bool {
	_, attached := m.attachments[path]
	return attached || regularFile(m.sourcePath(path))
}

func unescapeReferencePath(path string) string {
	var out strings.Builder
	escaped := false
	for _, r := range path {
		if r == '\\' && !escaped {
			escaped = true
			continue
		}
		out.WriteRune(r)
		escaped = false
	}
	return out.String()
}

func (m *AppModel) linkFileReferences(body string) string {
	var out strings.Builder
	end := 0
	for _, span := range m.referenceSpans(body) {
		out.WriteString(body[end:span.start])
		out.WriteString(linkedText(span.uri, "\x1b[4m"+body[span.start:span.end]+"\x1b[24m"))
		end = span.end
	}
	out.WriteString(body[end:])
	return out.String()
}

func (m *AppModel) referenceSpans(body string) []linkSpan {
	var spans []linkSpan
	for _, match := range fileReferencePattern.FindAllStringSubmatchIndex(body, -1) {
		if match[1] < len(body) && body[match[1]] == '\\' {
			continue // incomplete escape, not a reference to the shorter prefix
		}
		path, used := m.resolveReferencePath(body[match[4]:match[5]])
		if used == 0 {
			continue
		}
		if used < match[5]-match[4] {
			match[1], match[5], match[6] = match[4]+used, match[4]+used, -1
		}
		line := 0
		if match[6] >= 0 {
			var err error
			line, err = strconv.Atoi(body[match[6]:match[7]])
			if err != nil || line < 1 {
				continue
			}
		}
		start := match[4] - 1 // include @, preserve the preceding whitespace
		uri := url.URL{Scheme: "critty", Host: "source", RawQuery: url.Values{
			"path": {path}, "line": {strconv.Itoa(line)},
		}.Encode()}
		spans = append(spans, linkSpan{start: start, end: match[1], uri: uri.String()})
	}
	for _, match := range commentReferencePattern.FindAllStringIndex(body, -1) {
		id := body[match[0]:match[1]]
		if _, ok := m.commentReferenceTab(id); !ok {
			continue
		}
		uri := url.URL{Scheme: "critty", Host: "comment", RawQuery: url.Values{"id": {id}}.Encode()}
		spans = append(spans, linkSpan{start: match[0], end: match[1], uri: uri.String()})
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end > spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	end := 0
	result := spans[:0]
	for _, span := range spans {
		if span.start >= end {
			result = append(result, span)
			end = span.end
		}
	}
	return result
}

func (m *AppModel) documentLinkSpans(line int, before bool) []linkSpan {
	spans := m.imageSpans(line, before)
	t := m.tab()
	if t.attachment == nil || before || t.doc == nil || !t.doc.HasLine(line) {
		return spans
	}
	for _, ref := range m.referenceSpans(t.doc.Lines[line-1]) {
		overlaps := false
		if t.isMarkdown {
			t.ensureMarkdownCache()
			for _, token := range t.markdown.after.lines[line-1].tokens {
				if token.uri != "" && ref.start < token.end && ref.end > token.start {
					overlaps = true
					break
				}
			}
		}
		for _, span := range spans {
			if ref.start < span.end && ref.end > span.start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			spans = append(spans, ref)
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

func sourceLink(uri string) (sourceLocation, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "critty" || u.Host != "source" {
		return sourceLocation{}, false
	}
	path := u.Query().Get("path")
	line, err := strconv.Atoi(u.Query().Get("line"))
	return sourceLocation{path: path, line: line}, err == nil && line >= 0 && path != ""
}

// Read the composed terminal cell rather than estimating offsets in the raw
// comment. Canvas preserves links through wrapping, clipping and wide glyphs.
func (m *AppModel) referenceAt(x, y int) string {
	if m.width <= 0 || m.height <= 0 || m.tab().state == nil {
		return ""
	}
	screen, _ := m.renderReviewScreen()
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	cell := canvas.CellAt(x, y)
	if cell == nil {
		return ""
	}
	return cell.Link.URL
}

// Content links activate on release so dragging their labels can select text.
func (m *AppModel) activateReference(uri string) (tea.Cmd, bool) {
	if m.navigateCommentReference(uri) {
		return nil, true
	}
	if location, ok := sourceLink(uri); ok && m.referencePathExists(location.path) {
		return m.navigateSource(location), true
	}
	if cmd, ok := m.openImageLink(uri); ok {
		return cmd, true
	}
	cmd := m.openLink(uri, false)
	return cmd, uri != "" && cmd != nil
}
