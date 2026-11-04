package terminalbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const searchConcurrency = 8

// DiscoveryIssue explains a connection that could not supply a complete inventory.
// It does not invalidate inventories from other reachable connections.
type DiscoveryIssue struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type discovery struct {
	targets []Target
	issues  []DiscoveryIssue
}

// parallelSearch preserves input order while limiting concurrent CLI processes.
func parallelSearch[T any, R any](ctx context.Context, inputs []T, search func(context.Context, T) R) []R {
	results := make([]R, len(inputs))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(searchConcurrency, len(inputs)) {
		workers.Go(func() {
			for i := range jobs {
				results[i] = search(ctx, inputs[i])
			}
		})
	}
	for i := range inputs {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	return results
}

func discoverTargets(ctx context.Context, registered []Target) discovery {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	connections := []Target{
		{Kind: "tmux"},
		{Kind: "herdr", Socket: herdrDefaultSocketPath()},
		{Kind: "herdr-app", Socket: hcSocketPath(func(key string) string {
			if key == "HC_TERMINAL_SOCKET" || key == "HC_SOCKET" {
				return ""
			}
			return os.Getenv(key)
		})},
		{Kind: "cmux", Socket: cmuxDefaultSocketPath()},
		{Kind: "zellij"},
		{Kind: "orca", Socket: orcaDefaultProfilePath()},
	}
	hints := []Target{
		{Kind: "herdr", Socket: herdrSocketPath()},
		{Kind: "herdr-app", Socket: HCSocketPath()},
		{Kind: "cmux", Socket: CMUXSocketPath()},
		{Kind: "zellij", Socket: os.Getenv("ZELLIJ_SOCKET_DIR")},
		{Kind: "orca", Socket: orcaProfilePath()},
	}
	if raw := os.Getenv("TMUX"); raw != "" {
		if end := strings.LastIndexByte(raw, ','); end > 0 {
			if end = strings.LastIndexByte(raw[:end], ','); end > 0 {
				connections = append(connections, Target{Kind: "tmux", Socket: raw[:end]})
			}
		}
	}
	for _, target := range append(hints, registered...) {
		connection := Target{Kind: target.Kind, Socket: target.Socket, SourceID: target.SourceID, RuntimeID: target.RuntimeID}
		if target.Kind == "terminal" {
			continue
		}
		if !slices.ContainsFunc(connections, func(t Target) bool { return t.Kind == connection.Kind && t.Socket == connection.Socket }) {
			connections = append(connections, connection)
		}
	}
	result := discovery{}
	for _, target := range registered {
		if target.Validate() == nil && target.Kind != "terminal" && (target.Kind != "orca" || target.Pane != "") {
			result.targets = addTarget(result.targets, target)
		}
	}
	for _, found := range parallelSearch(ctx, connections, discoverConnection) {
		for _, target := range found.targets {
			for _, prior := range result.targets {
				if sameDestination(prior, target) && prior.Origin.Started != "" {
					target.Origin = prior.Origin
					break
				}
			}
			result.targets = addTarget(result.targets, target)
		}
		result.issues = append(result.issues, found.issues...)
	}
	return result
}

func discoverConnection(ctx context.Context, connection Target) discovery {
	var targets []Target
	var err error
	switch connection.Kind {
	case "tmux":
		targets, err = discoverTMUX(ctx, connection)
	case "herdr":
		targets, err = discoverHerdr(ctx, connection)
	case "herdr-app":
		var terminals []HCTerminal
		terminals, err = HCTerminals(ctx, connection)
		for _, terminal := range terminals {
			if terminal.State == "live" && !strings.Contains(terminal.Address, "/") {
				targets = append(targets, Target{Kind: connection.Kind, Socket: connection.Socket, Pane: terminal.Address, Origin: Origin{PID: terminal.PID, Terminal: connection.Kind}})
			}
		}
	case "cmux":
		var data []byte
		data, err = connection.Command(ctx, "--json", "--id-format", "uuids", "top", "--all", "--processes").Output()
		if err == nil {
			targets, err = CMUXTargets(data, connection.Socket)
		}
	case "zellij":
		targets, err = discoverZellij(ctx, connection)
	case "orca":
		targets, err = discoverOrca(ctx, connection)
	default:
		err = fmt.Errorf("terminal enumeration is unavailable")
	}
	if err != nil {
		reason := err.Error()
		if connection.Socket != "" {
			reason = connection.Socket + ": " + reason
		}
		return discovery{issues: []DiscoveryIssue{{Kind: connection.Kind, Reason: reason}}}
	}
	for _, target := range targets {
		if err := target.Validate(); err != nil {
			return discovery{issues: []DiscoveryIssue{{Kind: connection.Kind, Reason: "invalid terminal inventory: " + err.Error()}}}
		}
	}
	return discovery{targets: targets}
}

func discoverTMUX(ctx context.Context, connection Target) ([]Target, error) {
	args := []string{"list-panes", "-a", "-F", "#{socket_path}\t#{pane_id}\t#{pane_pid}"}
	var cmd *exec.Cmd
	if connection.Socket != "" {
		cmd = connection.Command(ctx, args...)
	} else {
		cmd = exec.CommandContext(ctx, "tmux", args...)
		cmd.Env = slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "TMUX=") })
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var targets []Target
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid tmux pane inventory")
		}
		pid, err := strconv.Atoi(fields[2])
		if err != nil || pid < 2 {
			return nil, fmt.Errorf("invalid tmux pane PID")
		}
		targets = append(targets, Target{Kind: "tmux", Socket: fields[0], Pane: fields[1], Origin: Origin{PID: pid}})
	}
	return targets, nil
}

