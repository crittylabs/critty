// Package history groups saved reviews by task and manages their retention.
package history

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/xdg"
)

// Policy applies at explicit task boundaries, never at individual review exits.
type Policy struct {
	Start      string `json:"start" toml:"start"`
	End        string `json:"end" toml:"end"`
	KeepDays   int    `json:"keep_days" toml:"keep_days"`
	ArchiveDir string `json:"archive_dir,omitempty" toml:"archive_dir"`
	MaxBytes   int64  `json:"max_bytes" toml:"max_bytes"`
}

func (p Policy) Normalize() (Policy, error) {
	if p.Start == "" {
		p.Start = "auto"
	}
	if p.End == "" {
		p.End = "auto"
	}
	if p.KeepDays == 0 {
		p.KeepDays = 30
	}
	if p.MaxBytes == 0 {
		p.MaxBytes = DefaultMaxBytes
	}
	if !ValidPolicyAction(p.Start) || !ValidPolicyAction(p.End) || p.KeepDays < 0 || p.MaxBytes < 0 {
		return p, fmt.Errorf("history requires auto, keep, ask, archive, or delete and positive keep_days and max_bytes")
	}
	if p.ArchiveDir != "" && !filepath.IsAbs(p.ArchiveDir) {
		return p, fmt.Errorf("history archive_dir must be an absolute path")
	}
	return p, nil
}

func ValidAction(action string) bool {
	return slices.Contains([]string{"keep", "ask", "archive", "delete"}, action)
}

func ValidPolicyAction(action string) bool {
	return action == "auto" || ValidAction(action)
}

type Task struct {
	ID             string                `json:"id"`
	Project        string                `json:"project"`
	Name           string                `json:"name"`
	CWD            string                `json:"cwd"`
	StartedAt      string                `json:"started_at"`
	CompletedAt    string                `json:"completed_at,omitempty"`
	Sessions       []string              `json:"sessions"`
	Archive        string                `json:"archive,omitempty"`
	Pruned         bool                  `json:"pruned"`
	Pruning        bool                  `json:"pruning,omitempty"`
	Action         string                `json:"action,omitempty"`
	Pending        []review.SessionEntry `json:"pending,omitempty"`
	Missing        []string              `json:"missing_sessions,omitempty"`
	DeferredUntil  string                `json:"deferred_until,omitempty"`
	DeferredUpdate string                `json:"deferred_update,omitempty"`
}

type Outcome struct {
	Task             Task          `json:"task"`
	Action           string        `json:"action"`
	DecisionRequired bool          `json:"decision_required"`
	Archive          string        `json:"archive,omitempty"`
	Reason           string        `json:"reason,omitempty"`
	Missing          []string      `json:"missing_sessions,omitempty"`
	Maintenance      []Outcome     `json:"maintenance,omitempty"`
	Storage          *StorageUsage `json:"storage,omitempty"`
	Limited          bool          `json:"limited,omitempty"`
	MaintenanceError string        `json:"maintenance_error,omitempty"`
	UpdatedAt        string        `json:"updated_at,omitempty"`
	Bytes            *int64        `json:"bytes,omitempty"`
	Warnings         []string      `json:"warnings,omitempty"`
}

type StartResult struct {
	Task             Task          `json:"task"`
	Maintenance      []Outcome     `json:"maintenance"`
	MaintenanceError string        `json:"maintenance_error,omitempty"`
	Storage          *StorageUsage `json:"storage,omitempty"`
	Limited          bool          `json:"limited,omitempty"`
	Reason           string        `json:"reason,omitempty"`
}

type Store struct {
	Root    string
	CWD     string
	Project string
	now     func() time.Time
}

var historyLockTimeout = 2 * time.Second

func New(cwd string) *Store {
	return &Store{Root: filepath.Join(xdg.StateHome(), "history"), CWD: cwd, Project: filepath.Base(cwd), now: time.Now}
}

func (s *Store) withLock(fn func() error) (err error) {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.Root, "history.lock"))
	ctx, cancel := context.WithTimeout(context.Background(), historyLockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && !locked) {
		return fmt.Errorf("%w: review history; retry after the current operation", ErrBusy)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return fn()
}

func (s *Store) read(id string) (*Task, error) {
	if !review.ValidSessionKey(id) {
		return nil, fmt.Errorf("invalid task ID %q", id)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, id+".json"))
	if err != nil {
		return nil, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	if task.ID != id || task.CWD != s.CWD {
		return nil, fmt.Errorf("task %s does not belong to this directory", id)
	}
	if task.Project == "" {
		task.Project = s.Project
	}
	return &task, nil
}

