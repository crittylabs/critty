package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

type cmuxContext struct {
	socket, workspace, surface string
}

func (cmuxContext) kind() string { return "cmux" }

func (c cmuxContext) target() terminalbinding.Target {
	return terminalbinding.Target{Kind: "cmux", Socket: c.socket, Workspace: c.workspace, Pane: c.surface}
}

func (c cmuxContext) command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := c.target().Command(ctx, args...)
	cmd.Path, cmd.Args[0] = bin, bin
	return cmd
}

type cmuxDetector struct{}

func (cmuxDetector) environmentPresent() bool {
	return os.Getenv("CMUX_WORKSPACE_ID") != "" || os.Getenv("CMUX_SURFACE_ID") != ""
}

func (cmuxDetector) environmentContext() reviewMultiplexerContext {
	c := cmuxContext{socket: terminalbinding.CMUXSocketPath(), workspace: os.Getenv("CMUX_WORKSPACE_ID"), surface: os.Getenv("CMUX_SURFACE_ID")}
	if c.target().Validate() != nil {
		return nil
	}
	return c
}

func (cmuxDetector) processContexts() map[int]reviewMultiplexerContext {
	bin, err := lookPath("cmux")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c := cmuxContext{socket: terminalbinding.CMUXSocketPath()}
	out, err := commandOutput(c.command(ctx, bin, "--json", "--id-format", "uuids", "top", "--all", "--processes"))
	if err != nil {
		return nil
	}
	contexts := make(map[int]reviewMultiplexerContext)
	for pid, t := range terminalbinding.CMUXProcessTargets(out, c.socket) {
		contexts[pid] = cmuxContext{socket: t.Socket, workspace: t.Workspace, surface: t.Pane}
	}
	return contexts
}

type cmuxLaunch struct {
	context      cmuxContext
	bin, surface string
}

func (l cmuxLaunch) close() error {
	err := runCommand(l.context.command(context.Background(), l.bin, "close-surface", "--workspace", l.context.workspace, "--surface", l.surface))
	if err == nil {
		return nil
	}
	// cmux may remove the surface as soon as the TUI process exits.
	out, checkErr := commandOutput(l.context.command(context.Background(), l.bin, "--json", "--id-format", "uuids", "list-panels", "--workspace", l.context.workspace))
	var response struct {
		Surfaces *[]struct {
			ID string `json:"id"`
		} `json:"surfaces"`
	}
	if checkErr == nil && json.Unmarshal(out, &response) == nil && response.Surfaces != nil {
		for _, surface := range *response.Surfaces {
			if surface.ID == l.surface {
				return fmt.Errorf("closing cmux tab: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("closing cmux tab: %w", err)
}

func (l cmuxLaunch) restoreFocus() {
	_ = runCommand(l.context.command(context.Background(), l.bin, "focus-panel", "--workspace", l.context.workspace, "--panel", l.context.surface))
}

func spawnTUICMUXTab(mode *reviewMode, c cmuxContext) (cmuxLaunch, error) {
	bin, err := lookPath("cmux")
	if err != nil {
		return cmuxLaunch{}, err
	}
	command, err := buildTUICommand(mode, "cmux")
	if err != nil {
		return cmuxLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return cmuxLaunch{}, err
	}
	ctx := context.Background()
	out, err := commandOutput(c.command(ctx, bin, "--json", "--id-format", "uuids", "identify", "--workspace", c.workspace, "--surface", c.surface))
	if err != nil {
		return cmuxLaunch{}, fmt.Errorf("locating cmux pane: %w", err)
	}
	var location struct {
		Caller struct {
			Pane string `json:"pane_id"`
		} `json:"caller"`
	}
	if err := json.Unmarshal(out, &location); err != nil {
		return cmuxLaunch{}, fmt.Errorf("parsing cmux location: %w", err)
	}
	if location.Caller.Pane == "" {
		return cmuxLaunch{}, fmt.Errorf("missing caller pane in cmux location")
	}
	out, err = commandOutput(c.command(ctx, bin, "--json", "--id-format", "uuids", "new-surface", "--type", "terminal", "--workspace", c.workspace, "--pane", location.Caller.Pane, "--working-directory", cwd, "--focus", "false", "--command", "exec "+command))
	if err != nil {
		return cmuxLaunch{}, fmt.Errorf("opening cmux tab: %w", err)
	}
	var created struct {
		Surface string `json:"surface_id"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		return cmuxLaunch{}, fmt.Errorf("parsing cmux tab: %w", err)
	}
	if created.Surface == "" {
		return cmuxLaunch{}, fmt.Errorf("missing surface ID in cmux tab response")
	}
	l := cmuxLaunch{context: c, bin: bin, surface: created.Surface}
	if err := runCommand(c.command(ctx, bin, "focus-panel", "--workspace", c.workspace, "--panel", l.surface)); err != nil {
		cleanupErr := l.close()
		l.restoreFocus()
		return cmuxLaunch{}, fmt.Errorf("focusing cmux tab: %w", errors.Join(err, cleanupErr))
	}
	fmt.Fprintln(os.Stderr, "Opened review in cmux tab")
	return l, nil
}
