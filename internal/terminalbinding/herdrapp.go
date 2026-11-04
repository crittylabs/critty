package terminalbinding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The Herdr desktop app shows terminals that run in herdr-core.  A target of
// kind "herdr-app" names the core socket and the terminal address inside it;
// the app's space and tab are looked up from the terminal's labels when needed.

const hcListRequest = `{"jsonrpc":"2.0","id":1,"method":"terminal.list","params":{}}` + "\n"

// HCSocketPath follows hc's own resolution: the terminal's core, an explicit
// override, the config file, then the runtime directory of the session.
func HCSocketPath() string {
	return hcSocketPath(os.Getenv)
}

func hcSocketPath(getenv func(string) string) string {
	for _, key := range []string{"HC_TERMINAL_SOCKET", "HC_SOCKET"} {
		if socket := getenv(key); socket != "" {
			return socket
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configHome := getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	var config struct {
		Socket string `toml:"socket"`
	}
	if data, err := os.ReadFile(filepath.Join(configHome, "herdr-core", "config.toml")); err == nil && toml.Unmarshal(data, &config) == nil && config.Socket != "" {
		return config.Socket
	}
	session := strings.TrimSpace(getenv("HC_SESSION"))
	if session == "" {
		session = "default"
	}
	run := getenv("XDG_RUNTIME_DIR")
	if run == "" {
		if info, err := os.Stat("/run/user/" + strconv.Itoa(os.Getuid())); err == nil && info.IsDir() {
			run = "/run/user/" + strconv.Itoa(os.Getuid())
		}
	}
	if run != "" {
		return filepath.Join(run, "herdr-core", session+".sock")
	}
	stateHome := getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "herdr-core", session, session+".sock")
}

// HCEnvironment is used in the real terminal, like the other multiplexers.
func HCEnvironment() Target {
	return Target{Kind: "herdr-app", Socket: HCSocketPath(), Pane: os.Getenv("HC_TERMINAL_ID")}
}

func validateHerdrApp(t Target) error {
	if !filepath.IsAbs(t.Socket) || strings.ContainsAny(t.Socket+t.Pane+t.Workspace+t.Tab, "\x00\r\n") {
		return fmt.Errorf("invalid terminal target")
	}
	if t.Pane == "" || strings.HasPrefix(t.Pane, "-") || strings.ContainsAny(t.Pane, "/ ") {
		return fmt.Errorf("invalid herdr-core terminal")
	}
	return nil
}

func (t Target) hcCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "hc", args...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HC_SOCKET=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "HC_SOCKET="+t.Socket)
	return cmd
}

// HCTerminal is the part of a herdr-core terminal record that locates it.
type HCTerminal struct {
	Address    string             `json:"address"`
	State      string             `json:"state"`
	PID        int                `json:"pid"`
	Foreground *struct{ PID int } `json:"foreground"`
	Labels     map[string]string  `json:"labels"`
}

// HCTerminals lists the core's live terminals through `hc connect`, which
// returns the full records that `hc ls` leaves out.
func HCTerminals(ctx context.Context, t Target) ([]HCTerminal, error) {
	cmd := t.hcCommand(ctx, "connect")
	cmd.Stdin = strings.NewReader(hcListRequest)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return DecodeHCTerminals(out)
}

// DecodeHCTerminals reads the JSON-RPC reply to a terminal.list request.
func DecodeHCTerminals(data []byte) ([]HCTerminal, error) {
	var reply struct {
		Result struct {
			Terminals []HCTerminal `json:"terminals"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&reply); err != nil {
		return nil, fmt.Errorf("parsing herdr-core terminal list: %w", err)
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("herdr-core: %s", reply.Error.Message)
	}
	if reply.Result.Terminals == nil {
		return nil, fmt.Errorf("missing herdr-core terminal inventory")
	}
	return reply.Result.Terminals, nil
}

// HCProcessTargets maps the processes that own each live local terminal.
// Terminals on other machines carry their machine in the address and have no
// local processes.
func HCProcessTargets(terminals []HCTerminal, socket string) map[int]Target {
	owners := make(map[int]Target)
	for _, terminal := range terminals {
		if terminal.State != "live" {
			continue
		}
		target := Target{Kind: "herdr-app", Socket: socket, Pane: terminal.Address}
		if target.Validate() != nil {
			continue
		}
		for _, pid := range terminal.pids() {
			if _, exists := owners[pid]; !exists {
				owners[pid] = target
			}
		}
	}
	return owners
}

func (t HCTerminal) pids() []int {
	var pids []int
	if t.PID > 1 {
		pids = append(pids, t.PID)
	}
	if t.Foreground != nil && t.Foreground.PID > 1 && t.Foreground.PID != t.PID {
		pids = append(pids, t.Foreground.PID)
	}
	return pids
}

func hcSnapshot(ctx context.Context, t Target) (string, error) {
	out, err := t.hcCommand(ctx, "read", "--json", t.Pane).Output()
	if err != nil {
		return "", err
	}
	var screen struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out, &screen); err != nil {
		return "", fmt.Errorf("parsing herdr-core screen: %w", err)
	}
	return screen.Text, nil
}
