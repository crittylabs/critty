package git

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTextContentsPreserveBytesAndRevision(t *testing.T) {
	dir := newRepo(t)
	data := map[string][]byte{
		"empty.txt":              nil,
		"plain.txt":              []byte("no final newline"),
		"tab\tquote\"line\n.txt": []byte("line\nblob 4\n\x00raw\n"),
		"large.txt":              []byte(strings.Repeat("blob 123\n", 20000)),
	}
	var files []FileChange
	for path, content := range data {
		writeFile(t, dir, path, content)
		files = append(files, FileChange{Path: path})
	}
	if err := os.Symlink("plain.txt", "link.txt"); err != nil {
		t.Fatal(err)
	}
	files = append(files, FileChange{Path: "link.txt"})
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "snapshot")
	writeFile(t, dir, "plain.txt", []byte("staged\n"))
	runGit(t, dir, "add", "plain.txt")
	writeFile(t, dir, "plain.txt", []byte("working\n"))
	files = append(files, FileChange{Path: "deleted.txt", Status: StatusDeleted}, FileChange{Path: "binary.bin", Binary: true})
	for _, scope := range []string{"range", "staged", "all", "unstaged"} {
		t.Run(scope, func(t *testing.T) {
			s := ReviewSource{Scope: scope, Head: "HEAD"}
			contents, err := s.TextContents(files)
			if err != nil {
				t.Fatal(err)
			}
			if len(contents) != len(files)-2 {
				t.Fatalf("loaded %d files, want %d", len(contents), len(files)-2)
			}
			for _, file := range files[:len(files)-2] {
				want, err := s.Content(file.Path)
				if err != nil || !bytes.Equal(contents[file.Path], want) {
					t.Fatalf("%q: content differs from individual read, err=%v", file.Path, err)
				}
			}
			if _, err := s.TextContents([]FileChange{{Path: "missing.txt"}}); err == nil {
				t.Fatal("missing content accepted")
			}
			if subset, err := s.TextContents(files[:1]); err != nil || len(subset) != 1 {
				t.Fatalf("subset size=%d, err=%v", len(subset), err)
			}
		})
	}
	if contents, err := (ReviewSource{Scope: "range", Head: "missing"}).TextContents(nil); err != nil || len(contents) != 0 {
		t.Fatalf("empty request read Git: %v", err)
	}
}

func TestTextContentsPreserveNonBlobAndUnmergedBehavior(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file.txt", []byte("content\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "snapshot")
	head, err := resolveCommit("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",module")
	s := ReviewSource{Scope: "staged"}
	contents, err := s.TextContents([]FileChange{{Path: "module"}})
	want, singleErr := s.Content("module")
	if err != nil || singleErr != nil || !bytes.Equal(contents["module"], want) {
		t.Fatalf("gitlink differs from individual read: batch=%v, single=%v", err, singleErr)
	}
	blob, err := gitCommand("rev-parse", "HEAD:file.txt")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "update-index", "--index-info")
	cmd.Stdin = strings.NewReader("100644 " + strings.TrimSpace(blob) + " 1\tconflict.txt\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create unmerged entry: %v, %s", err, out)
	}
	if _, err := s.TextContents([]FileChange{{Path: "conflict.txt"}}); err == nil {
		t.Fatal("unmerged file read without a stage-zero entry")
	}
}
