package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/v2/internal/review"
)

func (m *AppModel) copyFocusedContent() tea.Cmd {
	if m.focused == contentPane && m.hasTextSelection() {
		return m.copySelectedText()
	}
	if id := m.selectedCommentID(); id != "" {
		comment, _ := m.driftedComment(id)
		return tea.Batch(m.copyText(copyThreadText(comment)), m.flashNotice(copiedTextNotice))
	}
	if m.focused != contentPane || len(m.tabs) == 0 || m.tab().doc == nil || m.tab().isBinary {
		return nil
	}
	first, last := m.selectionRange()
	t := m.tab()
	var oldLines map[int]string
	if m.selectionSide() == "old" {
		oldLines = t.beforeSourceLines()
	}
	var lines []string
	for n := first; n <= last; n++ {
		var text string
		var known bool
		if oldLines != nil {
			text, known = oldLines[n]
		} else if t.doc.HasLine(n) {
			text, known = t.doc.Lines[n-1], true
		}
		if !known {
			return m.flashNotice("Cannot copy range: source lines unavailable")
		}
		lines = append(lines, text)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		return nil
	}
	return tea.Batch(m.copyText(text), m.flashNotice(copiedTextNotice))
}

func copyThreadText(comment review.Comment) string {
	if len(comment.Replies) == 0 {
		return comment.Body
	}
	var text strings.Builder
	appendMessage := func(author, body string) {
		if text.Len() > 0 {
			text.WriteString("\n\n")
		}
		if author == "" {
			author = "anonymous"
		}
		text.WriteString("**")
		for _, r := range strings.ReplaceAll(strings.ReplaceAll(author, "\r", " "), "\n", " ") {
			if r >= '!' && r <= '/' || r >= ':' && r <= '@' || r >= '[' && r <= '`' || r >= '{' && r <= '~' {
				text.WriteByte('\\')
			}
			text.WriteRune(r)
		}
		text.WriteString("**\n\n")
		text.WriteString(body)
	}
	appendMessage(comment.Author, comment.Body)
	for _, reply := range comment.Replies {
		appendMessage(reply.Author, reply.Body)
	}
	return text.String()
}
