package review

import (
	"fmt"
	"os"
	"sort"

	"github.com/crittylabs/critty/internal/git"
)

// OpenCodeSession opens the git-mode review session for the current working
// directory and branch.
func OpenCodeSession(dataRoot string) (*Session, error) {
	return OpenCodeSessionWithArgs(dataRoot, []string{"--scope", "all"})
}

// OpenCodeSessionWithArgs isolates comments by the selected review source.
func OpenCodeSessionWithArgs(dataRoot string, args []string) (*Session, error) {
	branch, err := git.CurrentBranch()
	if err != nil {
		branch = ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving cwd: %w", err)
	}
	key := SessionKey(cwd, branch, args)
	s, err := OpenSession(dataRoot, key)
	if err != nil {
		return nil, err
	}
	s.CJ.Branch = branch
	s.Meta = SessionEntry{Key: key, CWD: cwd, Branch: branch, Args: args}
	return s, nil
}

// OpenDocSession opens the files-mode review session for a single document.
func OpenDocSession(dataRoot, docPath string) (*Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving cwd: %w", err)
	}
	args := []string{NormalizePath(docPath)}
	key := SessionKey(cwd, "", args)
	s, err := OpenSession(dataRoot, key)
	if err != nil {
		return nil, err
	}
	s.Meta = SessionEntry{Key: key, CWD: cwd, Args: args}
	return s, nil
}

// SortedFiles returns the session's file paths in stable sorted order.
func (s *Session) SortedFiles() []string {
	paths := make([]string, 0, len(s.CJ.Files))
	for p := range s.CJ.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// CodeFileStatus represents a single file's review status in aggregate output.
type CodeFileStatus struct {
	File     string    `json:"file"`
	Comments []Comment `json:"comments"`
}

// CodeReviewStatus is the aggregate status for all files in a code review.
type CodeReviewStatus struct {
	Files         []CodeFileStatus `json:"files"`
	TotalComments int              `json:"total_comments"`
}

// AggregateStatus summarizes the current code review session.
func AggregateStatus(dataRoot string) (*CodeReviewStatus, error) {
	s, err := ResolveCode(dataRoot)
	if err != nil {
		return nil, err
	}
	if len(s.CJ.Files) == 0 {
		return nil, fmt.Errorf("no active code review session (run `critty review --code` first)")
	}

	result := &CodeReviewStatus{}
	for _, path := range s.SortedFiles() {
		comments := s.CJ.Files[path].Comments
		if comments == nil {
			comments = []Comment{}
		}
		result.Files = append(result.Files, CodeFileStatus{
			File:     path,
			Comments: comments,
		})
		result.TotalComments += len(comments)
	}
	return result, nil
}