func (s *Store) save(task *Task) error {
	return review.WriteJSON(filepath.Join(s.Root, task.ID+".json"), task)
}

func (s *Store) List() ([]Task, error) {
	tasks, _, err := s.scan()
	return tasks, err
}

func (s *Store) scan() ([]Task, bool, error) {
	entries, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
		return []Task{}, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	tasks := []Task{}
	incomplete := false
	warn := func(name string, err error) {
		incomplete = true
		fmt.Fprintf(os.Stderr, "critty: skipping history record %s: %v\n", name, err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") || !review.ValidSessionKey(strings.TrimSuffix(entry.Name(), ".json")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Root, entry.Name()))
		if err != nil {
			warn(entry.Name(), err)
			continue
		}
		var task Task
		if err := json.Unmarshal(data, &task); err != nil {
			warn(entry.Name(), err)
			continue
		}
		if task.CWD != s.CWD {
			continue
		}
		if task.ID != strings.TrimSuffix(entry.Name(), ".json") || !review.ValidSessionKey(task.ID) {
			warn(entry.Name(), fmt.Errorf("invalid task identity"))
			continue
		}
		if task.CWD == s.CWD {
			if task.Project == "" {
				task.Project = s.Project
			}
			tasks = append(tasks, task)
		}
	}
	slices.SortFunc(tasks, func(a, b Task) int { return strings.Compare(a.StartedAt, b.StartedAt) })
	return tasks, incomplete, nil
}

func (s *Store) Validate(id string) error {
	task, err := s.read(id)
	if err != nil {
		return err
	}
	if task.Pruning {
		return fmt.Errorf("task %s has interrupted %s; rerun history finish with that action", id, task.Action)
	}
	if task.CompletedAt != "" || task.Pruned {
		return fmt.Errorf("task %s is complete; reopen it or start another task", id)
	}
	return nil
}

func (s *Store) Start(name string, policy Policy) (result StartResult, err error) {
	if strings.TrimSpace(name) == "" {
		return result, fmt.Errorf("task name is required")
	}
	if !ValidPolicyAction(policy.Start) || policy.KeepDays < 0 || policy.MaxBytes < 0 {
		return result, fmt.Errorf("invalid start retention policy")
	}
	err = s.withLock(func() error {
		maintenance, err := s.maintain(policy, true, "")
		result.Maintenance, result.Storage = maintenance.Outcomes, maintenance.Storage
		result.Limited, result.Reason = maintenance.Limited, maintenance.Reason
		if err != nil {
			result.MaintenanceError = err.Error()
		}
		var id [6]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		result.Task = Task{ID: hex.EncodeToString(id[:]), Project: s.Project, Name: name, CWD: s.CWD, StartedAt: s.now().Format(time.RFC3339Nano), Sessions: []string{}}
		if _, err := os.Lstat(filepath.Join(s.Root, result.Task.ID+".json")); !os.IsNotExist(err) {
			return fmt.Errorf("task ID is already in use or inaccessible; retry start")
		}
		return s.save(&result.Task)
	})
	return result, err
}

// Add keeps task membership explicit so concurrent agents do not share an
// implicit current task.  A saved review belongs to at most one task.
func (s *Store) Add(id, session string) error {
	return s.withLock(func() error {
		task, err := s.read(id)
		if err != nil {
			return err
		}
		if task.CompletedAt != "" {
			return fmt.Errorf("task %s is complete; start another task", id)
		}
		if !review.ValidSessionKey(session) {
			return fmt.Errorf("invalid session ID %q", session)
		}
		entry, err := review.ReadSessionEntry(session)
		if err != nil {
			return err
		}
		if entry.CWD != s.CWD {
			return fmt.Errorf("review belongs to another directory")
		}
		if ipc.Alive(review.SocketPathFor(session)) {
			return fmt.Errorf("%w: review %s is active", ErrBusy, session)
		}
		member, err := readMembership(entry)
		if err != nil {
			return err
		}
		if member != "" && member != id {
			return fmt.Errorf("review already belongs to task %s", member)
		}
		tasks, err := s.List()
		if err != nil {
			return err
		}
		for _, other := range tasks {
			if other.ID != id && slices.Contains(other.Sessions, session) {
				return fmt.Errorf("review already belongs to task %s", other.ID)
			}
		}
		if slices.Contains(task.Sessions, session) {
			return writeMembership(entry, id)
		}
		task.Sessions = append(task.Sessions, session)
		if err := s.save(task); err != nil {
			return err
		}
		return writeMembership(entry, id)
	})
}

var ErrHistoryUnavailable = errors.New("some history records are unavailable")

func membershipPath(entry *review.SessionEntry) string {
	path := entry.ReviewPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(entry.CWD, path)
	}
	return filepath.Join(filepath.Dir(path), "history-task.json")
}

