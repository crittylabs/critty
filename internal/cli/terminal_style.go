package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	"github.com/crittylabs/critty/v2/internal/tui"
)

func tmuxColorOptions(w io.Writer) []tea.ProgramOption {
	out, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(out.Fd())) || !tmuxAdvertisesRGB() {
		return nil
	}
	return []tea.ProgramOption{tea.WithColorProfile(colorprofile.TrueColor)}
}

func tmuxAdvertisesRGB() bool {
	pane := os.Getenv("TMUX_PANE")
	if os.Getenv("TMUX") == "" || !validTMUXPaneID(pane) || colorprofile.Env(os.Environ()) != colorprofile.ANSI256 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// tmux can report RGB in client_termfeatures even when tmux info has
	// no RGB or Tc flag, which colorprofile uses for capability detection.
	features, err := commandOutput(exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#{client_termfeatures}"))
	return err == nil && slices.Contains(strings.Split(strings.TrimSpace(string(features)), ","), "RGB")
}

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
