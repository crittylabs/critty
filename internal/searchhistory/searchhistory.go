// Package searchhistory persists incremental search queries per project so a
// later review of the same repository, from any of its worktrees, can recall
// them with their search options.
package searchhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crittylabs/critty/internal/xdg"
)

// Entry is one recorded query.  Mode is "regexp", "literal", or "word".
type Entry struct {
	Query    string `json:"query"`
	Mode     string `json:"mode,omitempty"`
	CaseFold bool   `json:"case_fold,omitempty"`
}

// Limit caps the entries kept per project.
const Limit = 100

type file struct {
	Version  int                `json:"version"`
	Projects map[string][]Entry `json:"projects"`
}

// Store reads and writes one project's entries in the shared history file.
type Store struct {
	Path    string
	Project string // identity such as "git:<common dir>" or "directory:<path>"
}

// Open returns the store for a project identity under the Critty state home.
func Open(project string) *Store {
	return &Store{Path: filepath.Join(xdg.StateHome(), "isearch-history.json"), Project: project}
}

// Load returns the project's entries, oldest first.  A missing file yields none.
func (s *Store) Load() ([]Entry, error) {
	f, err := s.read()
	if err != nil {
		return nil, err
	}
	return f.Projects[s.Project], nil
}

// Add records an entry as the newest and returns the project's merged
// entries, so concurrent reviews of the project keep each other's additions.
func (s *Store) Add(entry Entry) ([]Entry, error) {
	f, err := s.read()
	if err != nil {
		return nil, err
	}
	entries := Push(f.Projects[s.Project], entry)
	f.Projects[s.Project] = entries
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".isearch-history-*.tmp")
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Write(data); err != nil {
		return nil, errors.Join(err, tmp.Close(), os.Remove(tmp.Name()))
	}
	if err := errors.Join(tmp.Chmod(0o600), tmp.Close()); err != nil {
		return nil, errors.Join(err, os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), s.Path); err != nil {
		return nil, errors.Join(err, os.Remove(tmp.Name()))
	}
	return entries, nil
}

func (s *Store) read() (file, error) {
	f := file{Version: 1, Projects: map[string][]Entry{}}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("parsing %s: %w", s.Path, err)
	}
	if f.Projects == nil {
		f.Projects = map[string][]Entry{}
	}
	return f, nil
}

// Push appends entry as the newest, dropping an identical older entry and
// the oldest entries beyond Limit.
func Push(entries []Entry, entry Entry) []Entry {
	merged := make([]Entry, 0, len(entries)+1)
	for _, e := range entries {
		if e != entry {
			merged = append(merged, e)
		}
	}
	merged = append(merged, entry)
	if len(merged) > Limit {
		merged = merged[len(merged)-Limit:]
	}
	return merged
}
