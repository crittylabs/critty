package history

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/xdg"
)

type ArchiveInfo struct {
	Path     string   `json:"path"`
	Task     Task     `json:"task"`
	SavedAt  string   `json:"saved_at"`
	Bytes    int64    `json:"bytes"`
	Warnings []string `json:"warnings,omitempty"`
}

type ArchiveList struct {
	Archives []ArchiveInfo `json:"archives"`
	Warnings []string      `json:"warnings,omitempty"`
}

func (s *Store) archiveIndex(path string) string {
	return filepath.Join(s.Root, "archives", fmt.Sprintf("%x.json", sha256.Sum256([]byte(path))))
}

func (s *Store) recordArchive(path string) error {
	info, err := s.archiveInfo(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.archiveIndex(path)), 0o700); err != nil {
		return err
	}
	return review.WriteJSON(s.archiveIndex(path), info)
}

func (s *Store) archiveInfo(path string) (ArchiveInfo, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return ArchiveInfo{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ArchiveInfo{}, err
	}
	if !info.IsDir() {
		return ArchiveInfo{}, fmt.Errorf("archive must be a directory without a symlink")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return ArchiveInfo{}, err
	}
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		return ArchiveInfo{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return ArchiveInfo{}, err
	}
	if m.Version != 1 || !review.ValidSessionKey(m.Task.ID) || m.Task.CWD != s.CWD || len(m.Files) == 0 {
		return ArchiveInfo{}, fmt.Errorf("not a history archive belonging to this directory")
	}
	usage := measureDirectories(s.CWD, 0, []string{path})
	return ArchiveInfo{Path: path, Task: m.Task, SavedAt: m.SavedAt, Bytes: usage.Bytes, Warnings: usage.Warnings}, nil
}

// Archives finds known exports, including earlier exports to custom destinations.
// --directory can discover copied archives without agents traversing their layout.
func (s *Store) Archives(archiveDir string) (ArchiveList, error) {
	result := ArchiveList{Archives: []ArchiveInfo{}}
	paths, roots := map[string]bool{}, map[string]bool{filepath.Join(xdg.StateHome(), "archives"): true}
	if archiveDir != "" {
		roots[archiveDir] = true
	}
	tasks, err := s.List()
	if err != nil {
		return result, err
	}
	for _, task := range tasks {
		if task.Archive != "" {
			paths[task.Archive] = true
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "archives"))
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Root, "archives", entry.Name()))
		var info ArchiveInfo
		if err != nil || json.Unmarshal(data, &info) != nil {
			result.Warnings = append(result.Warnings, "archive index unavailable: "+entry.Name())
			continue
		}
		if info.Task.CWD == s.CWD {
			paths[info.Path] = true
		}
	}
	for root := range roots {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", root, err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if info, err := s.archiveInfo(path); err == nil {
				paths[info.Path] = true
			}
		}
	}
	seen := map[string]bool{}
	for path := range paths {
		info, err := s.archiveInfo(path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if !seen[info.Path] {
			result.Archives = append(result.Archives, info)
			seen[info.Path] = true
		}
	}
	slices.SortFunc(result.Archives, func(a, b ArchiveInfo) int { return strings.Compare(a.Path, b.Path) })
	slices.Sort(result.Warnings)
	return result, nil
}

// DeleteArchive removes only the exact verified archive, never its live reviews.
func (s *Store) DeleteArchive(path string) (err error) {
	return s.withLock(func() error {
		info, err := s.archiveInfo(path)
		if err != nil {
			return err
		}
		tasks, incomplete, err := s.scan()
		if err != nil || incomplete {
			return fmt.Errorf("history unavailable; retain archives until task records can be checked")
		}
		for _, task := range tasks {
			if task.Pruning {
				archive, err := filepath.EvalSymlinks(task.Archive)
				if err == nil && archive == info.Path {
					return fmt.Errorf("archive is required by interrupted cleanup for task %s; finish recovery first", task.ID)
				}
			}
		}
		root, err := filepath.EvalSymlinks(info.Path)
		if err != nil {
			return err
		}
		protected := []string{s.CWD, s.Root, xdg.StateHome()}
		entries, err := review.ListSessionEntries()
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := entry.ReviewPath
			if !filepath.IsAbs(path) {
				path = filepath.Join(entry.CWD, path)
			}
			protected = append(protected, filepath.Dir(path), entry.CWD)
		}
		for _, path := range protected {
			resolved, err := filepath.EvalSymlinks(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, resolved)
			if err != nil || rel == "." || filepath.IsLocal(rel) {
				return fmt.Errorf("archive contains live review or working state: %s", path)
			}
		}
		if err := Verify(info.Path); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(info.Path, "manifest.json"))
		if err != nil {
			return err
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		// Reject extra files and links so deletion cannot consume unrelated additions.
		if err := filepath.WalkDir(info.Path, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(info.Path, path)
			if err != nil {
				return err
			}
			if !entry.Type().IsRegular() || (rel != "manifest.json" && m.Files[filepath.ToSlash(rel)] == "") {
				return fmt.Errorf("archive contains an unrecorded or unsupported file: %s", rel)
			}
			return nil
		}); err != nil {
			return err
		}
		if err := os.RemoveAll(info.Path); err != nil {
			return err
		}
		if err := os.Remove(s.archiveIndex(info.Path)); err != nil && !os.IsNotExist(err) {
			return err
		}
		task, err := s.read(info.Task.ID)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if task.Archive == info.Path {
			task.Archive = ""
			return s.save(task)
		}
		return nil
	})
}

func (s *Store) VerifyArchive(path string) error {
	info, err := s.archiveInfo(path)
	if err != nil {
		return err
	}
	return Verify(info.Path)
}
