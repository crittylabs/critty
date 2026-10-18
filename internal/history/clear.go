package history

import (
	"fmt"
	"path/filepath"

	"github.com/crittylabs/critty/internal/review"
)

// ClearSessions refuses task members before clearing any selected review.
// Holding the history lock prevents a concurrent Add from changing membership.
func (s *Store) ClearSessions(sessions []*review.Session) error {
	return s.withLock(func() error {
		ids := make([]string, 0, len(sessions))
		for _, session := range sessions {
			task, err := s.ForSession(session.Key)
			if err != nil {
				return err
			}
			if task != nil {
				return fmt.Errorf("review %s belongs to task %s; use history finish %s --action delete, or history detach before clearing this review", session.Key, task.ID, task.ID)
			}
			ids = append(ids, session.Key)
		}
		// Reuse the same session and run locks as task cleanup, including busy checks.
		task := Task{CWD: s.CWD, Sessions: ids}
		return withSessions(task, snapshotOptions{AllowMissing: true, MetadataOnly: true}, func(locked []*review.Session, _ []string) error {
			for _, session := range locked {
				if err := session.Clear(); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func ClearSession(session *review.Session) error {
	entry, err := review.ReadSessionEntry(session.Key)
	if err != nil {
		return err
	}
	if filepath.Clean(entry.ReviewPath) != filepath.Clean(session.Path()) {
		return fmt.Errorf("review %s has inconsistent storage", session.Key)
	}
	return New(entry.CWD).ClearSessions([]*review.Session{session})
}
