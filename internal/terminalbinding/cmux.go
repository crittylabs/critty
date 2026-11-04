package terminalbinding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CMUXSocketPath supports the current state directory and the legacy socket.
func CMUXSocketPath() string {
	if socket := os.Getenv("CMUX_SOCKET_PATH"); socket != "" {
		return socket
	}
	return cmuxDefaultSocketPath()
}

func cmuxDefaultSocketPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		socket := filepath.Join(home, ".local", "state", "cmux", "cmux.sock")
		if info, err := os.Stat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
			return socket
		}
	}
	return "/tmp/cmux.sock"
}

type cmuxTarget struct {
	target Target
	pids   []int
}

func decodeCMUXTargets(data []byte, socket string) ([]cmuxTarget, error) {
	var response struct {
		Windows *[]struct {
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
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if response.Windows == nil {
		return nil, fmt.Errorf("missing cmux window inventory")
	}
	var targets []cmuxTarget
	for _, window := range *response.Windows {
		for _, workspace := range window.Workspaces {
			for _, pane := range workspace.Panes {
				for _, surface := range pane.Surfaces {
					target := Target{Kind: "cmux", Socket: socket, Workspace: workspace.ID, Pane: surface.ID}
					if err := target.Validate(); err != nil {
						return nil, err
					}
					owners := make(map[int]Target)
					for _, pid := range surface.TopLevelPIDs {
						if pid > 1 {
							owners[pid] = target
						}
					}
					var processes any
					if json.Unmarshal(surface.Processes, &processes) == nil {
						collectOwners(processes, target, owners)
					}
					var pids []int
					for pid := range owners {
						pids = append(pids, pid)
					}
					targets = append(targets, cmuxTarget{target, pids})
				}
			}
		}
	}
	return targets, nil
}

// CMUXTargets includes surfaces without process metadata, for marker discovery.
func CMUXTargets(data []byte, socket string) ([]Target, error) {
	surfaces, err := decodeCMUXTargets(data, socket)
	if err != nil {
		return nil, err
	}
	var targets []Target
	for _, surface := range surfaces {
		targets = append(targets, surface.target)
	}
	return targets, nil
}

// CMUXProcessTargets decodes `cmux --json --id-format uuids top --all --processes`.
func CMUXProcessTargets(data []byte, socket string) map[int]Target {
	surfaces, err := decodeCMUXTargets(data, socket)
	if err != nil {
		return nil
	}
	owners := make(map[int]Target)
	for _, surface := range surfaces {
		for _, pid := range surface.pids {
			owners[pid] = surface.target
		}
	}
	return owners
}
