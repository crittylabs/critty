package tui

import (
	"image/color"
	"math"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

const diffTextContrast = 4.5

var linearRGB = func() [256]float64 {
	var table [256]float64
	for i := range table {
		v := float64(i) / 255
		if v <= 0.04045 {
			table[i] = v / 12.92
		} else {
			table[i] = math.Pow((v+0.055)/1.055, 2.4)
		}
	}
	return table
}()

func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.2126*linearRGB[r>>8] + 0.7152*linearRGB[g>>8] + 0.0722*linearRGB[b>>8]
}

func contrastRatio(a, b float64) float64 {
	return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}

func readableDiffForeground(foreground, background color.Color) color.Color {
	if foreground == nil || background == nil {
		return foreground
	}
	bg := relativeLuminance(background)
	if contrastRatio(relativeLuminance(foreground), bg) >= diffTextContrast {
		return foreground
	}
	neutral := 255
	if contrastRatio(0, bg) > contrastRatio(1, bg) {
		neutral = 0
	}
	r, g, b, _ := foreground.RGBA()
	blend := func(amount int) color.RGBA {
		channel := func(v uint32) uint8 {
			return uint8((int(v>>8)*(255-amount) + neutral*amount + 127) / 255)
		}
		return color.RGBA{R: channel(r), G: channel(g), B: channel(b), A: 255}
	}
	amount := sort.Search(255, func(amount int) bool {
		return contrastRatio(relativeLuminance(blend(amount)), bg) >= diffTextContrast
	})
	return blend(amount)
}

func inlineDiffBackground(style lipgloss.Style, content string) string {
	// Kitty placeholders encode image IDs in their foreground colors.
	adjust := !strings.ContainsRune(content, kitty.Placeholder)
	return inlineColorsWithOverride(style, content, false, false, adjust)
}
