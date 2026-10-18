package history

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/crittylabs/critty/internal/review"
)

const noticeCheckInterval = time.Hour
const noticeRepeatInterval = 7 * 24 * time.Hour

type noticeState struct {
	CheckedAt time.Time            `json:"checked_at"`
	Policy    Policy               `json:"policy"`
	WarnedAt  map[string]time.Time `json:"warned_at"`
}

// Notices observes saved records without organizing them.  A separate,
// nonblocking lock keeps concurrent CLI callers from repeating diagnostics.
func (s *Store) Notices(policy Policy) (messages []string, err error) {
	policy, err = policy.Normalize()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.Root, "notices")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(s.CWD))))
	lock := flock.New(path + ".lock")
	locked, err := lock.TryLock()
	if err != nil || !locked {
		return nil, err
	}
	defer func() {
		if unlockErr := lock.Unlock(); err == nil {
			err = unlockErr
		}
	}()
	state := noticeState{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(data) != 0 {
		// A damaged cache may be rebuilt; it owns no review or task data.
		if json.Unmarshal(data, &state) != nil {
			state = noticeState{}
		}
	}
	now := s.now()
	if state.Policy == policy && !state.CheckedAt.After(now) && now.Sub(state.CheckedAt) < noticeCheckInterval {
		return nil, nil
	}
	if state.WarnedAt == nil || state.Policy != policy {
		state.WarnedAt = map[string]time.Time{}
	}
	state.CheckedAt, state.Policy = now, policy
	observed := s.Inspect(policy).Issues
	for _, kind := range []string{"capacity", "age", "recovery", "incomplete"} {
		message := observed[kind]
		if message == "" {
			delete(state.WarnedAt, kind)
			continue
		}
		last := state.WarnedAt[kind]
		if last.IsZero() || last.After(now) || now.Sub(last) >= noticeRepeatInterval {
			messages = append(messages, kind+": "+message+". No saved records were changed; agents can follow the critty-history skill")
			state.WarnedAt[kind] = now
		}
	}
	return messages, review.WriteJSON(path, state)
}

// Inspection is an uncached, read-only diagnosis for explicit maintenance.
type Inspection struct {
	Storage StorageUsage
	Issues  map[string]string
}

func (s *Store) Inspect(policy Policy) Inspection {
	result := Inspection{Storage: s.Usage(policy.MaxBytes), Issues: map[string]string{}}
	usage := result.Storage
	if usage.Exceeded {
		result.Issues["capacity"] = fmt.Sprintf("saved review backups use %d allocated bytes, above max_bytes=%d; run `critty history clean` to apply the start policy. Archives and ongoing, busy, or ungrouped reviews are protected", usage.Bytes, usage.MaxBytes)
	}
	tasks, incomplete, err := s.scan()
	if err != nil || incomplete || len(usage.Warnings) != 0 {
		result.Issues["incomplete"] = "saved history could not be fully inspected; use `critty history usage` and `critty history list` to inspect warnings before maintenance"
	}
	members := map[string]bool{}
	completed, open, ungrouped, recovery := 0, 0, 0, 0
	cutoff := s.now().AddDate(0, 0, -policy.KeepDays)
	for _, task := range tasks {
		for _, id := range task.Sessions {
			members[id] = true
		}
		if task.Pruned {
			continue
		}
		if task.Pruning {
			recovery++
			continue
		}
		updated, err := lastUpdated(task)
		if err != nil {
			result.Issues["incomplete"] += fmt.Sprintf(" task %s: %v; use `critty history list` and `critty status`; use `critty history detach %s --session <id>` only for a confirmed missing member; retain uncertain records.", task.ID, err, task.ID)
			continue
		}
		if task.CompletedAt != "" && s.deferred(task, updated) {
			continue
		}
		if !updated.Before(cutoff) {
			continue
		}
		if task.CompletedAt == "" {
			open++
		} else {
			completed++
		}
	}
	entries, err := review.ListSessionEntries()
	if err != nil {
		result.Issues["incomplete"] = "saved reviews could not be listed; use `critty doctor` and `critty status` for diagnostics and retain unavailable records"
	}
	for _, entry := range entries {
		if entry.CWD != s.CWD || members[entry.Key] {
			continue
		}
		path := entry.ReviewPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(entry.CWD, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			ungrouped++
		}
	}
	if completed+open+ungrouped > 0 {
		result.Issues["age"] = fmt.Sprintf("saved records idle beyond keep_days=%d: %d completed task(s), %d open task(s), %d ungrouped review(s); inspect `critty history list` and `critty status`, then apply the history workflow once for this work task. Age does not authorize deletion", policy.KeepDays, completed, open, ungrouped)
	}
	if recovery > 0 {
		result.Issues["recovery"] = fmt.Sprintf("%d task(s) have interrupted history cleanup; inspect `critty history list` and resume only the recorded task/action with `critty history finish`", recovery)
	}
	return result
}
