package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/review"
)

func TestDoctorHistoryBypassesNoticeThrottleWithoutChangingRecords(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd := t.TempDir()
	t.Chdir(cwd)
	clean := doctorReport{}
	clean.checkHistory()
	for _, name := range []string{"capacity", "age", "recovery", "incomplete"} {
		if got := diagnosticFor(t, clean, "history/"+name); got.Status != "ok" {
			t.Fatalf("empty history is not healthy: %+v", got)
		}
	}
	if err := os.MkdirAll(filepath.Dir(config.GlobalPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.GlobalPath(), []byte("[history]\nmax_bytes = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := history.New(cwd)
	policy, err := (history.Policy{Start: "keep", MaxBytes: 1}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start("Old work", policy)
	if err != nil {
		t.Fatal(err)
	}
	session, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(started.Task.ID, session.Key); err != nil {
		t.Fatal(err)
	}
	task := started.Task
	task.Sessions = []string{session.Key}
	task.StartedAt, task.CompletedAt = "1999-12-31T00:00:00Z", "2000-01-01T00:00:00Z"
	if err := review.WriteJSON(filepath.Join(store.Root, task.ID+".json"), task); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(session.Path(), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Notices(policy); err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.Glob(filepath.Join(store.Root, "notices", "*.json"))
	if err != nil || len(cache) != 1 {
		t.Fatalf("missing notice cache: %v %v", cache, err)
	}
	before, err := os.ReadFile(cache[0])
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		report := doctorReport{}
		report.checkHistory()
		for _, name := range []string{"capacity", "age"} {
			d := diagnosticFor(t, report, "history/"+name)
			if d.Status != "warn" || !strings.Contains(d.Advice, "critty history") {
				t.Fatalf("doctor omitted fresh warning or remediation: %+v", d)
			}
		}
	}
	after, err := os.ReadFile(cache[0])
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("doctor consumed passive warnings: %v", err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("doctor removed saved review:", err)
	}
	if _, err := store.FinishPolicy(task.ID, history.Policy{End: "delete"}); err != nil {
		t.Fatal(err)
	}
	resolved := doctorReport{}
	resolved.checkHistory()
	if d := diagnosticFor(t, resolved, "history/capacity"); d.Status != "ok" {
		t.Fatalf("doctor used stale capacity after cleanup: %+v", d)
	}
}