func writeMembership(entry *review.SessionEntry, id string) error {
	return review.WriteJSON(membershipPath(entry), map[string]string{"task": id})
}

func readMembership(entry *review.SessionEntry) (string, error) {
	data, err := os.ReadFile(membershipPath(entry))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var membership struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(data, &membership); err != nil {
		return "", err
	}
	if !review.ValidSessionKey(membership.Task) {
		return "", fmt.Errorf("invalid task membership for %s", entry.Key)
	}
	return membership.Task, nil
}

func (s *Store) ForSession(id string) (*Task, error) {
	if entry, err := review.ReadSessionEntry(id); err == nil {
		member, err := readMembership(entry)
		if err != nil {
			return nil, err
		}
		if member != "" {
			task, err := s.read(member)
			if err != nil {
				return nil, err
			}
			if task.Pruned || !slices.Contains(task.Sessions, id) {
				return nil, fmt.Errorf("inconsistent membership for review %s", id)
			}
			return task, nil
		}
	}
	tasks, incomplete, err := s.scan()
	if err != nil {
		return nil, errors.Join(ErrHistoryUnavailable, err)
	}
	for _, task := range tasks {
		if !task.Pruned && slices.Contains(task.Sessions, id) {
			return &task, nil
		}
	}
	if incomplete {
		return nil, ErrHistoryUnavailable
	}
	return nil, nil
}

