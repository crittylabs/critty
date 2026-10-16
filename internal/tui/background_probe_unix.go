//go:build darwin || linux

package tui

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// HasDarkBackground asks the terminal on in and out for its background color
// and reports whether it is dark.  Unlike the lipgloss query it gives up after
// timeout, so command output is not delayed by a silent terminal.  Both files
// must be terminals; any failure reports a dark background, the TUI default.
func HasDarkBackground(in, out *os.File, timeout time.Duration) bool {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(out.Fd())) {
		return true
	}
	if ready, err := ttyReadable(fd, 0); err != nil || ready {
		return true
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return true
	}
	defer term.Restore(fd, state) //nolint:errcheck
	if _, err := out.WriteString(ansi.RequestBackgroundColor + ansi.RequestPrimaryDeviceAttributes); err != nil {
		return true
	}
	dark := true
	deadline := time.Now().Add(timeout)
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	var pending []byte
	var buf [256]byte
	var state8 byte
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return dark
		}
		if ready, err := ttyReadable(fd, remaining); err != nil || !ready {
			return dark
		}
		n, err := in.Read(buf[:])
		if err != nil {
			return dark
		}
		chunk := buf[:n]
		for len(chunk) > 0 {
			seq, _, read, next := ansi.DecodeSequence(chunk, state8, parser)
			pending = append(pending, seq...)
			if next == ansi.NormalState {
				if done := acceptBackgroundReply(string(pending), parser, &dark); done {
					return dark
				}
				pending = pending[:0]
			}
			state8 = next
			chunk = chunk[read:]
		}
	}
}

// acceptBackgroundReply records an OSC 11 color and reports whether the
// terminating DA1 reply has arrived.
func acceptBackgroundReply(seq string, parser *ansi.Parser, dark *bool) bool {
	switch {
	case ansi.HasOscPrefix(seq) && parser.Command() == 11:
		if parts := strings.Split(string(parser.Data()), ";"); len(parts) == 2 {
			if c := ansi.XParseColor(parts[1]); c != nil {
				*dark = tea.BackgroundColorMsg{Color: c}.IsDark()
			}
		}
	case ansi.HasCsiPrefix(seq) && parser.Command() == ansi.Command('?', 0, 'c'):
		return true
	}
	return false
}
