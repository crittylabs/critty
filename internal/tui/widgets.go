package tui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/icons"
)

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalBtnFocusedLabel, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalBtnNormalLabel, modalBtnNormalKey)
}

func (m AppModel) renderPrimaryModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalBtnPrimaryFocusedLabel, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalBtnPrimaryLabel, modalBtnNormalKey)
}

func (m AppModel) renderDangerModalButton(label, hint string, focused bool) string {
	if focused {
		return renderButton(label, hint, modalDangerBtnFocused, modalBtnFocusedKey)
	}
	return renderButton(label, hint, modalDangerBtnLabel, modalBtnNormalKey)
}

// renderBarButton renders a button for the banner or footer.  Its label is
// bold so it stands out from the surrounding key hints.
func renderBarButton(label, hint string) string {
	return renderButton(label, hint, modalBtnFocusedLabel.Bold(true), modalBtnFocusedKey)
}

// renderAccessKeyBarButton highlights the first letter with the bar key style.
func renderAccessKeyBarButton(label string) string {
	_, size := utf8.DecodeRuneInString(label)
	style := modalBtnFocusedLabel.Padding(0)
	return style.Render(" ") + modalBtnFocusedKey.Render(label[:size]) + style.Render(label[size:]+" ")
}

// renderAccessKey underlines the first character of label to show that it
// is the key that triggers the control.
func renderAccessKey(label string, style lipgloss.Style) string {
	_, size := utf8.DecodeRuneInString(label)
	return style.Underline(true).Render(label[:size]) + style.Padding(0).Render(label[size:])
}

func renderButton(label, hint string, labelStyle, keyStyle lipgloss.Style) string {
	if hint == "" {
		return labelStyle.Render(label + " ")
	}
	keys := strings.Split(keyHintNames.Replace(hint), " / ")
	var renderedHint strings.Builder
	for i, key := range keys {
		if i > 0 {
			renderedHint.WriteString(labelStyle.Padding(0).Render(" / "))
		}
		renderedHint.WriteString(keyStyle.Render(key))
	}
	return labelStyle.Render(label+" ") + renderedHint.String()
}

func layoutModalTextarea(before, textareaView string, width int) (string, modalMouseRegion) {
	before = lipgloss.Wrap(before, width, "")
	textareaView = lipgloss.Wrap(textareaView, width, "")
	top := strings.Count(before, "\n")
	return before + textareaView + "\n\n", modalMouseRegion{
		rect: mouseRect{
			left: 0, top: top,
			right: min(width, lipgloss.Width(textareaView)), bottom: top + lipgloss.Height(textareaView),
		},
		action: modalMouseAction{textarea: true},
	}
}

func renderScrollableModalBox(content string, width, maxHeight, offset int) (string, int, int) {
	return renderModalBoxAt(content, width, maxHeight, offset, -1)
}

func renderModalBoxAt(content string, width, maxHeight, offset, initialOffset int) (string, int, int) {
	if content == "" || maxHeight < 3 {
		return "", 0, 0
	}

	contentWidth := max(1, width-2)
	// Measure and render with the same tab width; lipgloss expands tabs only when rendering.
	wrapped := lipgloss.Wrap(expandDisplayTabs(content), contentWidth, "")
	lines := strings.Split(wrapped, "\n")
	maxContentHeight := maxHeight - 2
	maxOffset := max(0, len(lines)-maxContentHeight)
	if initialOffset < 0 {
		initialOffset = maxOffset
	}
	if offset < 0 {
		offset = initialOffset
	}
	offset = min(offset, maxOffset)
	moreBelow := offset+maxContentHeight < len(lines)
	lines = lines[offset:min(len(lines), offset+maxContentHeight)]

	borderStyle := lipgloss.NewStyle().Foreground(subtle)
	topBorder, bottomBorder := strings.Repeat("─", contentWidth), strings.Repeat("─", contentWidth)
	if offset > 0 {
		topBorder = "↑" + strings.Repeat("─", contentWidth-1)
	}
	if moreBelow {
		bottomBorder = "↓" + strings.Repeat("─", contentWidth-1)
	}
	top := borderStyle.Render("╭" + topBorder + "╮")
	bottom := borderStyle.Render("╰" + bottomBorder + "╯")
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, top)
	for _, line := range lines {
		line = ansi.Truncate(line, contentWidth, "")
		rows = append(rows, borderStyle.Render("│")+
			lipgloss.NewStyle().Width(contentWidth).Render(line)+borderStyle.Render("│"))
	}
	rows = append(rows, bottom)

	return strings.Join(rows, "\n"), offset, maxOffset
}

func (m AppModel) modalCloseButton() string {
	return closeButtonStyle.Render(m.icons.Glyph(icons.Close, "x"))
}

// modalTitleRow renders a dialog title with a close button at the right edge of
// the dialog body, returning the row and the region that closes the dialog.
func (m AppModel) modalTitleRow(title string, width int) (string, modalMouseRegion) {
	closeButton := m.modalCloseButton()
	closeWidth := lipgloss.Width(closeButton)
	title = ansi.Truncate(title, max(0, width-1-closeWidth), "…")
	gap := max(1, width-lipgloss.Width(title)-closeWidth)
	region := modalMouseRegion{
		rect:   mouseRect{left: width - closeWidth, right: width, bottom: 1},
		action: modalMouseAction{close: true, focus: -1}, // not a button focus index
	}
	return title + strings.Repeat(" ", gap) + closeButton, region
}

// modalTitle renders a dialog title row with a close button followed by a blank
// line, as modalTitleStyle's bottom margin used to provide.
func (m AppModel) modalTitle(text string, width int) (string, modalMouseRegion) {
	row, region := m.modalTitleRow(modalTitleStyle.MarginBottom(0).Render(text), width)
	return row + "\n", region
}
