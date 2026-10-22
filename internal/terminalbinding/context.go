// Package terminalbinding locates a review terminal without changing an agent's launch options.
package terminalbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tklauser/ps"
)

// Target identifies a multiplexer pane or a notifying client outside a multiplexer.
type Target struct {
	Origin    Origin `json:"origin,omitempty"`
	Kind      string `json:"kind"`
	Socket    string `json:"socket"`
	Pane      string `json:"pane"` // Surface ID for cmux.
	Workspace string `json:"workspace,omitempty"`
	Tab       string `json:"tab,omitempty"`
}

func (t Target) Validate() error {
	if t.Kind == "terminal" {
		if t.Socket != "" || t.Pane != "" || t.Workspace != "" || t.Tab != "" || t.Origin.PID < 2 || t.Origin.Started == "" {
			return fmt.Errorf("invalid terminal origin")
		}
		return nil
	}
	if !filepath.IsAbs(t.Socket) || strings.ContainsAny(t.Socket+t.Pane+t.Workspace+t.Tab, "\x00\r\n") || t.Pane == "" {
		return fmt.Errorf("invalid terminal target")
	}
	switch t.Kind {
	case "tmux":
		if !strings.HasPrefix(t.Pane, "%") {
			return fmt.Errorf("invalid tmux pane")
		}
		if _, err := strconv.ParseUint(t.Pane[1:], 10, 64); err != nil {
			return fmt.Errorf("invalid tmux pane")
		}
	case "herdr":
		if t.Tab == "" || t.Workspace == "" || strings.HasPrefix(t.Pane, "-") {
			return fmt.Errorf("incomplete Herdr target")
		}
	case "cmux":
		if t.Workspace == "" || strings.HasPrefix(t.Pane, "-") || strings.HasPrefix(t.Workspace, "-") {
			return fmt.Errorf("incomplete cmux target")
		}
	default:
		return fmt.Errorf("unknown terminal kind %q", t.Kind)
	}
	return nil
}

// Command pins requests to the recorded server, regardless of daemon environment.
func (t Target) Command(ctx context.Context, args ...string) *exec.Cmd {
	if t.Kind == "tmux" {
		return exec.CommandContext(ctx, "tmux", append([]string{"-S", t.Socket}, args...)...)
	}
	if t.Kind == "cmux" {
		cmd := exec.CommandContext(ctx, "cmux", append([]string{"--socket", t.Socket}, args...)...)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "CMUX_") || strings.HasPrefix(e, "CMUX_SOCKET_PASSWORD=") {
				cmd.Env = append(cmd.Env, e)
			}
		}
		return cmd
	}
	cmd := exec.CommandContext(ctx, "herdr", args...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HERDR_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "HERDR_SOCKET_PATH="+t.Socket)
	return cmd
}

// FromEnvironment is used in the real terminal, never to infer a daemon client's identity.
func FromEnvironment() (Target, error) {
	var targets []Target
	if raw, pane := os.Getenv("TMUX"), os.Getenv("TMUX_PANE"); raw != "" && pane != "" {
		// TMUX ends in ,server-pid,session-id; the socket path itself may contain commas.
		end := strings.LastIndexByte(raw, ',')
		if end > 0 {
			end = strings.LastIndexByte(raw[:end], ',')
		}
		if end > 0 {
			targets = append(targets, Target{Kind: "tmux", Socket: raw[:end], Pane: pane})
		}
	}
	if w, tab, pane := os.Getenv("HERDR_WORKSPACE_ID"), os.Getenv("HERDR_TAB_ID"), os.Getenv("HERDR_PANE_ID"); w != "" && tab != "" && pane != "" {
		socket := os.Getenv("HERDR_SOCKET_PATH")
		if socket == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Target{}, err
			}
			dir := filepath.Join(home, ".config", "herdr")
			if config := os.Getenv("HERDR_CONFIG_PATH"); config != "" {
				dir = filepath.Dir(config)
			}
			socket = filepath.Join(dir, "herdr.sock")
		}
		targets = append(targets, Target{Kind: "herdr", Socket: socket, Workspace: w, Tab: tab, Pane: pane})
	}
	if workspace, surface := os.Getenv("CMUX_WORKSPACE_ID"), os.Getenv("CMUX_SURFACE_ID"); workspace != "" && surface != "" {
		targets = append(targets, Target{Kind: "cmux", Socket: CMUXSocketPath(), Workspace: workspace, Pane: surface})
	}
	for _, t := range targets {
		if err := t.Validate(); err != nil {
			return Target{}, err
		}
	}
	if len(targets) == 0 {
		return Target{}, nil
	}
	if len(targets) == 1 {
		return targets[0], nil
	}
	// In a nested multiplexer, select the nearest pane process in our own ancestry.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	owners := make(map[int]Target)
	for _, t := range targets {
		if t.Kind == "tmux" {
			out, err := t.Command(ctx, "display-message", "-p", "-t", t.Pane, "#{pane_pid}").Output()
			if err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
					owners[pid] = t
				}
			}
		} else if t.Kind == "cmux" {
			out, err := t.Command(ctx, "--json", "--id-format", "uuids", "top", "--all", "--processes").Output()
			if err == nil {
				for pid, owner := range CMUXProcessTargets(out, t.Socket) {
					if owner.Workspace == t.Workspace && owner.Pane == t.Pane {
						if _, exists := owners[pid]; !exists {
							owners[pid] = t
						}
					}
				}
			}
		} else {
			out, err := t.Command(ctx, "pane", "process-info", "--pane", t.Pane).Output()
			if err == nil {
				var data any
				if json.Unmarshal(out, &data) == nil {
					collectOwners(data, t, owners)
				}
			}
		}
	}
	seen := map[int]bool{}
	for pid := os.Getppid(); pid > 1 && !seen[pid]; {
		seen[pid] = true
		if t, ok := owners[pid]; ok {
			return t, nil
		}
		p, err := ps.FindProcess(pid)
		if err != nil {
			break
		}
		pid = p.PPID()
	}
	return Target{}, fmt.Errorf("cannot distinguish nested multiplexer terminals")
}

func collectOwners(v any, target Target, owners map[int]Target) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if k == "pid" || k == "shell_pid" {
				if n, ok := x.(float64); ok && n > 1 {
					owners[int(n)] = target
				}
			}
			collectOwners(x, target, owners)
		}
	case []any:
		for _, x := range v {
			collectOwners(x, target, owners)
		}
	}
}

// Snapshot reads only visible pane text, without scrolling or collecting history.
func Snapshot(ctx context.Context, t Target) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	if t.Kind == "terminal" {
		return "", fmt.Errorf("terminal origin requires a filter notification")
	}
	if t.Kind == "tmux" {
		out, err := t.Command(ctx, "capture-pane", "-p", "-J", "-t", t.Pane).Output()
		return string(out), err
	}
	if t.Kind == "cmux" {
		out, err := t.Command(ctx, "read-screen", "--workspace", t.Workspace, "--surface", t.Pane).Output()
		return string(out), err
	}
	out, err := t.Command(ctx, "pane", "read", t.Pane, "--source", "visible", "--format", "text").Output()
	return string(out), err
}
