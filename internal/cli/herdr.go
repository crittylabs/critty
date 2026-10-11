package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

type herdrContext struct {
	socket    string
	workspace string
	tab       string
	pane      string
}

type herdrDetector struct{}

func (herdrDetector) environmentPresent() bool {
	return os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_WORKSPACE_ID") != "" ||
		os.Getenv("HERDR_TAB_ID") != "" || os.Getenv("HERDR_PANE_ID") != ""
}

func (herdrDetector) environmentContext() reviewMultiplexerContext {
	ctx := herdrContext{
		workspace: os.Getenv("HERDR_WORKSPACE_ID"),
		tab:       os.Getenv("HERDR_TAB_ID"),
		pane:      os.Getenv("HERDR_PANE_ID"),
	}
	if !ctx.active() {
		return nil
	}
	return ctx
}

func (herdrDetector) processContexts() map[int]reviewMultiplexerContext {
	contexts := make(map[int]reviewMultiplexerContext)
	for pid, ctx := range herdrProcessContexts() {
		contexts[pid] = ctx
	}
	return contexts
}

func (c herdrContext) launchReview(mode *reviewMode) (reviewMultiplexerLaunch, error) {
	return spawnTUIHerdrTab(mode, c)
}

func (c herdrContext) restoreFocus() {
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return
	}
	_ = runCommand(c.command(herdrBin, "tab", "focus", c.tab))
}

func (c herdrContext) active() bool {
	return c.workspace != "" && c.tab != "" && c.pane != ""
}

type herdrLaunch struct {
	context   herdrContext
	bin       string
	tab       string
	sourceTab string
}

func (l herdrLaunch) close() {
	_ = runCommand(l.context.command(l.bin, "tab", "close", l.tab))
}

func (l herdrLaunch) restoreFocus() {
	_ = runCommand(l.context.command(l.bin, "tab", "focus", l.sourceTab))
}

func focusCurrentHerdrTab() {
	tab := os.Getenv("HERDR_TAB_ID")
	if tab == "" {
		return
	}
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return
	}
	_ = runCommand(exec.Command(herdrBin, "tab", "focus", tab))
}

func herdrProcessContexts() map[int]herdrContext {
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return nil
	}

	discoveryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var panesResponse struct {
		Result struct {
			Panes []struct {
				WorkspaceID string `json:"workspace_id"`
				TabID       string `json:"tab_id"`
				PaneID      string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	out, err := commandOutput(exec.CommandContext(discoveryCtx, herdrBin, "pane", "list"))
	if err != nil || json.Unmarshal(out, &panesResponse) != nil {
		return nil
	}

	contexts := make(map[int]herdrContext)
	for _, pane := range panesResponse.Result.Panes {
		ctx := herdrContext{workspace: pane.WorkspaceID, tab: pane.TabID, pane: pane.PaneID}
		if !ctx.active() {
			continue
		}
		out, err := commandOutput(exec.CommandContext(discoveryCtx, herdrBin, "pane", "process-info", "--pane", pane.PaneID))
		if err != nil {
			continue
		}
		for _, pid := range herdrProcessIDs(out) {
			if _, exists := contexts[pid]; !exists {
				contexts[pid] = ctx
			}
		}
	}

	return contexts
}

func herdrProcessIDs(data []byte) []int {
	var response struct {
		Result struct {
			ProcessInfo json.RawMessage `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Result.ProcessInfo) == 0 {
		return nil
	}

	var processInfo any
	if json.Unmarshal(response.Result.ProcessInfo, &processInfo) != nil {
		return nil
	}
	seen := make(map[int]bool)
	collectHerdrProcessIDs(processInfo, seen)
	ids := make([]int, 0, len(seen))
	for pid := range seen {
		ids = append(ids, pid)
	}
	return ids
}

func collectHerdrProcessIDs(value any, ids map[int]bool) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "pid" || key == "shell_pid" {
				if pid, ok := child.(float64); ok && pid >= 2 {
					ids[int(pid)] = true
				}
			}
			collectHerdrProcessIDs(child, ids)
		}
	case []any:
		for _, child := range value {
			collectHerdrProcessIDs(child, ids)
		}
	}
}

func spawnTUIHerdrTab(mode *reviewMode, herdr herdrContext) (herdrLaunch, error) {
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return herdrLaunch{}, fmt.Errorf("herdr binary not found on PATH: %w", err)
	}
	tuiCmd, err := buildTUICommand(mode, "Herdr")
	if err != nil {
		return herdrLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return herdrLaunch{}, fmt.Errorf("resolving working directory: %w", err)
	}

	args := []string{"tab", "create", "--workspace", herdr.workspace, "--cwd", cwd, "--label", "Critty", "--no-focus"}
	for _, name := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		if value := os.Getenv(name); value != "" {
			args = append(args, "--env", name+"="+value)
		}
	}
	out, err := commandOutput(herdr.command(herdrBin, args...))
	if err != nil {
		return herdrLaunch{}, fmt.Errorf("failed to create Herdr tab: %w", err)
	}

	var response struct {
		Result struct {
			Tab struct {
				TabID string `json:"tab_id"`
			} `json:"tab"`
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return herdrLaunch{}, fmt.Errorf("parsing Herdr tab response: %w", err)
	}
	launch := herdrLaunch{context: herdr, bin: herdrBin, tab: response.Result.Tab.TabID, sourceTab: herdr.tab}
	if launch.tab == "" {
		return herdrLaunch{}, fmt.Errorf("missing tab or pane ID in Herdr tab response")
	}
	if response.Result.RootPane.PaneID == "" {
		launch.close()
		launch.restoreFocus()
		return herdrLaunch{}, fmt.Errorf("missing tab or pane ID in Herdr tab response")
	}

	if err := runCommand(herdr.command(herdrBin, "pane", "run", response.Result.RootPane.PaneID, "exec "+tuiCmd)); err != nil {
		launch.close()
		return herdrLaunch{}, fmt.Errorf("failed to start TUI in Herdr tab: %w", err)
	}
	if err := runCommand(herdr.command(herdrBin, "tab", "focus", launch.tab)); err != nil {
		launch.close()
		launch.restoreFocus()
		return herdrLaunch{}, fmt.Errorf("failed to focus Herdr tab: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Opened review in Herdr tab")
	return launch, nil
}

func (c herdrContext) command(bin string, args ...string) *exec.Cmd {
	if c.socket == "" {
		return exec.Command(bin, args...)
	}
	cmd := (terminalbinding.Target{Kind: "herdr", Socket: c.socket}).Command(context.Background(), args...)
	cmd.Path = bin
	cmd.Args[0] = bin
	return cmd
}
