package cli

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

//go:embed launchers/*.applescript launchers/*.sh
var desktopScripts embed.FS

var desktopNames = []string{"xdg-terminal", "ghostty-window", "kitty-window", "iterm-window", "rio-window", "wezterm-window", "alacritty-window", "terminal-app-window", "windows-terminal-wsl", "xterm-window"}

func defaultLauncherOrder(names []string) []string {
	order := slices.Clone(names)
	for _, name := range []string{"mux", "desktop-terminal"} {
		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}
	return order
}

func desktopOrder(platform string, origin terminalbinding.Origin) []string {
	var names []string
	var imageCount int
	switch platform {
	case "darwin":
		names = []string{"ghostty-window", "kitty-window", "iterm-window", "rio-window", "wezterm-window", "alacritty-window", "terminal-app-window"}
		imageCount = 3
	case "linux":
		if origin.WSLDistribution != "" {
			names = []string{"rio-window", "wezterm-window", "alacritty-window", "windows-terminal-wsl"}
		} else {
			names = []string{"ghostty-window", "kitty-window", "rio-window", "wezterm-window", "alacritty-window", "xdg-terminal", "xterm-window"}
			imageCount = 2
		}
	}
	// Prefer the client's terminal within its image-capability tier, keeping
	// the system terminal as the last resort. Explicit launcher order wins.
	if i := slices.Index(names, origin.Terminal+"-window"); i >= 0 && origin.Terminal != "terminal-app" && origin.Terminal != "xterm" {
		first := 0
		if i >= imageCount {
			first = imageCount
		}
		name := names[i]
		copy(names[first+1:i+1], names[first:i])
		names[first] = name
	}
	return names
}

func terminalTarget(terminal reviewMultiplexerContext) terminalbinding.Target {
	switch t := terminal.(type) {
	case zellijContext:
		return t.target()
	case cmuxContext:
		return t.target()
	case tmuxContext:
		socket := t.socket
		if socket == "" {
			raw := t.session
			end := strings.LastIndexByte(raw, ',')
			if end > 0 {
				end = strings.LastIndexByte(raw[:end], ',')
			}
			if end > 0 {
				socket = raw[:end]
			}
		}
		return terminalbinding.Target{Kind: "tmux", Socket: socket, Pane: t.pane}
	case herdrContext:
		return terminalbinding.Target{Kind: "herdr", Socket: t.socket, Pane: t.pane, Workspace: t.workspace, Tab: t.tab}
	}
	return terminalbinding.Target{}
}

func originForReview() terminalbinding.Origin {
	if terminalRequest != "" || reviewOrigin.PID != 0 {
		return reviewOrigin
	}
	return terminalbinding.CaptureOrigin(os.Getpid())
}

func desktopCandidates(name string, terminal reviewMultiplexerContext) ([]reviewLauncher, error) {
	origin := originForReview()
	names := []string{name}
	if name == "desktop-terminal" {
		names = desktopOrder(runtime.GOOS, origin)
	}
	if len(names) == 0 {
		return []reviewLauncher{{name: name, check: func() error { return fmt.Errorf("unsupported desktop platform %s", runtime.GOOS) }}}, nil
	}
	var result []reviewLauncher
	// Resolve SSH once per candidate group, and do not launch any GUI on a positive result.
	var remote, checkedRemote bool
	for _, candidate := range names {
		backend := &desktopLaunch{name: candidate, origin: origin}
		result = append(result, reviewLauncher{name: candidate, check: func() error {
			if !checkedRemote {
				remote = terminalbinding.Remote(terminalTarget(terminal), origin)
				checkedRemote = true
			}
			if remote {
				return fmt.Errorf("GUI terminal launch disabled for an SSH connection")
			}
			return backend.check()
		}, open: func(mode *reviewMode) (reviewLaunch, error) { return backend.open(mode) }})
	}
	return result, nil
}

type desktopLaunch struct {
	name   string
	origin terminalbinding.Origin
	token  []byte
}

func (l *desktopLaunch) check() error {
	if l.usesCLI() {
		if l.name == "xterm-window" && os.Getenv("DISPLAY") == "" {
			return fmt.Errorf("requires an X11 display (or Xwayland)")
		}
		if l.origin.WSLDistribution != "" {
			if _, err := lookPath("wsl.exe"); err != nil {
				return err
			}
		}
		_, err := terminalExecutable(l.name, runtime.GOOS, l.origin.WSLDistribution != "")
		return err
	}
	switch l.name {
	case "xdg-terminal":
		if runtime.GOOS != "linux" {
			return fmt.Errorf("requires Linux")
		}
		if _, err := lookPath("xdg-terminal-exec"); err != nil {
			return err
		}
	case "windows-terminal-wsl":
		if runtime.GOOS != "linux" || l.origin.WSLDistribution == "" {
			return fmt.Errorf("requires WSL")
		}
		for _, command := range []string{"wt.exe", "wsl.exe"} {
			if _, err := lookPath(command); err != nil {
				return err
			}
		}
	default:
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("requires macOS")
		}
		if _, err := lookPath("osascript"); err != nil {
			return err
		}
		app := map[string]string{"iterm-window": "iTerm.app", "ghostty-window": "Ghostty.app", "terminal-app-window": "Terminal.app"}[l.name]
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		var installed string
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications"), "/System/Applications/Utilities"} {
			candidate := filepath.Join(dir, app)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				installed = candidate
				break
			}
		}
		if installed == "" {
			return fmt.Errorf("%s is not installed in an Applications directory", app)
		}
		if l.name == "ghostty-window" {
			// Read the dictionary without starting the application or asking for automation permission.
			out, err := commandOutput(exec.Command("/usr/bin/sdef", installed))
			if err != nil {
				return fmt.Errorf("reading Ghostty scripting interface: %w", err)
			}
			if !ghosttyScriptingAvailable(out) {
				return fmt.Errorf("requires Ghostty 1.3+ AppleScript support")
			}
		}
		return nil
	}
	_, err := l.call("check", "", "")
	return err
}

