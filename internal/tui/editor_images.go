package tui

import (
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// editorImageDelay is the typing pause after which image references in the
// comment editor are re-read, so a reference being typed is not loaded on
// every keystroke.
const editorImageDelay = 400 * time.Millisecond

const maxEditorImages = 16

type editorImageRef struct {
	line int
	path string
}

// editorImageState tracks previews under image references in the comment
// editor.  value is the text the refs were read from; pending is the latest
// text awaiting the debounce.
type editorImageState struct {
	active  bool
	value   string
	pending string
	width   int
	refs    []editorImageRef
	seq     int
}

type editorImagesMsg struct{ seq int }

// editorModal returns the comment editor that is open or waits beneath a
// confirmation or the lightbox, or noModal.
func (m AppModel) editorModal() modalType {
	modal := m.modal
	switch modal {
	case discardChangesModal:
		modal = m.discardReturn
	case deleteConfirmModal:
		modal = m.deleteReturn
	case lightboxModal:
		modal = m.lightbox.returnTo
	}
	switch modal {
	case commentModal, fileCommentModal, replyModal, editModal:
		return modal
	}
	return noModal
}

func (m AppModel) editorImagesActive() bool {
	return m.editorModal() != noModal && len(m.editorImages.refs) > 0
}

// scheduleEditorImages reads image references when the editor opens and,
// after edits, once typing pauses.
func (m *AppModel) scheduleEditorImages() tea.Cmd {
	if m.editorModal() == noModal {
		if m.editorImages.active {
			m.editorImages = editorImageState{}
		}
		return nil
	}
	value := m.modalTextarea.Value()
	if !m.editorImages.active {
		m.editorImages.active = true
		m.applyEditorImages(value)
		return nil
	}
	if m.editorImages.width != m.modalTextarea.Width() {
		m.applyEditorImages(m.editorImages.value)
	}
	if value == m.editorImages.value || value == m.editorImages.pending {
		return nil
	}
	m.editorImages.pending = value
	m.editorImages.seq++
	seq := m.editorImages.seq
	return tea.Tick(editorImageDelay, func(time.Time) tea.Msg { return editorImagesMsg{seq} })
}

func (m *AppModel) finishEditorImagesDelay(msg editorImagesMsg) {
	if msg.seq == m.editorImages.seq && m.editorImages.active && m.editorModal() != noModal {
		m.applyEditorImages(m.editorImages.pending)
	}
}

func (m *AppModel) applyEditorImages(value string) {
	s := &m.editorImages
	s.value, s.pending, s.width, s.refs = value, value, m.modalTextarea.Width(), nil
	for _, ref := range markdownImageRefs(value) {
		if len(s.refs) >= maxEditorImages {
			break
		}
		path, ok := m.commentImagePath(ref.destination)
		if !ok {
			continue
		}
		s.refs = append(s.refs, editorImageRef{line: ref.line, path: path})
		m.commentImageSide(commentImageKey{path, s.width})
	}
}

// editorImageBlocks returns the preview rows to insert after each wrapped
// text row, keyed by that row's index in the whole editor text.
func (m AppModel) editorImageBlocks() map[int][]string {
	s := m.editorImages
	if !s.active || !m.images.enabled || m.images.hidden || len(s.refs) == 0 {
		return nil
	}
	lineEnds := textareaLineEnds(m.modalTextarea)
	byLine := make(map[int][]string)
	for _, ref := range s.refs {
		side := m.commentImages[commentImageKey{ref.path, s.width}]
		if side == nil {
			continue
		}
		if side.failed {
			byLine[ref.line] = append(byLine[ref.line], ansi.Truncate("  Image preview unavailable", s.width, ""))
		} else if side.image != nil && side.image.ready {
			byLine[ref.line] = append(byLine[ref.line], side.image.linkedPlaceholder(imageTarget{path: ref.path}.uri()))
		}
	}
	blocks := make(map[int][]string)
	indent := strings.Repeat(" ", lipgloss.Width(m.modalTextarea.Prompt))
	for line, items := range byLine {
		if line < 1 || line > len(lineEnds) {
			continue
		}
		for _, row := range packImageBlocks(items, s.width) {
			for _, text := range strings.Split(row, "\n") {
				blocks[lineEnds[line-1]] = append(blocks[lineEnds[line-1]], indent+text)
			}
		}
	}
	return blocks
}

// editorView renders the editor with previews under the visible rows of
// their references, and returns each view row's text row, -1 for previews.
func (m AppModel) editorView() (string, []int) {
	view := m.modalTextarea.View()
	blocks := m.editorImageBlocks()
	rows := strings.Split(view, "\n")
	if len(blocks) == 0 {
		textRows := make([]int, len(rows))
		for i := range rows {
			textRows[i] = i
		}
		return view, textRows
	}
	offset := m.modalTextarea.ScrollYOffset()
	var out []string
	var textRows []int
	for i, row := range rows {
		out = append(out, row)
		textRows = append(textRows, i)
		for _, block := range blocks[offset+i] {
			out = append(out, block)
			textRows = append(textRows, -1)
		}
	}
	return strings.Join(out, "\n"), textRows
}

// editorTextRow maps a row of the rendered editor to the visible text row
// it shows, or -1 for a preview row.
func (m AppModel) editorTextRow(viewRow int) int {
	_, textRows := m.editorView()
	if viewRow < 0 || viewRow >= len(textRows) {
		return viewRow
	}
	return textRows[viewRow]
}

// editorCursor returns the editor's terminal cursor, moved past preview
// rows inserted above it.
func (m AppModel) editorCursor() *tea.Cursor {
	cursor := m.modalTextarea.Cursor()
	if cursor == nil || m.editorModal() == noModal {
		return cursor
	}
	_, textRows := m.editorView()
	for i, row := range textRows {
		if row == cursor.Y {
			cursor.Y = i
			break
		}
	}
	return cursor
}

// textareaLineEnds returns, for each line of the text, the index of its last
// wrapped row in the editor.
func textareaLineEnds(ta textarea.Model) []int {
	width := ta.Width()
	ends := make([]int, 0, ta.LineCount())
	row := -1
	for _, line := range strings.Split(ta.Value(), "\n") {
		row += len(textareaWrap([]rune(line), width))
		ends = append(ends, row)
	}
	return ends
}

// textareaWrap mirrors the soft wrapping of charm.land/bubbles textarea so
// previews can be placed after the rows of a line.  Keep in step with the
// textarea's wrap function; textareaLineEnds is checked against the real
// textarea in tests.
func textareaWrap(runes []rune, width int) [][]rune {
	var (
		lines  = [][]rune{{}}
		word   = []rune{}
		row    int
		spaces int
	)
	spaceRunes := func(n int) []rune { return []rune(strings.Repeat(" ", n)) }
	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}
		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
			}
			lines[row] = append(lines[row], word...)
			lines[row] = append(lines[row], spaceRunes(spaces)...)
			spaces = 0
			word = nil
		} else {
			lastCharLen := runewidth.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}
	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		spaces++
		lines[row+1] = append(lines[row+1], spaceRunes(spaces)...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], spaceRunes(spaces)...)
	}
	return lines
}
