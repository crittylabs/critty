//go:build !darwin && !linux

package tui

import (
	"os"
	"time"
)

// HasDarkBackground reports the TUI default on platforms without the probe.
func HasDarkBackground(in, out *os.File, timeout time.Duration) bool {
	return true
}
