package tui

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Match crit's bare @path syntax, also allowing extensionless files and
// unescaped letters, marks and digits from any script. Only the optional
// Ldigits suffix has no trailing boundary, so L40に and L40-45 work.
var fileReferencePattern = regexp.MustCompile(`(^|[\s\p{Z}])@((?:[\p{L}\p{M}\p{N}._/-]|\\[^\r\n])+)(?: +L([0-9]+))?`)

var commentReferencePattern = regexp.MustCompile(`\bc_[a-f0-9]{6,}\b`)

func (m *AppModel) copyReference() {
	text := m.selectedCommentID()
	if text == "" {
		t := m.tab()
		if m.focused != contentPane || t.path == "" || strings.ContainsAny(t.path, "\r\n") {
			return
		}
		text = "@" + escapeReferencePath(t.path)
		if t.cursorSide != "old" && t.doc != nil && t.doc.HasLine(t.cursorLine) {
			text += " L" + strconv.Itoa(t.cursorLine)
		}
	}
	m.killRing.interrupt()
	m.killRing.add(text, false)
	m.killRing.interrupt()
	m.locationError = "Copied to kill ring: " + text
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

func (m *AppModel) linkCommentReferences(body string) string {
	return commentReferencePattern.ReplaceAllStringFunc(body, func(id string) string {
		if _, ok := m.commentReferenceTab(id); !ok {
			return id
		}
		uri := url.URL{Scheme: "tcrit", Host: "comment", RawQuery: url.Values{"id": {id}}.Encode()}
		return ansi.SetHyperlink(uri.String()) + "\x1b[4m" + id + "\x1b[24m" + ansi.ResetHyperlink()
	})
}

func (m *AppModel) navigateCommentReference(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "tcrit" || u.Host != "comment" {
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
		out.WriteString(m.linkCommentReferences(body[end:start]))
		uri := url.URL{Scheme: "tcrit", Host: "source", RawQuery: url.Values{
			"path": {path}, "line": {strconv.Itoa(line)},
		}.Encode()}
		out.WriteString(ansi.SetHyperlink(uri.String()))
		out.WriteString("\x1b[4m")
		out.WriteString(body[start:match[1]])
		out.WriteString("\x1b[24m")
		out.WriteString(ansi.ResetHyperlink())
		end = match[1]
	}
	out.WriteString(m.linkCommentReferences(body[end:]))
	return out.String()
}

func sourceLink(uri string) (sourceLocation, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "tcrit" || u.Host != "source" {
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
