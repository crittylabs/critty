package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

type fileMatch struct {
	index     int
	positions []int // byte offsets of matched runes in the tab path
}

type fileSelectState struct {
	input    textinput.Model
	query    string
	matches  []fileMatch
	selected int
}

// rankFileMatches requires every space-separated word of the query to match
// a tab path, as in VS Code's file picker. Tabs whose basename matches more
// words come first, then higher basename and whole-path scores, then tab
// order. An empty query keeps every tab in order.
func rankFileMatches(paths []string, query string) []fileMatch {
	words := strings.Fields(query)
	if len(words) == 0 {
		matches := make([]fileMatch, len(paths))
		for i := range paths {
			matches[i] = fileMatch{index: i}
		}
		return matches
	}
	bases := make([]string, len(paths))
	for i, path := range paths {
		bases[i] = filepath.Base(path)
	}
	type scores struct {
		baseHits, base, path int
		positions            map[int]bool
	}
	byIndex := make(map[int]*scores, len(paths))
	for i := range paths {
		byIndex[i] = &scores{positions: make(map[int]bool)}
	}
	for _, word := range words {
		baseMatches := make(map[int]fuzzy.Match)
		for _, match := range fuzzy.Find(word, bases) {
			baseMatches[match.Index] = match
		}
		pathMatches := make(map[int]fuzzy.Match)
		for _, match := range fuzzy.Find(word, paths) {
			pathMatches[match.Index] = match
		}
		for index, s := range byIndex {
			path, ok := pathMatches[index]
			if !ok {
				delete(byIndex, index)
				continue
			}
			s.path += path.Score
			positions := path.MatchedIndexes
			if base, ok := baseMatches[index]; ok {
				s.baseHits++
				s.base += base.Score
				offset := len(paths[index]) - len(bases[index])
				positions = make([]int, len(base.MatchedIndexes))
				for i, p := range base.MatchedIndexes {
					positions[i] = p + offset
				}
			}
			for _, p := range positions {
				s.positions[p] = true
			}
		}
	}
	matches := make([]fileMatch, 0, len(byIndex))
	for index, s := range byIndex {
		match := fileMatch{index: index}
		for p := range s.positions {
			match.positions = append(match.positions, p)
		}
		sort.Ints(match.positions)
		matches = append(matches, match)
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := byIndex[matches[i].index], byIndex[matches[j].index]
		if a.baseHits != b.baseHits {
			return a.baseHits > b.baseHits
		}
		if a.base != b.base {
			return a.base > b.base
		}
		if a.path != b.path {
			return a.path > b.path
		}
		return matches[i].index < matches[j].index
	})
	return matches
}

func (m *AppModel) openFileSelect() tea.Cmd {
	input := textinput.New()
	input.SetStyles(textinput.DefaultStyles(terminalIsDark))
	input.SetVirtualCursor(false)
	input.Prompt = "> "
	input.Placeholder = "file name"
	input.SetWidth(max(1, m.modalInnerWidth()-lipgloss.Width(input.Prompt)))
	m.fileSelect = fileSelectState{input: input}
	m.refreshFileSelect()
	m.modal = fileSelectModal
	return m.fileSelect.input.Focus()
}

func (m *AppModel) closeFileSelect() {
	m.modal = noModal
	m.fileSelect.input.Blur()
}

// refreshFileSelect recomputes matches when the query changes. An empty
// query preselects the active tab so Enter is a no-op.
func (m *AppModel) refreshFileSelect() {
	s := &m.fileSelect
	query := s.input.Value()
	if query == s.query && s.matches != nil {
		return
	}
	paths := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		paths[i] = t.displayPath()
	}
	s.query, s.matches, s.selected = query, rankFileMatches(paths, query), 0
	if query == "" {
		for i, match := range s.matches {
			if match.index == m.activeTab {
				s.selected = i
			}
		}
	}
}

func (m *AppModel) pickFile(row int) {
	if row >= 0 && row < len(m.fileSelect.matches) {
		m.activeTab = m.fileSelect.matches[row].index
		m.rebuildContent()
		m.updateCommentSidebar()
	}
	m.closeFileSelect()
}

