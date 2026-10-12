package tui

import (
	"fmt"
	"image/color"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

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
var (
	subtle  = lipgloss.BrightBlack
	accent  = lipgloss.Magenta
	success = lipgloss.Green
	warning = lipgloss.Yellow
	muted   = lipgloss.White // normal white (dimmer than BrightWhite)

	// textPrimary and textSecondary are the foregrounds of emphasized and
	// regular text on the terminal background.  initAdaptiveStyles swaps
	// them for dark colors on a light background.
	textPrimary   color.Color = lipgloss.BrightWhite
	textSecondary color.Color = muted

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

	// headerCountStyle shows the comment count in plain white, which reads
	// on the magenta header where blue does not.
	headerCountStyle = headerPathStyle.Bold(false).Foreground(lipgloss.White)

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
				Foreground(lipgloss.Red)

	commentNavButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Cyan).
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
			Padding(0, 1)

	modalBtnFocusedLabel = lipgloss.NewStyle().
				Foreground(lipgloss.Black).
				Background(lipgloss.White).
				PaddingLeft(1)

	modalBtnNormalLabel = modalBtnLabel.
				PaddingLeft(1)

	modalBtnFocusedKey = lipgloss.NewStyle().
				Background(lipgloss.BrightYellow).
				Foreground(lipgloss.Black).
				Bold(true)

	modalBtnNormalKey = lipgloss.NewStyle().
				Background(lipgloss.Yellow).
				Foreground(lipgloss.Black).
				Bold(true)

	modalDeleteBtnLabel = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Red)

	modalDeleteBtnFocused = lipgloss.NewStyle().
				Background(lipgloss.Red).
				Foreground(lipgloss.BrightWhite).
				Bold(true).
				Padding(0, 1)

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
	modalBtnNormalLabel = modalBtnLabel.PaddingLeft(1)
	inactiveTabStyle = inactiveTabStyle.Foreground(textSecondary)
	activeTabStyle = activeTabStyle.Foreground(textPrimary)
	tabCountDelimiter = tabCountDelimiter.Foreground(textSecondary)
	contextBoxStyle = contextBoxStyle.Foreground(textPrimary)
	modalBtnFocusedLabel = lipgloss.NewStyle().
		Foreground(ld(lipgloss.BrightWhite, lipgloss.Black)).
		Background(ld(lipgloss.Black, lipgloss.White)).
		PaddingLeft(1)

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
		Foreground(lipgloss.BrightYellow).
		Background(ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#3a3a3a")))

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
}

// bgToAnsi converts a lipgloss color to a raw ANSI background escape
// sequence.  Palette colors keep their index so the terminal theme applies,
// and other colors become truecolor.  Returns "" if the color is nil or
// zero-alpha.
func bgToAnsi(c color.Color) string {
	switch c := c.(type) {
	case nil:
		return ""
	case ansi.BasicColor:
		if c < 8 {
			return fmt.Sprintf("\033[%dm", 40+int(c))
		}
		return fmt.Sprintf("\033[%dm", 92+int(c))
	case ansi.IndexedColor:
		return fmt.Sprintf("\033[48;5;%dm", int(c))
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return ""
	}
	// RGBA() returns 16-bit values; scale to 8-bit.
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}
