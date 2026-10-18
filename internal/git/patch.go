package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// Patch is a self-contained snapshot: reopening it never reads worktree files.
type Patch struct {
	Raw   string
	Files []PatchFile
}

type PatchFile struct {
	Change  FileChange
	Content string
	Diff    *DiffInfo
	// Known is nil for complete content, otherwise it identifies the lines
	// supplied by the patch.  Empty placeholders must never become anchors.
	Known map[int]bool
}

// ParsePatch accepts Git-style unified diffs, including git show preambles.
func ParsePatch(r io.Reader) (*Patch, error) {
	const maxBytes = 64 << 20
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading diff: %w", err)
	}
	if len(raw) > maxBytes {
		return nil, fmt.Errorf("diff exceeds 64 MiB")
	}
	if len(raw) == 0 {
		return &Patch{}, nil
	}
	files, _, err := gitdiff.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing diff: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no file changes found in diff")
	}
	p := &Patch{Raw: string(raw)}
	seen := make(map[string]bool)
	for _, f := range files {
		path := f.NewName
		if f.IsDelete {
			path = f.OldName
		}
		if !filepath.IsLocal(path) || strings.ContainsRune(path, '\x00') {
			return nil, fmt.Errorf("invalid diff path %q", path)
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if seen[path] {
			return nil, fmt.Errorf("duplicate diff path %q; supply a single combined diff", path)
		}
		seen[path] = true
		pf := PatchFile{Change: FileChange{Path: path, Binary: f.IsBinary}, Diff: diffInfo([]*gitdiff.File{f})}
		switch {
		case f.IsDelete:
			pf.Change.Status = StatusDeleted
		case f.IsNew:
			pf.Change.Status = StatusAdded
		case f.IsRename || f.IsCopy:
			pf.Change.Status = StatusRenamed
			pf.Change.OldPath = f.OldName
		}
		if !f.IsBinary && !f.IsDelete {
			pf.Content, pf.Known, err = patchContent(f)
			if err != nil {
				return nil, fmt.Errorf("reading diff for %s: %w", path, err)
			}
		}
		p.Files = append(p.Files, pf)
	}
	return p, nil
}

func patchContent(f *gitdiff.File) (string, map[int]bool, error) {
	// Apply to the old blob rather than using the current file, which may
	// belong to an entirely different revision.  This also handles worktree
	// diffs whose new blob has never been written to the object database.
	if f.IsNew {
		var out bytes.Buffer
		if err := gitdiff.Apply(&out, bytes.NewReader(nil), f); err != nil {
			return "", nil, err
		}
		return out.String(), nil, nil
	}
	if validObjectPrefix(f.OldOIDPrefix) {
		if old, err := gitCommand("cat-file", "blob", f.OldOIDPrefix); err == nil {
			var out bytes.Buffer
			if err := gitdiff.Apply(&out, strings.NewReader(old), f); err == nil {
				return out.String(), nil, nil
			}
		}
	}

	// A unified diff need not contain enough context to reconstruct a file.
	// Keep its original coordinates while explicitly tracking unknown lines.
	known := make(map[int]bool)
	var lines []string
	lastEnd := int64(0)
	for _, frag := range f.TextFragments {
		end := frag.NewPosition + frag.NewLines
		if frag.NewPosition < lastEnd || end > 1_000_000 {
			return "", nil, fmt.Errorf("overlapping hunks or line number exceeds 1000000")
		}
		lastEnd = end
		pos := int(frag.NewPosition)
		for _, line := range frag.Lines {
			if !line.New() {
				continue
			}
			for len(lines) < pos {
				lines = append(lines, "")
			}
			lines[pos-1] = strings.TrimSuffix(line.Line, "\n")
			known[pos] = true
			pos++
		}
		// A zero-context deletion still needs its new-side anchor represented.
		for len(lines) < int(frag.NewPosition) {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n"), known, nil
}

func validObjectPrefix(s string) bool {
	if len(s) < 4 || len(s) > 64 || strings.Trim(s, "0") == "" {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}

func (p *Patch) Changes() []FileChange {
	files := make([]FileChange, 0, len(p.Files))
	for _, f := range p.Files {
		files = append(files, f.Change)
	}
	return files
}

func (p *Patch) File(path string) *PatchFile {
	for i := range p.Files {
		if p.Files[i].Change.Path == path {
			return &p.Files[i]
		}
	}
	return nil
}

func LoadPatch(path string) (*Patch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading diff snapshot: %w", err)
	}
	var p Patch
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing diff snapshot: %w", err)
	}
	if len(p.Files) == 0 && p.Raw != "" {
		return nil, fmt.Errorf("empty diff snapshot")
	}
	// Recover display coordinates in older snapshots from the saved patch,
	// without reading files or applying the patch again.
	missing := false
	for _, file := range p.Files {
		missing = missing || file.Diff != nil && file.Diff.CommonLines == nil
	}
	if missing && p.Raw != "" {
		files, _, err := gitdiff.Parse(strings.NewReader(p.Raw))
		if err != nil {
			return nil, fmt.Errorf("parsing saved diff: %w", err)
		}
		for _, file := range files {
			path := file.NewName
			if file.IsDelete {
				path = file.OldName
			}
			if saved := p.File(filepath.ToSlash(filepath.Clean(path))); saved != nil && saved.Diff != nil && saved.Diff.CommonLines == nil {
				saved.Diff.CommonLines = diffInfo([]*gitdiff.File{file}).CommonLines
			}
		}
	}
	return &p, nil
}
