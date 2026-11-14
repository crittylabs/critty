// Package nerdfont resolves Critty's icon setting in the rendering process.
package nerdfont

import detector "charm.land/x/nerdfont"

// Detect returns expected glyph support and its reason.  Explicit settings
// take precedence over the detector's NERDFONT environment override.
// Call once at startup, rather than from rendering paths.
func Detect(setting string) detector.Result {
	switch setting {
	case "on", "off":
		return detector.Result{Supported: setting == "on", Reason: "nerd_fonts=" + setting}
	default:
		return detector.Detect()
	}
}