func (l *desktopLaunch) open(mode *reviewMode) (reviewLaunch, error) {
	command, err := buildTUICommand(mode, l.name)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// GUI applications do not inherit the invoking shell's cwd or environment.
	command = "/bin/sh -c " + shellEscape("cd "+shellEscape(cwd)+" && exec "+strings.Replace(command, "env ", "env -u NO_COLOR -u TMUX -u TMUX_PANE -u HERDR_ENV -u HERDR_SOCKET_PATH -u HERDR_WORKSPACE_ID -u HERDR_TAB_ID -u HERDR_PANE_ID ", 1))
	if l.name == "xdg-terminal" || l.name == "windows-terminal-wsl" || l.usesCLI() {
		if err := l.start(command, cwd); err != nil {
			return nil, err
		}
		return l, nil
	}
	token, err := l.call("open", command, cwd)
	if err != nil {
		return nil, err
	}
	l.token = bytes.TrimSpace(token)
	if strings.HasSuffix(l.name, "-window") && len(l.token) == 0 {
		return nil, fmt.Errorf("launcher %s returned no window ID", l.name)
	}
	fmt.Fprintf(os.Stderr, "Opened review with %s\n", l.name)
	return l, nil
}

func (l *desktopLaunch) call(operation, command, cwd string) ([]byte, error) {
	suffix := ".applescript"
	if l.name == "xdg-terminal" || l.name == "windows-terminal-wsl" {
		suffix = ".sh"
	}
	script, err := desktopScripts.ReadFile("launchers/" + l.name + suffix)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if suffix == ".applescript" {
		cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-", operation, command, cwd)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", string(script), l.name, operation, command, cwd, l.origin.WSLDistribution)
	}
	cmd.Stdin = bytes.NewReader(script)
	cmd.WaitDelay = time.Second
	// Tool hosts may set NO_COLOR for their own output. GUI reviews should use color.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "NO_COLOR=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("launcher %s %s: %w: %s", l.name, operation, err, strings.TrimSpace(diagnostic.String()))
	}
	return out, nil
}

func (l *desktopLaunch) close() error {
	if l.usesCLI() {
		return nil
	}
	if _, err := l.call("close", string(l.token), ""); err != nil {
		// The terminal may already have removed its window, or be waiting for
		// confirmation. Cleanup must not invalidate the completed review.
		fmt.Fprintf(os.Stderr, "critty: review finished; could not close %s window: %v\n", l.name, err)
	}
	return nil
}
func (*desktopLaunch) restoreFocus() {}

// Generic desktop launchers have no window API. Let the terminal own the
// command lifetime; do not kill its process when the review is submitted.
func (l *desktopLaunch) start(command, cwd string) error {
	var cmd *exec.Cmd
	if l.usesCLI() {
		var err error
		cmd, err = l.terminalCommand(runtime.GOOS, command, cwd)
		if err != nil {
			return err
		}
	} else {
		script, err := desktopScripts.ReadFile("launchers/" + l.name + ".sh")
		if err != nil {
			return err
		}
		if l.name == "windows-terminal-wsl" {
			command = base64.StdEncoding.EncodeToString([]byte(command))
		}
		cmd = exec.Command("/bin/sh", "-c", string(script), l.name, "open", command, cwd, l.origin.WSLDistribution)
	}
	for _, entry := range launcherEnvironment(nil) {
		if !strings.HasPrefix(entry, "NO_COLOR=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launcher %s open: %w", l.name, err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "critty: launcher %s: %v\n", l.name, err)
		}
	}()
	fmt.Fprintf(os.Stderr, "Opened review with %s\n", l.name)
	return nil
}

func ghosttyScriptingAvailable(dictionary []byte) bool {
	var definition struct {
		Suites []struct {
			Commands []struct {
				Name string `xml:"name,attr"`
			} `xml:"command"`
			Records []struct {
				Name string `xml:"name,attr"`
			} `xml:"record-type"`
		} `xml:"suite"`
	}
	if xml.Unmarshal(dictionary, &definition) != nil {
		return false
	}
	var window, activation, configuration bool
	for _, suite := range definition.Suites {
		for _, command := range suite.Commands {
			window = window || command.Name == "new window"
			activation = activation || command.Name == "activate window"
		}
		for _, record := range suite.Records {
			configuration = configuration || record.Name == "surface configuration"
		}
	}
	return window && activation && configuration
}
