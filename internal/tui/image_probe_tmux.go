package tui

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

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
		"show-options", "-A", "-p", "-v", "-t", pane, "allow-passthrough", ";",
		"display-message", "-p", "-t", pane, "#{window_id}", ";",
		"list-clients", "-F", "#{pane_id} #{window_id} #{W:#{window_id} }").Output()
	if err != nil {
		return false
	}
	return tmuxProbeHasSingleReceiver(pane, string(output))
}

// Passthrough replies go to the client's selected pane, not the querying pane.
// With allow-passthrough=on, only clients displaying the window receive queries.
// With all, include clients whose sessions contain linked or hidden windows.
func tmuxProbeHasSingleReceiver(pane, output string) bool {
	mode, outputText, ok := strings.Cut(strings.TrimSpace(output), "\n")
	if !ok || mode != "on" && mode != "all" {
		return false
	}
	window, clients, ok := strings.Cut(outputText, "\n")
	if !ok || !strings.HasPrefix(window, "@") {
		return false
	}
	count := 0
	for line := range strings.SplitSeq(clients, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return false
		}
		windows := fields[2:]
		if mode == "on" {
			windows = fields[1:2]
		}
		if !slices.Contains(windows, window) {
			continue
		}
		if fields[0] != pane {
			return false
		}
		count++
	}
	return count == 1
}
