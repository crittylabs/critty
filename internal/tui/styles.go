package tui

import (
	"fmt"
	"image/color"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

func renderBrand() string {
	return renderBrandFor(terminalIsDark)
}

// BrandVersion renders the Critty brand mark followed by the build version,
// as shown in the Help dialog footer, for a dark or light background.
func BrandVersion(version string, dark bool) string {
	brand := renderBrandFor(dark)
	if version == "" {
		return brand
	}
	ld := lipgloss.LightDark(dark)
	return brand + lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Blue, lipgloss.BrightBlue)).Render(version)
}

func renderBrandFor(dark bool) string {
	ld := lipgloss.LightDark(dark)
	tile := lipgloss.NewStyle().Bold(true).Background(lipgloss.BrightBlack)
	word := lipgloss.NewStyle().Bold(true).
		Foreground(ld(lipgloss.Black, lipgloss.BrightWhite)).
		Background(ld(lipgloss.BrightWhite, lipgloss.Black))
	return tile.Foreground(lipgloss.Color("#3fb950")).Render(">") +
		tile.Foreground(lipgloss.BrightWhite).Render("_") +
		word.Render(" crit") +
		word.Foreground(ld(lipgloss.Color("#2ea043"), lipgloss.Color("#3fb950"))).Render("ty ")
}

func (m *AppModel) commentAuthorStyle(author string) lipgloss.Style {
	if author == m.author {
		return lipgloss.NewStyle().Foreground(textPrimary)
	}
	colors := [...]color.Color{lipgloss.Cyan, lipgloss.Green, lipgloss.Color("208"), lipgloss.Magenta, lipgloss.Yellow}
	if m.authorColors == nil {
		m.authorColors = make(map[string]int)
	}
	index, ok := m.authorColors[author]
	if !ok {
		index = len(m.authorColors)
		m.authorColors[author] = index
	}
	return lipgloss.NewStyle().Foreground(colors[index%len(colors)])
}

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

var (
	activeTabBorder   = tabBorderWithBottom("┘", " ", "└")
	inactiveTabBorder = tabBorderWithBottom("┴", "─", "┴")
)

