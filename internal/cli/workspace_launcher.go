package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

type workspaceContext struct{ place terminalbinding.Target }

func (c workspaceContext) kind() string                   { return c.place.Kind }
func (c workspaceContext) target() terminalbinding.Target { return c.place }

type workspaceDetector struct{}

func (workspaceDetector) environmentPresent() bool {
	return os.Getenv("ORCA_TERMINAL_HANDLE") != "" || os.Getenv("ORCA_WORKTREE_ID") != "" || os.Getenv("ORCA_WORKSPACE_ID") != ""
}

func (d workspaceDetector) environmentContext() reviewMultiplexerContext {
	t := terminalbinding.WorkspaceEnvironment("orca")
	if t.Validate() != nil {
		return nil
	}
	return workspaceContext{t}
}

func (workspaceDetector) processContexts() map[int]reviewMultiplexerContext { return nil }

func (c workspaceContext) command(bin string, args ...string) *exec.Cmd {
	cmd := c.place.Command(context.Background(), args...)
	cmd.Path, cmd.Args[0] = bin, bin
	return cmd
}

// orcaResult checks the RPC envelope even when the CLI exits successfully.
func orcaResult(cmd *exec.Cmd, result any) error {
	out, err := commandOutput(cmd)
	if err != nil {
		return err
	}
	var reply struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return err
	}
	if !reply.OK {
		return fmt.Errorf("Orca: %s", reply.Error.Message)
	}
	return json.Unmarshal(reply.Result, result)
}

type orcaTerminal struct {
	Handle   string `json:"handle"`
	Worktree string `json:"worktreeId"`
	Host     string `json:"executionHostId"`
}

type workspaceLaunch struct {
	context       workspaceContext
	bin, terminal string
}

func (l workspaceLaunch) restoreFocus() {}

func (l workspaceLaunch) close() error {
	c := l.context
	err := runCommand(c.command(l.bin, "terminal", "close", "--terminal", l.terminal, "--json"))
	if err == nil {
		return nil
	}
	// A host may remove a terminal when the TUI exits. Only a successful,
	// complete inventory of the pinned host can establish that it is gone.
	var result struct {
		Terminals *[]orcaTerminal `json:"terminals"`
		Truncated bool            `json:"truncated"`
		HostScope struct {
			Hosts []string `json:"hostIds"`
		} `json:"hostScope"`
	}
	checkErr := orcaResult(c.command(l.bin, "terminal", "list", "--worktree", "id:"+c.place.Workspace, "--json"), &result)
	if checkErr == nil && result.Terminals != nil && !result.Truncated {
		for _, t := range *result.Terminals {
			if t.Handle == l.terminal {
				return fmt.Errorf("closing Orca review: %w", err)
			}
		}
		for _, host := range result.HostScope.Hosts {
			if host == "local" {
				return nil
			}
		}
	}
	return fmt.Errorf("closing Orca review: %w", err)
}

func checkWorkspaceLauncher(c workspaceContext) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%s reviews require a Unix host with a POSIX shell", c.kind())
	}
	if err := c.place.Validate(); err != nil {
		return err
	}
	bin, err := lookPath(terminalbinding.WorkspaceExecutable(c.kind()))
	if err != nil {
		return err
	}
	if c.place.Pane == "" {
		var result struct {
			Worktree struct {
				ID   string `json:"id"`
				Host string `json:"hostId"`
			} `json:"worktree"`
		}
		if err := orcaResult(c.command(bin, "worktree", "show", "--worktree", "id:"+c.place.Workspace, "--json"), &result); err != nil {
			return err
		}
		if result.Worktree.ID != c.place.Workspace || result.Worktree.Host != "local" {
			return fmt.Errorf("Orca workspace identity changed")
		}
		return nil
	}
	var result struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	if err := orcaResult(c.command(bin, "terminal", "show", "--terminal", c.place.Pane, "--json"), &result); err != nil {
		return err
	}
	if result.Terminal.Handle != c.place.Pane || result.Terminal.Worktree != c.place.Workspace || result.Terminal.Host != "local" {
		return fmt.Errorf("Orca caller is not a local terminal in the recorded workspace")
	}
	return nil
}

func spawnWorkspaceReview(mode *reviewMode, c workspaceContext) (workspaceLaunch, error) {
	bin, err := lookPath(terminalbinding.WorkspaceExecutable(c.kind()))
	if err != nil {
		return workspaceLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return workspaceLaunch{}, err
	}
	command, err := buildTUICommand(mode, "Orca")
	if err != nil {
		return workspaceLaunch{}, err
	}
	l := workspaceLaunch{context: c, bin: bin}
	var result struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	err = orcaResult(c.command(bin, "terminal", "create", "--worktree", "id:"+c.place.Workspace, "--title", "Critty", "--command", "cd "+shellEscape(cwd)+" && exec "+command, "--focus", "--json"), &result)
	if err != nil {
		return l, fmt.Errorf("opening Orca review: %w", err)
	}
	l.terminal = result.Terminal.Handle
	if l.terminal == "" {
		return l, fmt.Errorf("missing terminal handle in Orca reply")
	}
	if result.Terminal.Worktree != c.place.Workspace || result.Terminal.Host != "local" {
		return l, errors.Join(fmt.Errorf("Orca created a review outside the recorded local workspace"), l.close())
	}
	fmt.Fprintln(os.Stderr, "Opened review in Orca terminal")
	return l, nil
}

func workspaceEnvironment(t terminalbinding.Target) []envVar {
	return []envVar{{"ORCA_USER_DATA_PATH", t.Socket}, {"ORCA_RUNTIME_SOURCE_PROFILE_PATH", t.Socket}, {"ORCA_RUNTIME_SOURCE_ID", t.SourceID}, {"ORCA_RUNTIME_SOURCE_INCARNATION", t.RuntimeID}, {"ORCA_WORKTREE_ID", t.Workspace}, {"ORCA_TERMINAL_HANDLE", t.Pane}}
}

func workspaceTerminalVariable(key string) bool {
	return strings.HasPrefix(key, "ORCA_")
}
