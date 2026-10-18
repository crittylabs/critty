package migration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

func setup(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("CRITTY_HOST", "")
	project := filepath.Join(base, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(base, "state"), filepath.Join(base, "config"), project
}

func write(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRenamedDataIsReadUnderCurrentNames(t *testing.T) {
	state, configRoot, project := setup(t)
	old := filepath.Join(state, "tcrit")
	id := "123456abcdef"
	oldReview := filepath.Join(old, "reviews", id, "review.json")
	write(t, oldReview, map[string]any{
		"files": map[string]any{}, "review_round": 3,
		"tcrit_state":     map[string]any{"verdict": "comment", "review_comment_draft": "keep me"},
		"review_comments": []map[string]any{{"id": "r_saved", "body": "saved feedback"}},
		"future_field":    "untouched",
	})
	write(t, filepath.Join(old, "sessions", id+".json"), map[string]any{"key": id, "cwd": project, "review_path": oldReview, "socket_path": filepath.Join(old, "sessions", id+".sock")})
	write(t, filepath.Join(old, "archives", "saved", "review.json"), map[string]any{"tcrit_state": "immutable"})
	write(t, filepath.Join(configRoot, "tcrit", "config.json"), map[string]any{"author": "Migrated", "cleanup_on_approve": false})
	write(t, filepath.Join(project, ".tcrit.config.json"), map[string]any{"quiet": true})
	write(t, filepath.Join(project, ".tcrit", "prompts", "example.md"), "prompt")
	var output bytes.Buffer
	if err := Run(project, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "critty: migrated ") != 4 {
		t.Fatal(output.String())
	}
	for _, path := range []string{old, filepath.Join(configRoot, "tcrit"), filepath.Join(project, ".tcrit.config.json"), filepath.Join(project, ".tcrit")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("old name remains: %s: %v", path, err)
		}
	}
	cfg, err := config.Load(project)
	if err != nil || cfg.Author != "Migrated" || !cfg.Quiet || cfg.CleanupOnApprove {
		t.Fatalf("config was not reread: %+v %v", cfg, err)
	}
	entry, err := review.ReadSessionEntry(id)
	if err != nil {
		t.Fatal(err)
	}
	session, err := review.OpenSessionFromEntry(*entry)
	if err != nil || session.CJ.RoundState.ReviewCommentDraft != "keep me" || session.CJ.ReviewRound != 3 || session.CJ.ReviewComments[0].Body != "saved feedback" {
		t.Fatalf("review state lost: %+v %v", session, err)
	}
	data, err := os.ReadFile(session.Path())
	if err != nil || !bytes.Contains(data, []byte("future_field")) || bytes.Contains(data, []byte("tcrit_state")) {
		t.Fatalf("metadata changed incorrectly: %s %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(state, "critty", "archives", "saved", "review.json"))
	if err != nil || !bytes.Contains(data, []byte("tcrit_state")) {
		t.Fatalf("archive was rewritten: %s %v", data, err)
	}
	output.Reset()
	if err := Run(project, &output); err != nil || output.Len() != 0 {
		t.Fatalf("migration repeated: %s %v", output.String(), err)
	}
}

func TestCurrentDestinationsIgnoreOldPaths(t *testing.T) {
	state, configRoot, project := setup(t)
	// Invalid old data must never be examined when the destination exists.
	write(t, filepath.Join(state, "critty", "marker"), "current")
	write(t, filepath.Join(state, "tcrit", "reviews", "123456abcdef", "review.json"), "invalid metadata")
	write(t, filepath.Join(configRoot, "critty", "config.json"), map[string]any{"author": "Current"})
	write(t, filepath.Join(project, ".critty.config.json"), map[string]any{})
	write(t, filepath.Join(project, ".critty", "marker"), "current")
	for _, path := range []string{filepath.Join(configRoot, "tcrit"), filepath.Join(project, ".tcrit.config.json"), filepath.Join(project, ".tcrit")} {
		if err := os.Symlink("missing-target", path); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := Run(project, &output); err != nil || output.Len() != 0 {
		t.Fatalf("old paths affected current data: %s %v", output.String(), err)
	}
	cfg, err := config.Load(project)
	if err != nil || cfg.Author != "Current" {
		t.Fatalf("wrong configuration: %+v %v", cfg, err)
	}
}

func TestActiveAndInvalidStateStayUnderOldName(t *testing.T) {
	for _, mode := range []string{"active", "busy", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			state, _, project := setup(t)
			if mode == "active" {
				short, err := os.MkdirTemp("", "cm-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(short); err != nil {
						t.Error(err)
					}
				})
				state = short
				t.Setenv("XDG_STATE_HOME", state)
			}
			old := filepath.Join(state, "tcrit")
			path := filepath.Join(old, "reviews", "123456abcdef", "review.json")
			write(t, path, map[string]any{"tcrit_state": map[string]any{}})
			switch mode {
			case "active":
				if err := os.MkdirAll(filepath.Join(old, "sessions"), 0700); err != nil {
					t.Fatal(err)
				}
				listener, err := ipc.Listen(filepath.Join(old, "sessions", "active.sock"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := listener.Close(); err != nil {
						t.Error(err)
					}
				})
			case "busy":
				lock := flock.New(filepath.Join(filepath.Dir(path), "run.lock"))
				if ok, err := lock.TryLock(); err != nil || !ok {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := lock.Unlock(); err != nil {
						t.Error(err)
					}
				})
			case "invalid":
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			if err := Run(project, &output); err == nil {
				t.Fatal("unsafe migration succeeded")
			}
			if output.Len() != 0 {
				t.Fatal("reported success before migration")
			}
			if _, err := os.Stat(old); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(state, "critty")); !os.IsNotExist(err) {
				t.Fatalf("destination created on failure: %v", err)
			}
		})
	}
}