// ANSI 16 named colors — these adapt to the user's terminal color scheme.
// initAdaptiveStyles replaces success and warning with textColors' shades on
// a light background, where the ANSI colors are too pale to read.
var (
	subtle                 = lipgloss.BrightBlack
	accent                 = lipgloss.Magenta
	textColors             = newTextPalette(lipgloss.LightDark(true))
	success    color.Color = lipgloss.Green
	warning    color.Color = lipgloss.Yellow
	// activeAttachmentColor marks the current attachment in the file tree.
	activeAttachmentColor color.Color = lipgloss.Yellow
	muted                             = lipgloss.White // normal white (dimmer than BrightWhite)

	// textPrimary and textSecondary are the foregrounds of emphasized and
	// regular text on the terminal background.  initAdaptiveStyles swaps
	// them for dark colors on a light background.
	textPrimary      color.Color = lipgloss.BrightWhite
	textSecondary    color.Color = muted
	buttonBackground color.Color = lipgloss.Color("#3a3a3a")

	// terminalIsDark records the background probe's answer for inputs that
	// are created later and pick their default styles from it.
	terminalIsDark = true

	commentBorderColor        = lipgloss.BrightBlack
	commentFocusedBorderColor = lipgloss.BrightBlue

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite).
			Background(accent).
			Padding(0, 1)

	reviewBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightWhite).
			Background(lipgloss.Red)

	headerPathStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite).
			Background(accent)

	commentStyle = lipgloss.NewStyle().
			Foreground(muted).
			PaddingLeft(1)

	commentLineStyle = lipgloss.NewStyle().
				Foreground(subtle)

	footerStyle = lipgloss.NewStyle().
			Foreground(subtle)

	// Footer key hints sit on a gray background so they read as keycaps,
	// unlike the yellow keys of clickable buttons.
	footerKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Cyan).
			Background(lipgloss.BrightBlack)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(muted)

	helpHeadingStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.BrightYellow)

	helpLegendStyle = lipgloss.NewStyle().Foreground(lipgloss.Blue)

	// Search matches keep the default foreground on a colored background.
	isearchMatchStyle   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Cyan)
	isearchCurrentStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("208"))
	isearchPromptStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan)
	isearchFailingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red)

	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightCyan)

	helpDescriptionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.BrightWhite)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2).
			Width(60)

	closeButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Red).
				Background(buttonBackground)

	commentNavButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Cyan).
				Background(buttonBackground).
				Bold(true)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			MarginBottom(1)

	lineNumStyle = lipgloss.NewStyle().
			Foreground(subtle).
			Width(5).
			Align(lipgloss.Right)

	cursorLineNumStyle = lipgloss.NewStyle().
				Foreground(warning).
				Bold(true).
				Width(5).
				Align(lipgloss.Right)

	selectedLineNumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.BrightMagenta).
				Bold(true).
				Width(5).
				Align(lipgloss.Right)

	cursorMarker = lipgloss.NewStyle().
			Foreground(warning).
			Bold(true)

	commentGutterMarker = lipgloss.NewStyle().
				Foreground(warning).
				Bold(true)

	selectedMarker = lipgloss.NewStyle().
			Foreground(lipgloss.BrightMagenta).
			Bold(true)

	// Inline annotation box styles
	inlineCommentBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(commentBorderColor).
				Foreground(muted).
				PaddingLeft(1).
				PaddingRight(1)

	inlineLabelComment = lipgloss.NewStyle().
				Foreground(lipgloss.Blue).
				Bold(true)

	resolvedBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Green).
			Bold(true)

	// Annotation gutter marker
	annotationGutter = lipgloss.NewStyle().
				Foreground(lipgloss.BrightCyan).
				Bold(true)

	gutterOverlap = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	// Continuation line gutter (for wrapped lines)
	continuationGutter = lipgloss.NewStyle().
				Foreground(subtle).
				Width(5).
				Align(lipgloss.Right).
				Render("↪")

	// Markdown syntax highlighting styles
	mdH1Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Magenta)

	mdH2Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightMagenta)

	mdH3Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Cyan)

	mdH4Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Cyan).
			Italic(true)

	mdBoldStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite)

	mdItalicStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(textPrimary)

	mdCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightYellow)

	mdListMarkerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Magenta).
				Bold(true)

	formChip = newFormChipPalette(lipgloss.LightDark(true))

	progressColors = newProgressPalette()

	mdCheckboxOpen = lipgloss.NewStyle().
			Foreground(subtle)

	mdCheckboxDone = lipgloss.NewStyle().
			Foreground(success).
			Bold(true)

	mdCheckboxDoneText = lipgloss.NewStyle().
				Foreground(subtle).
				Strikethrough(true)

	mdBlockquoteBar = lipgloss.NewStyle().
			Foreground(lipgloss.Magenta).
			Bold(true)

	mdBlockquoteStyle = lipgloss.NewStyle().
				Foreground(textPrimary).
				Italic(true)

	mdHrStyle = lipgloss.NewStyle().
			Foreground(subtle)

	mdLinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightCyan).
			Underline(true)

	mdTablePipe = lipgloss.NewStyle().
			Foreground(subtle)

	mdTableSepStyle = lipgloss.NewStyle().
			Foreground(subtle)

	mdTableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Magenta).
				Bold(true)

	mdTableCellStyle = lipgloss.NewStyle().
				Foreground(muted)

	// Modal button styles
	modalBtnLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.BrightWhite)

	modalBtnNormal = lipgloss.NewStyle().
			Background(buttonBackground).
			Padding(0, 1)

	modalBtnFocusedLabel = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Black).
				Background(lipgloss.White).
				PaddingLeft(1)

	modalBtnNormalLabel = modalBtnLabel.
				Background(buttonBackground).
				PaddingLeft(1)

	modalBtnFocusedKey = lipgloss.NewStyle().
				Background(lipgloss.BrightYellow).
				Foreground(lipgloss.Black).
				Bold(true)

	modalBtnNormalKey = lipgloss.NewStyle().
				Background(lipgloss.Yellow).
				Foreground(lipgloss.Black).
				Bold(true)

	modalBtnPrimaryLabel, modalBtnPrimaryFocusedLabel = newModalPrimaryStyles(lipgloss.LightDark(true))
	modalDangerBtnLabel, modalDangerBtnFocused        = newModalDangerStyles(lipgloss.LightDark(true))

	// Tab bar styles — bordered tabs with open-bottom active tab
	inactiveTabStyle = lipgloss.NewStyle().
				Border(inactiveTabBorder, true).
				BorderForeground(accent).
				Foreground(muted).
				Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Border(activeTabBorder, true).
			BorderForeground(accent).
			Foreground(lipgloss.BrightWhite).
			Padding(0, 1)

	fileMatchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(warning)

	// Diff gutter markers
	diffAddedGutter = lipgloss.NewStyle().
			Foreground(lipgloss.Green).
			Bold(true)

	diffDeletedGutter = lipgloss.NewStyle().
				Foreground(lipgloss.Red).
				Bold(true)

	diffDeletedLineNum = lipgloss.NewStyle().
				Foreground(lipgloss.Red).
				Width(5).
				Align(lipgloss.Right)

	// Change count in tab labels
	tabAddedCount = lipgloss.NewStyle().
			Foreground(lipgloss.Green)
	tabDeletedCount = lipgloss.NewStyle().
			Foreground(lipgloss.Red)
	tabCountDelimiter = lipgloss.NewStyle().
				Foreground(muted)

	// Context box in comment/edit modals
	contextBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Foreground(lipgloss.BrightWhite)

	// Background-dependent styles — initialized with dark defaults,
	// updated at runtime via tea.BackgroundColorMsg.
	selectedLineBg = lipgloss.NewStyle().
			Faint(true)

	sidebarHighlightBg = lipgloss.NewStyle().
				Reverse(true)

	diffChangedLineBg = lipgloss.NewStyle()
	diffDeletedLineBg = lipgloss.NewStyle()
	diffCommonTextBg  = lipgloss.NewStyle()
	diffAddedTextBg   = lipgloss.NewStyle()
	diffDeletedTextBg = lipgloss.NewStyle()
	// rankMovedUp and rankMovedDown tint the line numbers of ranked items
	// displayed above or below their source position, in yellow and blue
	// rather than the diff colors so a lower rank does not read as removal.
	rankMovedUp   = lipgloss.NewStyle()
	rankMovedDown = lipgloss.NewStyle()

	visualModeIndicator = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.BrightMagenta).
				Reverse(true).
				Padding(0, 1)
)

