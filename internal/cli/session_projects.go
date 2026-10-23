package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crittylabs/critty/internal/review"
)

type sessionProject struct {
	id, name string
}

func collectProjectSessions(directory string, all bool) ([]sessionStatus, error) {
	directory, err := resolveSessionDirectory(directory)
	if err != nil {
		return nil, err
	}
	entries, err := review.ListSessionEntries()
	if err != nil {
		return nil, err
	}
	projects := map[string]sessionProject{}
	resolve := func(entry review.SessionEntry) sessionProject {
		if entry.GitCommonDir != "" {
			return sessionProject{id: "git:" + entry.GitCommonDir, name: gitProjectName(entry.GitCommonDir)}
		}
		if project, ok := projects[entry.CWD]; ok {
			return project
		}
		path := canonicalProjectDirectory(entry.CWD)
		project := sessionProject{id: "directory:" + path, name: filepath.Base(path)}
		if common := reviewGitCommonDir(entry.CWD); common != "" {
			project = sessionProject{id: "git:" + common, name: gitProjectName(common)}
		}
		projects[entry.CWD] = project
		return project
	}
	var selected sessionProject
	var roots []string
	if !all {
		selected = resolve(review.SessionEntry{CWD: directory})
		if strings.HasPrefix(selected.id, "git:") {
			roots = projectWorktrees(strings.TrimPrefix(selected.id, "git:"))
		}
	} else {
		missing := map[string][]string{}
		for _, entry := range entries {
			projects[entry.CWD] = resolve(entry)
			if entry.GitCommonDir == "" {
				if _, err := os.Stat(entry.CWD); os.IsNotExist(err) {
					path := canonicalProjectDirectory(entry.CWD)
					missing[path] = append(missing[path], entry.CWD)
				}
			}
		}
		if len(missing) > 0 {
			known := map[string]sessionProject{}
			for _, project := range projects {
				if strings.HasPrefix(project.id, "git:") {
					known[project.id] = project
				}
			}
			for id, project := range known {
				for _, root := range projectWorktrees(strings.TrimPrefix(id, "git:")) {
					for _, path := range missing[root] {
						projects[path] = project
					}
				}
			}
		}
	}
	items := []sessionStatus{}
	for _, entry := range entries {
		if !all && entry.GitCommonDir == "" && roots != nil {
			path := canonicalProjectDirectory(entry.CWD)
			contained := false
			for _, root := range roots {
				if path == root {
					projects[entry.CWD] = selected
					contained = true
					break
				}
				if strings.HasPrefix(path, root+string(filepath.Separator)) {
					contained = true
				}
			}
			if !contained {
				continue
			}
		}
		project := resolve(entry)
		if !all && project.id != selected.id {
			continue
		}
		if entry.GitCommonDir == "" {
			if common, ok := strings.CutPrefix(project.id, "git:"); ok {
				if err := review.RecordSessionProject(entry, common); err != nil {
					return nil, fmt.Errorf("recording project for session %s: %w", entry.Key, err)
				}
				entry.GitCommonDir = common
			}
		}
		item := inspectSessionStatus(savedSessionStatus(entry))
		item.ProjectID, item.ProjectName = project.id, project.name
		items = append(items, item)
	}
	return items, nil
}

// A nil result keeps the legacy per-directory lookup if Git cannot list worktrees.
func projectWorktrees(common string) []string {
	cmd := exec.Command("git", "--git-dir", common, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = common
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	roots := []string{}
	for field := range strings.SplitSeq(string(out), "\x00") {
		if root, ok := strings.CutPrefix(field, "worktree "); ok {
			roots = append(roots, canonicalProjectDirectory(root))
		}
	}
	return roots
}

// Resolve the surviving parent when a removed worktree used a symlink alias.
func canonicalProjectDirectory(path string) string {
	for parent := filepath.Clean(path); ; parent = filepath.Dir(parent) {
		if canonical, err := filepath.EvalSymlinks(parent); err == nil {
			if suffix, err := filepath.Rel(parent, path); err == nil {
				return filepath.Join(canonical, suffix)
			}
		}
		if parent == filepath.Dir(parent) {
			return filepath.Clean(path)
		}
	}
}
