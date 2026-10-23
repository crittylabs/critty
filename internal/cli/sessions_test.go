package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/review"
)

func TestSessionsListsStatesAndDirectoryScope(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	t.Cleanup(func() { time.Local = previousLocal })
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var sessions []*review.Session
	for i, verdict := range []review.Verdict{"", review.VerdictApprove, review.VerdictComment, review.VerdictReject} {
		file := "a.md"
		if verdict == review.VerdictApprove {
			file = "レビュー.md"
		}
		sess, err := review.NewSession("", review.SessionEntry{
			CWD: cwd, Mode: "files", Args: []string{file},
			StartedAt: time.Date(2026, 10, 23, 0, i, 0, 0, time.UTC).Format(time.RFC3339),
		})
		if err != nil {
			t.Fatal(err)
		}
		if verdict != "" {
			sess.SetSubmission(verdict, "", "Reviewer")
			sess.CJ.RoundState.Finished = true
		}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, sess)
	}
	otherDirectory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := review.NewSession("", review.SessionEntry{CWD: otherDirectory, Mode: "git", StartedAt: "2027-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := newSessionsCommand()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	var items []sessionStatus
	if err := json.Unmarshal([]byte(run("--json")), &items); err != nil {
		t.Fatal(err)
	}
	if run("list", "--json") != run("--json") {
		t.Fatal("default listing differs from sessions list")
	}
	if len(items) != 4 {
		t.Fatalf("current directory has %d sessions, want 4", len(items))
	}
	states := map[string]bool{}
	for _, item := range items {
		states[item.Status] = true
		if item.CWD != cwd || item.Round != 1 || item.Running {
			t.Fatalf("wrong session details: %+v", item)
		}
	}
	for _, state := range []string{"stopped", "approved", "feedback", "changes-requested"} {
		if !states[state] {
			t.Errorf("missing state %s", state)
		}
	}
	if err := json.Unmarshal([]byte(run("--json", "--all")), &items); err != nil || len(items) != 5 {
		t.Fatalf("all sessions: %d, %v", len(items), err)
	}
	if err := json.Unmarshal([]byte(run("list", "--json", "--all", "--sort", "time")), &items); err != nil || len(items) != 5 {
		t.Fatalf("chronological sessions: %d, %v", len(items), err)
	}
	if items[0].Key != other.Key || items[1].Key != sessions[3].Key || items[4].Key != sessions[0].Key {
		t.Fatalf("chronological listing has the wrong order: %+v", items)
	}
	if err := os.Symlink(otherDirectory, "linked directory"); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{otherDirectory, "linked directory"} {
		t.Run(directory, func(t *testing.T) {
			if err := json.Unmarshal([]byte(run("--json", "--directory", directory)), &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].Key != other.Key {
				t.Fatalf("directory filter returned %+v", items)
			}
		})
	}
	if got, err := os.Getwd(); err != nil || got != cwd {
		t.Fatalf("listing changed directory: %q, %v", got, err)
	}
	table := run()
	for _, want := range []string{"PROJECT", "ID", "STATUS", "STARTED", "2026-10-23T09:00:00+09:00", "a.md", "approved"} {
		if !strings.Contains(table, want) {
			t.Errorf("table omits %q", want)
		}
	}
	header, _, _ := strings.Cut(table, "\n")
	idColumn := ansi.StringWidth(header[:strings.Index(header, "ID")])
	for _, line := range strings.Split(table, "\n") {
		for _, sess := range sessions {
			if i := strings.Index(line, sess.Key); i >= 0 && ansi.StringWidth(line[:i]) != idColumn {
				t.Fatalf("ID column shifts: %q", line)
			}
		}
	}
	if strings.Contains(table, cwd) {
		t.Fatal("listing exposes the session working directory")
	}
	entry := savedSession{SessionEntry: sessions[1].Meta, Running: true}
	if item := inspectSessionStatus(entry); item.Status != "running" {
		t.Fatalf("active session shown as %s", item.Status)
	}
	if err := os.Remove(sessions[0].Path()); err != nil {
		t.Fatal(err)
	}
	if item := inspectSessionStatus(savedSession{SessionEntry: sessions[0].Meta}); item.Status != "unavailable" || item.Error == "" {
		t.Fatalf("missing review shown as %+v", item)
	}
}

func TestSessionsLifecycleCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"stop", "remove"} {
		cmd := newSessionsCommand()
		cmd.SetArgs([]string{verb, sess.Key})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(sess.Path())
		if verb == "stop" && err != nil {
			t.Fatalf("stop lost review data: %v", err)
		}
		if verb == "remove" && !os.IsNotExist(err) {
			t.Fatalf("remove retained review data: %v", err)
		}
	}
	entries, err := review.ListSessionEntries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Key == sess.Key {
			t.Fatal("remove retained registry entry")
		}
	}
	cmd, _, err := rootCmd.Find([]string{"sessions", "resume"})
	if err != nil || cmd != resumeCmd {
		t.Fatalf("sessions resume is not registered: %v", err)
	}
	cmd, _, _ = rootCmd.Find([]string{"resume"})
	if cmd == resumeCmd {
		t.Fatal("resume was registered at the top level")
	}
}

func TestSessionsRejectsInvalidDirectoryFilters(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("file", []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--all", "--directory", "."},
		{"--directory="},
		{"--directory", "missing"},
		{"--directory", "file"},
		{"--sort", "invalid"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newSessionsCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("invalid directory filter succeeded")
			}
		})
	}
}

func TestSessionsSortOrder(t *testing.T) {
	item := func(key, project, started string, running bool) sessionStatus {
		return sessionStatus{
			savedSession: savedSession{SessionEntry: review.SessionEntry{Key: key, StartedAt: started}, Running: running},
			ProjectID:    project, ProjectName: project,
		}
	}
	items := []sessionStatus{
		item("beta-new", "beta", "2026-10-23T00:30:00Z", false),
		item("alpha-new", "alpha", "2026-10-23T10:00:00+09:00", false),
		item("alpha-running", "alpha", "2026-10-23T00:00:00Z", true),
		item("beta-unknown", "beta", "invalid", false),
		item("alpha-fraction", "alpha", "2026-10-23T01:00:00.5Z", false),
	}
	for _, tc := range []struct {
		order string
		want  []string
	}{
		{"project", []string{"alpha-running", "alpha-fraction", "alpha-new", "beta-new", "beta-unknown"}},
		{"time", []string{"alpha-fraction", "alpha-new", "beta-new", "alpha-running", "beta-unknown"}},
	} {
		t.Run(tc.order, func(t *testing.T) {
			sorted := slices.Clone(items)
			sortSessionStatuses(sorted, tc.order)
			var got []string
			for _, item := range sorted {
				got = append(got, item.Key)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("session order = %v, want %v", got, tc.want)
			}
		})
	}
}