// initAdaptiveStyles updates background-dependent styles based on terminal background.
func initAdaptiveStyles(hasDarkBG bool) {
	ld := lipgloss.LightDark(hasDarkBG)
	terminalIsDark = hasDarkBG
	textPrimary = ld(lipgloss.Black, lipgloss.BrightWhite)
	textSecondary = ld(lipgloss.BrightBlack, muted)
	textColors = newTextPalette(ld)
	success = textColors.green
	warning = textColors.yellow
	mdCheckboxDone = mdCheckboxDone.Foreground(success)
	fileMatchStyle = fileMatchStyle.Foreground(warning)
	activeAttachmentColor = ld(textColors.brightYellow, lipgloss.Yellow)
	mdH3Style = mdH3Style.Foreground(textColors.cyan)
	mdH4Style = mdH4Style.Foreground(textColors.cyan)
	mdLinkStyle = mdLinkStyle.Foreground(textColors.brightCyan)
	helpHeadingStyle = helpHeadingStyle.Foreground(textColors.brightYellow)
	helpKeyStyle = helpKeyStyle.Foreground(textColors.brightCyan)
	annotationGutter = annotationGutter.Foreground(textColors.brightCyan)
	isearchPromptStyle = isearchPromptStyle.Foreground(textColors.cyan)
	resolvedBadge = resolvedBadge.Foreground(textColors.green)
	diffAddedGutter = diffAddedGutter.Foreground(textColors.green)
	diffDeletedGutter = diffDeletedGutter.Foreground(textColors.red)
	diffDeletedLineNum = diffDeletedLineNum.Foreground(textColors.red)
	tabAddedCount = tabAddedCount.Foreground(textColors.green)
	tabDeletedCount = tabDeletedCount.Foreground(textColors.red)
	buttonBackground = ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#3a3a3a"))
	closeButtonStyle = closeButtonStyle.Background(buttonBackground)
	commentNavButtonStyle = commentNavButtonStyle.Foreground(textColors.cyan).Background(buttonBackground)
	modalBtnNormal = modalBtnNormal.Background(buttonBackground)
	commentStyle = commentStyle.Foreground(textSecondary)
	footerDescStyle = footerDescStyle.Foreground(textSecondary)
	helpDescriptionStyle = helpDescriptionStyle.Foreground(textPrimary)
	helpLegendStyle = helpLegendStyle.Foreground(ld(lipgloss.Blue, lipgloss.BrightBlue))
	inlineCommentBox = inlineCommentBox.Foreground(textSecondary)
	mdBoldStyle = mdBoldStyle.Foreground(textPrimary)
	mdItalicStyle = mdItalicStyle.Foreground(textPrimary)
	mdBlockquoteStyle = mdBlockquoteStyle.Foreground(textPrimary)
	mdTableCellStyle = mdTableCellStyle.Foreground(textSecondary)
	modalBtnLabel = modalBtnLabel.Foreground(textPrimary)
	modalBtnNormalLabel = modalBtnLabel.PaddingLeft(1).Background(buttonBackground)
	inactiveTabStyle = inactiveTabStyle.Foreground(textSecondary)
	activeTabStyle = activeTabStyle.Foreground(textPrimary)
	tabCountDelimiter = tabCountDelimiter.Foreground(textSecondary)
	contextBoxStyle = contextBoxStyle.Foreground(textPrimary)
	cursorColor := ld(textColors.brightYellow, warning)
	cursorLineNumStyle = cursorLineNumStyle.Foreground(cursorColor)
	cursorMarker = cursorMarker.Foreground(cursorColor)
	commentGutterMarker = commentGutterMarker.Foreground(cursorColor)
	// Black on yellow looks smudged on a light background; use white keys
	// on dark yellow there instead.
	keyForeground := ld(lipgloss.Color("#FFFFFF"), lipgloss.Black)
	modalBtnNormalKey = modalBtnNormalKey.
		Foreground(keyForeground).
		Background(ld(lipgloss.Color("#A38A00"), lipgloss.Yellow))
	modalBtnFocusedKey = modalBtnFocusedKey.
		Foreground(keyForeground).
		Background(ld(lipgloss.Color("#C0A300"), lipgloss.BrightYellow))
	// Light themes often render ANSI bright black nearly as dark as text.
	gutterColor := ld(lipgloss.Color("#A0A0A0"), subtle)
	lineNumStyle = lineNumStyle.Foreground(gutterColor)
	continuationGutter = lipgloss.NewStyle().
		Foreground(gutterColor).
		Width(5).
		Align(lipgloss.Right).
		Render("↪")
	modalBtnFocusedLabel = lipgloss.NewStyle().
		Bold(true).
		Foreground(ld(lipgloss.BrightWhite, lipgloss.Black)).
		Background(ld(lipgloss.Color("#505050"), lipgloss.White)).
		PaddingLeft(1)
	modalBtnPrimaryLabel, modalBtnPrimaryFocusedLabel = newModalPrimaryStyles(ld)
	modalDangerBtnLabel, modalDangerBtnFocused = newModalDangerStyles(ld)

	selectedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#E8E0F8"), lipgloss.Color("#2D2B55")))

	sidebarHighlightBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#D8E8F8"), lipgloss.Color("#1E3A5F"))).
		Foreground(ld(lipgloss.Color("#333333"), lipgloss.Color("#E0E0E0")))

	visualModeIndicator = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.BrightMagenta).
		Background(ld(lipgloss.Color("#E8E0F8"), lipgloss.Color("#2D2B55"))).
		Padding(0, 1)

	mdCodeStyle = lipgloss.NewStyle().
		Foreground(textColors.brightYellow).
		Background(ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#3a3a3a")))

	formChip = newFormChipPalette(ld)
	progressColors = newProgressPalette()

	diffChangedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#D8F0D8"), lipgloss.Color("#1A3A1A")))

	diffDeletedLineBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#F0D8D8"), lipgloss.Color("#3A1A1A")))

	diffCommonTextBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#303030")))

	diffAddedTextBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#A8DDA8"), lipgloss.Color("#286028")))

	diffDeletedTextBg = lipgloss.NewStyle().
		Background(ld(lipgloss.Color("#E8A8A8"), lipgloss.Color("#682828")))

	rankMovedUp = lipgloss.NewStyle().Background(ld(lipgloss.Color("#EEDC9A"), lipgloss.Color("#5E5220")))
	rankMovedDown = lipgloss.NewStyle().Background(ld(lipgloss.Color("#B8CCF0"), lipgloss.Color("#2A3F6E")))
}

