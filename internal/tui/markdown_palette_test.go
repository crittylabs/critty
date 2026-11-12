package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownCodeBackgroundAfterInitialDarkProbe(t *testing.T) {
	previousStyle, previousDark := mdCodeStyle, terminalIsDark
	t.Cleanup(func() {
		initAdaptiveStyles(previousDark)
		mdCodeStyle = previousStyle
	})
	// Match startup before the first terminal-background response.
	terminalIsDark = true
	mdCodeStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightYellow)
	app := setupPrivateAppWithDoc(t, "Literal source: `example`.")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 35})
	app, _ = updateApp(app, tea.BackgroundColorMsg{Color: color.Black})
	view := app.contentViewport.GetContent()
	_, styles := renderedCells(view)
	plain := strings.ReplaceAll(ansi.Strip(view), "\n", "")
	start := strings.Index(plain, "example")
	if start < 0 {
		t.Fatal("code label missing from viewport")
	}
	if !sameColor(styles[start].Bg, mdCodeStyle.GetBackground()) {
		t.Fatalf("inline code background = %v, want %v after dark probe", styles[start].Bg, mdCodeStyle.GetBackground())
	}
}
