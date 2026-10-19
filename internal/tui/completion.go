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

	"github.com/crittylabs/critty/internal/form"
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

// completionToken is the path being typed, split at the last directory
// separator before the cursor.
type completionToken struct {
	start      int    // rune index of '@', or zero for a path field
	queryStart int    // rune index at the start of the basename
	raw        string // text up to the cursor, escaped only for @references
	dir        string // unescaped directory part, empty or ending with '/'
	query      string // unescaped text after dir
	plain      bool   // a path field, without @ or reference escaping
}

type completionKey struct {
	row, column int
	line        string
	kind        form.InputType
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

func findPathCompletionToken(line []rune, cursor int) completionToken {
	start := 0
	for i, r := range line[:cursor] {
		if r == '/' {
			start = i + 1
		}
	}
	return completionToken{plain: true, queryStart: start, raw: string(line[:cursor]),
		dir: string(line[:start]), query: string(line[start:cursor])}
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

func (m *AppModel) completionItems(token completionToken, kind form.InputType) []completionItem {
	dir := token.dir
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
		}
	}
	dir = m.sourcePath(dir)
	listing := &m.completion.listing
	if listing.dir != dir {
		*listing = completionListing{dir: dir, entries: readCompletionDir(dir)}
	}
	items := filterCompletionItems(listing.entries, token.query)
	if kind != form.InputDirectory {
		return items
	}
	var dirs []completionItem
	for _, item := range items {
		if item.dir {
			dirs = append(dirs, item)
		}
	}
	return dirs
}

// refreshCompletion derives the menu from the textarea after every update,
// keeping the selection while the text and cursor are unchanged.
func (m *AppModel) refreshCompletion() {
	m.refreshPathInfo()
	c := &m.completion
	if !m.isTextModal() || m.modalFocus != 0 || !m.modalTextarea.Focused() {
		*c = completionState{listing: c.listing}
		return
	}
	row, column := m.modalTextarea.Line(), m.modalTextarea.Column()
	line := []rune(strings.Split(m.modalTextarea.Value(), "\n")[row])
	kind := m.pathInputType()
	key := completionKey{row: row, column: column, line: string(line), kind: kind}
	if key == c.key {
		return
	}
	c.key = key
	c.items, c.selected, c.offset = nil, -1, 0
	token, ok := findCompletionToken(line, column)
	if kind != "" {
		token, ok = findPathCompletionToken(line, column), true
	}
	if !ok {
		c.token, c.dismissed = completionToken{}, false
		return
	}
	c.token = token
	if c.dismissed && c.dismissedRaw == token.raw {
		return
	}
	c.dismissed = false
	c.items = m.completionItems(token, kind)
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
	if c.token.plain {
		text = item.name
		// Replace the rest of this component when the cursor is within it.
		line := []rune(c.key.line)
		end := c.key.column
		for end < len(line) && line[end] != '/' {
			end++
		}
		if item.dir && end < len(line) && line[end] == '/' {
			end++
		}
		for range end - c.key.column {
			m.modalTextarea, _ = m.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
		}
	}
	if item.dir {
		text += "/"
	} else if !c.token.plain {
		text += " "
	}
	for range m.modalTextarea.Column() - c.token.queryStart {
		m.modalTextarea, _ = m.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.modalTextarea.InsertString(text)
	if c.token.plain {
		m.resizePathInput()
		if !item.dir {
			c.dismissed = true
			c.dismissedRaw = string([]rune(m.modalTextarea.Value())[:m.modalTextarea.Column()])
			c.items, c.selected = nil, -1
		}
	}
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

// completionLayout places the menu under the path, or above it when the
// screen bottom is too close, and shares its bounds with mouse hit testing.
func (m AppModel) completionLayout(regions []modalMouseRegion, width, height int) (string, mouseRect) {
	c := m.completion
	if !c.active() || (m.pathInputType() != "" && m.clipboardStatus != "") {
		return "", mouseRect{}
	}
	cursor := m.editorCursor()
	if cursor == nil {
		return "", mouseRect{}
	}
	for _, region := range regions {
		if !region.action.textarea {
			continue
		}
		x, y := region.rect.left+cursor.X, region.rect.top+cursor.Y
		if y < region.rect.top || y >= region.rect.bottom {
			return "", mouseRect{}
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
			return "", mouseRect{}
		}
		left := max(0, min(x, width-menuWidth))
		return menu, mouseRect{left: left, right: left + menuWidth, top: top, bottom: top + menuHeight}
	}
	return "", mouseRect{}
}

func (m AppModel) completionLayer(regions []modalMouseRegion, width, height int) *lipgloss.Layer {
	menu, rect := m.completionLayout(regions, width, height)
	if menu == "" {
		return nil
	}
	return lipgloss.NewLayer(menu).X(rect.left).Y(rect.top).Z(2)
}

func (m *AppModel) handleCompletionClick(mouse tea.Mouse) bool {
	if !m.completion.active() {
		return false
	}
	_, rect := m.completionLayout(m.modalMouseRegions(), m.width, m.height)
	if !rect.contains(mouse) {
		return false
	}
	index := m.completion.offset + mouse.Y - rect.top - 1
	if mouse.Y > rect.top && mouse.Y < rect.bottom-1 && index < len(m.completion.items) {
		m.completion.selected = index
		m.acceptCompletion()
	}
	return true
}
