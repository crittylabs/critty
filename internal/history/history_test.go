package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/crittylabs/critty/internal/review"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd := t.TempDir()
	t.Chdir(cwd)
	store := New(cwd)
	store.Project = "Test Project"
	store.now = func() time.Time { return time.Date(2026, 10, 18, 13, 0, 0, 0, time.FixedZone("JST", 9*3600)) }
	return store
}

func startTask(t *testing.T, store *Store, name string) Task {
	t.Helper()
	policy, err := (Policy{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Start(name, policy)
	if err != nil {
		t.Fatal(err)
	}
	return result.Task
}

func addReview(t *testing.T, store *Store, task Task) *review.Session {
	t.Helper()
	session, err := review.NewSession("", review.SessionEntry{CWD: store.CWD, Mode: "files", Args: []string{"source.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(task.ID, session.Key); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestArchiveIsPortableAndPreservesOriginals(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "日本語 / review [one]")
	first := addReview(t, store, task)
	second := addReview(t, store, task)
	imageName := "画像 (one).png"
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imageName, imageData.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	managed, err := first.SaveAttachment(imageData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	first.CJ.ReviewComments = []review.Comment{{ID: "r_1", Body: "Final direction", Resolved: true, Replies: []review.Reply{{ID: "rp_1", Body: "![managed](" + managed + ")", Author: "Agent"}}}}
	content := "# Source\n\n![inline](<画像 (one).png>)\n\n![reference][picture]\n\n[picture]: <画像 (one).png>\n\n<img src=\"画像 (one).png\" alt=\"日本語\">\n\n[Open image](<画像 (one).png>)\n\n[Another document](another.md#section)\n\n```md\n![example](missing.png)\n```\n\n![web](https://example.test/remote.png)\n"
	first.CJ.RoundState.Files = map[string]review.SnapshotFile{"source.md": {Content: content}}
	documents := []review.AttachedDocument{
		{ID: "a_doc", Name: "attached.md", Source: filepath.Join(store.CWD, "attached.md"), Revision: "sha256:original", Content: content},
		{ID: "a_other", Name: "another.md", Source: filepath.Join(store.CWD, "another.md"), Content: "# Section\n"},
	}
	first.CJ.RoundState.Files[documents[0].Path()] = review.SnapshotFile{Content: content}
	if err := first.ReplaceDocuments(documents); err != nil {
		t.Fatal(err)
	}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	// Keep fields unknown to Critty byte-for-byte in the original JSON.
	raw, err := os.ReadFile(first.Path())
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"future_crit_field\": true,\n"), 1)
	if err := os.WriteFile(first.Path(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := store.Export(task.ID, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	manifestData, err := os.ReadFile(filepath.Join(archive, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestData, &m); err != nil {
		t.Fatal(err)
	}
	origins := map[string]bool{}
	for _, asset := range m.Assets {
		if asset.Source == "" || asset.CapturedAt == "" {
			t.Fatal("asset lacks provenance")
		}
		origins[asset.Origin] = true
	}
	if !origins["local file at export"] || !origins["session attachment"] {
		t.Fatalf("incomplete provenance: %+v", m.Assets)
	}
	if !strings.HasPrefix(filepath.Base(archive), "2026-10-18 130000 - Test Project - 日本語") {
		t.Fatalf("unexpected archive folder: %s", archive)
	}
	for _, pattern := range []string{
		filepath.Join(archive, "assets", "*", "*.png"),
		filepath.Join(archive, "attachments", first.Key, "a_doc", "assets", "*", "*.png"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) != 1 {
			t.Fatalf("image is outside its document's assets directory: %s: %v %v", pattern, matches, err)
		}
	}
	if _, err := os.Stat(second.Path()); err != nil {
		t.Fatal("export removed originals:", err)
	}
	archivedRaw, err := os.ReadFile(filepath.Join(archive, "sessions", first.Key, "review.json"))
	if err != nil || !bytes.Equal(raw, archivedRaw) {
		t.Fatalf("changed original JSON: %v", err)
	}
	main, err := os.ReadFile(filepath.Join(archive, "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{first.Key, second.Key, "Final direction", "Reply by Agent", "Resolved: true"} {
		if !strings.Contains(string(main), want) {
			t.Fatalf("missing %q in archive", want)
		}
	}
	// Relocating the archive and removing source images must not break it.
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(archive, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(imageName); err != nil {
		t.Fatal(err)
	}
	if err := Verify(moved); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(moved, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".md" || strings.Contains(path, string(filepath.Separator)+"sessions"+string(filepath.Separator)) {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte("src=\"画像 (one).png\"")) {
			t.Fatalf("HTML image was not rewritten: %s", path)
		}
		return ast.Walk(parser.New().Parse(data), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var destination string
			switch node := node.(type) {
			case *ast.Image:
				destination = node.Destination.Str(data)
			case *ast.Link:
				destination = node.Destination.Str(data)
			default:
				return ast.WalkContinue, nil
			}
			u, err := url.Parse(destination)
			if err != nil {
				return ast.WalkStop, err
			}
			if u.Scheme != "" || u.Path == "" {
				return ast.WalkContinue, nil
			}
			_, err = os.ReadFile(filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path)))
			return ast.WalkContinue, err
		})
	})
	if err != nil {
		t.Fatal("archive image reference is broken:", err)
	}
	if err := os.WriteFile(filepath.Join(moved, "review.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(moved); err == nil {
		t.Fatal("checksum verification accepted changed data")
	}
}

func TestTaskBoundaryActions(t *testing.T) {
	for _, action := range []string{"keep", "ask", "archive", "delete"} {
		t.Run(action, func(t *testing.T) {
			store := testStore(t)
			task := startTask(t, store, "One task")
			first := addReview(t, store, task)
			second := addReview(t, store, task)
			result, err := store.Finish(task.ID, action, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			pruned := action == "archive" || action == "delete"
			if result.Task.Pruned != pruned || result.DecisionRequired != (action == "ask") || result.Task.CompletedAt == "" {
				t.Fatalf("unexpected task outcome: %+v", result)
			}
			for _, session := range []*review.Session{first, second} {
				_, err := os.Stat(session.Path())
				if os.IsNotExist(err) != pruned {
					t.Fatalf("incorrect retention for %s: %v", action, err)
				}
			}
			if action == "archive" {
				if err := Verify(result.Archive); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestArchiveFailureAndActiveReviewPreserveTask(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Failure")
	session := addReview(t, store, task)
	session.CJ.ReviewComments = []review.Comment{{ID: "r_missing", Body: "![missing](not-there.png)"}}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := store.Finish(task.ID, "archive", root); err == nil {
		t.Fatal("missing referenced image did not fail archive")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed archive left partial files: %v %v", entries, err)
	}
	saved, err := store.read(task.ID)
	if err != nil || saved.CompletedAt != "" {
		t.Fatalf("failed export completed task: %+v %v", saved, err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("failed export removed original:", err)
	}
	lock := flock.New(filepath.Join(session.Dir, "run.lock"))
	if locked, err := lock.TryLock(); err != nil || !locked {
		t.Fatal("could not simulate active review", err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			t.Error(err)
		}
	}()
	for _, action := range []string{"keep", "ask", "archive", "delete"} {
		if _, err := store.Finish(task.ID, action, root); !errors.Is(err, ErrBusy) {
			t.Fatalf("active review allowed %s: %v", action, err)
		}
	}
}

func TestAgeCleanupOnlyIncludesIdleCompletedTasks(t *testing.T) {
	store := testStore(t)
	idle := startTask(t, store, "Old complete")
	session := addReview(t, store, idle)
	if _, err := store.Finish(idle.ID, "keep", ""); err != nil {
		t.Fatal(err)
	}
	ongoing := startTask(t, store, "Still working")
	other := New(t.TempDir())
	other.now = store.now
	foreign := startTask(t, other, "Another project")
	if _, err := other.Finish(foreign.ID, "keep", ""); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
	// A recently resumed review protects an otherwise old completed task.
	now := store.now()
	if err := os.Chtimes(session.Path(), now, now); err != nil {
		t.Fatal(err)
	}
	results, err := store.Clean("ask", 30, "")
	if err != nil || len(results) != 0 {
		t.Fatalf("included recently updated task: %+v %v", results, err)
	}
	old := now.AddDate(0, 0, -31)
	if err := os.Chtimes(session.Path(), old, old); err != nil {
		t.Fatal(err)
	}
	results, err = store.Clean("ask", 30, "")
	if err != nil || len(results) != 1 || results[0].Task.ID != idle.ID || !results[0].DecisionRequired {
		t.Fatalf("wrong cleanup candidates: %+v %v", results, err)
	}
	lock := flock.New(filepath.Join(session.Dir, "run.lock"))
	if locked, err := lock.TryLock(); err != nil || !locked {
		t.Fatalf("could not simulate active review: %v", err)
	}
	results, err = store.Clean("archive", 30, t.TempDir())
	if err != nil || len(results) != 1 || results[0].Action != "skip" {
		t.Fatalf("did not skip active task: %+v %v", results, err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	started, err := store.Start("Next task", Policy{Start: "archive", KeepDays: 30, ArchiveDir: t.TempDir()})
	if err != nil || len(started.Maintenance) != 1 || !started.Maintenance[0].Task.Pruned {
		t.Fatalf("start cleanup failed: %+v %v", started, err)
	}
	if err := Verify(started.Maintenance[0].Archive); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ongoing.ID, foreign.ID} {
		data, err := os.ReadFile(filepath.Join(store.Root, id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var task Task
		if err := json.Unmarshal(data, &task); err != nil || task.Pruned {
			t.Fatalf("removed unrelated task: %+v %v", task, err)
		}
	}
}

func TestInterruptedPruningCanResumeWithoutLostArchive(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Recovery")
	first := addReview(t, store, task)
	second := addReview(t, store, task)
	path, err := store.Export(task.ID, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.read(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved.CompletedAt, saved.Archive, saved.Pruning, saved.Action = store.now().Format(time.RFC3339Nano), path, true, "archive"
	saved.Pending = []review.SessionEntry{first.Meta, second.Meta}
	if err := store.save(saved); err != nil {
		t.Fatal(err)
	}
	// Simulate removal succeeding just before its progress update was saved.
	if err := first.Clear(); err != nil {
		t.Fatal(err)
	}
	result, err := store.Finish(task.ID, "archive", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Task.Pruned || result.Task.Pruning || result.Task.Archive != path {
		t.Fatalf("incorrect recovery: %+v", result)
	}
	if err := Verify(path); err != nil {
		t.Fatal(err)
	}
}

func TestMissingSessionCanBeKeptDetachedOrDeleted(t *testing.T) {
	for _, action := range []string{"keep", "ask", "delete", "detach", "export"} {
		t.Run(action, func(t *testing.T) {
			store := testStore(t)
			task := startTask(t, store, "Recovery")
			missing := addReview(t, store, task)
			remaining := addReview(t, store, task)
			if err := missing.Clear(); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Finish(task.ID, "archive", t.TempDir()); err == nil {
				t.Fatal("strict archive accepted a missing session")
			}
			switch action {
			case "detach":
				if err := store.Detach(task.ID, missing.Key); err != nil {
					t.Fatal(err)
				}
				result, err := store.Finish(task.ID, "archive", t.TempDir())
				if err != nil || len(result.Task.Sessions) != 1 {
					t.Fatalf("detach recovery failed: %+v %v", result, err)
				}
			case "export":
				path, err := store.Export(task.ID, t.TempDir(), true)
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
				if err != nil || !bytes.Contains(data, []byte(missing.Key)) || !bytes.Contains(data, []byte("warnings")) {
					t.Fatalf("missing export warning: %s %v", data, err)
				}
				if _, err := os.Stat(remaining.Path()); err != nil {
					t.Fatal("incomplete export pruned original", err)
				}
			default:
				result, err := store.Finish(task.ID, action, "")
				if err != nil || len(result.Missing) != 1 || result.Missing[0] != missing.Key {
					t.Fatalf("missing session blocked %s: %+v %v", action, result, err)
				}
			}
		})
	}
}

func TestCorruptUnrelatedHistoryDoesNotBlockMembership(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Valid")
	session := addReview(t, store, task)
	if err := os.WriteFile(filepath.Join(store.Root, "aaaaaaaaaaaa.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ForSession(session.Key); err != nil || got == nil || got.ID != task.ID {
		t.Fatalf("unrelated corruption blocked grouped review: %+v %v", got, err)
	}
	if tasks, err := store.List(); err != nil || len(tasks) != 1 {
		t.Fatalf("unrelated corruption blocked list: %+v %v", tasks, err)
	}
	if _, err := store.ForSession("bbbbbbbbbbbb"); !errors.Is(err, ErrHistoryUnavailable) {
		t.Fatalf("unknown membership did not preserve cleanup safety: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, task.ID+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ForSession(session.Key); err == nil {
		t.Fatal("direct membership ignored its corrupt task")
	}
}

func TestConcurrentMembershipWaitsForOtherWriters(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Concurrent")
	var sessions []*review.Session
	for range 8 {
		sessions = append(sessions, addReview(t, store, task))
	}
	start := make(chan struct{})
	results := make(chan error, len(sessions))
	for _, session := range sessions {
		go func() { <-start; results <- store.Add(task.ID, session.Key) }()
	}
	close(start)
	for range sessions {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Detach(task.ID, sessions[0].Key); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ForSession(sessions[0].Key); err != nil || got != nil {
		t.Fatalf("detached membership survived: %+v %v", got, err)
	}
}

func TestStartReportsArchiveFailureAndContinues(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Old failure")
	session := addReview(t, store, task)
	session.CJ.ReviewComments = []review.Comment{{ID: "r_missing", Body: "![missing](missing.png)"}}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(task.ID, "keep", ""); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
	old := store.now().AddDate(0, 0, -31)
	if err := os.Chtimes(session.Path(), old, old); err != nil {
		t.Fatal(err)
	}
	result, err := store.Start("New work", Policy{Start: "archive", KeepDays: 30, ArchiveDir: t.TempDir()})
	if err != nil || result.Task.ID == "" || len(result.Maintenance) != 1 || result.Maintenance[0].Action != "error" || result.Maintenance[0].Reason == "" {
		t.Fatalf("maintenance blocked or hid failure: %+v %v", result, err)
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("failed maintenance removed original", err)
	}
}

func TestIncompleteExportLeavesExternalReferencesAndProtectsManagedImages(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Incomplete images")
	session := addReview(t, store, task)
	session.CJ.RoundState.Files = map[string]review.SnapshotFile{"doc.md": {Content: "![missing](missing.png)\n\n[ordinary](invalid%zz)\n"}}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := store.Export(task.ID, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil || len(m.Warnings) != 1 {
		t.Fatalf("warning missing: %+v %v", m, err)
	}
	if err := Verify(path); err != nil {
		t.Fatal(err)
	}
	session.CJ.ReviewComments = []review.Comment{{ID: "r_managed", Body: "![managed](attachments/missing.png)"}}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Export(task.ID, t.TempDir(), true); err == nil {
		t.Fatal("incomplete export ignored missing managed attachment")
	}
	if _, err := os.Stat(session.Path()); err != nil {
		t.Fatal("incomplete export removed original", err)
	}
}

func TestHistoryLockTimeoutReportsBusy(t *testing.T) {
	store := testStore(t)
	task := startTask(t, store, "Lock timeout")
	lock := flock.New(filepath.Join(store.Root, "history.lock"))
	if locked, err := lock.TryLock(); err != nil || !locked {
		t.Fatalf("lock setup: %v", err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			t.Error(err)
		}
	}()
	if err := store.withLock(func() error { t.Fatal("entered locked store"); return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("timeout did not report busy: %v (task %s)", err, task.ID)
	}
}
