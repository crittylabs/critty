package tui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/fsnotify/fsnotify"
)

type sourceWatcher struct {
	stamps    map[string]os.FileInfo
	done      chan struct{}
	closeOnce sync.Once
	watcher   *fsnotify.Watcher
	root      string
	tree      bool
	files     map[string]bool
	known     map[string]bool
	ignored   map[string]bool
}

type watchReadyMsg struct {
	epoch   uint64
	watcher *sourceWatcher
	err     error
}
type watchChangedMsg struct {
	epoch   uint64
	changed bool
	refresh bool
	err     error
}

func (m AppModel) startWatch() tea.Cmd {
	epoch, ctx := m.reload.epoch, m.reload.context
	if ctx == nil {
		ctx = context.Background()
	}
	var source *gitpkg.ReviewSource
	if m.source != nil {
		s := *m.source
		source = &s
	} else if m.filePath == "" && m.patch == nil {
		scope := "all"
		if m.staged {
			scope = "staged"
		}
		source = &gitpkg.ReviewSource{Scope: scope}
	}
	var files []string
	if m.filePath != "" && !m.frozenDocument() {
		files = append(files, m.filePath)
	}
	for _, d := range m.attachments {
		if !d.Removed && d.Source != "" {
			files = append(files, d.Source)
		}
	}
	return func() tea.Msg {
		w, err := newSourceWatcher(ctx, source, files)
		return watchReadyMsg{epoch, w, err}
	}
}

func newSourceWatcher(ctx context.Context, source *gitpkg.ReviewSource, files []string) (*sourceWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &sourceWatcher{watcher: fw, done: make(chan struct{}), files: map[string]bool{}, stamps: map[string]os.FileInfo{}, known: map[string]bool{}, ignored: map[string]bool{}}
	if err := w.setup(source, files); err != nil {
		w.Close()
		return nil, err
	}
	for path := range w.files {
		info, err := os.Stat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			w.Close()
			return nil, err
		}
		w.stamps[path] = info
	}
	go func() {
		select {
		case <-ctx.Done():
			w.Close()
		case <-w.done:
		}
	}()
	return w, nil
}

func (w *sourceWatcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		// Closing releases OS watches and wakes pending readers.
		w.watcher.Close()
	})
}

func (w *sourceWatcher) setup(source *gitpkg.ReviewSource, files []string) error {
	if source != nil && source.Scope != "range" {
		root, metadata, err := source.WatchInputs()
		if err != nil {
			return err
		}
		w.root = root
		files = append(files, metadata...)
		w.tree = source.Scope != "staged"
	}
	for _, path := range files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		w.files[abs] = true
		// Watch the nearest existing ancestor so atomic saves and newly created
		// refs directories remain visible without observing unrelated metadata.
		parent := filepath.Dir(abs)
		for {
			if _, err := os.Stat(parent); err == nil {
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			parent = next
		}
		if err := w.watcher.Add(parent); err != nil {
			return err
		}
	}
	if !w.tree {
		return nil
	}
	paths, err := gitpkg.WatchFiles(nil)
	if err != nil {
		return err
	}
	parents := map[string]bool{}
	for _, path := range paths {
		abs := gitpkg.AbsoluteWatchPath(w.root, path)
		w.known[abs] = true
		for dir := filepath.Dir(abs); dir != w.root && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			parents[dir] = true
		}
	}
	ignored, err := gitpkg.IgnoredDirectories()
	if err != nil {
		return err
	}
	for _, path := range ignored {
		if path != "" && strings.HasSuffix(path, "/") {
			abs := gitpkg.AbsoluteWatchPath(w.root, path)
			if !parents[abs] {
				w.ignored[abs] = true
			}
		}
	}
	return w.addTree(w.root)
}

func (w *sourceWatcher) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" || w.ignored[path] {
			return filepath.SkipDir
		}
		return w.watcher.Add(path)
	})
}

// next coalesces a burst of filesystem events before checking unknown paths.
// Idle watchers run no timers, Git commands, or directory scans.
func (w *sourceWatcher) next(epoch uint64) tea.Cmd {
	return func() tea.Msg {
		changed, refresh, err := w.wait()
		return watchChangedMsg{epoch, changed, refresh, err}
	}
}

func (w *sourceWatcher) wait() (bool, bool, error) {
	paths := map[string]bool{}
	var timer *time.Timer
	var deadline <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return false, false, nil
			}
			path := filepath.Clean(event.Name)

			if w.files[path] {
				info, err := os.Stat(path)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return false, false, err
				}
				previous := w.stamps[path]
				if info == nil && previous == nil || info != nil && previous != nil && os.SameFile(info, previous) && info.Size() == previous.Size() && info.ModTime() == previous.ModTime() && info.Mode() == previous.Mode() {
					continue
				}
				w.stamps[path] = info
				if filepath.Base(path) == "index" {
					for candidate, relevant := range w.known {
						if !relevant {
							delete(w.known, candidate)
						}
					}
				}
				paths[path] = true
			} else if w.tree && within(w.root, path) && !within(filepath.Join(w.root, ".git"), path) {
				info, statErr := os.Stat(path)
				if statErr == nil && info.IsDir() && !event.Has(fsnotify.Create) {
					continue
				}
				if statErr == nil && info.IsDir() && event.Has(fsnotify.Create) {
					ignored, err := gitpkg.IgnoredPath(path)
					if err != nil {
						return false, false, err
					}
					if ignored {
						w.ignored[path] = true
						continue
					}
					ignoredPaths, err := gitpkg.IgnoredDirectories(path)
					if err != nil {
						return false, false, err
					}
					for _, ignored := range ignoredPaths {
						if strings.HasSuffix(ignored, "/") {
							w.ignored[gitpkg.AbsoluteWatchPath(w.root, ignored)] = true
						}
					}
					if err := w.addTree(path); err != nil {
						return false, false, err
					}
				}
				skip := false
				for dir := filepath.Dir(path); within(w.root, dir); dir = filepath.Dir(dir) {
					if w.ignored[dir] {
						skip = true
						break
					}
					if dir == w.root {
						break
					}
				}
				if skip {
					continue
				}
				if relevant, known := w.known[path]; known && !relevant {
					continue
				}
				paths[path] = true
			} else {
				// A missing parent of an exact metadata path may just have appeared.
				for target := range w.files {
					if within(path, target) && path != target {
						if info, err := os.Stat(path); err == nil && info.IsDir() {
							if err := w.watcher.Add(path); err != nil {
								return false, false, err
							}
						}
					}
				}
				continue
			}
			if timer == nil {
				timer = time.NewTimer(150 * time.Millisecond)
				deadline = timer.C
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return false, false, nil
			}
			return false, false, err
		case <-deadline:

			var unknown []string
			relevant, refresh := false, false
			for path := range paths {
				if w.files[path] || w.known[path] {
					relevant = true
				} else {
					unknown = append(unknown, path)
				}
				if filepath.Base(path) == ".gitignore" || filepath.Base(path) == ".gitattributes" || w.files[path] && (filepath.Base(path) == "HEAD" || filepath.Base(path) == "exclude") {
					relevant, refresh = true, true
				}
			}
			if len(unknown) > 0 {
				for _, path := range unknown {
					w.known[path] = false
				}
				included, err := gitpkg.WatchFiles(unknown)
				if err != nil {
					return false, false, err
				}
				for _, path := range included {
					w.known[gitpkg.AbsoluteWatchPath(w.root, path)] = true
					relevant = true
				}
			}
			return relevant, refresh, nil
		}
	}
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
