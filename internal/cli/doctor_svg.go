package cli

func (r *doctorReport) checkSVG() {
	for _, name := range []string{"rsvg-convert", "merman-cli", "resvg"} {
		if path, err := lookPath(name); err == nil {
			r.add("svg/"+name, "ok", path, "")
		} else {
			r.add("svg/"+name, "info", "not on PATH (optional SVG converter)", "")
		}
	}
}
