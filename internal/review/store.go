package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Session holds review state with optional disk storage.
type Session struct {
	Key       string
	Dir       string
	CJ        CritJSON
	Meta      SessionEntry
	saved     []byte
	memory    map[string][]byte
	memoryDir string
}

// NormalizePath cleans a user-supplied file path into the forward-slash
// form used both in session-key derivation and as Files map keys.  Paths
// under the current working directory normalize to the same relative form
// whether given as absolute or relative, so all commands agree on keys.
func NormalizePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(p))
	}
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(abs)
}

// OpenSession loads the review folder for key, creating an empty round-1
// document when none exists yet.
func OpenSession(dataRoot, key string) (*Session, error) {
	return openSessionAt(key, Dir(dataRoot, key))
}

// Path returns the session's review.json path.
func (s *Session) Path() string {
	return JSONPath(s.Dir)
}

// FileComments returns the comments recorded for a (normalized) file path.
func (s *Session) FileComments(path string) []Comment {
	return s.CJ.Files[NormalizePath(path)].Comments
}

// SetFileComments replaces the comments for a file, creating its entry when
// missing.  An empty status leaves the recorded status untouched.
func (s *Session) SetFileComments(path, status string, comments []Comment) {
	key := NormalizePath(path)
	f := s.CJ.Files[key]
	if status != "" {
		f.Status = status
	}
	if f.Status == "" {
		f.Status = "modified"
	}
	if comments == nil {
		comments = []Comment{}
	}
	f.Comments = comments
	s.CJ.Files[key] = f
}

// Save merges local edits with updates since the last load or save, then
// writes the result atomically under an advisory file lock.
func (s *Session) Save() error {
	if s.memory != nil {
		s.CJ.Touch()
		return nil
	}
	return s.withLock(func() error {
		fresh, err := openSessionAt(s.Key, s.Dir)
		if err != nil {
			return err
		}
		merged, err := mergeReview(s.saved, s.CJ, fresh.CJ)
		if err != nil {
			return err
		}
		s.CJ = merged
		return s.saveLocked()
	})
}

// Update reloads the latest review state and applies fn while holding the
// advisory lock, then saves the result.  Keeping the full read-modify-write
// cycle under one lock prevents concurrent CLI commands from losing updates.
func (s *Session) Update(fn func(*Session) error) error {
	if s.memory != nil {
		if err := fn(s); err != nil {
			return err
		}
		return s.Save()
	}
	return s.withLock(func() error {
		fresh, err := openSessionAt(s.Key, s.Dir)
		if err != nil {
			return err
		}
		s.CJ = fresh.CJ
		s.saved = fresh.saved
		if err := fn(s); err != nil {
			return err
		}
		return s.saveLocked()
	})
}

func (s *Session) withLock(fn func() error) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating review dir: %w", err)
	}

	reviewPath := s.Path()
	lockPath := reviewPath + ".lock"
	fileLock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire lock on %s — another process may be writing. Try again", lockPath)
	}
	defer func() { _ = fileLock.Unlock() }()
	return fn()
}

func (s *Session) saveLocked() error {
	reviewPath := s.Path()
	s.CJ.Touch()
	data, err := json.MarshalIndent(&s.CJ, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling review: %w", err)
	}
	data = append(data, '\n')

	tmpPath := reviewPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, reviewPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	s.saved = data

	if s.Meta.Key != "" {
		if s.Meta.StartedAt == "" {
			s.Meta.StartedAt = Now()
		}
		s.Meta.ReviewPath = reviewPath
		if err := writeSessionEntry(s.Meta); err != nil {
			return fmt.Errorf("recording session: %w", err)
		}
	}
	return nil
}

// Clear removes the whole review folder and its registry entry.  Missing
// folders are not an error.
func (s *Session) Clear() error {
	if err := os.RemoveAll(s.Dir); err != nil {
		return fmt.Errorf("removing review dir: %w", err)
	}
	return RemoveSessionEntry(s.Key)
}
