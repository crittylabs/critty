package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crittylabs/critty/internal/xdg"
)

// SessionEntry records where a review session's data lives, so commands run
// later (status, clear, comment) can find review folders belonging to a
// working directory without re-deriving every possible key.
type SessionEntry struct {
	Project    *string  `json:"project,omitempty"`
	Topic      *string  `json:"topic,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	PlanSlug   string   `json:"plan_slug,omitempty"`
	Key        string   `json:"key"`
	CWD        string   `json:"cwd"`
	Args       []string `json:"args,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	ReviewPath string   `json:"review_path"`
	StartedAt  string   `json:"started_at"`
	PID        int      `json:"pid,omitempty"`
	SocketPath string   `json:"socket_path,omitempty"`
}

// SocketPathFor returns the Unix socket path for a session key.
func SocketPathFor(key string) string {
	return filepath.Join(SessionsDir(), key+".sock")
}

// SessionsDir returns the directory holding session registry entries.
func SessionsDir() string {
	return filepath.Join(xdg.StateHome(), "sessions")
}

func sessionEntryPath(key string) string {
	return filepath.Join(SessionsDir(), key+".json")
}

// writeSessionEntry records or refreshes the registry entry for a session.
func writeSessionEntry(e SessionEntry) error {
	if err := os.MkdirAll(SessionsDir(), 0o700); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(&e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session entry: %w", err)
	}
	data = append(data, '\n')
	path := sessionEntryPath(e.Key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing session entry: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming session entry: %w", err)
	}
	return nil
}

// ListSessionEntries returns all registered session entries.
func ListSessionEntries() ([]SessionEntry, error) {
	entries, err := os.ReadDir(SessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}
	var result []SessionEntry
	for _, de := range entries {
		if !strings.HasSuffix(de.Name(), ".json") || !de.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(SessionsDir(), de.Name()))
		if err != nil {
			continue
		}
		var e SessionEntry
		if err := json.Unmarshal(data, &e); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

// RemoveSessionEntry deletes the registry entry for a key, ignoring missing
// files.
func RemoveSessionEntry(key string) error {
	err := os.Remove(sessionEntryPath(key))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
