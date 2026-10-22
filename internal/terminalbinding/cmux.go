package terminalbinding

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CMUXSocketPath supports the current state directory and the legacy socket.
func CMUXSocketPath() string {
	if socket := os.Getenv("CMUX_SOCKET_PATH"); socket != "" {
		return socket
	}
	if home, err := os.UserHomeDir(); err == nil {
		socket := filepath.Join(home, ".local", "state", "cmux", "cmux.sock")
		if info, err := os.Stat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
			return socket
		}
	}
	return "/tmp/cmux.sock"
}

// CMUXProcessTargets decodes `cmux --json --id-format uuids top --all --processes`.
func CMUXProcessTargets(data []byte, socket string) map[int]Target {
	var response struct {
		Windows []struct {
			Workspaces []struct {
				ID    string `json:"id"`
				Panes []struct {
					Surfaces []struct {
						ID           string          `json:"id"`
						TopLevelPIDs []int           `json:"top_level_pids"`
						Processes    json.RawMessage `json:"processes"`
					} `json:"surfaces"`
				} `json:"panes"`
			} `json:"workspaces"`
		} `json:"windows"`
	}
	if json.Unmarshal(data, &response) != nil {
		return nil
	}
	owners := make(map[int]Target)
	for _, window := range response.Windows {
		for _, workspace := range window.Workspaces {
			for _, pane := range workspace.Panes {
				for _, surface := range pane.Surfaces {
					target := Target{Kind: "cmux", Socket: socket, Workspace: workspace.ID, Pane: surface.ID}
					if target.Validate() != nil {
						continue
					}
					for _, pid := range surface.TopLevelPIDs {
						if pid > 1 {
							owners[pid] = target
						}
					}
					var processes any
					if json.Unmarshal(surface.Processes, &processes) == nil {
						collectOwners(processes, target, owners)
					}
				}
			}
		}
	}
	return owners
}
