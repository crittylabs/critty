package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// TextContents reads reviewable files together when their contents are Git
// blobs.  Working files and text conversions retain their individual reads.
func (s ReviewSource) TextContents(files []FileChange) (map[string][]byte, error) {
	contents := make(map[string][]byte, len(files))
	var paths []string
	for _, file := range files {
		if file.IsBinary() || file.Status == StatusDeleted {
			continue
		}
		if (s.Scope == "range" || s.Scope == "staged") && file.Textconv.Command == "" {
			paths = append(paths, file.Path)
			continue
		}
		content, err := s.TextContent(file.Path, file.Textconv)
		if err != nil {
			return nil, err
		}
		contents[file.Path] = content
	}
	if len(paths) == 0 {
		return contents, nil
	}
	objects, err := s.contentObjects()
	if err != nil {
		return nil, err
	}
	var ids, batchPaths []string
	for _, path := range paths {
		if id, ok := objects[path]; ok {
			ids = append(ids, id)
			batchPaths = append(batchPaths, path)
			continue
		}
		// Preserve individual errors and non-blob handling (for example gitlinks).
		content, err := s.Content(path)
		if err != nil {
			return nil, err
		}
		contents[path] = content
	}
	if len(ids) == 0 {
		return contents, nil
	}
	cmd := exec.Command("git", "cat-file", "--batch")
	// Object IDs keep paths containing line breaks out of the batch protocol.
	cmd.Stdin = strings.NewReader(strings.Join(ids, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading Git blobs: %w", err)
	}
	for i, id := range ids {
		header, rest, ok := bytes.Cut(out, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || fields[0] != id || fields[1] != "blob" {
			return nil, fmt.Errorf("invalid Git blob response for %s", batchPaths[i])
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= len(rest) || rest[size] != '\n' {
			return nil, fmt.Errorf("invalid Git blob size for %s", batchPaths[i])
		}
		contents[batchPaths[i]] = rest[:size:size]
		out = rest[size+1:]
	}
	if len(out) != 0 {
		return nil, fmt.Errorf("unexpected trailing Git blob data")
	}
	return contents, nil
}

func (s ReviewSource) contentObjects() (map[string]string, error) {
	args := []string{"ls-tree", "-r", "-z", "--full-tree", s.Head, "--"}
	if s.Scope == "staged" {
		args = []string{"ls-files", "--stage", "--full-name", "-z"}
	}
	out, err := gitCommand(args...)
	if err != nil {
		return nil, err
	}
	objects := make(map[string]string)
	for record := range strings.SplitSeq(strings.TrimSuffix(out, "\x00"), "\x00") {
		if record == "" {
			continue
		}
		metadata, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("invalid Git object listing")
		}
		if s.Scope == "staged" {
			if fields[2] == "0" && (fields[0] == "100644" || fields[0] == "100755" || fields[0] == "120000") {
				objects[path] = fields[1]
			}
		} else if fields[1] == "blob" {
			objects[path] = fields[2]
		}
	}
	return objects, nil
}
