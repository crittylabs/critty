package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

type zellijContext struct {
	socket, session, pane string
}

func (zellijContext) kind() string { return "zellij" }

func (c zellijContext) target() terminalbinding.Target {
	return terminalbinding.Target{Kind: "zellij", Socket: c.socket, Workspace: c.session, Pane: c.pane}
}

func (c zellijContext) command(bin string, args ...string) *exec.Cmd {
	cmd := c.target().Command(context.Background(), args...)
	cmd.Path, cmd.Args[0] = bin, bin
	return cmd
}

type zellijDetector struct{}

func (zellijDetector) environmentPresent() bool {
	return os.Getenv("ZELLIJ_SESSION_NAME") != "" || os.Getenv("ZELLIJ_PANE_ID") != ""
}

func (zellijDetector) environmentContext() reviewMultiplexerContext {
	t := terminalbinding.ZellijEnvironment()
	if t.Validate() != nil {
		return nil
	}
	return zellijContext{socket: t.Socket, session: t.Workspace, pane: t.Pane}
}

func (d zellijDetector) processContexts() map[int]reviewMultiplexerContext {
	c, ok := d.environmentContext().(zellijContext)
	if !ok {
		return nil
	}
	owners := make(map[int]reviewMultiplexerContext)
	for pid := range terminalbinding.ZellijOwners(c.target()) {
		owners[pid] = c
	}
	return owners
}

type zellijPane struct {
	ID     uint32 `json:"id"`
	Plugin bool   `json:"is_plugin"`
	Tab    uint32 `json:"tab_id"`
}

func zellijPanes(c zellijContext, bin string) ([]zellijPane, error) {
	out, err := commandOutput(c.command(bin, "action", "list-panes", "--all", "--json"))
	if err != nil {
		return nil, err
	}
	var response []struct {
		ID     *uint32 `json:"id"`
		Plugin *bool   `json:"is_plugin"`
		Tab    *uint32 `json:"tab_id"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("missing Zellij pane list")
	}
	panes := make([]zellijPane, 0, len(response))
	for _, pane := range response {
		if pane.ID == nil || pane.Plugin == nil || pane.Tab == nil {
			return nil, fmt.Errorf("incomplete Zellij pane metadata")
		}
		panes = append(panes, zellijPane{ID: *pane.ID, Plugin: *pane.Plugin, Tab: *pane.Tab})
	}
	return panes, nil
}

type zellijLaunch struct {
	context   zellijContext
	bin, pane string
}

func (l zellijLaunch) close() error {
	err := runCommand(l.context.command(l.bin, "action", "close-pane", "--pane-id", l.pane))
	if err == nil {
		return nil
	}
	// The command may have exited and closed its pane before cleanup.
	panes, checkErr := zellijPanes(l.context, l.bin)
	if checkErr == nil {
		for _, pane := range panes {
			if !pane.Plugin && strconv.FormatUint(uint64(pane.ID), 10) == l.pane {
				return fmt.Errorf("closing Zellij review pane: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("closing Zellij review pane: %w", err)
}

func (l zellijLaunch) restoreFocus() {
	_ = runCommand(l.context.command(l.bin, "action", "focus-pane-id", l.context.pane))
}

func spawnTUIZellijPane(mode *reviewMode, c zellijContext) (zellijLaunch, error) {
	bin, err := lookPath("zellij")
	if err != nil {
		return zellijLaunch{}, err
	}
	command, err := buildTUICommand(mode, "Zellij")
	if err != nil {
		return zellijLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return zellijLaunch{}, err
	}
	panes, err := zellijPanes(c, bin)
	if err != nil {
		return zellijLaunch{}, fmt.Errorf("locating Zellij pane: %w", err)
	}
	for _, pane := range panes {
		if pane.Plugin || strconv.FormatUint(uint64(pane.ID), 10) != c.pane {
			continue
		}
		out, err := commandOutput(c.command(bin, "action", "new-pane", "--floating", "--width", "90%", "--height", "90%", "--tab-id", strconv.FormatUint(uint64(pane.Tab), 10), "--cwd", cwd, "--name", "Critty", "--close-on-exit", "--", "/bin/sh", "-c", "exec "+command))
		if err != nil {
			return zellijLaunch{}, fmt.Errorf("opening Zellij review pane: %w", err)
		}
		id, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "terminal_")
		target := c.target()
		target.Pane = id
		if !ok || target.Validate() != nil {
			return zellijLaunch{}, fmt.Errorf("invalid pane ID in Zellij response: %q", out)
		}
		fmt.Fprintln(os.Stderr, "Opened review in Zellij floating pane terminal_"+id)
		return zellijLaunch{context: c, bin: bin, pane: id}, nil
	}
	return zellijLaunch{}, fmt.Errorf("invoking Zellij pane no longer exists")
}
