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
	styles := make([][]uv.Style, len(source.lines))
	for i, line := range source.lines {
		styles[i] = editorLineStyles(&line)
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
				if matches && column < len(styles[row.line]) && !inSelection {
					style := styles[row.line][column]
					if style.Fg != nil {
						cell.Style.Fg = style.Fg
					}
					cell.Style.Attrs |= style.Attrs
					if style.Underline != 0 {
						cell.Style.Underline = style.Underline
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

func editorLineStyles(line *markdownLine) []uv.Style {
	var result []uv.Style
	palette := codeHighlightStyle()
	tokens := line.tokens
	if line.markdownSuggestion {
		tokens = line.sourceTokens()
	}
	for _, token := range tokens {
		var style uv.Style
		if line.markdownSuggestion {
			syntax := markdownTokenStyle(token, false, nil).Inherit(markdownHeadingStyle(line.headingLevel))
			if fg := syntax.GetForeground(); fg != (lipgloss.NoColor{}) {
				style.Fg = fg
			}
			if syntax.GetBold() {
				style.Attrs |= uv.AttrBold
			}
			if syntax.GetItalic() {
				style.Attrs |= uv.AttrItalic
			}
			style.Underline = uv.Underline(syntax.GetUnderlineStyle())
		} else if token.syntax != 0 {
			entry := palette.Get(token.syntax)
			if entry.Colour.IsSet() {
				style.Fg = lipgloss.Color(entry.Colour.String())
			}
			if entry.Bold == chroma.Yes {
				style.Attrs |= uv.AttrBold
			}
			if entry.Italic == chroma.Yes {
				style.Attrs |= uv.AttrItalic
			}
		}
		if style.IsZero() {
			continue
		}
		if result == nil {
			result = make([]uv.Style, utf8.RuneCountInString(line.source))
		}
		from := utf8.RuneCountInString(line.source[:token.start])
		to := from + utf8.RuneCountInString(line.source[token.start:token.end])
		for i := from; i < to; i++ {
			result[i] = style
		}
	}
	return result
}
