package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

const completionMenuRows = 10

var (
	completionMenuStyle      = lipgloss.NewStyle().Faint(true)
	completionFirstStyle     = lipgloss.NewStyle().Faint(true).Bold(true)
	completionFocusStyle     = lipgloss.NewStyle()
	completionFocusBoldStyle = lipgloss.NewStyle().Bold(true)
)

type completionItem struct {
	name string
	dir  bool
}

func (item completionItem) label() string {
	if item.dir {
		return item.name + "/"
	}
	return item.name
}

// completionToken is the @path being typed, split at the last directory
// separator before the cursor.
type completionToken struct {
	start      int    // rune index of '@' in the line
	queryStart int    // rune index after the last unescaped '/' or the '@'
	raw        string // escaped text between '@' and the cursor
	dir        string // unescaped directory part, empty or ending with '/'
	query      string // unescaped text after dir
}

type completionKey struct {
	row, column int
	line        string
}

type completionListing struct {
	dir     string
	entries []completionItem
}

type completionState struct {
	key          completionKey
	token        completionToken
	items        []completionItem
	selected     int // -1 while typing; otherwise the focused menu row
	offset       int
	dismissed    bool
	dismissedRaw string
	listing      completionListing
}

func (c completionState) active() bool {
	return len(c.items) > 0
}

// findCompletionToken locates an @path that starts the line or follows
// whitespace and reaches the cursor. Backslash pairs never end the token.
func findCompletionToken(line []rune, cursor int) (completionToken, bool) {
	for i := 0; i < cursor; i++ {
		if line[i] != '@' || (i > 0 && !unicode.IsSpace(line[i-1])) {
			continue
		}
		queryStart := i + 1
		j := i + 1
		for j < cursor {
			switch {
			case line[j] == '\\':
				j += 2
				continue
			case unicode.IsSpace(line[j]):
			default:
				if line[j] == '/' {
					queryStart = j + 1
				}
				j++
				continue
			}
			break
		}
		if j < cursor {
			i = j
			continue
		}
		if cursor == i+1 {
			return completionToken{}, false
		}
		return completionToken{
			start:      i,
			queryStart: queryStart,
			raw:        string(line[i+1 : cursor]),
			dir:        unescapeReferencePath(string(line[i+1 : queryStart])),
			query:      unescapeReferencePath(string(line[queryStart:cursor])),
		}, true
	}
	return completionToken{}, false
}

func readCompletionDir(dir string) []completionItem {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	items := make([]completionItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || strings.ContainsAny(name, "\r\n\t") {
			continue
		}
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		items = append(items, completionItem{name: name, dir: isDir})
	}
	return items
}

// filterCompletionItems keeps the directory's name order for ties and for an
// empty query, which lists everything after a directory is completed.
func filterCompletionItems(entries []completionItem, query string) []completionItem {
	var pool []completionItem
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.name, ".") && !strings.HasPrefix(query, ".") {
			continue
		}
		pool = append(pool, entry)
		names = append(names, entry.name)
	}
	if query == "" {
		return pool
	}
	matches := fuzzy.Find(query, names)
	items := make([]completionItem, len(matches))
	for i, match := range matches {
		items[i] = pool[match.Index]
	}
	return items
}

func (m *AppModel) completionItems(token completionToken) []completionItem {
	dir := m.sourcePath(token.dir)
	listing := &m.completion.listing
	if listing.dir != dir {
		*listing = completionListing{dir: dir, entries: readCompletionDir(dir)}
	}
	return filterCompletionItems(listing.entries, token.query)
}

// refreshCompletion derives the menu from the textarea after every update,
// keeping the selection while the text and cursor are unchanged.
func (m *AppModel) refreshCompletion() {
	c := &m.completion
	if !m.isTextModal() || m.modalFocus != 0 || !m.modalTextarea.Focused() {
		*c = completionState{listing: c.listing}
		return
	}
	row, column := m.modalTextarea.Line(), m.modalTextarea.Column()
	line := []rune(strings.Split(m.modalTextarea.Value(), "\n")[row])
	key := completionKey{row: row, column: column, line: string(line)}
	if key == c.key {
		return
	}
	c.key = key
	c.items, c.selected, c.offset = nil, -1, 0
	token, ok := findCompletionToken(line, column)
	if !ok {
		c.token, c.dismissed = completionToken{}, false
		return
	}
	c.token = token
	if c.dismissed && c.dismissedRaw == token.raw {
		return
	}
	c.dismissed = false
	c.items = m.completionItems(token)
}

