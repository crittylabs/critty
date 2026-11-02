package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

// herdrAppContext is a terminal running in herdr-core.  The Herdr desktop app
// shows it when its labels name a space; the launcher opens the review as a
// tab of that space through the app's command socket.
type herdrAppContext struct {
	socket, terminal string
}

func (herdrAppContext) kind() string { return "herdr-app" }

func (c herdrAppContext) target() terminalbinding.Target {
	return terminalbinding.Target{Kind: "herdr-app", Socket: c.socket, Pane: c.terminal}
}

func (c herdrAppContext) command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := c.target().Command(ctx, args...)
	cmd.Path, cmd.Args[0] = bin, bin
	return cmd
}

type herdrAppDetector struct{}

func (herdrAppDetector) environmentPresent() bool {
	return os.Getenv("HC_TERMINAL_ID") != ""
}

func (herdrAppDetector) environmentContext() reviewMultiplexerContext {
	t := terminalbinding.HCEnvironment()
	if t.Validate() != nil {
		return nil
	}
	return herdrAppContext{socket: t.Socket, terminal: t.Pane}
}

func (herdrAppDetector) processContexts() map[int]reviewMultiplexerContext {
	if _, err := lookPath("hc"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	socket := terminalbinding.HCSocketPath()
	terminals, err := hcTerminals(ctx, herdrAppContext{socket: socket})
	if err != nil {
		return nil
	}
	contexts := make(map[int]reviewMultiplexerContext)
	for pid, t := range terminalbinding.HCProcessTargets(terminals, socket) {
		contexts[pid] = herdrAppContext{socket: t.Socket, terminal: t.Pane}
	}
	return contexts
}

func hcTerminals(ctx context.Context, c herdrAppContext) ([]terminalbinding.HCTerminal, error) {
	bin, err := lookPath("hc")
	if err != nil {
		return nil, err
	}
	cmd := c.command(ctx, bin, "connect")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"terminal.list","params":{}}` + "\n")
	out, err := commandOutput(cmd)
	if err != nil {
		return nil, err
	}
	return terminalbinding.DecodeHCTerminals(out)
}

// herdrAppPlace reads the space and tab that show the terminal.
func herdrAppPlace(c herdrAppContext) (space, tab string, err error) {
	bin, err := lookPath("hc")
	if err != nil {
		return "", "", err
	}
	out, err := commandOutput(c.command(context.Background(), bin, "get", "--json", c.terminal))
	if err != nil {
		return "", "", fmt.Errorf("reading herdr-core terminal %s: %w", c.terminal, err)
	}
	var record terminalbinding.HCTerminal
	if err := json.Unmarshal(out, &record); err != nil {
		return "", "", fmt.Errorf("parsing herdr-core terminal %s: %w", c.terminal, err)
	}
	space, tab = record.Labels["herdr.space"], record.Labels["herdr.tab"]
	if space == "" {
		return "", "", fmt.Errorf("terminal %s is not shown in a Herdr window", c.terminal)
	}
	return space, tab, nil
}

type herdrAppLaunch struct {
	context                    herdrAppContext
	bin, tab, space, sourceTab string
}

// herdrAppDo runs a window command and returns its JSON reply.
func herdrAppDo(bin, action string, args any) (map[string]any, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := commandOutput(exec.CommandContext(ctx, bin, "do", action, string(encoded)))
	var reply map[string]any
	if len(out) > 0 && json.Unmarshal(out, &reply) == nil {
		if message, failed := reply["error"].(string); failed {
			return reply, fmt.Errorf("%s: %s", action, message)
		}
	}
	if err != nil {
		return reply, fmt.Errorf("%s: %w", action, err)
	}
	return reply, nil
}

func (l herdrAppLaunch) close() error {
	_, err := herdrAppDo(l.bin, "tab.close", map[string]any{"id": l.tab, "confirmed": true})
	if err == nil || herdrAppHasTab(l.bin, l.tab) {
		return err
	}
	// The window may drop the tab once the TUI process exits.
	return nil
}

func herdrAppHasTab(bin, tab string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := commandOutput(exec.CommandContext(ctx, bin, "ls"))
	if err != nil {
		return true
	}
	var world struct {
		Spaces []struct {
			Tabs []struct {
				ID string `json:"id"`
			} `json:"tabs"`
		} `json:"spaces"`
	}
	if json.Unmarshal(out, &world) != nil {
		return true
	}
	for _, space := range world.Spaces {
		for _, t := range space.Tabs {
			if t.ID == tab {
				return true
			}
		}
	}
	return false
}

func (l herdrAppLaunch) restoreFocus() {
	if l.sourceTab != "" {
		_, _ = herdrAppDo(l.bin, "tab.select", map[string]any{"id": l.sourceTab})
	}
	_, _ = herdrAppDo(l.bin, "pane.focus", map[string]any{"terminal": l.context.terminal})
}

func spawnTUIHerdrAppTab(mode *reviewMode, c herdrAppContext) (herdrAppLaunch, error) {
	bin, err := lookPath("herdr-app")
	if err != nil {
		return herdrAppLaunch{}, err
	}
	space, sourceTab, err := herdrAppPlace(c)
	if err != nil {
		return herdrAppLaunch{}, err
	}
	command, err := buildTUICommand(mode, "Herdr")
	if err != nil {
		return herdrAppLaunch{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return herdrAppLaunch{}, err
	}
	reply, err := herdrAppDo(bin, "tab.new", map[string]any{
		"space":  space,
		"name":   "Critty",
		"folder": cwd,
		"argv":   []string{"/bin/sh", "-c", "exec " + command},
	})
	if err != nil {
		return herdrAppLaunch{}, fmt.Errorf("opening Herdr tab: %w", err)
	}
	tab, _ := reply["tab"].(string)
	terminal, _ := reply["terminal"].(string)
	if tab == "" || terminal == "" {
		return herdrAppLaunch{}, fmt.Errorf("missing tab or terminal in Herdr tab reply")
	}
	l := herdrAppLaunch{context: c, bin: bin, tab: tab, space: space, sourceTab: sourceTab}
	if _, err := herdrAppDo(bin, "pane.focus", map[string]any{"terminal": terminal}); err != nil {
		cleanupErr := l.close()
		l.restoreFocus()
		return herdrAppLaunch{}, fmt.Errorf("focusing Herdr tab: %w", errors.Join(err, cleanupErr))
	}
	fmt.Fprintln(os.Stderr, "Opened review in Herdr tab")
	return l, nil
}
