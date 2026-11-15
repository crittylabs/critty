package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Passthrough replies go to the client's selected pane, not the querying pane.
// Count every client whose session contains the window, including linked and
// hidden windows: allow-passthrough=all can send to those clients as well.
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
	output, err := exec.CommandContext(ctx, "tmux", "-S", value,
		"display-message", "-p", "-t", pane, "#{window_id}", ";",
		"list-clients", "-F", "#{pane_id} #{W:#{window_id} }").Output()
	if err != nil {
		return false
	}
	window, clients, ok := strings.Cut(strings.TrimSpace(string(output)), "\n")
	if !ok || !strings.HasPrefix(window, "@") {
		return false
	}
	count := 0
	for line := range strings.SplitSeq(clients, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return false
		}
		for _, id := range fields[1:] {
			if id != window {
				continue
			}
			if fields[0] != pane {
				return false
			}
			count++
			break
		}
	}
	return count == 1
}
