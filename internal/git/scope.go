package git

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// ReviewSource describes both sides of a code review without treating refs as
// command options or reading working-tree contents for committed comparisons.
type ReviewSource struct {
	Scope string
	Range string
	Base  string
	Head  string
}

func ResolveRange(value string) (ReviewSource, error) {
	separator := ".."
	if strings.Contains(value, "...") {
		separator = "..."
	}
	left, right, ok := strings.Cut(value, separator)
	if !ok || strings.Contains(right, "..") {
		return ReviewSource{}, fmt.Errorf("range must be A..B or A...B")
	}
	if left == "" {
		left = "HEAD"
	}
	if right == "" {
		right = "HEAD"
	}
	resolve := func(ref string) (string, error) {
		out, err := gitCommand("rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		return strings.TrimSpace(out), err
	}
	base, err := resolve(left)
	if err != nil {
		return ReviewSource{}, err
	}
	head, err := resolve(right)
	if err != nil {
		return ReviewSource{}, err
	}
	if separator == "..." {
		out, err := gitCommand("merge-base", "--all", base, head)
		if err != nil {
			return ReviewSource{}, err
		}
		bases := strings.Fields(out)
		if len(bases) != 1 {
			return ReviewSource{}, fmt.Errorf("range requires a unique merge base")
		}
		base = bases[0]
	}
	return ReviewSource{Scope: "range", Range: value, Base: base, Head: head}, nil
}

func (s ReviewSource) diffArgs() []string {
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--find-renames"}
	switch s.Scope {
	case "staged":
		return append(args, "--cached")
	case "unstaged":
		return args
	case "range":
		return append(args, s.Base, s.Head)
	default:
		base := s.Base
		if base == "" {
			base = "HEAD"
		}
		return append(args, base)
	}
}

func (s ReviewSource) Files() ([]FileChange, error) {
	out, err := gitCommand(append(s.diffArgs(), "--name-status", "-z", "--")...)
	if err != nil {
		return nil, err
	}
	files := parseNameStatusZ(out)
	out, err = gitCommand(append(s.diffArgs(), "--numstat", "-z", "--")...)
	if err != nil {
		return nil, err
	}
	binary := parseNumstatZ(out)
	for i := range files {
		if binary[files[i].Path] {
			files[i].Binary = true
		}
	}
	if s.Scope == "all" || s.Scope == "unstaged" {
		paths, err := untrackedFiles()
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			files = append(files, FileChange{Path: path, Status: StatusUntracked})
		}
	}
	return files, nil
}

// Diff returns the diff of path.  oldPath is the file's previous path when it
// was renamed, else "".
func (s ReviewSource) Diff(path, oldPath string) (*DiffInfo, error) {
	out, err := gitCommand(append(s.diffArgs(), diffPathspec(path, oldPath)...)...)
	if err != nil {
		return nil, err
	}
	files, _, err := gitdiff.Parse(strings.NewReader(out))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	return diffInfo(files), nil
}

func (s ReviewSource) Content(path string) ([]byte, error) {
	switch s.Scope {
	case "staged":
		return FileContentFromIndex(path)
	case "range":
		out, err := gitCommand("show", s.Head+":"+path)
		return []byte(out), err
	default:
		return os.ReadFile(path)
	}
}

// PreviewContent bounds reads before loading binary data into memory.
func (s ReviewSource) PreviewContent(path string, before bool, limit int64) ([]byte, error) {
	if !before && s.Scope != "staged" && s.Scope != "range" {
		return readPreviewFile(path, limit)
	}
	ref := ":" + path
	if before && s.Scope != "unstaged" {
		base := s.Base
		if base == "" {
			base = "HEAD"
		}
		ref = base + ":" + path
	} else if !before && s.Scope == "range" {
		ref = s.Head + ":" + path
	}
	out, err := gitCommand("rev-parse", "--verify", "--end-of-options", ref)
	if err != nil {
		return nil, err
	}
	object := strings.TrimSpace(out)
	out, err = gitCommand("cat-file", "-s", object)
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return nil, err
	}
	if size > limit {
		return nil, fmt.Errorf("image exceeds the size limit")
	}
	out, err = gitCommand("cat-file", "blob", object)
	return []byte(out), err
}

func readPreviewFile(path string, limit int64) (data []byte, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("image is not a regular file within the size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("image is not a regular file within the size limit")
	}
	data, err = io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("image exceeds the size limit")
	}
	return data, err
}

// Snapshot records raw changes, including mode and binary changes, without
// applying the display's whitespace filtering.
func (s ReviewSource) Snapshot() (ReviewSource, string, error) {
	out, err := gitCommand(append(s.diffArgs(), "--binary", "--full-index", "--")...)
	if err != nil {
		return s, "", err
	}
	if s.Scope != "unstaged" && s.Scope != "range" {
		base := s.Base
		if base == "" {
			base = "HEAD"
		}
		sha, err := gitCommand("rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if err != nil {
			return s, "", err
		}
		s.Base = strings.TrimSpace(sha)
	}
	return s, out, nil
}
