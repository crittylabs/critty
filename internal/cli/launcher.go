package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var reviewLauncherNames []string

var errNoReviewLauncher = errors.New("no usable review launcher")

func init() {
	rootCmd.PersistentFlags().StringArrayVar(&reviewLauncherNames, "launcher", nil, "review launcher (repeat to set priority, default mux, desktop-terminal): mux, tmux, tmux-floating, tmux-split, herdr, cmux, desktop-terminal, xdg-terminal, ghostty-window, iterm-window, terminal-app-window, rio-window, windows-terminal-wsl, or custom:EXECUTABLE")
}

type reviewLauncher struct {
	name  string
	check func() error
	open  func(*reviewMode) (reviewLaunch, error)
}

// Selection checks availability before opening anything.  Once open starts,
// failures must be returned rather than risk creating a second review.
func selectReviewLauncher(names []string, terminal reviewMultiplexerContext, mode *reviewMode) (*reviewLauncher, error) {
	names = defaultLauncherOrder(names)
	groups := make([][]reviewLauncher, len(names))
	for i, name := range names {
		candidates, err := launcherCandidates(name, terminal, mode)
		if err != nil {
			return nil, err
		}
		groups[i] = candidates
	}
	checked := make(map[string]bool)
	var unavailable []error
	for i, candidates := range groups {
		if names[i] == "mux" && terminal == nil {
			if !checked["mux"] {
				unavailable = append(unavailable, errors.New("launcher mux: no tmux, Herdr, or cmux terminal detected"))
				checked["mux"] = true
			}
		}
		for _, candidate := range candidates {
			if checked[candidate.name] {
				continue
			}
			checked[candidate.name] = true
			if err := candidate.check(); err != nil {
				unavailable = append(unavailable, fmt.Errorf("launcher %s: %w", candidate.name, err))
				continue
			}
			return &candidate, nil
		}
	}
	return nil, fmt.Errorf("%w: %w", errNoReviewLauncher, errors.Join(unavailable...))
}

func launcherCandidates(name string, terminal reviewMultiplexerContext, mode *reviewMode) ([]reviewLauncher, error) {
	if name == "desktop-terminal" || slices.Contains(desktopNames, name) {
		return desktopCandidates(name, terminal)
	}
	if name == "mux" {
		if terminal == nil {
			return nil, nil
		}
		name = terminal.kind()
	}
	switch name {
	case "tmux":
		floating, err := launcherCandidates("tmux-floating", terminal, mode)
		if err != nil {
			return nil, err
		}
		split, err := launcherCandidates("tmux-split", terminal, mode)
		return append(floating, split...), err
	case "tmux-floating", "tmux-split":
		ctx, ok := launcherTerminal("tmux", terminal).(tmuxContext)
		floating := name == "tmux-floating"
		return []reviewLauncher{{name: name, check: func() error {
			if !ok {
				return fmt.Errorf("no tmux terminal")
			}
			bin, err := lookPath("tmux")
			if err != nil || !floating {
				return err
			}
			supported, err := supportsTMUXFloat(bin, ctx)
			if err != nil {
				return err
			}
			if !supported {
				return fmt.Errorf("floating panes require tmux 3.8+")
			}
			return nil
		}, open: func(mode *reviewMode) (reviewLaunch, error) {
			return spawnTMUXPane(mode, ctx, floating)
		}}}, nil
	case "cmux":
		ctx, ok := launcherTerminal(name, terminal).(cmuxContext)
		return []reviewLauncher{{name: name, check: func() error {
			if !ok {
				return fmt.Errorf("no cmux terminal")
			}
			_, err := lookPath("cmux")
			return err
		}, open: func(mode *reviewMode) (reviewLaunch, error) {
			return spawnTUICMUXTab(mode, ctx)
		}}}, nil
	case "herdr":
		ctx, ok := launcherTerminal(name, terminal).(herdrContext)
		return []reviewLauncher{{name: name, check: func() error {
			if !ok {
				return fmt.Errorf("no Herdr terminal")
			}
			_, err := lookPath("herdr")
			return err
		}, open: func(mode *reviewMode) (reviewLaunch, error) {
			return spawnTUIHerdrTab(mode, ctx)
		}}}, nil
	default:
		file, custom := strings.CutPrefix(name, "custom:")
		if !custom || file == "" {
			return nil, fmt.Errorf("unknown launcher %q; use mux, tmux, tmux-floating, tmux-split, herdr, cmux, desktop-terminal, a named desktop launcher, or custom:EXECUTABLE", name)
		}
		launcher, err := newCustomLauncher(file, terminal, mode)
		if err != nil {
			return nil, err
		}
		return []reviewLauncher{launcher}, nil
	}
}

