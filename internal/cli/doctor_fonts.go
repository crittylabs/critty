package cli

import (
	"fmt"

	"github.com/crittylabs/critty/v2/internal/config"
	"github.com/crittylabs/critty/v2/internal/nerdfont"
)

func (r *doctorReport) checkNerdFonts() {
	cfg, err := config.LoadCurrent()
	if err != nil {
		r.add("fonts/nerd", "warn", err.Error(), "Check the global and project configuration.")
		return
	}
	result := nerdfont.Detect(cfg.NerdFonts)
	state := "off"
	if result.Supported {
		state = "on"
	}
	r.add("fonts/nerd", "info", fmt.Sprintf("%s (nerd_fonts=%s): %s", state, cfg.NerdFonts, result.Reason),
		"Detection estimates glyph availability, not the terminal's selected font. Set nerd_fonts to on or off to override it.")
}
