package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/aymanbagabas/go-udiff/lcs"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// ChangeStatus represents the type of change to a file.
type ChangeStatus int

const (
	StatusModified ChangeStatus = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
	StatusUntracked
	StatusBinary
)

// String returns the review.json status string for a change status.
func (s ChangeStatus) String() string {
	switch s {
	case StatusAdded:
		return "added"
	case StatusDeleted:
		return "deleted"
	case StatusRenamed:
		return "renamed"
	case StatusUntracked:
		return "untracked"
	case StatusBinary:
		return "binary"
	default:
		return "modified"
	}
}

// FileChange describes a single changed file.
type FileChange struct {
	Textconv Textconv
	Path     string
	Status   ChangeStatus
	OldPath  string // for renames
	Binary   bool
}

// IsBinary also recognizes snapshots saved before binary detection was
// separated from the change status.
func (f FileChange) IsBinary() bool {
	return f.Binary || f.Status == StatusBinary
}

// ReviewStatus preserves the Crit-compatible status for binary files.
func (f FileChange) ReviewStatus() string {
	if f.IsBinary() {
		return StatusBinary.String()
	}
	return f.Status.String()
}

// DiffInfo contains parsed diff information for a single file.
type DiffInfo struct {
	// PairedLines maps added source lines to similar deleted lines.  Zero marks
	// an unpaired addition; missing entries retain legacy positional alignment.
	PairedLines   map[int]int             `json:",omitempty"`
	CommonLines   map[int]int             `json:",omitempty"` // new-to-old coordinates supplied by the patch
	ChangedLines  map[int]bool            // added/modified line numbers (1-based) in new file
	InlineChanges map[int][]InlineSegment // inline changes keyed by new-file line number
	DeletedAfter  map[int][]DeletedLine   // deleted lines keyed by the new-file line they appear after (0 = before line 1)
}

// DeletedLine represents a line that was deleted from the old version.
type DeletedLine struct {
	OldLineNum int
	Content    string
	Inline     []InlineSegment
}

// InlineSegment is a changed or unchanged portion of a replacement line.
type InlineSegment struct {
	Content string
	Changed bool
}

// DiffFile returns full diff information for a file relative to the given
// ref.  oldPath is the file's previous path when it was renamed, else "".
func DiffFile(path, oldPath, ref string) (*DiffInfo, error) {
	return diffFile(path, oldPath, ref, false)
}

// DiffFileStaged returns diff information for the indexed version of a file.
// oldPath is the file's previous path when it was renamed, else "".
func DiffFileStaged(path, oldPath string) (*DiffInfo, error) {
	return diffFile(path, oldPath, "HEAD", true)
}

func diffFile(path, oldPath, ref string, staged bool) (*DiffInfo, error) {
	args := []string{"diff", "--find-renames"}
	if staged {
		args = append(args, "--cached")
	} else {
		args = append(args, ref)
	}
	args = append(args, diffPathspec(path, oldPath)...)
	out, err := gitCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("git diff for %s: %w", path, err)
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	files, _, err := gitdiff.Parse(strings.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("parsing diff for %s: %w", path, err)
	}

	return diffInfo(files), nil
}

// diffPathspec limits a diff to path.  A renamed file's previous path is
// included so Git pairs both sides instead of reporting a new file whose
// every line was added.
func diffPathspec(path, oldPath string) []string {
	if oldPath != "" && oldPath != path {
		return []string{"--", oldPath, path}
	}
	return []string{"--", path}
}

