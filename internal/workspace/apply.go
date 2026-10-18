package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Receipt struct {
	Snapshot string   `json:"snapshot"`
	Pending  string   `json:"pending,omitempty"`
	Temp     string   `json:"temp,omitempty"`
	Applied  []string `json:"applied"`
	Complete bool     `json:"complete"`
}

// Apply verifies the entire batch before writing, then journals each replacement.
// The caller must validate human authorization for the supplied snapshot.
func (w *Workspace) Apply(id string, authorize func(Snapshot) error) (receipt Receipt, result error) {
	l, err := w.lock()
	if err != nil {
		return receipt, err
	}
	defer l.Unlock()
	s, err := w.ReadSnapshot(id)
	if err != nil {
		return receipt, err
	}
	if err := authorize(s); err != nil {
		return receipt, err
	}
	canonical, err := filepath.EvalSymlinks(w.Root)
	if err != nil {
		return receipt, err
	}
	if canonical != w.Root {
		return receipt, fmt.Errorf("source root redirected")
	}
	root, err := openRoot(w.Root)
	if err != nil {
		return receipt, err
	}
	defer root.Close()
	candidate, err := openRoot(filepath.Join(w.Dir, "snapshots", id, "candidate"))
	if err != nil {
		return receipt, err
	}
	defer candidate.Close()
	receiptPath := filepath.Join(w.Dir, "receipt.json")
	err = readJSON(receiptPath, &receipt)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return receipt, err
	}
	if receipt.Snapshot != "" && receipt.Snapshot != id {
		return receipt, fmt.Errorf("workspace already applied another snapshot")
	}
	receipt.Snapshot = id
	done := map[string]bool{}
	for _, name := range receipt.Applied {
		done[name] = true
	}
	contents := map[string][]byte{}
	for _, name := range sortedNames(w.Files) {
		after, data, err := inspect(candidate, name)
		if err != nil {
			return receipt, err
		}
		if after != s.After[name] {
			return receipt, fmt.Errorf("candidate changed: %s", name)
		}
		contents[name] = data
		current, _, err := inspect(root, name)
		if err != nil {
			return receipt, err
		}
		want := s.Files[name]
		if done[name] {
			want = after
		}
		if name == receipt.Pending && current == after {
			done[name] = true
			receipt.Applied = append(receipt.Applied, name)
			receipt.Pending = ""
			want = after
		}
		if current != want {
			return receipt, fmt.Errorf("original changed: %s", name)
		}
	}
	if receipt.Complete {
		return receipt, nil
	}
	// Baseline copies are retained for recovery; verify them before any writes.
	originals, err := openRoot(filepath.Join(w.Dir, "originals"))
	if err != nil {
		return receipt, err
	}
	defer originals.Close()
	for name, before := range s.Files {
		f, _, err := inspect(originals, name)
		if err != nil {
			return receipt, err
		}
		if f != before {
			return receipt, fmt.Errorf("backup changed: %s", name)
		}
	}
	if err := w.cleanTemp(root, receipt.Temp); err != nil {
		return receipt, err
	}
	receipt.Temp = ""
	if err := save(receiptPath, receipt); err != nil {
		return receipt, err
	}
	names := sortedNames(w.Files)
	slices.SortStableFunc(names, func(a, b string) int {
		rank := func(name string) int {
			if !s.After[name].Exists {
				return 2
			}
			if s.Files[name].Exists {
				return 1
			}
			return 0
		}
		return rank(a) - rank(b)
	})
	for _, name := range names {
		if done[name] || s.Files[name] == s.After[name] {
			continue
		}
		current, _, err := inspect(root, name)
		if err != nil {
			return receipt, err
		}
		if current != s.Files[name] {
			return receipt, fmt.Errorf("original changed: %s", name)
		}
		receipt.Pending = name
		after := s.After[name]
		if after.Exists {
			receipt.Temp = filepath.Join(filepath.Dir(name), ".critty-"+identifier())
			if _, ok := w.Files[receipt.Temp]; ok {
				return receipt, fmt.Errorf("temporary path overlaps a target")
			}
		}
		if err := save(receiptPath, receipt); err != nil {
			return receipt, err
		}
		if after.Exists {
			// Stage on the destination filesystem before replacing the destination.
			temp := receipt.Temp
			if err := root.MkdirAll(filepath.Dir(name), 0777); err != nil {
				return receipt, err
			}
			if err := write(root, temp, contents[name], after.Mode); err != nil {
				return receipt, err
			}
			current, _, err = inspect(root, name)
			if err != nil || current != s.Files[name] {
				root.Remove(temp)
				if err != nil {
					return receipt, err
				}
				return receipt, fmt.Errorf("original changed: %s", name)
			}
			if err := root.Rename(temp, name); err != nil {
				root.Remove(temp)
				return receipt, err
			}
		} else {
			if err := root.Remove(name); err != nil {
				return receipt, err
			}
		}
		got, _, err := inspect(root, name)
		if err != nil {
			return receipt, err
		}
		if got != after {
			return receipt, fmt.Errorf("verification failed: %s", name)
		}
		receipt.Applied = append(receipt.Applied, name)
		receipt.Pending = ""
		receipt.Temp = ""
		if err := save(receiptPath, receipt); err != nil {
			return receipt, err
		}
	}
	receipt.Complete = true
	return receipt, save(receiptPath, receipt)
}

func (w *Workspace) cleanTemp(root *os.Root, name string) error {
	if name == "" {
		return nil
	}
	valid := validPath(name) && validID(strings.TrimPrefix(filepath.Base(name), ".critty-")) && strings.HasPrefix(filepath.Base(name), ".critty-")
	if _, registered := w.Files[name]; registered {
		valid = false
	}
	parent := false
	for target := range w.Files {
		parent = parent || filepath.Dir(target) == filepath.Dir(name)
	}
	if !valid || !parent {
		return fmt.Errorf("invalid pending temporary file")
	}
	info, err := root.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("pending temporary file is not regular")
		}
		if err := root.Remove(name); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
