package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crittylabs/critty/internal/review"
)

func TestBudgetIgnoresOtherDirectoriesOnFinish(t *testing.T) {
	s := testStore(t)
	local := startTask(t, s, "Local")
	session := addReview(t, s, local)
	before := s.Usage(DefaultMaxBytes)
	other := New(t.TempDir())
	foreign := startTask(t, other, "Foreign")
	addReview(t, other, foreign)
	policy, err := (Policy{MaxBytes: before.Bytes + 1}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.FinishPolicy(local.ID, policy)
	if err != nil || result.Limited || result.Storage.Exceeded || result.Storage.CWD != s.CWD {
		t.Fatalf("foreign usage affected completion: %+v %v", result, err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal(err)
	}
}

func TestDeferralPersistsAndChangesInvalidateIt(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Old discussion")
	session := addReview(t, s, task)
	if _, err := s.FinishPolicy(task.ID, Policy{End: "keep"}); err != nil {
		t.Fatal(err)
	}
	now := s.now().AddDate(0, 0, 60)
	s.now = func() time.Time { return now }
	policy, err := (Policy{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Defer(task.ID, 7); err != nil {
		t.Fatal(err)
	}
	// Another host sees the same deferral without a shared progress file.
	other := New(s.CWD)
	other.now = s.now
	if got, err := other.Maintain(policy); err != nil || len(got.Outcomes) != 0 {
		t.Fatalf("deferral lost: %+v %v", got, err)
	}
	if issue := other.Inspect(policy).Issues["age"]; issue != "" {
		t.Fatal("deferred task warned:", issue)
	}
	now = now.AddDate(0, 0, 8)
	if got, err := other.Maintain(policy); err != nil || len(got.Outcomes) != 1 || !got.Outcomes[0].DecisionRequired {
		t.Fatalf("expired deferral hidden: %+v %v", got, err)
	}
	if _, err := s.Defer(task.ID, 0); err != nil {
		t.Fatal(err)
	}
	changed := now.AddDate(0, 0, -40)
	if err := os.Chtimes(session.Path(), changed, changed); err != nil {
		t.Fatal(err)
	}
	if got, err := other.Maintain(policy); err != nil || len(got.Outcomes) != 1 {
		t.Fatalf("changed content did not invalidate deferral: %+v %v", got, err)
	}
	if _, err := s.Defer(task.ID, 7); err != nil {
		t.Fatal(err)
	}
	policy.MaxBytes = 1
	if got, err := s.Maintain(policy); err != nil || len(got.Outcomes) != 1 || !got.Outcomes[0].Task.Pruned {
		t.Fatalf("deferral disabled rotation: %+v %v", got, err)
	}
}

func TestClearProtectsMembershipBeforeAnyRemoval(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Owned")
	member := addReview(t, s, task)
	loose, err := review.NewSession("", review.SessionEntry{CWD: s.CWD, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := loose.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearSessions([]*review.Session{loose, member}); err == nil {
		t.Fatal("member cleared")
	}
	for _, session := range []*review.Session{loose, member} {
		if _, err := os.Stat(session.Path()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Detach(task.ID, member.Key); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearSessions([]*review.Session{member, loose}); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveInventoryAndSafeDeletion(t *testing.T) {
	s := testStore(t)
	task := startTask(t, s, "Durable")
	session := addReview(t, s, task)
	first, err := s.Export(task.ID, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Export(task.ID, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.Archives("")
	if err != nil || len(list.Archives) != 2 || len(list.Warnings) != 0 {
		t.Fatalf("exports missing: %+v %v", list, err)
	}
	if err := os.WriteFile(filepath.Join(first, "unrelated"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArchive(first); err == nil {
		t.Fatal("deleted unrelated file")
	}
	if err := os.Remove(filepath.Join(first, "unrelated")); err != nil {
		t.Fatal(err)
	}
	foreign := New(t.TempDir())
	if err := foreign.DeleteArchive(first); err == nil {
		t.Fatal("deleted another directory's archive")
	}
	receipt, err := s.read(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Pruning, receipt.Action, receipt.Archive = true, "archive", first
	if err := s.save(receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArchive(first); err == nil {
		t.Fatal("deleted archive needed for recovery")
	}
	receipt.Pruning, receipt.Archive = false, second
	if err := s.save(receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArchive(first); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyArchive(second); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArchive(second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("archive deletion touched originals:", err)
	}
	list, err = s.Archives("")
	if err != nil || len(list.Archives) != 0 || len(list.Warnings) != 0 {
		t.Fatalf("stale inventory: %+v %v", list, err)
	}
	details, err := s.Details()
	if err != nil || len(details) != 1 || details[0].UpdatedAt == "" || details[0].Bytes <= 0 {
		t.Fatalf("missing task diagnostics: %+v %v", details, err)
	}
}
