package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type tmuxPane struct {
	session, window, pane string
	width, height         int
}

func spawnTUIPane(mode *reviewMode, target tmuxContext) (tmuxLaunch, error) {
	bin, err := lookPath("tmux")
	if err != nil {
		return tmuxLaunch{}, err
	}
	source, err := describeTMUXPane(bin, target)
	if err != nil {
		return tmuxLaunch{}, err
	}
	target.pane = source.pane
	cwd, err := os.Getwd()
	if err != nil {
		return tmuxLaunch{}, err
	}
	command, err := buildTUICommand(mode, "tmux")
	if err != nil {
		return tmuxLaunch{}, err
	}
	// Keep the pane before the TUI can exit.  An empty pane followed by
	// respawn-pane retains tmux's CRLF mode and breaks differential drawing.
	command = shellEscape(bin) + " set-option -p -t \"$TMUX_PANE\" remain-on-exit on && exec " + command
	args := []string{"split-window", "-h", "-p", "70"}
	args = append(args, "-d", "-P", "-F", "#{pane_id}", "-t", source.pane, "-c", cwd, command)
	out, err := commandOutput(tmuxCommand(bin, target, args...))
	if err != nil {
		args = append([]string{"split-window", "-h"}, args[4:]...)
		out, err = commandOutput(tmuxCommand(bin, target, args...))
	}
	if err != nil {
		return tmuxLaunch{}, fmt.Errorf("opening tmux pane: %w", err)
	}
	pane := strings.TrimSpace(string(out))
	if !validTMUXPaneID(pane) {
		return tmuxLaunch{}, fmt.Errorf("tmux did not return a pane ID")
	}
	launch := tmuxLaunch{bin: bin, context: target, pane: pane, source: source}
	started := false
	defer func() {
		if !started {
			launch.close()
		}
	}()
	client, err := focusedTMUXClient(bin, target, source.session, source.pane)
	if err != nil {
		fmt.Fprintf(os.Stderr, "critty: checking tmux focus: %v\n", err)
	} else if client != "" {
		if err := runCommand(tmuxCommand(bin, target, "select-pane", "-t", pane)); err != nil {
			return tmuxLaunch{}, fmt.Errorf("focusing tmux review: %w", err)
		}
	}
	started = true
	fmt.Fprintf(os.Stderr, "Opened review in tmux pane %s; focused: %t\n", pane, client != "")
	return launch, nil
}

func validTMUXPaneID(pane string) bool {
	if !strings.HasPrefix(pane, "%") {
		return false
	}
	_, err := strconv.ParseUint(strings.TrimPrefix(pane, "%"), 10, 64)
	return err == nil
}