func (m *AppModel) handleFileSelectModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &m.fileSelect
	switch msg.String() {
	case "esc", "ctrl+c":
		m.closeFileSelect()
	case "enter":
		m.pickFile(s.selected)
	case "down", "ctrl+n":
		if n := len(s.matches); n > 0 {
			s.selected = (s.selected + 1) % n
		}
	case "up", "ctrl+p":
		if n := len(s.matches); n > 0 {
			s.selected = (s.selected + n - 1) % n
		}
	default:
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		m.refreshFileSelect()
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) handleFileSelectMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if !region.rect.contains(mouse) {
			continue
		}
		if region.action.close {
			m.closeFileSelect()
			return m, nil
		}
		if region.action.lineInput {
			cursor := m.fileSelect.input.Cursor()
			if cursor != nil {
				m.fileSelect.input.SetCursor(m.fileSelect.input.Position() + mouse.X - region.rect.left - cursor.X)
			}
			return m, nil
		}
		if region.action.pick {
			m.pickFile(region.action.pickIndex)
		}
		return m, nil
	}
	return m, nil
}

// fileSelectWindow keeps the selection visible with the list scrolled as
// little as possible.
func fileSelectWindow(count, selected, rows int) (int, int) {
	start := min(max(0, selected-rows/2), max(0, count-rows))
	return start, min(count, start+rows)
}

func renderFileRow(path string, positions []int, suffix string, selected bool) string {
	base, hit := lipgloss.NewStyle(), fileMatchStyle
	if selected {
		base, hit = base.Reverse(true), hit.Reverse(true)
	}
	var out strings.Builder
	var segment strings.Builder
	segmentHit := false
	flush := func() {
		if segment.Len() == 0 {
			return
		}
		style := base
		if segmentHit {
			style = hit
		}
		out.WriteString(style.Render(segment.String()))
		segment.Reset()
	}
	next := 0
	for i := 0; i < len(path); {
		_, size := utf8.DecodeRuneInString(path[i:])
		matched := next < len(positions) && positions[next] == i
		if matched {
			next++
		}
		if matched != segmentHit {
			flush()
			segmentHit = matched
		}
		segment.WriteString(path[i : i+size])
		i += size
	}
	flush()
	if suffix != "" {
		out.WriteString(base.Render(" " + suffix))
	}
	return out.String()
}

func (m AppModel) fileSelectModalContent(width, maxRows int) (string, []modalMouseRegion) {
	s := m.fileSelect
	title, closeRegion := modalTitle("Open file", width)
	prefix := title + "\n"
	content := prefix + s.input.View() + "\n\n"
	regions := []modalMouseRegion{{
		rect:   mouseRect{top: strings.Count(prefix, "\n"), bottom: strings.Count(prefix, "\n") + 1, right: width},
		action: modalMouseAction{lineInput: true},
	}}
	rows := max(1, maxRows)
	start, end := fileSelectWindow(len(s.matches), s.selected, rows)
	for row := start; row < end; row++ {
		match := s.matches[row]
		t := &m.tabs[match.index]
		line := ansi.Truncate(renderFileRow(t.displayPath(), match.positions, t.changeSummary(), row == s.selected), width, "…")
		top := strings.Count(content, "\n")
		regions = append(regions, modalMouseRegion{
			rect:   mouseRect{top: top, bottom: top + 1, right: width},
			action: modalMouseAction{pick: true, pickIndex: row},
		})
		content += line + "\n"
	}
	// Keep the dialog the same size for every query so it does not jump.
	content += strings.Repeat("\n", rows-(end-start))
	status := "No matching files"
	switch {
	case len(s.matches) > rows:
		status = fmt.Sprintf("%d-%d of %s", start+1, end, countNoun(len(s.matches), "file", "files"))
	case len(s.matches) > 0:
		status = fmt.Sprintf("%d of %s", len(s.matches), countNoun(len(m.tabs), "file", "files"))
	}
	return content + footerStyle.Render(status) + "\n" + footerStyle.Render("↑/↓ choose · enter open · esc cancel"), append(regions, closeRegion)
}
