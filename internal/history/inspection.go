package history

import (
	"fmt"
	"time"
)

type TaskInfo struct {
	Task
	UpdatedAt string   `json:"updated_at,omitempty"`
	Bytes     int64    `json:"bytes"`
	Warnings  []string `json:"warnings,omitempty"`
}

// Details supplies decision data without exposing storage layout to callers.
func (s *Store) Details() ([]TaskInfo, error) {
	tasks, err := s.List()
	if err != nil {
		return nil, err
	}
	result := make([]TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, s.describeTask(task))
	}
	return result, nil
}

func (s *Store) describeTask(task Task) TaskInfo {
	info := TaskInfo{Task: task}
	if task.Pruned {
		return info
	}
	updated, err := lastUpdated(task)
	if err != nil {
		info.Warnings = append(info.Warnings, err.Error())
	} else {
		info.UpdatedAt = updated.Format(time.RFC3339Nano)
	}
	selected := map[string]bool{}
	for _, id := range task.Sessions {
		selected[id] = true
	}
	usage := s.usage(DefaultMaxBytes, selected)
	info.Bytes = usage.Bytes
	info.Warnings = append(info.Warnings, usage.Warnings...)
	return info
}

func (s *Store) deferred(task Task, updated time.Time) bool {
	if task.DeferredUpdate == "" || task.DeferredUpdate != updated.Format(time.RFC3339Nano) {
		return false
	}
	if task.DeferredUntil == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339Nano, task.DeferredUntil)
	return err == nil && s.now().Before(until)
}

// Defer suppresses age questions, not capacity rotation.  Zero means until changed.
func (s *Store) Defer(id string, days int) (task *Task, err error) {
	if days < 0 {
		return nil, fmt.Errorf("days must not be negative")
	}
	err = s.withLock(func() error {
		task, err = s.read(id)
		if err != nil {
			return err
		}
		if task.CompletedAt == "" || task.Pruned || task.Pruning {
			return fmt.Errorf("only retained completed tasks can be deferred")
		}
		updated, err := lastUpdated(*task)
		if err != nil {
			return err
		}
		task.DeferredUpdate = updated.Format(time.RFC3339Nano)
		task.DeferredUntil = ""
		if days > 0 {
			task.DeferredUntil = s.now().AddDate(0, 0, days).Format(time.RFC3339Nano)
		}
		return s.save(task)
	})
	return task, err
}