func TestRenameCannotReplaceExistingDestination(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			base := t.TempDir()
			from, to := filepath.Join(base, "old"), filepath.Join(base, "current")
			if directory {
				if err := os.Mkdir(from, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(to, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, from, "old")
				write(t, to, "current")
			}
			if err := renameAbsent(from, to); err == nil {
				t.Fatal("replaced destination")
			}
			if _, err := os.Lstat(from); err != nil {
				t.Fatal("removed original", err)
			}
		})
	}
}

func TestHostEnvironmentRename(t *testing.T) {
	_, _, project := setup(t)
	t.Setenv("TCRIT_HOST", "tmux")
	if err := os.Unsetenv("CRITTY_HOST"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run(project, &output); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CRITTY_HOST") != "tmux" || output.Len() == 0 {
		t.Fatal("host was not renamed")
	}
	// With a current value, even an old variable is left alone and unreported.
	t.Setenv("TCRIT_HOST", "other")
	output.Reset()
	if err := Run(project, &output); err != nil || output.Len() != 0 || os.Getenv("CRITTY_HOST") != "tmux" {
		t.Fatalf("current host did not win: %v %s", err, output.String())
	}
}

func TestFreshStartupHasNoMigrationEffects(t *testing.T) {
	state, configRoot, project := setup(t)
	var output bytes.Buffer
	if err := Run(project, &output); err != nil || output.Len() != 0 {
		t.Fatalf("unexpected migration: %s %v", output.String(), err)
	}
	for _, path := range []string{state, configRoot, filepath.Join(project, ".critty")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("startup created %s: %v", path, err)
		}
	}
}

func TestConcurrentStartupMigratesOnce(t *testing.T) {
	_, configRoot, project := setup(t)
	write(t, filepath.Join(configRoot, "tcrit", "config.json"), map[string]any{"author": "Keep"})
	type result struct {
		output bytes.Buffer
		err    error
	}
	results := make(chan *result, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			r := &result{}
			r.err = Run(project, &r.output)
			results <- r
		}()
	}
	close(start)
	reports := 0
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		reports += strings.Count(r.output.String(), "critty: migrated ")
	}
	if reports != 1 {
		t.Fatalf("reported %d migrations", reports)
	}
}

func TestFailedMetadataWriteRestoresOldState(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions")
	}
	state, _, project := setup(t)
	old := filepath.Join(state, "tcrit")
	reviewPath := filepath.Join(old, "reviews", "123456abcdef", "review.json")
	sessionPath := filepath.Join(old, "sessions", "123456abcdef.json")
	write(t, reviewPath, map[string]any{"tcrit_state": map[string]any{"review_comment_draft": "original"}})
	write(t, sessionPath, map[string]any{"review_path": reviewPath})
	before, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(reviewPath)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Error(err)
		}
	})
	var output bytes.Buffer
	if err := Run(project, &output); err == nil {
		t.Fatal("failed write reported success")
	}
	if output.Len() != 0 {
		t.Fatal(output.String())
	}
	after, err := os.ReadFile(sessionPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("registry was not rolled back: %s %v", after, err)
	}
	if _, err := os.Lstat(filepath.Join(state, "critty")); !os.IsNotExist(err) {
		t.Fatalf("failed migration left destination: %v", err)
	}
}

func TestStateMigrationDoesNotFollowMetadataLinks(t *testing.T) {
	state, _, project := setup(t)
	old := filepath.Join(state, "tcrit")
	outside := filepath.Join(t.TempDir(), "review.json")
	write(t, outside, map[string]any{"tcrit_state": map[string]any{"review_comment_draft": "outside"}})
	before, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(old, "reviews"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(old, "reviews", "linked")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run(project, &output); err == nil {
		t.Fatal("followed a metadata directory link")
	}
	after, err := os.ReadFile(outside)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("external data changed: %s %v", after, err)
	}
}
