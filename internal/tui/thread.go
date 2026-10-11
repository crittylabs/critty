package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/review"
)

const maxThreadLines = 10

type threadLayout struct {
	lines  []string
	starts []int
}

func (m *AppModel) layoutThread(author, body string, replies []review.Reply, editingID string, width int) threadLayout {
	var layout threadLayout
	appendComment := func(author, header, body string) {
		if len(layout.lines) > 0 {
			layout.lines = append(layout.lines, "")
		}
		layout.starts = append(layout.starts, len(layout.lines))
		style := m.commentAuthorStyle(author)
		content := style.Bold(true).Render(header) + "\n" + style.Render(m.linkFileReferences(body))
		layout.lines = append(layout.lines, strings.Split(lipgloss.Wrap(expandDisplayTabs(content), max(1, width), ""), "\n")...)
	}
	name := author
	if name == "" {
		name = "anonymous"
	}
	appendComment(author, "Comment — "+name, body)
	for i, reply := range replies {
		if editingID != "" && reply.ID == editingID {
			continue
		}
		name := reply.Author
		if name == "" {
			name = "anonymous"
		}
		header := "↳ " + name + " " + commentLineStyle.Bold(false).Render(fmt.Sprintf("(#%d)", i+1))
		appendComment(reply.Author, header, reply.Body)
	}
	return layout
}

func (t threadLayout) initialOffset(height int) int {
	if len(t.starts) == 0 {
		return 0
	}
	return min(t.starts[len(t.starts)-1], max(0, len(t.lines)-height))
}

type threadViewKey struct {
	path    string
	id      string
	sidebar bool
}

type threadScroll struct {
	offset, maxOffset, height int
	manual                    bool
	lastReply                 string
	replies                   int
	revealReply               string
}

func (m *AppModel) threadHeight(sidebar bool) int {
	height := m.contentViewport.Height()
	if sidebar {
		height = m.commentViewport.Height()
	}
	if height == 0 {
		return maxThreadLines
	}
	return max(1, min(maxThreadLines, height-4))
}

func (m *AppModel) renderThread(key threadViewKey, author, body string, replies []review.Reply, width int, focused bool) string {
	key.path = m.tab().path
	c, drifted := m.driftedComment(key.id)
	if drifted {
		body = driftedContext(c) + "\n\n" + body
	}
	layout := m.layoutThread(author, body, replies, "", width)
	height := min(m.threadHeight(key.sidebar), len(layout.lines))
	initial := layout.initialOffset(height)
	lastReply := ""
	if len(replies) > 0 {
		lastReply = replies[len(replies)-1].ID
	}
	if m.threadScrolls == nil {
		m.threadScrolls = make(map[threadViewKey]threadScroll)
	}
	scroll := m.threadScrolls[key]
	if !focused || scroll.replies != len(replies) || scroll.lastReply != lastReply {
		scroll.manual = false
	}
	scroll.height = height
	scroll.maxOffset = max(0, len(layout.lines)-height)
	scroll.replies, scroll.lastReply = len(replies), lastReply
	if focused && scroll.revealReply != "" {
		for i, reply := range replies {
			if reply.ID == scroll.revealReply {
				scroll.offset = min(layout.starts[i+1], scroll.maxOffset)
				scroll.manual = true
				break
			}
		}
		scroll.revealReply = ""
	}
	if scroll.manual {
		scroll.offset = min(scroll.offset, scroll.maxOffset)
	} else {
		scroll.offset = initial
	}
	m.threadScrolls[key] = scroll
	if !focused {
		return m.renderLatestComment(author, body, replies, width, height)
	}
	rows := append([]string(nil), layout.lines[scroll.offset:min(len(layout.lines), scroll.offset+height)]...)
	if scroll.maxOffset > 0 {
		up, down := " ", " "
		if scroll.offset > 0 {
			up = "↑"
		}
		if scroll.offset+height < len(layout.lines) {
			down = "↓"
		}
		status := fmt.Sprintf("%s %d–%d/%d %s", up, scroll.offset+1,
			min(len(layout.lines), scroll.offset+height), len(layout.lines), down)
		rows = append(rows, footerStyle.Render(ansi.Truncate(status, max(1, width), "")))
	}
	return strings.Join(rows, "\n")
}

func (m *AppModel) renderLatestComment(author, body string, replies []review.Reply, width, height int) string {
	if len(replies) > 0 {
		latest := replies[len(replies)-1]
		author, body = latest.Author, latest.Body
	}
	name := author
	if name == "" {
		name = "anonymous"
	}
	style := m.commentAuthorStyle(author)
	content := style.Bold(true).Render(name+": ") + style.Render(m.linkFileReferences(body))
	lines := strings.Split(lipgloss.Wrap(expandDisplayTabs(content), max(1, width), ""), "\n")
	if len(lines) > height {
		lines = append(lines[:height], footerStyle.Render("↓"))
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) scrollThread(key threadViewKey, direction int, page bool) bool {
	key.path = m.tab().path
	scroll, ok := m.threadScrolls[key]
	if !ok || scroll.maxOffset == 0 {
		return false
	}
	delta := 3
	if page {
		delta = scroll.height
	}
	offset := max(0, min(scroll.maxOffset, scroll.offset+direction*delta))
	if offset == scroll.offset {
		return false
	}
	scroll.offset = offset
	scroll.manual = true
	m.threadScrolls[key] = scroll
	m.rebuildContent()
	m.updateCommentSidebar()
	return true
}
