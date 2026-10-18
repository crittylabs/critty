package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/review"
)

type Maintenance struct {
	Outcomes []Outcome     `json:"outcomes"`
	Storage  *StorageUsage `json:"storage,omitempty"`
	Limited  bool          `json:"limited,omitempty"`
	Reason   string        `json:"reason,omitempty"`
}

// Maintain applies the start policy within this working directory.  Auto asks
// about age, but rotates completed backups without a question under pressure.
func (s *Store) Maintain(policy Policy) (result Maintenance, err error) {
	err = s.withLock(func() error {
		result, err = s.maintain(policy, true, "")
		return err
	})
	return result, err
}

// FinishPolicy keeps routine completion quiet and checks capacity only for auto.
func (s *Store) FinishPolicy(id string, policy Policy) (result Outcome, err error) {
	err = s.withLock(func() error {
		result, err = s.finish(id, policy.End, policy.ArchiveDir)
		if err != nil || policy.End != "auto" {
			return err
		}
		maintenance, err := s.maintain(policy, false, id)
		result.Maintenance, result.Storage = maintenance.Outcomes, maintenance.Storage
		result.Limited, result.Reason = maintenance.Limited, maintenance.Reason
		if err != nil {
			result.MaintenanceError = err.Error()
		}
		task, readErr := s.read(id)
		if readErr == nil {
			result.Task = *task
		}
		return readErr
	})
	return result, err
}

func (s *Store) maintain(policy Policy, askAge bool, protected string) (result Maintenance, err error) {
	if !ValidPolicyAction(policy.Start) || policy.KeepDays < 0 || policy.MaxBytes < 0 {
		return result, fmt.Errorf("invalid history retention policy")
	}
	if policy.Start != "auto" && askAge {
		result.Outcomes, err = s.clean(policy.Start, policy.KeepDays, policy.ArchiveDir)
		return result, err
	}
	usage := s.Usage(policy.MaxBytes)
	result.Storage = &usage
	result.Outcomes = []Outcome{}
	if !usage.Exceeded {
		if askAge {
			result.Outcomes, err = s.clean("ask", policy.KeepDays, policy.ArchiveDir)
		}
		return result, err
	}
	if len(usage.Warnings) != 0 {
		return result, ErrHistoryUnavailable
	}
	// Dividing first avoids overflow and leaves headroom after a cleanup pass.
	target := usage.MaxBytes/5*4 + usage.MaxBytes%5*4/5
	tasks, incomplete, err := s.scan()
	if err != nil || incomplete {
		return result, errors.Join(err, ErrHistoryUnavailable)
	}
	type candidate struct {
		task    Task
		updated time.Time
	}
	candidates := []candidate{}
	for _, task := range tasks {
		if task.ID == protected || task.CompletedAt == "" || task.Pruned || task.Pruning {
			continue
		}
		updated, err := lastUpdated(task)
		if err != nil {
			result.Outcomes = append(result.Outcomes, Outcome{Task: task, Action: "skip", Reason: err.Error()})
			continue
		}
		candidates = append(candidates, candidate{task, updated})
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		if order := a.updated.Compare(b.updated); order != 0 {
			return order
		}
		return strings.Compare(a.task.ID, b.task.ID)
	})
	for _, candidate := range candidates {
		if usage.Bytes <= target {
			break
		}
		outcome, applyErr := s.applyChecked(&candidate.task, "delete", "", &candidate.updated)
		outcome.Reason = "automatic backup rotation: storage limit exceeded"
		if applyErr != nil {
			outcome.Action, outcome.Reason = "skip", applyErr.Error()
		}
		result.Outcomes = append(result.Outcomes, outcome)
		usage = s.Usage(policy.MaxBytes)
	}
	result.Storage = &usage
	result.Limited = usage.Bytes > target
	if result.Limited {
		result.Reason = fmt.Sprintf("backup cleanup could not reach %d bytes; ongoing, busy, ungrouped, inconsistent, or just-finished records remain", target)
	}
	return result, nil
}

func lastUpdated(task Task) (time.Time, error) {
	base := task.CompletedAt
	if base == "" {
		base = task.StartedAt
	}
	updated, err := time.Parse(time.RFC3339Nano, base)
	if err != nil {
		return time.Time{}, err
	}
	for _, id := range task.Sessions {
		entry, err := review.ReadSessionEntry(id)
		if err != nil {
			return time.Time{}, err
		}
		path := entry.ReviewPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(entry.CWD, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(updated) {
			updated = info.ModTime()
		}
	}
	return updated, nil
}
