// Package migration retires old local names before normal Critty reads begin.
package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/xdg"
)

type move struct {
	from, to string
	state    bool
}

// Run renames absent destinations only.  Existing Critty paths short-circuit
// before the corresponding old path is inspected.  Reports go to stderr.
func Run(project string, report io.Writer) error {
	state := xdg.StateHome()
	config := xdg.ConfigHome()
	moves := []move{
		{from: filepath.Join(filepath.Dir(state), "tcrit"), to: state, state: true},
		{from: filepath.Join(filepath.Dir(config), "tcrit"), to: config},
	}
	if project != "" {
		moves = append(moves,
			move{from: filepath.Join(project, ".tcrit.config.json"), to: filepath.Join(project, ".critty.config.json")},
			move{from: filepath.Join(project, ".tcrit"), to: filepath.Join(project, ".critty")},
		)
	}
	var pending []move
	for _, item := range moves {
		needed, err := item.needed()
		if err != nil {
			return err
		}
		if needed {
			pending = append(pending, item)
		}
	}
	if len(pending) > 0 {
		if err := renameAll(pending, filepath.Dir(state), report); err != nil {
			return err
		}
	}
	if _, current := os.LookupEnv("CRITTY_HOST"); !current {
		if value, old := os.LookupEnv("TCRIT_HOST"); old {
			if err := os.Setenv("CRITTY_HOST", value); err != nil {
				return err
			}
			if err := os.Unsetenv("TCRIT_HOST"); err != nil {
				return err
			}
			_, err := fmt.Fprintln(report, "critty: migrated TCRIT_HOST to CRITTY_HOST for this process; update the launcher environment")
			return err
		}
	}
	return nil
}

func (m move) needed() (bool, error) {
	if _, err := os.Lstat(m.to); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	info, err := os.Lstat(m.from)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("cannot migrate symbolic link %s; rename its data directory first", m.from)
	}
	if m.state && !info.IsDir() {
		return false, fmt.Errorf("state path is not a directory: %s", m.from)
	}
	return true, nil
}

func renameAll(moves []move, base string, report io.Writer) (err error) {
	if err := os.MkdirAll(base, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(base, ".critty-migration.lock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("local migration is busy; retry")
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	for _, item := range moves {
		needed, err := item.needed()
		if err != nil {
			return err
		}
		if !needed {
			continue
		}
		if item.state {
			err = renameState(item)
		} else {
			err = renameAbsent(item.from, item.to)
		}
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("migrating %s to %s: %w", item.from, item.to, err)
		}
		if _, err := fmt.Fprintf(report, "critty: migrated %s to %s\n", item.from, item.to); err != nil {
			return err
		}
	}
	return nil
}

type edit struct {
	path          string
	before, after []byte
	mode          os.FileMode
}

func stateEdits(m move) ([]edit, error) {
	from, err := filepath.Abs(m.from)
	if err != nil {
		return nil, err
	}
	to, err := filepath.Abs(m.to)
	if err != nil {
		return nil, err
	}
	var edits []edit
	for _, group := range [][2]string{{"sessions", ".json"}, {"reviews", "review.json"}} {
		paths, err := stateFiles(m.from, group[0], group[1])
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			info, err := regularStateFile(m.from, path)
			if err != nil {
				return nil, err
			}
			before, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(before, &doc); err != nil {
				return nil, fmt.Errorf("reading %s: %w", path, err)
			}
			changed := false
			if filepath.Base(path) == "review.json" {
				if _, exists := doc["critty_state"]; !exists {
					if old, exists := doc["tcrit_state"]; exists {
						doc["critty_state"] = old
						delete(doc, "tcrit_state")
						changed = true
					}
				}
			} else {
				for _, key := range []string{"review_path", "socket_path"} {
					if raw, exists := doc[key]; exists {
						var value string
						if err := json.Unmarshal(raw, &value); err != nil {
							return nil, err
						}
						if strings.HasPrefix(value, from+string(filepath.Separator)) {
							next, err := json.Marshal(to + strings.TrimPrefix(value, from))
							if err != nil {
								return nil, err
							}
							doc[key], changed = next, true
						}
					}
				}
			}
			if changed {
				after, err := json.MarshalIndent(doc, "", "  ")
				if err != nil {
					return nil, err
				}
				rel, err := filepath.Rel(m.from, path)
				if err != nil {
					return nil, err
				}
				edits = append(edits, edit{path: rel, before: before, after: append(after, '\n'), mode: info.Mode().Perm()})
			}
		}
	}
	return edits, nil
}

func stateFiles(root, directory, name string) ([]string, error) {
	parent := filepath.Join(root, directory)
	info, err := os.Lstat(parent)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("state directory is not regular: %s", parent)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		path := filepath.Join(parent, entry.Name())
		if directory == "sessions" {
			if strings.HasSuffix(entry.Name(), name) {
				paths = append(paths, path)
			}
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("state directory is not regular: %s", path)
		}
		if !entry.IsDir() {
			continue
		}
		path = filepath.Join(path, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func regularStateFile(root, path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("state file is not regular: %s", path)
	}
	for parent := filepath.Dir(path); parent != root; parent = filepath.Dir(parent) {
		entry, err := os.Lstat(parent)
		if err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("state directory is not regular: %s", parent)
		}
	}
	return info, nil
}

func renameState(m move) (err error) {
	sockets, err := stateFiles(m.from, "sessions", ".sock")
	if err != nil {
		return err
	}
	for _, path := range sockets {
		if ipc.Alive(path) {
			return fmt.Errorf("stop active reviews before migrating state")
		}
	}
	var locks []*flock.Flock
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			err = errors.Join(err, locks[i].Unlock())
		}
	}()
	for _, name := range []string{"run.lock", "review.json.lock"} {
		paths, err := stateFiles(m.from, "reviews", name)
		if err != nil {
			return err
		}
		for _, path := range paths {
			if _, err := regularStateFile(m.from, path); err != nil {
				return err
			}
			lock := flock.New(path)
			locked, err := lock.TryLock()
			if err != nil {
				return err
			}
			if !locked {
				return fmt.Errorf("stop busy reviews before migrating state")
			}
			locks = append(locks, lock)
		}
	}
	edits, err := stateEdits(m)
	if err != nil {
		return err
	}
	if err := renameAbsent(m.from, m.to); err != nil {
		return err
	}
	written := 0
	for _, item := range edits {
		if err = replace(filepath.Join(m.to, item.path), item.after, item.mode); err != nil {
			break
		}
		written++
	}
	if err != nil {
		for _, item := range edits[:written] {
			err = errors.Join(err, replace(filepath.Join(m.to, item.path), item.before, item.mode))
		}
		return errors.Join(err, renameAbsent(m.to, m.from))
	}
	return nil
}

func replace(path string, data []byte, mode os.FileMode) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".migrating-")
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(file.Name()); !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}()
	err = file.Chmod(mode)
	if err == nil {
		_, err = file.Write(data)
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
