package terminalbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func WorkspaceExecutable(kind string) string {
	if kind == "orca" && runtime.GOOS == "linux" {
		return "orca-ide"
	}
	return kind
}

// WorkspaceEnvironment captures public host identity, never activity tokens or
// pairing credentials. Orca does not expose local process ownership.
func WorkspaceEnvironment(kind string) Target {
	profile := os.Getenv("ORCA_RUNTIME_SOURCE_PROFILE_PATH")
	if profile == "" {
		profile = os.Getenv("ORCA_USER_DATA_PATH")
	}
	workspace := os.Getenv("ORCA_WORKTREE_ID")
	if workspace == "" {
		workspace = os.Getenv("ORCA_WORKSPACE_ID")
	}
	return Target{Kind: kind, Socket: profile, Workspace: workspace, Pane: os.Getenv("ORCA_TERMINAL_HANDLE"), SourceID: os.Getenv("ORCA_RUNTIME_SOURCE_ID"), RuntimeID: os.Getenv("ORCA_RUNTIME_SOURCE_INCARNATION")}
}

func validateWorkspace(t Target) error {
	if !filepath.IsAbs(t.Socket) || t.Workspace == "" || strings.ContainsAny(t.Socket+t.Workspace+t.Pane+t.SourceID+t.RuntimeID, "\x00\r\n") || strings.HasPrefix(t.Workspace, "-") || strings.HasPrefix(t.Pane, "-") {
		return fmt.Errorf("invalid %s workspace target", t.Kind)
	}
	if t.SourceID == "" || t.RuntimeID == "" {
		return fmt.Errorf("incomplete Orca terminal identity")
	}
	return nil
}

func (t Target) workspaceCommand(ctx context.Context, args ...string) *exec.Cmd {
	bin := WorkspaceExecutable(t.Kind)
	cmd := exec.CommandContext(ctx, bin, args...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ORCA_") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "ORCA_USER_DATA_PATH="+t.Socket, "ORCA_RUNTIME_SOURCE_PROFILE_PATH="+t.Socket, "ORCA_RUNTIME_SOURCE_ID="+t.SourceID, "ORCA_RUNTIME_SOURCE_INCARNATION="+t.RuntimeID)
	return cmd
}

func workspaceSnapshot(ctx context.Context, t Target) (string, error) {
	if t.Pane == "" {
		return "", fmt.Errorf("%s chat requires a terminal filter notification", t.Kind)
	}
	out, err := t.Command(ctx, "terminal", "read", "--terminal", t.Pane, "--screen", "--json").Output()
	if err != nil {
		return "", err
	}
	var reply struct {
		OK     bool `json:"ok"`
		Result struct {
			Terminal struct {
				Tail   *[]string `json:"tail"`
				Source string    `json:"source"`
			} `json:"terminal"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return "", err
	}
	if !reply.OK || reply.Result.Terminal.Source != "screen" || reply.Result.Terminal.Tail == nil {
		return "", fmt.Errorf("Orca did not return a rendered terminal screen")
	}
	return strings.Join(*reply.Result.Terminal.Tail, "\n"), nil
}
