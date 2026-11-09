package cli

func (r *doctorReport) checkSVG() {
	start := len(r.Checks)
	available := false
	for _, name := range []string{"rsvg-convert", "merman-cli", "resvg"} {
		if path, err := lookPath(name); err == nil {
			available = true
			r.add("svg/"+name, "ok", path, "")
		} else {
			r.add("svg/"+name, "info", "not on PATH (optional SVG converter)", "")
		}
	}
	if !available {
		r.Checks[start].Advice = "Run `critty install tools svg` to install an SVG converter with mise or Homebrew."
	}
}
