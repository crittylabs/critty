package cli

import (
	"io"
	"os"
	"time"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	"github.com/crittylabs/critty/internal/tui"
)

// outputStyle describes how decorated text should be written to a writer.
type outputStyle struct {
	// profile is the writer's color profile; NoTTY and ASCII mean plain text.
	profile colorprofile.Profile
	// dark reports whether the terminal background is dark.  It is probed
	// only for color-capable terminals and defaults to dark otherwise.
	dark bool
}

// backgroundProbeTimeout bounds the wait for a terminal's background color
// reply; a silent terminal is treated as dark.
const backgroundProbeTimeout = 500 * time.Millisecond

func (s outputStyle) color() bool { return s.profile >= colorprofile.ANSI }

// writer returns a writer that downsamples or strips color sequences for w.
func (s outputStyle) writer(w io.Writer) io.Writer {
	return &colorprofile.Writer{Forward: w, Profile: s.profile}
}

// detectOutputStyle inspects w, honoring NO_COLOR and related variables.
// For color-capable terminals it also asks the terminal for its background
// color, so brand colors match the Help dialog.
func detectOutputStyle(w io.Writer) outputStyle {
	s := outputStyle{profile: colorprofile.Detect(w, os.Environ()), dark: true}
	if out, ok := w.(*os.File); ok && s.color() && term.IsTerminal(int(out.Fd())) {
		s.dark = tui.HasDarkBackground(os.Stdin, out, backgroundProbeTimeout)
	}
	return s
}
