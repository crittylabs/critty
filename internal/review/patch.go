package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crittylabs/critty/internal/git"
)

// OpenDiffSession keeps piped reviews separate from Git and document reviews.
func OpenDiffSession(dataRoot string) (*Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	args := []string{"--diff"}
	s, err := OpenSession(dataRoot, SessionKey(cwd, "", args))
	if err != nil {
		return nil, err
	}
	s.Meta = SessionEntry{Key: s.Key, CWD: cwd, Args: args}
	return s, nil
}

func (s *Session) DiffPath() string {
	return filepath.Join(s.Dir, "diff.json")
}

// SaveDiff atomically replaces the snapshot consumed at the next round.
func (s *Session) SaveDiff(p *git.Patch) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, "diff-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "critty: removing temporary diff: %v\n", err)
		}
	}()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), s.DiffPath()); err != nil {
		return fmt.Errorf("saving diff snapshot: %w", err)
	}
	return nil
}
