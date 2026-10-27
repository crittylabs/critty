package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestAutoKeepsRecentWorkAndOffersOldTasks(t *testing.T) {
	s := testStore(t)
	policy, err := (Policy{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	task := startTask(t, s, "Useful discussion")
	session := addReview(t, s, task)
	finished, err := s.FinishPolicy(task.ID, policy)
	if err != nil || finished.DecisionRequired || finished.Task.Pruned || len(finished.Maintenance) != 0 {
		t.Fatalf("routine completion was not quiet retention: %+v %v", finished, err)
	}
	recent, err := s.Maintain(policy)
	if err != nil || len(recent.Outcomes) != 0 {
		t.Fatalf("recent work caused a question: %+v %v", recent, err)
	}
	s.now = func() time.Time { return time.Now().AddDate(0, 0, 31) }
	old, err := s.Maintain(policy)
	if err != nil || len(old.Outcomes) != 1 || !old.Outcomes[0].DecisionRequired || old.Outcomes[0].Task.ID != task.ID {
		t.Fatalf("old work was not offered: %+v %v", old, err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("age alone removed a review:", err)
	}
}

func TestCapacityRotatesOldestWithHeadroom(t *testing.T) {
	s := testStore(t)
	policy, err := (Policy{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	tasks := []Task{}
	paths := []string{}
	// Creation order differs from saved-update order.  Busy work is oldest.
	for i, name := range []string{"Newer", "Oldest available", "Busy", "Ongoing"} {
		task := startTask(t, s, name)
		session := addReview(t, s, task)
		if err := os.WriteFile(filepath.Join(session.Dir, "payload"), []byte(strings.Repeat("x", 10000)), 0o600); err != nil {
			t.Fatal(err)
		}
		if i != 3 {
			if _, err := s.FinishPolicy(task.ID, Policy{End: "keep"}); err != nil {
				t.Fatal(err)
			}
		}
		updated := time.Date(2026, 11, 3-i, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(session.Path(), updated, updated); err != nil {
			t.Fatal(err)
		}
		tasks, paths = append(tasks, task), append(paths, session.Path())
	}
	archive, err := s.Export(tasks[1].ID, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(filepath.Dir(paths[2]), "run.lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Unlock(); err != nil {
			t.Error(err)
		}
	})
	policy.MaxBytes = s.Usage(DefaultMaxBytes).Bytes - 1
	result, err := s.Maintain(policy)
	if err != nil || result.Storage.Bytes > policy.MaxBytes/5*4 {
		t.Fatalf("rotation did not reach headroom: %+v %v", result, err)
	}
	if len(result.Outcomes) != 2 || result.Outcomes[0].Action != "skip" || result.Outcomes[1].Task.ID != tasks[1].ID || result.Outcomes[1].Action != "delete" {
		t.Fatalf("wrong rotation order: %+v", result.Outcomes)
	}
	for i, path := range paths {
		_, err := os.Stat(path)
		if i == 1 {
			if !os.IsNotExist(err) {
				t.Fatalf("old backup survived: %v", err)
			}
		} else if err != nil {
			t.Fatalf("protected or newer review disappeared: %v", err)
		}
	}
	if err := Verify(archive); err != nil {
		t.Fatal("rotation damaged explicit archive:", err)
	}
	again, err := s.Maintain(policy)
	if err != nil || len(again.Outcomes) != 0 {
		t.Fatalf("repeated rotation below limit: %+v %v", again, err)
	}
}

func TestCapacityDoesNotExpandScopeOrOverrideKeep(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Current work")
	session := addReview(t, s, task)
	if _, err := s.FinishPolicy(task.ID, Policy{End: "keep"}); err != nil {
		t.Fatal(err)
	}
	other := New(t.TempDir())
	otherTask := startTask(t, other, "Other directory")
	otherSession := addReview(t, other, otherTask)
	if _, err := other.FinishPolicy(otherTask.ID, Policy{End: "keep"}); err != nil {
		t.Fatal(err)
	}
	policy, err := (Policy{Start: "keep", MaxBytes: 1}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Maintain(policy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("fixed keep lost data:", err)
	}
	policy.Start = "auto"
	result, err := s.Maintain(policy)
	if err != nil || result.Storage.Bytes != 0 || result.Limited {
		t.Fatalf("other directory contributed to this budget: %+v %v", result, err)
	}
	if _, err := os.Stat(otherSession.Path()); err != nil {
		t.Fatal("rotation crossed directories:", err)
	}
}

func TestUsageExcludesArchivesAndSymlinkContents(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Archive")
	session := addReview(t, s, task)
	before := s.Usage(DefaultMaxBytes)
	if _, err := s.Export(task.ID, "", false); err != nil {
		t.Fatal(err)
	}
	// Export changes the task receipt, which is also outside the backup budget.
	if after := s.Usage(DefaultMaxBytes); after.Bytes != before.Bytes {
		t.Fatalf("archive counted as backup: %+v %+v", before, after)
	}
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, []byte("not part of a backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(session.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	after := s.Usage(DefaultMaxBytes)
	if after.Bytes != before.Bytes || len(after.Warnings) == 0 {
		t.Fatalf("symlink followed or not reported: %+v", after)
	}
}

func TestAutoFinishChecksCapacityAndProtectsChangedReviews(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Final task")
	session := addReview(t, s, task)
	policy, err := (Policy{MaxBytes: 1}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.FinishPolicy(task.ID, policy)
	if err != nil || result.Task.Pruned || len(result.Maintenance) != 0 || result.DecisionRequired || !result.Limited || result.Reason == "" {
		t.Fatalf("end did not preserve the just-finished backup: %+v %v", result, err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatalf("just-finished backup was removed: %v", err)
	}

	next := startTask(t, s, "Changed after selection")
	saved := addReview(t, s, next)
	finished, err := s.FinishPolicy(next.ID, Policy{End: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := lastUpdated(finished.Task)
	if err != nil {
		t.Fatal(err)
	}
	changed := selected.Add(time.Hour)
	if err := os.Chtimes(saved.Path(), changed, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.applyChecked(&finished.Task, "delete", "", &selected); err == nil {
		t.Fatal("changed review removed using stale selection")
	}
	if _, err := os.Stat(saved.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved.Path(), []byte("invalid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	limited, err := s.Maintain(policy)
	if err != nil || !limited.Limited {
		t.Fatalf("corrupt review not protected: %+v %v", limited, err)
	}
	if _, err := os.Stat(saved.Path()); err != nil {
		t.Fatal("corrupt review was deleted:", err)
	}
}
