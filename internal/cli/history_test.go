package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/review"
)

func historyCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	command := newHistoryCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestHistoryCommandTaskLifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd := t.TempDir()
	t.Chdir(cwd)
	var started history.StartResult
	if err := json.Unmarshal(historyCommand(t, "start", "Task with several reviews", "--project", "Selected project"), &started); err != nil {
		t.Fatal(err)
	}
	if started.Task.Project != "Selected project" {
		t.Fatalf("project override was lost: %+v", started.Task)
	}
	session, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "files", Args: []string{"doc.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	historyCommand(t, "add", started.Task.ID, "--session", session.Key)
	// Grouped reviews defer even explicitly enabled approval cleanup.
	cfg, err := taskReviewConfig(&config.Config{CleanupOnApprove: true}, session)
	if err != nil || cfg.CleanupOnApprove {
		t.Fatalf("task did not defer approval cleanup: %+v %v", cfg, err)
	}
	var quiet history.Outcome
	if err := json.Unmarshal(historyCommand(t, "finish", started.Task.ID), &quiet); err != nil {
		t.Fatal(err)
	}
	if quiet.DecisionRequired || quiet.Task.Pruned || quiet.Storage == nil {
		t.Fatalf("default completion was not quiet: %+v", quiet)
	}
	var usage history.StorageUsage
	if err := json.Unmarshal(historyCommand(t, "usage"), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.Bytes <= 0 || usage.Exceeded {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	historyCommand(t, "reopen", started.Task.ID)
	var exported map[string]string
	if err := json.Unmarshal(historyCommand(t, "export", started.Task.ID, "--output", t.TempDir()), &exported); err != nil {
		t.Fatal(err)
	}
	if err := history.Verify(exported["archive"]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("export removed original:", err)
	}
	var result history.Outcome
	if err := json.Unmarshal(historyCommand(t, "finish", started.Task.ID, "--action", "ask"), &result); err != nil {
		t.Fatal(err)
	}
	if !result.DecisionRequired || result.Task.Pruned {
		t.Fatalf("ask removed data or omitted decision: %+v", result)
	}
	if _, err := taskReviewConfig(&config.Config{}, session); err == nil {
		t.Fatal("completed task resumed without reopen")
	}
	historyCommand(t, "reopen", started.Task.ID)
	if _, err := taskReviewConfig(&config.Config{}, session); err != nil {
		t.Fatal(err)
	}
	historyCommand(t, "finish", started.Task.ID, "--action", "archive")
	if _, err := os.Stat(session.Dir); !os.IsNotExist(err) {
		t.Fatalf("archive action retained original: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exported["archive"], "review.md")); err != nil {
		t.Fatal("organization removed previous export:", err)
	}

	var backup history.StartResult
	if err := json.Unmarshal(historyCommand(t, "start", "Rolling backup"), &backup); err != nil {
		t.Fatal(err)
	}
	original, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	historyCommand(t, "add", backup.Task.ID, "--session", original.Key)
	if err := os.MkdirAll(filepath.Dir(config.GlobalPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.GlobalPath(), []byte("[history]\nkeep_days = 30\nmax_bytes = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var rotated history.Outcome
	if err := json.Unmarshal(historyCommand(t, "finish", backup.Task.ID), &rotated); err != nil {
		t.Fatal(err)
	}
	if !rotated.Task.Pruned || rotated.DecisionRequired || len(rotated.Maintenance) != 1 || rotated.Storage.Bytes != 0 {
		t.Fatalf("configured capacity did not rotate backup: %+v", rotated)
	}
	if _, err := os.Stat(original.Dir); !os.IsNotExist(err) {
		t.Fatalf("rotation retained original: %v", err)
	}
	if err := history.Verify(exported["archive"]); err != nil {
		t.Fatal("rotation damaged archive:", err)
	}
}

func TestUnavailableHistoryRetainsUnrelatedReview(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd := t.TempDir()
	t.Chdir(cwd)
	store := history.New(cwd)
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "aaaaaaaaaaaa.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := taskReviewConfig(&config.Config{CleanupOnApprove: true}, session)
	if err != nil || cfg.CleanupOnApprove {
		t.Fatalf("unrelated review blocked or exposed to cleanup: %+v %v", cfg, err)
	}
}
