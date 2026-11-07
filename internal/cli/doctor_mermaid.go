package cli

import (
	"os"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/mermaidrender"
)

var findMermaid = mermaidrender.Find

func (r *doctorReport) checkMermaid() {
	cfg, err := config.LoadCurrent()
	if err != nil {
		r.add("mermaid/config", "warn", err.Error(), "Check the global and project configuration.")
		return
	}
	for _, tool := range []struct{ setting, name, key string }{
		{cfg.MermaidMerman, "merman-cli", "merman"},
		{cfg.MermaidMmdr, "mmdr", "mmdr"},
	} {
		if tool.setting == "off" {
			r.add("mermaid/"+tool.key, "info", "disabled by mermaid_"+tool.key, "")
			continue
		}
		path, err := findMermaid(tool.setting, tool.name)
		if err != nil {
			r.add("mermaid/"+tool.key, "warn", err.Error(), "Select an installed command or use off.")
		} else if path == "" {
			r.add("mermaid/"+tool.key, "info", "not on PATH (optional Mermaid renderer)", "")
		} else {
			r.add("mermaid/"+tool.key, "ok", path, "")
		}
	}
	if len(cfg.MermaidFonts) == 0 {
		if path, err := lookPath("fc-match"); err == nil {
			r.add("mermaid/fonts", "ok", path+" (automatic font selection)", "")
		} else {
			r.add("mermaid/fonts", "info", "fc-match not on PATH; renderers use their system-font defaults", "Install Fontconfig or set global mermaid_fonts if labels are missing or font discovery fails.")
		}
		return
	}
	for _, font := range cfg.MermaidFonts {
		if info, err := os.Stat(font); err != nil || !info.Mode().IsRegular() {
			r.add("mermaid/fonts", "warn", "font unavailable: "+font, "Check global mermaid_fonts.")
		} else {
			r.add("mermaid/fonts", "ok", font+" (Merman font file)", "")
		}
	}
}