// Explicit host selection may find an enclosing multiplexer.  A marker-bound
// terminal is authoritative; do not substitute the daemon's own ancestry.
func launcherTerminal(kind string, terminal reviewMultiplexerContext) reviewMultiplexerContext {
	if terminal != nil && terminal.kind() == kind {
		return terminal
	}
	if terminalRequest != "" {
		return nil
	}
	var detector multiplexerDetector = tmuxDetector{}
	if kind == "herdr" {
		detector = herdrDetector{}
	}
	if kind == "cmux" {
		detector = cmuxDetector{}
	}
	if ctx := walkAncestorContexts([]multiplexerDetector{detector}); ctx != nil {
		return ctx
	}
	if detector.environmentPresent() {
		return detector.environmentContext()
	}
	return nil
}

type launcherRequest struct {
	Session string `json:"session"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

type customLaunch struct {
	executable  string
	environment []string
	token       []byte
}

func newCustomLauncher(file string, terminal reviewMultiplexerContext, mode *reviewMode) (reviewLauncher, error) {
	command, err := buildTUICommand(mode, "custom")
	if err != nil {
		return reviewLauncher{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return reviewLauncher{}, err
	}
	request, err := json.Marshal(launcherRequest{Session: mode.sessionKey, Command: command, Cwd: cwd})
	if err != nil {
		return reviewLauncher{}, err
	}
	launch := &customLaunch{}
	return reviewLauncher{name: "custom:" + file, check: func() error {
		// LookPath also handles explicit paths.  ErrDot is allowed only when
		// PATH explicitly contains a relative entry (including an empty one).
		path, err := exec.LookPath(file)
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return err
		}
		launch.executable, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		vars, err := environmentForTerminal(terminal)
		if err != nil {
			return err
		}
		launch.environment = launcherEnvironment(vars)
		_, err = launch.call("check", request)
		return err
	}, open: func(*reviewMode) (reviewLaunch, error) {
		token, err := launch.call("open", request)
		if err != nil {
			return nil, err
		}
		launch.token = token
		return launch, nil
	}}, nil
}

func launcherEnvironment(vars []envVar) []string {
	var environment []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if key == "TMUX" || key == "TMUX_PANE" || key == "TTY" || key == "CRITTY_SSH" || key == "CRITTY_TERMINAL_APP" || strings.HasPrefix(key, "HERDR_") || strings.HasPrefix(key, "CMUX_") && key != "CMUX_SOCKET_PASSWORD" {
			continue
		}
		environment = append(environment, value)
	}
	return mergeEnvironment(environment, vars)
}

func (l *customLaunch) call(operation string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, l.executable, operation)
	cmd.WaitDelay = time.Second
	cmd.Env, cmd.Stdin, cmd.Stderr = l.environment, bytes.NewReader(input), os.Stderr
	var diagnostic bytes.Buffer
	if operation == "check" {
		cmd.Stderr = &diagnostic
	}
	output, err := cmd.Output()
	if err != nil {
		if detail := strings.TrimSpace(diagnostic.String()); detail != "" {
			return nil, fmt.Errorf("custom launcher %s: %w: %s", operation, err, detail)
		}
		return nil, fmt.Errorf("custom launcher %s: %w", operation, err)
	}
	return output, nil
}

func (l *customLaunch) close() error {
	_, err := l.call("close", l.token)
	return err
}

func (*customLaunch) restoreFocus() {}
