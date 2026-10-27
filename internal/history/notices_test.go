package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/crittylabs/critty/internal/review"
)

func TestNoticesSurfaceBacklogWithoutChangingReviews(t *testing.T) {
	s := testStore(t)
	done := startTask(t, s, "Completed")
	first := addReview(t, s, done)
	if _, err := s.FinishPolicy(done.ID, Policy{End: "keep"}); err != nil {
		t.Fatal(err)
	}
	open := startTask(t, s, "Open")
	second := addReview(t, s, open)
	loose, err := review.NewSession("", review.SessionEntry{CWD: s.CWD, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := loose.Save(); err != nil {
		t.Fatal(err)
	}
	recovery := startTask(t, s, "Interrupted")
	recovery.Pruning, recovery.Action = true, "delete"
	if err := s.save(&recovery); err != nil {
		t.Fatal(err)
	}
	other := New(t.TempDir())
	foreign := startTask(t, other, "Other directory")
	addReview(t, other, foreign)
	now := time.Now().AddDate(0, 0, 45)
	s.now = func() time.Time { return now }
	messages, err := s.Notices(Policy{MaxBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(messages, "\n")
	for _, want := range []string{"above max_bytes=1", "1 completed task(s), 1 open task(s), 1 ungrouped review(s)", "1 task(s) have interrupted"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	for _, session := range []*review.Session{first, second, loose} {
		if _, err := os.Stat(session.Path()); err != nil {
			t.Fatal("notice removed review:", err)
		}
	}
	kept, err := s.read(done.ID)
	if err != nil || kept.Pruned {
		t.Fatalf("notice organized completed work: %+v %v", kept, err)
	}
	// Frequent commands do not repeat either scans or warning categories.
	if repeated, err := s.Notices(Policy{MaxBytes: 1}); err != nil || len(repeated) != 0 {
		t.Fatalf("immediate repeat: %v %v", repeated, err)
	}
	now = now.Add(2 * time.Hour)
	if repeated, err := s.Notices(Policy{MaxBytes: 1}); err != nil || len(repeated) != 0 {
		t.Fatalf("warning cooldown ignored: %v %v", repeated, err)
	}
	now = now.Add(noticeRepeatInterval)
	if repeated, err := s.Notices(Policy{MaxBytes: 1}); err != nil || len(repeated) != len(messages) {
		t.Fatalf("due warning not repeated: %v %v", repeated, err)
	}
}

func TestNoticeCacheAndConditionChanges(t *testing.T) {
	s := testStore(t)
	policy := Policy{}
	if messages, err := s.Notices(policy); err != nil || len(messages) != 0 {
		t.Fatalf("empty state warned: %v %v", messages, err)
	}
	task := startTask(t, s, "Recent backup")
	addReview(t, s, task)
	policy.MaxBytes = 1
	if messages, err := s.Notices(policy); err != nil || len(messages) != 1 {
		t.Fatalf("policy change did not invalidate cache: %v %v", messages, err)
	}
	cache, err := filepath.Glob(filepath.Join(s.Root, "notices", "*.json"))
	if err != nil || len(cache) != 1 {
		t.Fatalf("missing cache: %v %v", cache, err)
	}
	lock := flock.New(cache[0] + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	if messages, err := s.Notices(policy); err != nil || len(messages) != 0 {
		t.Fatalf("concurrent notice did not stay quiet: %v %v", messages, err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache[0], []byte("invalid cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if messages, err := s.Notices(policy); err != nil || len(messages) != 1 {
		t.Fatalf("damaged cache could not rebuild: %v %v", messages, err)
	}
	now := s.now().Add(2 * time.Hour)
	s.now = func() time.Time { return now }
	if _, err := s.FinishPolicy(task.ID, Policy{End: "delete"}); err != nil {
		t.Fatal(err)
	}
	if messages, err := s.Notices(policy); err != nil || len(messages) != 0 {
		t.Fatalf("resolved capacity warning survived: %v %v", messages, err)
	}
	// A new capacity episode is visible before the previous 7-day cooldown.
	next := startTask(t, s, "New capacity episode")
	addReview(t, s, next)
	now = now.Add(2 * time.Hour)
	if messages, err := s.Notices(policy); err != nil || len(messages) != 1 {
		t.Fatalf("new capacity episode suppressed: %v %v", messages, err)
	}
}
