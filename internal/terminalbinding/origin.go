package terminalbinding

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tklauser/ps"
)

// Origin records client hints, never SSH addresses or a daemon's environment.
// Started guards against PID reuse when a marker is resolved later.
type Origin struct {
	PID             int    `json:"pid,omitempty"`
	Started         string `json:"started,omitempty"`
	SSH             bool   `json:"ssh,omitempty"`
	Terminal        string `json:"terminal,omitempty"`
	WSLDistribution string `json:"wsl_distribution,omitempty"`
}

func CaptureOrigin(pid int) Origin {
	started, err := processStamp(pid)
	if err != nil {
		pid = 0
	}
	return Origin{PID: pid, Started: started, SSH: os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "", Terminal: terminalApplication(os.Getenv), WSLDistribution: os.Getenv("WSL_DISTRO_NAME")}
}

func terminalApplication(getenv func(string) string) string {
	for _, key := range []string{"TERMINAL_APP", "TERM_PROGRAM"} {
		switch strings.ToLower(getenv(key)) {
		case "ghostty":
			return "ghostty"
		case "iterm", "iterm2", "iterm.app":
			return "iterm"
		case "terminal", "terminal.app", "apple_terminal":
			return "terminal-app"
		}
	}
	if getenv("ITERM_SESSION_ID") != "" {
		return "iterm"
	}
	if getenv("WT_SESSION") != "" {
		return "windows-terminal"
	}
	return ""
}

// Remote suppresses GUI launchers only on positive evidence. A current, unique
// tmux client supersedes the wrapper's startup environment after reattachment.
func Remote(target Target, origin Origin) bool {
	if target.Kind == "tmux" {
		if remote, known := tmuxRemote(target); known {
			return remote
		}
	}
	if origin.SSH {
		return true
	}
	if origin.PID > 1 && origin.Started != "" {
		stamp, err := processStamp(origin.PID)
		if err == nil && stamp == origin.Started {
			return sshAncestor(origin.PID, parentProcess)
		}
	}
	return false
}

type processInfo struct {
	parent     int
	executable string
}

func parentProcess(pid int) (processInfo, error) {
	p, err := ps.FindProcess(pid)
	if err != nil {
		return processInfo{}, err
	}
	return processInfo{p.PPID(), p.Command()}, nil
}

func sshAncestor(pid int, inspect func(int) (processInfo, error)) bool {
	seen := make(map[int]bool)
	for depth := 0; pid > 1 && !seen[pid] && depth < 128; depth++ {
		seen[pid] = true
		p, err := inspect(pid)
		if err != nil {
			return false
		}
		switch filepath.Base(p.executable) {
		case "sshd", "sshd-session":
			return true
		}
		pid = p.parent
	}
	return false
}

func tmuxRemote(target Target) (bool, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := target.Command(ctx, "display-message", "-p", "-t", target.Pane, "#{session_id}").Output()
	if err != nil {
		return false, false
	}
	session := strings.TrimSpace(string(out))
	if session == "" {
		return false, false
	}
	out, err = target.Command(ctx, "list-clients", "-t", session, "-F", "#{client_pid}").Output()
	if err != nil {
		return false, false
	}
	clients := strings.Fields(string(out))
	// With multiple clients the session environment may describe another client.
	if len(clients) != 1 {
		return false, false
	}
	pid, err := strconv.Atoi(clients[0])
	if err != nil || pid < 2 {
		return false, false
	}
	if sshAncestor(pid, parentProcess) {
		return true, true
	}
	// SSH_CONNECTION is refreshed by tmux's default update-environment.
	// SSH_CLIENT and SSH_TTY can remain stale across a local reattachment.
	out, err = target.Command(ctx, "show-environment", "-t", session, "SSH_CONNECTION").Output()
	if err != nil {
		return false, false
	}
	value := strings.TrimSpace(string(out))
	if value == "-SSH_CONNECTION" || value == "SSH_CONNECTION=" {
		return false, true
	}
	if strings.HasPrefix(value, "SSH_CONNECTION=") {
		return true, true
	}
	return false, false
}

func sameDestination(a, b Target) bool {
	if a.Kind == "terminal" || b.Kind == "terminal" {
		return a.Kind == b.Kind && a.Origin.PID == b.Origin.PID && a.Origin.Started == b.Origin.Started
	}
	a.Origin, b.Origin = Origin{}, Origin{}
	return a == b
}

func addTarget(targets []Target, target Target) []Target {
	for i, t := range targets {
		if sameDestination(t, target) {
			if target.Origin.PID != 0 {
				target.Origin.SSH = target.Origin.SSH || t.Origin.SSH
				targets[i] = target
			}
			return targets
		}
	}
	return append(targets, target)
}
