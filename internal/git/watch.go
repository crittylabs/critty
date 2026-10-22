package git

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// WatchInputs resolves worktree-local metadata through Git, including linked
// worktrees whose index and HEAD live outside the working directory.
func (s ReviewSource) WatchInputs() (string, []string, error) {
	out, err := gitCommand("rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, err
	}
	root := strings.TrimSpace(out)
	names := []string{"index", "HEAD", "config", "config.worktree", "info/exclude", "info/attributes", "packed-refs"}

	branch, err := gitCommand("symbolic-ref", "-q", "HEAD")
	if err == nil {
		names = append(names, strings.TrimSpace(branch))
	}
	var paths []string
	for _, name := range names {
		out, err := gitCommand("rev-parse", "--path-format=absolute", "--git-path", name)
		if err != nil {
			return "", nil, err
		}
		paths = append(paths, strings.TrimSpace(out))
	}
	return root, paths, nil
}

// IgnoredDirectories returns collapsed ignored paths so watchers can avoid
// descending into build outputs and dependency directories at startup.
func IgnoredDirectories(paths ...string) ([]string, error) {
	args := []string{"ls-files", "--full-name", "--others", "--ignored", "--exclude-standard", "--directory", "-z", "--"}
	out, err := gitCommand(append(args, paths...)...)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
}

func WatchFiles(paths []string) ([]string, error) {
	args := []string{"ls-files", "--full-name", "--cached", "--others", "--exclude-standard", "-z", "--"}
	out, err := gitCommand(append(args, paths...)...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
}

func IgnoredPath(path string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "--quiet", "--", path)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func AbsoluteWatchPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}

// ObjectIDs returns the selected immutable blob IDs in one Git invocation.
func (s ReviewSource) ObjectIDs(paths []string) (map[string]string, error) {
	refs := make([]string, len(paths))
	for i, path := range paths {
		refs[i] = ":" + path
		if s.Scope == "range" {
			refs[i] = s.Head + ":" + path
		}
	}
	ids := make(map[string]string, len(paths))
	if len(refs) == 0 {
		return ids, nil
	}
	out, err := gitCommand(append([]string{"rev-parse", "--revs-only", "--end-of-options"}, refs...)...)
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(out)
	if len(lines) != len(paths) {
		return nil, errors.New("unexpected blob ID count")
	}
	for i, path := range paths {
		ids[path] = lines[i]
	}
	return ids, nil
}
