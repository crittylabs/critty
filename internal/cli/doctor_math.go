package cli

import (
	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/mathrender"
)

var findMathRaTeX = mathrender.FindRaTeX

func (r *doctorReport) checkMath() {
	cfg, err := config.LoadCurrent()
	switch {
	case err != nil:
		r.add("math/ratex", "warn", err.Error(), "Check the global and project configuration.")
	case cfg.MathRaTeX == "off":
		r.add("math/ratex", "info", "disabled by math_ratex", "")
	default:
		path, err := findMathRaTeX(cfg.MathRaTeX)
		if err != nil {
			status := "warn"
			advice := "Use a RaTeX PNG CLI on PATH, set `math_ratex` to its command or path, or use `off` to skip RaTeX."
			if cfg.MathRaTeX == "" || cfg.MathRaTeX == "auto" {
				status = "info"
				advice += " Run `critty install tools math --manager mise` to install RaTeX."
			}
			r.add("math/ratex", status, err.Error(), advice)
		} else if path == "" {
			r.add("math/ratex", "info", "not found on PATH (optional single-line renderer)", "Run `critty install tools math --manager mise` to install RaTeX.")
		} else {
			r.add("math/ratex", "ok", path+" (single-line expressions)", "")
		}
	}
	advice := "Run `critty install tools math` to install missing LaTeX and dvipng tools with mise or Homebrew."
	for _, name := range []string{"latex", "dvipng"} {
		if path, err := lookPath(name); err != nil {
			r.add("math/"+name, "info", "not on PATH (optional; needed for multiline math and RaTeX fallback)", advice)
			advice = ""
		} else {
			r.add("math/"+name, "ok", path, "")
		}
	}
}
