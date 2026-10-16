//go:build darwin || linux

package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// cellPixelSize reads the terminal's cell size in pixels from the
// controlling TTY, as kitty, Ghostty, iTerm2, and tmux report it.  Zero
// means unknown.
var cellPixelSize = func() cellSize {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return cellSize{}
	}
	defer tty.Close()
	ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		return cellSize{}
	}
	return cellSize{width: int(ws.Xpixel) / int(ws.Col), height: int(ws.Ypixel) / int(ws.Row)}
}
