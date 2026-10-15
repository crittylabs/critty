package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Passthrough replies go to the client's selected pane, not the querying pane.
func imageProbeAllowed() bool {
	value := os.Getenv("TMUX")
	if value == "" {
		return true
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return false
	}
	for range 2 {
		i := strings.LastIndexByte(value, ',')
		if i < 0 {
			return false
		}
		value = value[:i]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tmux", "-S", value, "list-clients", "-F", "#{pane_id}").Output()
	return err == nil && strings.TrimSpace(string(output)) == pane
}
