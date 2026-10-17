package tui

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests size previews in cells only; the developer's terminal must not
	// cap them at the test images' tiny pixel sizes.
	cellPixelSize = func() cellSize { return cellSize{} }
	os.Exit(runIsolated(m))
}

// runIsolated keeps parallel tests, which cannot use t.Setenv, away from the
// developer's tmux session and review state.
func runIsolated(m *testing.M) int {
	dir, err := os.MkdirTemp("", "critty-tui-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	for _, key := range []string{"TMUX", "TMUX_PANE"} {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	for _, key := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		if err := os.Setenv(key, dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}