func describeTMUXPane(bin string, target tmuxContext) (tmuxPane, error) {
	args := []string{"display-message", "-p"}
	if target.pane != "" {
		args = append(args, "-t", target.pane)
	}
	args = append(args, "#{session_id}\t#{window_id}\t#{pane_id}\t#{window_width}\t#{window_height}")
	out, err := commandOutput(tmuxCommand(bin, target, args...))
	if err != nil {
		return tmuxPane{}, fmt.Errorf("querying tmux source pane: %w", err)
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\t")
	if len(fields) != 5 || fields[0] == "" || fields[1] == "" || !validTMUXPaneID(fields[2]) {
		return tmuxPane{}, fmt.Errorf("invalid tmux source pane description")
	}
	width, widthErr := strconv.Atoi(fields[3])
	height, heightErr := strconv.Atoi(fields[4])
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 {
		return tmuxPane{}, fmt.Errorf("invalid tmux window size")
	}
	return tmuxPane{fields[0], fields[1], fields[2], width, height}, nil
}

func focusedTMUXClient(bin string, target tmuxContext, session, pane string) (string, error) {
	out, err := commandOutput(tmuxCommand(bin, target, "show-options", "-sv", "focus-events"))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(out)) != "on" {
		return "", nil
	}
	out, err = commandOutput(tmuxCommand(bin, target, "list-clients", "-F", "#{client_name}\t#{session_id}\t#{pane_id}\t#{client_flags}"))
	if err != nil {
		return "", err
	}
	var clients [][]string
	for line := range strings.SplitSeq(strings.TrimSuffix(string(out), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 4 && fields[1] == session {
			clients = append(clients, fields)
		}
	}
	if len(clients) != 1 || clients[0][2] != pane {
		return "", nil
	}
	for flag := range strings.SplitSeq(clients[0][3], ",") {
		if flag == "focused" {
			return clients[0][0], nil
		}
	}
	return "", nil
}

type tmuxContext struct {
	session string
	pane    string
	socket  string
}

type tmuxDetector struct{}

func (tmuxDetector) environmentPresent() bool {
	return os.Getenv("TMUX") != "" || os.Getenv("TMUX_PANE") != ""
}

func (tmuxDetector) environmentContext() reviewMultiplexerContext {
	ctx := tmuxContext{session: os.Getenv("TMUX"), pane: os.Getenv("TMUX_PANE")}
	if !ctx.active() {
		return nil
	}
	return ctx
}

func (tmuxDetector) processContexts() map[int]reviewMultiplexerContext {
	contexts := make(map[int]reviewMultiplexerContext)
	for pid, ctx := range tmuxProcessContexts() {
		contexts[pid] = ctx
	}
	return contexts
}

func (tmuxContext) kind() string { return "tmux" }

type tmuxLaunch struct {
	bin     string
	context tmuxContext
	pane    string
	source  tmuxPane
}

func (l tmuxLaunch) close() error {
	if l.source.pane != "" {
		client, err := focusedTMUXClient(l.bin, l.context, l.source.session, l.pane)
		if err != nil {
			fmt.Fprintf(os.Stderr, "critty: checking tmux focus: %v\n", err)
		} else if client != "" {
			// Resolve the source again: it may have moved or closed during review.
			window, err := commandOutput(tmuxCommand(l.bin, l.context, "display-message", "-p", "-t", l.source.pane, "#{window_id}"))
			if err == nil && len(window) > 0 {
				if err := runCommand(tmuxCommand(l.bin, l.context, "select-window", "-t", strings.TrimSpace(string(window)))); err != nil {
					fmt.Fprintf(os.Stderr, "critty: restoring tmux window: %v\n", err)
				} else if err := runCommand(tmuxCommand(l.bin, l.context, "select-pane", "-t", l.source.pane)); err != nil {
					fmt.Fprintf(os.Stderr, "critty: restoring tmux pane: %v\n", err)
				}
			}
		}
	}
	if err := runCommand(tmuxCommand(l.bin, l.context, "kill-pane", "-t", l.pane)); err != nil {
		return fmt.Errorf("closing tmux pane: %w", err)
	}
	return nil
}
func (tmuxLaunch) restoreFocus() {}

func (c tmuxContext) active() bool {
	return c.session != "" || c.socket != "" || c.pane != ""
}

func tmuxProcessContexts() map[int]tmuxContext {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	panes := tmuxPIDContexts(ctx, tmuxBin, "list-panes", "-a", "-F", "#{pane_pid}\t#{socket_path}\t#{pid}\t#{session_id}\t#{pane_id}")
	clients := tmuxPIDContexts(ctx, tmuxBin, "list-clients", "-F", "#{client_pid}\t#{socket_path}\t#{pid}\t#{session_id}")
	if panes == nil {
		panes = make(map[int]tmuxContext)
	}
	for pid, ctx := range clients {
		if _, exists := panes[pid]; !exists {
			panes[pid] = ctx
		}
	}
	return panes
}

func tmuxPIDContexts(ctx context.Context, tmuxBin string, args ...string) map[int]tmuxContext {
	out, err := commandOutput(exec.CommandContext(ctx, tmuxBin, args...))
	if err != nil {
		return nil
	}
	contexts := make(map[int]tmuxContext)
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(fields) != 4 && len(fields) != 5 {
			continue
		}
		processID, err := strconv.Atoi(fields[0])
		if err != nil || processID < 2 {
			continue
		}
		socket, serverPID, sessionID := fields[1], fields[2], strings.TrimPrefix(fields[3], "$")
		if socket == "" || serverPID == "" || sessionID == "" {
			continue
		}
		ctx := tmuxContext{session: strings.Join([]string{socket, serverPID, sessionID}, ",")}
		if len(fields) == 5 {
			ctx.pane = fields[4]
		}
		if _, exists := contexts[processID]; !exists {
			contexts[processID] = ctx
		}
	}
	return contexts
}

func tmuxCommand(tmuxBin string, tmux tmuxContext, args ...string) *exec.Cmd {
	if tmux.socket != "" {
		return exec.Command(tmuxBin, append([]string{"-S", tmux.socket}, args...)...)
	}
	cmd := exec.Command(tmuxBin, args...)
	if os.Getenv("TMUX") == "" && tmux.session != "" {
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "TMUX=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "TMUX="+tmux.session)
	}
	return cmd
}
