package tui

import (
	"image/color"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/v2/internal/git"
)

type markdownRender struct {
	text, plain string
	width       int
	dark, set   bool
	palette     uint64
	foreground  string
	with        markdownInlineKey
}

func (line *markdownLine) cacheSlot(key markdownInlineKey) *markdownRender {
	if key.n == 0 {
		return &line.plainRender
	}
	return &line.inlineRender
}

// markdownInlineKey identifies a word-diff segment slice by its backing
// array, which diff information keeps stable while a tab is displayed.
type markdownInlineKey struct {
	first *gitpkg.InlineSegment
	n     int
}

func inlineKey(inline []gitpkg.InlineSegment) markdownInlineKey {
	if len(inline) == 0 {
		return markdownInlineKey{}
	}
	return markdownInlineKey{&inline[0], len(inline)}
}

func markdownTokenStyle(token markdownToken, quoted bool, foreground color.Color) lipgloss.Style {
	style := lipgloss.NewStyle()
	if quoted && token.style&markdownQuoteBar == 0 {
		style = style.Italic(true)
	}
	if token.style&markdownQuoteBar != 0 {
		style = mdBlockquoteBar.Inherit(style)
	}
	if token.style&markdownCalloutTitle != 0 {
		style = style.Background(calloutTitleBackground(token.callout)).Bold(true)
	}
	if token.syntax != 0 {
		entry := codeHighlightStyle().Get(token.syntax)
		if entry.Colour.IsSet() {
			style = style.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		style = style.Bold(entry.Bold == chroma.Yes).Italic(entry.Italic == chroma.Yes).Underline(entry.Underline == chroma.Yes)
	}
	if token.style&markdownDelimiter != 0 {
		style = mdHrStyle.Inherit(style)
	}
	if token.style&markdownFenceLanguage != 0 {
		style = mdFenceLanguageStyle.Inherit(style)
	}
	if token.style&markdownFenceAttribute != 0 {
		style = mdFenceAttributeStyle.Inherit(style)
	}
	if token.style&markdownBold != 0 {
		style = mdBoldStyle.Inherit(style)
	}
	if token.style&markdownItalic != 0 {
		style = mdItalicStyle.Inherit(style)
	}
	if token.callout != 0 {
		style = style.Foreground(calloutColor(token.callout))
	}
	if token.style&markdownLink != 0 {
		style = mdLinkStyle.Inherit(style)
	}
	if token.style&markdownCode != 0 {
		style = mdCodeStyle.Inherit(style)
	}
	if token.style&markdownKeyboard != 0 {
		style = keyboardStyle().Inherit(style)
	}
	if foreground != nil && token.callout == 0 && token.syntax == 0 && token.style&(markdownLink|markdownCode|markdownKeyboard|markdownDelimiter|markdownFenceLanguage|markdownFenceAttribute) == 0 {
		style = style.Foreground(foreground)
	}
	return style
}

func (line *markdownLine) render(inline []gitpkg.InlineSegment, base, changed lipgloss.Style, links []linkSpan) string {
	if line.thematicBreak {
		prefix := highlightMarkdownBlock(line.source[:line.ruleStart])
		width := max(1, line.previewWidth-ansi.StringWidth(prefix))
		style := mdHrStyle
		if len(inline) > 0 {
			background := base.GetBackground()
			for _, segment := range inline {
				if segment.Changed {
					background = changed.GetBackground()
					break
				}
			}
			style = style.Background(background)
		}
		return prefix + style.Render(strings.Repeat("─", width))
	}
	if line.previewTokens != nil {
		return line.renderPreview(inline, base, changed, links)
	}
	key := inlineKey(inline)
	foreground := base.GetForeground()
	foregroundKey := fgToAnsi(foreground)
	cacheable := len(links) == 0 && line.previewTokens == nil
	slot := line.cacheSlot(key)
	if cacheable && slot.set && slot.dark == terminalIsDark && slot.palette == markdownPaletteRevision && slot.foreground == foregroundKey && slot.with == key {
		return slot.text
	}
	var b strings.Builder
	segmentIndex, segmentStart := 0, 0
	for _, token := range linkedMarkdownTokens(line.displayTokens(), links) {
		if token.text == "" {
			continue
		}
		if token.style&markdownRendered != 0 {
			b.WriteString(token.text)
			continue
		}
		style := markdownTokenStyle(token, line.quoted && !line.calloutBody && !line.codeBlock, foreground)
		uri := token.uri
		write := func(value string, background lipgloss.Style) {
			tokenStyle := style
			if len(inline) > 0 {
				tokenStyle = tokenStyle.Background(background.GetBackground())
			}
			value = tokenStyle.Render(displaySourceText(value))
			if len(inline) > 0 {
				value = inlineBackground(background, value)
			}
			b.WriteString(linkedText(uri, value))
		}
		if len(inline) == 0 || token.decoration != noDecoration {
			write(token.text, base)
			continue
		}
		if token.style&markdownAtomic != 0 {
			background := base
			offset := 0
			for _, segment := range inline {
				end := offset + len(segment.Content)
				if segment.Changed && offset < token.end && end > token.start {
					background = changed
					break
				}
				offset = end
			}
			write(token.text, background)
			continue
		}
		for segmentIndex < len(inline) && segmentStart+len(inline[segmentIndex].Content) <= token.start {
			segmentStart += len(inline[segmentIndex].Content)
			segmentIndex++
		}
		offset := segmentStart
		for i := segmentIndex; i < len(inline) && offset < token.end; i++ {
			segment := inline[i]
			end := offset + len(segment.Content)
			from, to := max(offset, token.start), min(end, token.end)
			if from < to {
				background := base
				if segment.Changed {
					background = changed
				}
				value := token.text
				if len(value) == token.end-token.start {
					value = value[from-token.start : to-token.start]
				}
				write(value, background)
			}
			offset = end
		}
	}
	result := b.String()
	if !line.literal {
		result = highlightMarkdownBlock(result)
	}
	if cacheable {
		*slot = markdownRender{text: result, plain: ansi.Strip(result), width: ansi.StringWidth(result), dark: terminalIsDark, palette: markdownPaletteRevision, foreground: foregroundKey, set: true, with: key}
	}
	return result
}

// Split unchanged source tokens at link boundaries before applying styles.
// Parser replacements (entities and escapes) retain their source range.
func linkedMarkdownTokens(tokens []markdownToken, links []linkSpan) []markdownToken {
	if len(links) == 0 {
		return tokens
	}
	var result []markdownToken
	for _, token := range tokens {
		if token.decoration != noDecoration {
			result = append(result, token)
			continue
		}
		cuts := []int{token.start, token.end}
		if len(token.text) == token.end-token.start {
			for _, link := range links {
				for _, boundary := range []int{link.start, link.end} {
					if boundary > token.start && boundary < token.end {
						cuts = append(cuts, boundary)
					}
				}
			}
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		for i := 1; i < len(cuts); i++ {
			part := token
			part.start, part.end = cuts[i-1], cuts[i]
			if len(token.text) == token.end-token.start {
				part.text = token.text[part.start-token.start : part.end-token.start]
			}
			for _, link := range links {
				if part.start >= link.start && part.end <= link.end {
					// Keep explicit Markdown destinations; image previews still
					// replace their outer link as before.
					if part.uri == "" || strings.HasPrefix(link.uri, "critty://image?") {
						part.uri = link.uri
						part.style |= markdownLink
					}
				}
			}
			result = append(result, part)
		}
	}
	return result
}

func markdownDisplayRows(content string, width int, decorate func(string) (string, bool)) []string {
	if decorate != nil {
		content, _ = decorate(content)
	}
	return strings.Split(lipgloss.Wrap(content, width, ""), "\n")
}