func diffInfo(files []*gitdiff.File) *DiffInfo {
	info := &DiffInfo{
		CommonLines:   make(map[int]int),
		ChangedLines:  make(map[int]bool),
		InlineChanges: make(map[int][]InlineSegment),
		DeletedAfter:  make(map[int][]DeletedLine),
	}

	for _, f := range files {
		for _, frag := range f.TextFragments {
			newLine := frag.NewPosition
			oldLine := frag.OldPosition
			// Track the last new-file line we've seen (for anchoring deletions).
			// A hunk with no new-side lines (a deleted file) starts at
			// position 0, which still anchors before line 1.
			lastNewLine := max(int(newLine)-1, 0)
			if frag.NewLines == 0 {
				lastNewLine = int(newLine)
			}

			for _, line := range frag.Lines {
				switch line.Op {
				case gitdiff.OpAdd:
					info.ChangedLines[int(newLine)] = true
					lastNewLine = int(newLine)
					newLine++
				case gitdiff.OpContext:
					info.CommonLines[int(newLine)] = int(oldLine)
					lastNewLine = int(newLine)
					newLine++
					oldLine++
				case gitdiff.OpDelete:
					content := strings.TrimSuffix(line.Line, "\n")
					info.DeletedAfter[lastNewLine] = append(info.DeletedAfter[lastNewLine], DeletedLine{
						OldLineNum: int(oldLine),
						Content:    content,
					})
					oldLine++
				}
			}
		}
	}
	deletedByLine := make(map[int]*DeletedLine)
	for _, lines := range info.DeletedAfter {
		for i := range lines {
			deletedByLine[lines[i].OldLineNum] = &lines[i]
		}
	}
	for _, f := range files {
		for _, frag := range f.TextFragments {
			annotateInlineChanges(info, frag, deletedByLine)
		}
	}

	return info
}

func annotateInlineChanges(info *DiffInfo, frag *gitdiff.TextFragment, deletedByLine map[int]*DeletedLine) {
	oldLine := int(frag.OldPosition)
	newLine := int(frag.NewPosition)
	for i := 0; i < len(frag.Lines); {
		if frag.Lines[i].Op != gitdiff.OpDelete {
			switch frag.Lines[i].Op {
			case gitdiff.OpAdd:
				newLine++
			case gitdiff.OpContext:
				oldLine++
				newLine++
			}
			i++
			continue
		}

		var deleted []*DeletedLine
		for i < len(frag.Lines) && frag.Lines[i].Op == gitdiff.OpDelete {
			deleted = append(deleted, deletedByLine[oldLine])
			oldLine++
			i++
		}

		var added []replacementLine
		for i < len(frag.Lines) && frag.Lines[i].Op == gitdiff.OpAdd {
			added = append(added, replacementLine{newLine, strings.TrimSuffix(frag.Lines[i].Line, "\n")})
			newLine++
			i++
		}

		info.annotateReplacement(deleted, added, false)
	}
}

func inlineDiffTokens(beforeTokens, afterTokens []string) ([]InlineSegment, []InlineSegment) {
	diffs := lcs.DiffLines(beforeTokens, afterTokens)
	if len(diffs) == 0 {
		return nil, nil
	}

	oldSegments := make([]InlineSegment, 0, len(diffs)*2+1)
	newSegments := make([]InlineSegment, 0, len(diffs)*2+1)
	lastEnd := 0
	for i := 0; i < len(diffs); i++ {
		diff := diffs[i]
		// Keep a changed phrase together, including its internal whitespace.
		for i+1 < len(diffs) && strings.TrimSpace(strings.Join(beforeTokens[diff.End:diffs[i+1].Start], "")) == "" {
			i++
			diff.End = diffs[i].End
			diff.ReplEnd = diffs[i].ReplEnd
		}
		// Merging can expose matching whitespace at a phrase boundary.
		for diff.Start < diff.End && diff.ReplStart < diff.ReplEnd &&
			beforeTokens[diff.Start] == afterTokens[diff.ReplStart] && strings.TrimSpace(beforeTokens[diff.Start]) == "" {
			diff.Start++
			diff.ReplStart++
		}
		for diff.Start < diff.End && diff.ReplStart < diff.ReplEnd &&
			beforeTokens[diff.End-1] == afterTokens[diff.ReplEnd-1] && strings.TrimSpace(beforeTokens[diff.End-1]) == "" {
			diff.End--
			diff.ReplEnd--
		}
		common := strings.Join(beforeTokens[lastEnd:diff.Start], "")
		oldSegments = appendInlineSegment(oldSegments, common, false)
		newSegments = appendInlineSegment(newSegments, common, false)
		oldSegments = appendInlineSegment(oldSegments, strings.Join(beforeTokens[diff.Start:diff.End], ""), true)
		newSegments = appendInlineSegment(newSegments, strings.Join(afterTokens[diff.ReplStart:diff.ReplEnd], ""), true)
		lastEnd = diff.End
	}
	common := strings.Join(beforeTokens[lastEnd:], "")
	oldSegments = appendInlineSegment(oldSegments, common, false)
	newSegments = appendInlineSegment(newSegments, common, false)
	return oldSegments, newSegments
}