// formChipPalette styles form labels, which sit on a gray chip after a
// colored badge.  On light backgrounds, colored text on the chip uses darker
// shades than the ANSI colors to keep its contrast.
type formChipPalette struct {
	label, code, link lipgloss.Style
	cyan, green, red  color.Color
	// requiredBadge is the background of unanswered required badges, where
	// ANSI red looks muddy on light backgrounds.
	requiredBadge color.Color
	// badgeSymbol draws the symbol on every badge.
	badgeSymbol color.Color
}

func newFormChipPalette(ld lipgloss.LightDarkFunc) formChipPalette {
	return formChipPalette{
		label: lipgloss.NewStyle().
			Foreground(ld(lipgloss.Black, lipgloss.BrightWhite)).
			Background(ld(lipgloss.Color("#D9D9D9"), lipgloss.Color("#3a3a3a"))),
		code: lipgloss.NewStyle().
			Foreground(ld(lipgloss.Color("#6B5200"), lipgloss.BrightYellow)).
			Background(ld(lipgloss.Color("#C4C4C4"), lipgloss.Color("#3a3a3a"))),
		link: lipgloss.NewStyle().
			Foreground(ld(lipgloss.Color("#005A80"), lipgloss.BrightCyan)).
			Underline(true),
		cyan:  ld(lipgloss.Color("#00646E"), lipgloss.Cyan),
		green: ld(lipgloss.Color("#22652A"), lipgloss.Green),
		red:   textColors.red,

		requiredBadge: ld(lipgloss.Color("#E09494"), lipgloss.Red),
		badgeSymbol:   ld(lipgloss.Color("#FFFFFF"), lipgloss.Black),
	}
}

