package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

type rexContext struct{ place terminalbinding.Target }

func (rexContext) kind() string                     { return "rex" }
func (c rexContext) target() terminalbinding.Target { return c.place }
func (c rexContext) command(args ...string) *exec.Cmd {
	return c.place.Command(context.Background(), args...)
}

type rexDetector struct{}

func (rexDetector) environmentPresent() bool {
	return os.Getenv("REX_SESSION") != "" || os.Getenv("REX_BLOCK") != ""
}

func (rexDetector) environmentContext() reviewMultiplexerContext {
	t := terminalbinding.RexEnvironment()
	if t.Validate() != nil {
		return nil
	}
	return rexContext{t}
}

func (d rexDetector) processContexts() map[int]reviewMultiplexerContext {
	c, ok := d.environmentContext().(rexContext)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	owners := make(map[int]reviewMultiplexerContext)
	for pid := range terminalbinding.RexOwners(ctx, c.place) {
		owners[pid] = c
	}
	return owners
}

type rexBlock struct {
	ID      string `json:"block_id"`
	Creator string `json:"creator_name"`
	Window  string `json:"window_id"`
	Owner   string `json:"owner"`
}

func rexBlocks(c rexContext) ([]rexBlock, error) {
	out, err := commandOutput(c.command("block", "ls", "-s", c.place.Workspace, "--json"))
	if err != nil {
		return nil, err
	}
	var result struct {
		Blocks *[]rexBlock `json:"blocks"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	if result.Blocks == nil {
		return nil, fmt.Errorf("missing Rex block inventory")
	}
	for _, b := range *result.Blocks {
		if !terminalbinding.RexID(b.ID, "block") {
			return nil, fmt.Errorf("invalid Rex block inventory")
		}
	}
	return *result.Blocks, nil
}

func checkRexLauncher(c rexContext) error {
	_, err := rexCaller(c)
	return err
}

func rexCaller(c rexContext) (rexBlock, error) {
	if runtime.GOOS == "windows" {
		return rexBlock{}, fmt.Errorf("Rex reviews require a Unix host with a POSIX shell")
	}
	if err := c.place.Validate(); err != nil {
		return rexBlock{}, err
	}
	blocks, err := rexBlocks(c)
	if err != nil {
		return rexBlock{}, err
	}
	for _, b := range blocks {
		if b.ID == c.place.Pane && b.Creator == "com.superlogical.terminal" {
			return b, nil
		}
	}
	return rexBlock{}, fmt.Errorf("Rex caller is not a terminal in the recorded session")
}

type rexLaunch struct {
	context rexContext
	block   string
	source  rexBlock
}

func (l rexLaunch) restoreFocus() {
	_ = runCommand(l.context.command("focus", "-s", l.context.place.Workspace, l.context.place.Pane))
	if err := l.focusWindow(l.source.Window); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not return to the Rex source tab: %v\n", err)
	}
}

func (l rexLaunch) focusWindow(window string) error {
	if !terminalbinding.RexID(l.source.Owner, "client") || !terminalbinding.RexID(window, "window") {
		return fmt.Errorf("Rex app ownership is unavailable; select the tab manually")
	}
	cmd := l.context.command("-C", l.source.Owner, "do", "session.select", "session_id="+l.context.place.Workspace, "window_id="+window)
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err := runCommand(cmd); err != nil {
		if message := strings.TrimSpace(diagnostic.String()); message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func (l rexLaunch) close() error {
	err := runCommand(l.context.command("block", "close", "-s", l.context.place.Workspace, l.block))
	if err == nil {
		return nil
	}
	blocks, checkErr := rexBlocks(l.context)
	if checkErr == nil {
		for _, b := range blocks {
			if b.ID == l.block {
				return fmt.Errorf("closing Rex review: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("closing Rex review: %w", err)
}

func spawnRexReview(mode *reviewMode, c rexContext) (rexLaunch, error) {
	source, err := rexCaller(c)
	if err != nil {
		return rexLaunch{}, err
	}
	command, err := buildTUICommand(mode, "Rex")
	if err != nil {
		return rexLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return rexLaunch{}, err
	}
	out, err := commandOutput(c.command("run", "-s", c.place.Workspace, "-c", cwd, "--label", "Critty", "--shell=none", "--focus", "--json", "--", "/bin/sh", "-c", "exec "+command))
	if err != nil {
		return rexLaunch{}, fmt.Errorf("opening Rex review: %w", err)
	}
	var result struct {
		Window string   `json:"window_id"`
		Blocks []string `json:"block_ids"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return rexLaunch{}, err
	}
	if !terminalbinding.RexID(result.Window, "window") || len(result.Blocks) != 1 || !terminalbinding.RexID(result.Blocks[0], "block") || result.Blocks[0] == c.place.Pane {
		return rexLaunch{}, fmt.Errorf("invalid Rex review creation result")
	}
	launch := rexLaunch{context: c, block: result.Blocks[0], source: source}
	if err := launch.focusWindow(result.Window); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Rex review opened without app focus: %v. Select the review tab manually.\n", err)
	}
	fmt.Fprintln(os.Stderr, "Opened review in Rex tab")
	return launch, nil
}

func (r *doctorReport) checkRex() {
	d := rexDetector{}
	const check = "launchers/rex"
	if _, err := terminalbinding.RexExecutable(); err != nil {
		if d.environmentPresent() {
			r.add(check, "warn", err.Error(), "Install the matching Rex CLI.")
		}
		return
	}
	if !d.environmentPresent() {
		r.add(check, "info", "Rex CLI is available; no invoking terminal identity", "Run this check inside a Rex terminal.")
		return
	}
	c, ok := d.environmentContext().(rexContext)
	if !ok {
		r.add(check, "warn", "incomplete or unsupported Rex terminal identity", "Use a local Rex Unix endpoint with REX_SERVER, REX_SESSION, and REX_BLOCK.")
		return
	}
	if err := checkRexLauncher(c); err != nil {
		r.add(check, "warn", err.Error(), "Check the Rex server connection.")
		return
	}
	r.add(check, "ok", "Rex terminal is available in the recorded session", "Preview marker binding requires a notifying terminal filter; capture includes scrollback.")
}
