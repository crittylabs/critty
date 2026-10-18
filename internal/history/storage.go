package history

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/crittylabs/critty/internal/review"
)

const DefaultMaxBytes int64 = 1 << 30

// StorageUsage counts allocated bytes in known original review directories.
// Explicit archives and task receipts are outside the rolling backup budget.
type StorageUsage struct {
	CWD      string   `json:"cwd"`
	Bytes    int64    `json:"bytes"`
	MaxBytes int64    `json:"max_bytes"`
	Exceeded bool     `json:"exceeded"`
	Roots    []string `json:"roots"`
	Warnings []string `json:"warnings,omitempty"`
}

type diskAllocation struct {
	bytes  int64
	device uint64
	inode  uint64
}

type fileIdentity struct {
	device uint64
	inode  uint64
}

func (s *Store) Usage(maxBytes int64) StorageUsage {
	return s.usage(maxBytes, nil)
}

func (s *Store) usage(maxBytes int64, selected map[string]bool) StorageUsage {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	var roots, warnings []string
	entries, err := review.ListSessionEntries()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("%s: %v", review.SessionsDir(), err))
	}
	for _, entry := range entries {
		if entry.CWD != s.CWD || (selected != nil && !selected[entry.Key]) {
			continue
		}
		path := entry.ReviewPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(entry.CWD, path)
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(dir, entry.CWD)
		if filepath.Base(path) != "review.json" || err != nil || rel == "." || filepath.IsLocal(rel) {
			warnings = append(warnings, fmt.Sprintf("%s: review directory cannot contain its working directory", path))
			continue
		}
		roots = append(roots, dir)
	}
	u := measureDirectories(s.CWD, maxBytes, roots)
	u.Warnings = append(u.Warnings, warnings...)
	return u
}

func measureDirectories(cwd string, maxBytes int64, roots []string) StorageUsage {
	u := StorageUsage{CWD: cwd, MaxBytes: maxBytes, Roots: []string{}}
	warn := func(path string, err error) {
		u.Warnings = append(u.Warnings, fmt.Sprintf("%s: %v", path, err))
	}
	seen := map[string]bool{}
	counted := map[fileIdentity]bool{}
	include := func(root string) {
		original := root
		info, err := os.Lstat(root)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			warn(root, err)
			return
		}
		if !info.IsDir() {
			warn(root, fmt.Errorf("review root is not a directory without a symlink"))
			return
		}
		root, err = filepath.EvalSymlinks(root)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			warn(original, err)
			return
		}
		root, err = filepath.Abs(root)
		if err != nil {
			warn(original, err)
			return
		}
		for _, parent := range u.Roots {
			if rel, err := filepath.Rel(parent, root); err == nil && (rel == "." || filepath.IsLocal(rel)) {
				return
			}
		}
		u.Roots = append(u.Roots, root)
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				warn(path, err)
				return nil
			}
			if seen[path] {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			seen[path] = true
			if entry.Type()&os.ModeSymlink != 0 {
				warn(path, fmt.Errorf("symbolic link excluded"))
				return nil
			}
			if !entry.Type().IsRegular() && !entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				warn(path, err)
				return nil
			}
			allocation, err := allocatedDiskSpace(info)
			if err != nil {
				warn(path, err)
				return nil
			}
			id := fileIdentity{allocation.device, allocation.inode}
			if !counted[id] {
				counted[id] = true
				u.Bytes += allocation.bytes
			}
			return nil
		}); err != nil {
			warn(root, err)
		}
	}
	for _, root := range roots {
		include(root)
	}
	slices.Sort(u.Roots)
	u.Exceeded = u.Bytes > u.MaxBytes
	return u
}