// Primary role colors remain visible independently of keyboard focus.
func newModalPrimaryStyles(ld lipgloss.LightDarkFunc) (lipgloss.Style, lipgloss.Style) {
	return modalBtnNormalLabel.Foreground(ld(lipgloss.Color("#2456A6"), lipgloss.Color("#78ADFF"))).Bold(true),
		modalBtnFocusedLabel.Foreground(ld(lipgloss.Color("#F3F7FF"), lipgloss.Color("#12325F"))).Background(ld(lipgloss.Color("#285BA5"), lipgloss.Color("#90B8F0"))).Bold(true).Underline(true)
}

func newModalDangerStyles(ld lipgloss.LightDarkFunc) (lipgloss.Style, lipgloss.Style) {
	return lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#B42318"), lipgloss.Color("#FF9B91"))).Background(buttonBackground).Bold(true).PaddingLeft(1),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#FFF3F1"), lipgloss.Color("#7B1E1A"))).Background(ld(lipgloss.Color("#A63D36"), lipgloss.Color("#FCE2DE"))).Bold(true).Underline(true).PaddingLeft(1)
}

// textPalette holds foregrounds for text on the terminal background.  On
// dark backgrounds they are the ANSI colors, whose bright variants stand
// out.  On light backgrounds, where ANSI colors are too pale to read, they
// are fixed shades, and the bright variants are darker so they stand out
// there in the same way.
type textPalette struct {
	red, green, yellow, cyan color.Color
	brightYellow, brightCyan color.Color
}

func newTextPalette(ld lipgloss.LightDarkFunc) textPalette {
	return textPalette{
		red:          ld(lipgloss.Color("#E8604A"), lipgloss.Red),
		green:        ld(lipgloss.Color("#4CAF50"), lipgloss.Green),
		yellow:       ld(lipgloss.Color("#E0A100"), lipgloss.Yellow),
		cyan:         ld(lipgloss.Color("#1A9BAE"), lipgloss.Cyan),
		brightYellow: ld(lipgloss.Color("#D29600"), lipgloss.BrightYellow),
		brightCyan:   ld(lipgloss.Color("#00768A"), lipgloss.BrightCyan),
	}
}

// progressPalette colors item counts and tab markers by completion state.
// On light backgrounds, ANSI colors are too pale on the gray summary chip.
type progressPalette struct {
	required, pending, complete color.Color
}

func newProgressPalette() progressPalette {
	return progressPalette{
		required: textColors.red,
		pending:  warning,
		complete: success,
	}
}

// bgToAnsi converts a lipgloss color to a raw ANSI background escape
// sequence.  Palette colors keep their index so the terminal theme applies,
// and other colors become truecolor.  Returns "" if the color is nil or
// zero-alpha.
func bgToAnsi(c color.Color) string {
	return colorToAnsi(c, 40)
}

func fgToAnsi(c color.Color) string {
	return colorToAnsi(c, 30)
}

// colorToAnsi returns the SGR sequence for c, with base 30 for the
// foreground or 40 for the background.
func colorToAnsi(c color.Color, base int) string {
	switch c := c.(type) {
	case nil:
		return ""
	case ansi.BasicColor:
		if c < 8 {
			return fmt.Sprintf("\033[%dm", base+int(c))
		}
		return fmt.Sprintf("\033[%dm", base+52+int(c))
	case ansi.IndexedColor:
		return fmt.Sprintf("\033[%d;5;%dm", base+8, int(c))
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return ""
	}
	// RGBA() returns 16-bit values; scale to 8-bit.
	return fmt.Sprintf("\033[%d;2;%d;%d;%dm", base+8, r>>8, g>>8, b>>8)
}