// handleCompletionKey consumes the menu keys while candidates exist, so Tab
// completes even when the menu has no room on screen.
func (m *AppModel) handleCompletionKey(msg tea.KeyPressMsg) bool {
	c := &m.completion
	if m.modalFocus != 0 || !c.active() {
		return false
	}
	switch msg.String() {
	case "tab":
		m.acceptCompletion()
	case "enter":
		if c.selected < 0 {
			return false
		}
		m.acceptCompletion()
	case "down":
		m.moveCompletion(1)
	case "up":
		m.moveCompletion(-1)
	case "esc":
		c.dismissed, c.dismissedRaw = true, c.token.raw
		c.items, c.selected = nil, -1
	default:
		return false
	}
	return true
}

func (m *AppModel) moveCompletion(delta int) {
	c := &m.completion
	n := len(c.items)
	switch {
	case c.selected < 0 && delta > 0:
		c.selected = 0
	case c.selected < 0:
		c.selected = n - 1
	default:
		c.selected = (c.selected + delta + n) % n
	}
	if c.selected < c.offset {
		c.offset = c.selected
	}
	if c.selected >= c.offset+completionMenuRows {
		c.offset = c.selected - completionMenuRows + 1
	}
}

// acceptCompletion replaces the query with the candidate. A file ends the
// reference with a space; a directory keeps the menu open on its entries.
func (m *AppModel) acceptCompletion() {
	c := &m.completion
	item := c.items[max(0, c.selected)]
	text := escapeReferencePath(item.name)
	if item.dir {
		text += "/"
	} else {
		text += " "
	}
	for range m.modalTextarea.Column() - c.token.queryStart {
		m.modalTextarea, _ = m.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.modalTextarea.InsertString(text)
}

func renderCompletionMenu(items []completionItem, selected, offset, maxWidth int) string {
	rows := items[offset:min(len(items), offset+completionMenuRows)]
	width := 0
	for _, item := range rows {
		width = max(width, ansi.StringWidth(item.label()))
	}
	width = max(1, min(width, maxWidth-4))
	border := lipgloss.RoundedBorder()
	horizontal := strings.Repeat(border.Top, width+2)
	lines := []string{completionMenuStyle.Render(border.TopLeft + horizontal + border.TopRight)}
	for i, item := range rows {
		label := ansi.Truncate(item.label(), width, "…")
		label += strings.Repeat(" ", width-ansi.StringWidth(label))
		style := completionMenuStyle
		switch index := offset + i; {
		case index == selected && index == 0:
			style = completionFocusBoldStyle
		case index == selected:
			style = completionFocusStyle
		case index == 0:
			style = completionFirstStyle
		}
		lines = append(lines,
			completionMenuStyle.Render(border.Left+" ")+style.Render(label)+completionMenuStyle.Render(" "+border.Right))
	}
	lines = append(lines, completionMenuStyle.Render(border.BottomLeft+horizontal+border.BottomRight))
	return strings.Join(lines, "\n")
}

// completionLayer places the menu under the '@', or above it when the screen
// bottom is too close. Without room in either direction, no menu is shown.
func (m AppModel) completionLayer(regions []modalMouseRegion, width, height int) *lipgloss.Layer {
	c := m.completion
	if !c.active() {
		return nil
	}
	cursor := m.editorCursor()
	if cursor == nil {
		return nil
	}
	for _, region := range regions {
		if !region.action.textarea {
			continue
		}
		x, y := region.rect.left+cursor.X, region.rect.top+cursor.Y
		if y < region.rect.top || y >= region.rect.bottom {
			return nil
		}
		line := []rune(c.key.line)
		x -= ansi.StringWidth(string(line[c.token.start:min(len(line), c.key.column)]))
		x = max(x, region.rect.left)
		menu := renderCompletionMenu(c.items, c.selected, c.offset, width)
		menuWidth, menuHeight := lipgloss.Width(menu), lipgloss.Height(menu)
		top := y + 1
		if top+menuHeight > height {
			top = y - menuHeight
		}
		if top < 0 {
			return nil
		}
		return lipgloss.NewLayer(menu).X(max(0, min(x, width-menuWidth))).Y(top).Z(2)
	}
	return nil
}