func (s *Store) Detach(id, session string) error {
	return s.withLock(func() error {
		task, err := s.read(id)
		if err != nil {
			return err
		}
		if task.Pruning || task.Pruned {
			return fmt.Errorf("task has already been organized")
		}
		if !slices.Contains(task.Sessions, session) {
			return fmt.Errorf("review does not belong to this task")
		}
		if ipc.Alive(review.SocketPathFor(session)) {
			return fmt.Errorf("%w: review %s is active", ErrBusy, session)
		}
		if entry, err := review.ReadSessionEntry(session); err == nil {
			member, err := readMembership(entry)
			if err != nil {
				return err
			}
			if member == id {
				if err := os.Remove(membershipPath(entry)); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
		task.Sessions = slices.DeleteFunc(task.Sessions, func(key string) bool { return key == session })
		task.Missing = slices.DeleteFunc(task.Missing, func(key string) bool { return key == session })
		return s.save(task)
	})
}

func (s *Store) Reopen(id string) (task *Task, err error) {
	err = s.withLock(func() error {
		var err error
		task, err = s.read(id)
		if err != nil {
			return err
		}
		if task.Pruned || task.Pruning {
			return fmt.Errorf("task %s has already been organized", id)
		}
		task.CompletedAt = ""
		return s.save(task)
	})
	return task, err
}

func (s *Store) finish(id, action, archiveDir string) (Outcome, error) {
	if action == "auto" {
		action = "keep"
	}
	task, err := s.read(id)
	if err != nil {
		return Outcome{}, err
	}
	if task.CompletedAt == "" {
		task.CompletedAt = s.now().Format(time.RFC3339Nano)
	}
	return s.apply(task, action, archiveDir)
}

func (s *Store) Clean(action string, days int, archiveDir string) (results []Outcome, err error) {
	err = s.withLock(func() error {
		var err error
		results, err = s.clean(action, days, archiveDir)
		return err
	})
	return results, err
}

func (s *Store) clean(action string, days int, archiveDir string) ([]Outcome, error) {
	if !ValidAction(action) || days < 0 {
		return nil, fmt.Errorf("invalid history action or age")
	}
	tasks, err := s.List()
	if err != nil {
		return nil, err
	}
	results := []Outcome{}
	if action == "keep" {
		return results, nil
	}
	for _, task := range tasks {
		if task.CompletedAt == "" || task.Pruned {
			continue
		}
		// Interrupted removal requires an explicit retry of the recorded task.
		if task.Pruning {
			results = append(results, Outcome{Task: task, Action: "skip", Reason: "interrupted organization; rerun finish with --action " + task.Action})
			continue
		}
		updated, err := lastUpdated(task)
		if err != nil {
			results = append(results, Outcome{Task: task, Action: "skip", Reason: err.Error()})
			continue
		}
		if days > 0 && updated.After(s.now().AddDate(0, 0, -days)) {
			continue
		}
		if action == "ask" && s.deferred(task, updated) {
			continue
		}
		result, err := s.apply(&task, action, archiveDir)
		if errors.Is(err, ErrBusy) {
			results = append(results, Outcome{Task: task, Action: "skip", Reason: err.Error()})
			continue
		}
		if err != nil {
			result.Action, result.Reason = "error", err.Error()
			results = append(results, result)
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) apply(task *Task, action, archiveDir string) (Outcome, error) {
	return s.applyChecked(task, action, archiveDir, nil)
}

func (s *Store) applyChecked(task *Task, action, archiveDir string, expectedUpdate *time.Time) (Outcome, error) {
	result := Outcome{Task: *task, Action: action, DecisionRequired: action == "ask"}
	if !ValidAction(action) {
		return result, fmt.Errorf("invalid history action %q", action)
	}
	if task.Pruned {
		return result, fmt.Errorf("task %s has already been organized", task.ID)
	}
	err := withSessions(*task, snapshotOptions{AllowMissing: action != "archive", MetadataOnly: action != "archive" && expectedUpdate == nil}, func(sessions []*review.Session, missing []string) error {
		if expectedUpdate != nil {
			updated, err := lastUpdated(*task)
			if err != nil {
				return err
			}
			if !updated.Equal(*expectedUpdate) {
				return fmt.Errorf("%w: task %s changed during cleanup", ErrBusy, task.ID)
			}
		}
		task.Missing, result.Missing = missing, missing
		if task.Pruning && task.Action != action {
			return fmt.Errorf("resume interrupted organization with --action %s", task.Action)
		}
		if action == "keep" || action == "ask" {
			if action == "ask" {
				info := s.describeTask(*task)
				result.UpdatedAt, result.Bytes, result.Warnings = info.UpdatedAt, &info.Bytes, info.Warnings
			}
			return s.save(task)
		}
		if action == "archive" && !task.Pruning {
			path, err := archive(*task, sessions, archiveDir, s.now(), archiveOptions{})
			if err != nil {
				return err
			}
			task.Archive, result.Archive = path, path
			if err := s.recordArchive(path); err != nil {
				return err
			}
			// Record the verified archive before removing any original data.
			if err := s.save(task); err != nil {
				return err
			}
		}
		if action == "archive" {
			if err := Verify(task.Archive); err != nil {
				return err
			}
		}
		if !task.Pruning {
			task.Pruning, task.Action = true, action
			for _, session := range sessions {
				task.Pending = append(task.Pending, session.Meta)
			}
			if err := s.save(task); err != nil {
				return err
			}
		}
		for _, session := range sessions {
			if err := session.Clear(); err != nil {
				return err
			}
			task.Pending = slices.DeleteFunc(task.Pending, func(entry review.SessionEntry) bool { return entry.Key == session.Key })
			if err := s.save(task); err != nil {
				return err
			}
		}
		task.Pruned, task.Pruning = true, false
		return s.save(task)
	})
	result.Task = *task
	return result, err
}

// Export preserves the live sessions; Finish with archive saves and prunes.
func (s *Store) Export(id, archiveDir string, allowIncomplete bool) (path string, err error) {
	err = s.withLock(func() error {
		task, err := s.read(id)
		if err != nil {
			return err
		}
		if task.Pruned || task.Pruning {
			return fmt.Errorf("task is already organized; archive: %s", task.Archive)
		}
		return withSessions(*task, snapshotOptions{AllowMissing: allowIncomplete}, func(sessions []*review.Session, missing []string) error {
			path, err = archive(*task, sessions, archiveDir, s.now(), archiveOptions{AllowIncomplete: allowIncomplete, Missing: missing})
			if err != nil {
				return err
			}
			task.Archive = path
			if err := s.recordArchive(path); err != nil {
				return fmt.Errorf("archive saved at %s, but inventory update failed: %w", path, err)
			}
			if err := s.save(task); err != nil {
				return fmt.Errorf("archive saved at %s, but task update failed: %w", path, err)
			}
			return nil
		})
	})
	return path, err
}
