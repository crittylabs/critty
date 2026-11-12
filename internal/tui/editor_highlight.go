package tui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Color the textarea's own cells, preserving its wrapping, selection, padding
// and cursor coordinates.  The editable value never contains ANSI escapes.
func (m *AppModel) highlightEditor(view string) string {
	value := m.modalTextarea.Value()
	if value == "" || (!strings.Contains(value, "```") && !strings.Contains(value, "~~~")) {
		return view
	}
	lexer := m.commentSuggestionLexer()
	if m.modal == finishModal {
		lexer = nil
	}
	source := m.parsedComment(value, lexer).source
	type wrappedRow struct {
		line, start int
		text        string
	}
	var wrapped []wrappedRow
	kinds := make([][]chroma.TokenType, len(source.lines))
	for i, line := range source.lines {
		kinds[i] = make([]chroma.TokenType, utf8.RuneCountInString(line.source))
		for _, token := range line.tokens {
			if token.syntax == 0 {
				continue
			}
			from := utf8.RuneCountInString(line.source[:token.start])
			to := from + utf8.RuneCountInString(token.text)
			for j := from; j < to; j++ {
				kinds[i][j] = token.syntax
			}
		}
		start := 0
		for _, runes := range textareaWrap([]rune(line.source), m.modalTextarea.Width()) {
			text := string(runes)
			if ansi.StringWidth(text) > m.modalTextarea.Width() {
				text = strings.TrimSuffix(text, " ")
			}
			wrapped = append(wrapped, wrappedRow{i, start, text})
			start += utf8.RuneCountInString(text)
		}
	}
	selectionStart, selectionEnd, selected := m.modalTextarea.Selection()
	promptWidth := lipgloss.Width(m.modalTextarea.Prompt)
	rows := uv.NewStyledString(view).Lines(ansi.GraphemeWidth)
	palette := codeHighlightStyle()
	var out strings.Builder
	for y, cells := range rows {
		if y > 0 {
			out.WriteByte('\n')
		}
		index := m.modalTextarea.ScrollYOffset() + y
		byteOffset, runeOffset := 0, 0
		for x, cell := range cells {
			if cell.Content == "" {
				continue
			}
			if index < len(wrapped) && x >= promptWidth {
				row := wrapped[index]
				column := row.start + runeOffset
				matches := strings.HasPrefix(row.text[byteOffset:], cell.Content)
				inSelection := selected && (row.line > selectionStart.Row || row.line == selectionStart.Row && column+utf8.RuneCountInString(cell.Content) > selectionStart.Col) &&
					(row.line < selectionEnd.Row || row.line == selectionEnd.Row && column < selectionEnd.Col)
				if matches && column < len(kinds[row.line]) && !inSelection {
					if kind := kinds[row.line][column]; kind != 0 {
						entry := palette.Get(kind)
						if entry.Colour.IsSet() {
							cell.Style.Fg = lipgloss.Color(entry.Colour.String())
						}
						if entry.Bold == chroma.Yes {
							cell.Style.Attrs |= uv.AttrBold
						}
						if entry.Italic == chroma.Yes {
							cell.Style.Attrs |= uv.AttrItalic
						}
					}
				}
				if matches {
					byteOffset += len(cell.Content)
				} else {
					byteOffset = len(row.text)
				}
				runeOffset += utf8.RuneCountInString(cell.Content)
			}
			out.WriteString(cell.Style.Styled(cell.Content))
		}
	}
	return out.String()
}