// splitInlineTokens separates words and whitespace runs so shared whitespace
// can match independently while preserving the original line exactly.
func splitInlineTokens(text string) []string {
	if text == "" {
		return nil
	}

	var tokens []string
	start := 0
	previousWasSpace := false
	for i, r := range text {
		isSpace := unicode.IsSpace(r)
		if i > 0 && isSpace != previousWasSpace {
			tokens = append(tokens, text[start:i])
			start = i
		}
		previousWasSpace = isSpace
	}
	return append(tokens, text[start:])
}

func appendInlineSegment(segments []InlineSegment, content string, changed bool) []InlineSegment {
	if content == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].Changed == changed {
		segments[len(segments)-1].Content += content
		return segments
	}
	return append(segments, InlineSegment{Content: content, Changed: changed})
}

// parseNameStatusZ parses NUL-separated `git diff --name-status -z` output:
// each entry is a status token followed by one path, or two paths for
// renames and copies.
func parseNameStatusZ(out string) []FileChange {
	tokens := strings.Split(out, "\x00")

	var files []FileChange
	for i := 0; i+1 < len(tokens); {
		status := tokens[i]
		if status == "" {
			i++
			continue
		}

		fc := FileChange{Path: tokens[i+1]}
		i += 2
		switch {
		case status == "A":
			fc.Status = StatusAdded
		case status == "D":
			fc.Status = StatusDeleted
		case strings.HasPrefix(status, "R"), strings.HasPrefix(status, "C"):
			fc.Status = StatusRenamed
			if i < len(tokens) {
				fc.OldPath = fc.Path
				fc.Path = tokens[i]
				i++
			}
		default:
			fc.Status = StatusModified
		}

		files = append(files, fc)
	}

	return files
}

// FileContentFromIndex returns a file's staged content.
func FileContentFromIndex(path string) ([]byte, error) {
	out, err := gitCommand("show", ":"+path)
	if err != nil {
		return nil, fmt.Errorf("reading staged file %s: %w", path, err)
	}
	return []byte(out), nil
}

// parseNumstatZ parses NUL-separated `git diff --numstat -z` output.
// Each entry is "added\tdeleted\tpath"; binary files show "-" for both
// counts. For renames and copies the path field is empty and the old and
// new paths follow as two separate NUL-terminated tokens.
func parseNumstatZ(out string) map[string]bool {
	binaries := make(map[string]bool)
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		parts := strings.SplitN(tokens[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}

		binary := parts[0] == "-" && parts[1] == "-"
		path := parts[2]
		if path == "" && i+2 < len(tokens) {
			path = tokens[i+2]
			i += 2
		}
		if binary && path != "" {
			binaries[path] = true
		}
	}

	return binaries
}

// untrackedFiles returns untracked file paths.
func untrackedFiles() ([]string, error) {
	out, err := gitCommand("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// gitCommand runs a git command and returns stdout.
func gitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	// Reading diffs must not refresh the index and wake our own watcher.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// IsGitRepo checks if the current directory is inside a git repository.
func IsGitRepo() bool {
	_, err := gitCommand("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// CurrentBranch returns the current branch name, or "HEAD" when detached.
func CurrentBranch() (string, error) {
	if out, err := gitCommand("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		return strings.TrimSpace(out), nil
	}
	out, err := gitCommand("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
