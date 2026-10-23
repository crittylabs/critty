package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

func waitWatch(t *testing.T, w *sourceWatcher, mutate func()) watchChangedMsg {
	t.Helper()
	result := make(chan watchChangedMsg, 1)
	go func() { result <- w.next(0)().(watchChangedMsg) }()
	mutate()
	select {
	case msg := <-result:
		if msg.err != nil {
			t.Fatal(msg.err)
		}
		return msg
	case <-time.After(5 * time.Second):
		w.Close()
		t.Fatal("filesystem notification timed out")
		return watchChangedMsg{}
	}
}

func TestWatchWorktreeIsolation(t *testing.T) {
	dir := newGitRepo(t)
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("tracked.txt", "original\n")
	write(".gitignore", "ignored/\n*.log\n")
	if err := os.MkdirAll("ignored/deep", 0700); err != nil {
		t.Fatal(err)
	}
	write("ignored/deep/build", "output")
	runGitIn(t, dir, "add", "tracked.txt", ".gitignore")
	runGitIn(t, dir, "commit", "-qm", "initial")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(parent, "other")
	runGitIn(t, dir, "worktree", "add", "-qb", "other", other)
	t.Chdir(other)
	source := &gitpkg.ReviewSource{Scope: "all", Base: "HEAD"}
	w, err := newSourceWatcher(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, watched := range w.watcher.WatchList() {
		if strings.Contains(watched, "ignored") || strings.Contains(watched, filepath.Join(".git", "objects")) || strings.Contains(watched, filepath.Join(".git", "logs")) {
			t.Fatalf("unnecessary watch: %s", watched)
		}
	}
	ownIndex := filepath.Join(dir, ".git", "worktrees", "other", "index")
	if !w.files[ownIndex] || w.files[filepath.Join(dir, ".git", "index")] {
		t.Fatalf("wrong worktree index: %+v", w.files)
	}
	// Shared object writes and the other index must not be selected as inputs.
	for _, path := range []string{filepath.Join(dir, ".git", "index"), filepath.Join(dir, ".git", "objects", "unrelated"), filepath.Join(dir, "tracked.txt")} {
		if w.files[path] || within(w.root, path) {
			t.Fatalf("other worktree is in scope: %s", path)
		}
	}
	msg := waitWatch(t, w, func() { write("noise.log", "ignored") })
	if msg.changed {
		t.Fatal("ignored file caused a reload")
	}
	msg = waitWatch(t, w, func() { write("tracked.txt", "changed\n") })
	if !msg.changed || msg.refresh {
		t.Fatal("tracked change must reload without rebuilding watches")
	}
	msg = waitWatch(t, w, func() {
		write("saved.tmp", "atomic\n")
		if err := os.Rename("saved.tmp", "tracked.txt"); err != nil {
			t.Fatal(err)
		}
	})
	if !msg.changed {
		t.Fatal("atomic save was missed")
	}
	msg = waitWatch(t, w, func() {
		if err := os.MkdirAll("new/nested", 0700); err != nil {
			t.Fatal(err)
		}
		write("new/nested/file.txt", "new\n")
	})
	if !msg.changed || !slices.Contains(w.watcher.WatchList(), filepath.Join(other, "new", "nested")) {
		t.Fatal("new directory was not watched")
	}
}

func TestWatchStagedIgnoresWorktree(t *testing.T) {
	dir := newGitRepo(t)
	if err := os.WriteFile("tracked", []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "add", "tracked")
	runGitIn(t, dir, "commit", "-qm", "base")
	w, err := newSourceWatcher(context.Background(), &gitpkg.ReviewSource{Scope: "staged"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.tree {
		t.Fatal("staged scope watches working files")
	}
	msg := waitWatch(t, w, func() {
		if err := os.WriteFile("tracked", []byte("next"), 0600); err != nil {
			t.Fatal(err)
		}
		runGitIn(t, dir, "add", "tracked")
	})
	if !msg.changed {
		t.Fatal("index change was missed")
	}
}

func TestWatchExternalDocumentAtomicSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := newSourceWatcher(context.Background(), nil, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	msg := waitWatch(t, w, func() {
		if err := os.WriteFile(path+".tmp", []byte("new"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
	})
	if !msg.changed {
		t.Fatal("document atomic save missed")
	}
}
