package terminalbinding

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ZellijEnvironment records the session name and optional socket directory.
func ZellijEnvironment() Target {
	return Target{Kind: "zellij", Socket: os.Getenv("ZELLIJ_SOCKET_DIR"), Workspace: os.Getenv("ZELLIJ_SESSION_NAME"), Pane: os.Getenv("ZELLIJ_PANE_ID")}
}

// ZellijOwners finds the pane's root process below its Zellij server. Zellij's
// pane listing does not expose process IDs, so this requires inherited identity.
func ZellijOwners(t Target) map[int]Target {
	return zellijOwners(t, os.Getppid(), parentProcess)
}

func zellijOwners(t Target, pid int, inspect func(int) (processInfo, error)) map[int]Target {
	if t.Validate() != nil {
		return nil
	}
	seen := map[int]bool{}
	child := 0
	for pid > 1 && !seen[pid] {
		seen[pid] = true
		p, err := inspect(pid)
		if err != nil {
			break
		}
		if filepath.Base(p.executable) == "zellij" && child > 1 {
			return map[int]Target{child: t}
		}
		child, pid = pid, p.parent
	}
	return nil
}

func (t Target) zellijCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "zellij", append([]string{"--session", t.Workspace}, args...)...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "ZELLIJ") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "ZELLIJ=0", "ZELLIJ_SESSION_NAME="+t.Workspace, "ZELLIJ_PANE_ID="+t.Pane)
	if t.Socket != "" {
		cmd.Env = append(cmd.Env, "ZELLIJ_SOCKET_DIR="+t.Socket)
	}
	return cmd
}

func validateZellij(t Target) error {
	if t.Workspace == "" || strings.HasPrefix(t.Workspace, "-") || strings.ContainsAny(t.Workspace, "/\\\x00\r\n") || (t.Socket != "" && !filepath.IsAbs(t.Socket)) || strings.ContainsAny(t.Socket, "\x00\r\n") {
		return fmt.Errorf("invalid Zellij session")
	}
	if t.Pane == "" || strings.Trim(t.Pane, "0123456789") != "" {
		return fmt.Errorf("invalid Zellij pane")
	}
	if _, err := strconv.ParseUint(t.Pane, 10, 32); err != nil {
		return fmt.Errorf("invalid Zellij pane")
	}
	return nil
}