func herdrSocketPath() string {
	if socket := os.Getenv("HERDR_SOCKET_PATH"); socket != "" {
		return socket
	}
	if config := os.Getenv("HERDR_CONFIG_PATH"); config != "" {
		return filepath.Join(filepath.Dir(config), "herdr.sock")
	}
	return herdrDefaultSocketPath()
}

func herdrDefaultSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

func discoverHerdr(ctx context.Context, connection Target) ([]Target, error) {
	out, err := connection.Command(ctx, "pane", "list").Output()
	if err != nil {
		return nil, err
	}
	var response struct {
		Result struct {
			Panes *[]struct {
				Workspace string `json:"workspace_id"`
				Tab       string `json:"tab_id"`
				Pane      string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}
	if response.Result.Panes == nil {
		return nil, fmt.Errorf("missing Herdr pane inventory")
	}
	var targets []Target
	for _, pane := range *response.Result.Panes {
		targets = append(targets, Target{Kind: "herdr", Socket: connection.Socket, Workspace: pane.Workspace, Tab: pane.Tab, Pane: pane.Pane})
	}
	return targets, nil
}

func discoverZellij(ctx context.Context, connection Target) ([]Target, error) {
	cmd := exec.CommandContext(ctx, "zellij", "list-sessions", "--no-formatting", "--short")
	cmd.Env = slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "ZELLIJ") })
	if connection.Socket != "" {
		cmd.Env = append(cmd.Env, "ZELLIJ_SOCKET_DIR="+connection.Socket)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var targets []Target
	for line := range strings.Lines(string(out)) {
		connection.Workspace = strings.TrimSuffix(line, "\n")
		connection.Pane = "0"
		if err := connection.Validate(); err != nil {
			return nil, err
		}
		data, err := connection.Command(ctx, "action", "list-panes", "--all", "--json").Output()
		if err != nil {
			return nil, err
		}
		var panes []struct {
			ID     *uint32 `json:"id"`
			Plugin *bool   `json:"is_plugin"`
		}
		if err := json.Unmarshal(data, &panes); err != nil {
			return nil, err
		}
		if panes == nil {
			return nil, fmt.Errorf("missing Zellij pane inventory")
		}
		for _, pane := range panes {
			if pane.ID == nil || pane.Plugin == nil {
				return nil, fmt.Errorf("incomplete Zellij pane inventory")
			}
			if !*pane.Plugin {
				target := connection
				target.Pane = strconv.FormatUint(uint64(*pane.ID), 10)
				targets = append(targets, target)
			}
		}
	}
	return targets, nil
}

func orcaProfilePath() string {
	for _, key := range []string{"ORCA_RUNTIME_SOURCE_PROFILE_PATH", "ORCA_USER_DATA_PATH"} {
		if path := os.Getenv(key); path != "" {
			return path
		}
	}
	return orcaDefaultProfilePath()
}

func orcaDefaultProfilePath() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "orca")
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "orca")
}

func discoverOrca(ctx context.Context, connection Target) ([]Target, error) {
	// These public identity fields pin CLI requests to this profile and runtime.
	// Authentication fields in the runtime record are never used or logged.
	var installation struct {
		ID string `json:"installationId"`
	}
	var running struct {
		ID string `json:"runtimeId"`
	}
	for _, record := range []struct {
		name  string
		value any
	}{
		{"host-installation.json", &installation}, {"orca-runtime.json", &running},
	} {
		data, err := os.ReadFile(filepath.Join(connection.Socket, record.name))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, record.value); err != nil {
			return nil, err
		}
	}
	connection.SourceID, connection.RuntimeID = installation.ID, running.ID
	connection.Workspace = "discovery"
	if err := connection.Validate(); err != nil {
		return nil, err
	}
	out, err := connection.Command(ctx, "terminal", "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	var response struct {
		OK     bool `json:"ok"`
		Result struct {
			Terminals *[]struct {
				Handle   string `json:"handle"`
				Worktree string `json:"worktreeId"`
				Host     string `json:"executionHostId"`
			} `json:"terminals"`
			Truncated bool `json:"truncated"`
			HostScope struct {
				Hosts []string `json:"hostIds"`
			} `json:"hostScope"`
		} `json:"result"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("Orca: %s", response.Error.Message)
	}
	if response.Result.Terminals == nil || response.Result.Truncated || !slices.Contains(response.Result.HostScope.Hosts, "local") {
		return nil, fmt.Errorf("incomplete local Orca terminal inventory")
	}
	var targets []Target
	for _, terminal := range *response.Result.Terminals {
		if terminal.Host != "local" {
			continue
		}
		target := connection
		target.Workspace, target.Pane = terminal.Worktree, terminal.Handle
		target.Origin.Terminal = "orca"
		targets = append(targets, target)
	}
	return targets, nil
}
