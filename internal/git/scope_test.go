package git

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestResolveCommit(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file.txt", []byte("root\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "root")
	runGit(t, dir, "tag", "root")
	runGit(t, dir, "checkout", "-qb", "side")
	writeFile(t, dir, "side.txt", []byte("side\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "side")
	runGit(t, dir, "checkout", "-qb", "mainline", "root")
	writeFile(t, dir, "file.txt", []byte("next\n"))
	runGit(t, dir, "commit", "-qam", "next")
	runGit(t, dir, "tag", "next")
	runGit(t, dir, "merge", "--no-ff", "-qm", "merge", "side")
	sha, err := resolveCommit("next")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "file.txt", []byte("working\n"))
	writeFile(t, dir, "untracked.txt", []byte("untracked\n"))
	for _, tc := range []struct {
		ref, path, before, after string
		status                   ChangeStatus
	}{
		{"root", "file.txt", "", "root\n", StatusAdded},
		{"next", "file.txt", "root\n", "next\n", StatusModified},
		{sha, "file.txt", "root\n", "next\n", StatusModified},
		{sha[:10], "file.txt", "root\n", "next\n", StatusModified},
		{"HEAD~1", "file.txt", "root\n", "next\n", StatusModified},
		{"HEAD", "side.txt", "", "side\n", StatusAdded},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			s, err := ResolveCommit(tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			files, err := s.Files()
			if err != nil || len(files) != 1 || files[0].Path != tc.path || files[0].Status != tc.status {
				t.Fatalf("files=%+v err=%v", files, err)
			}
			content, err := s.Content(tc.path)
			if err != nil || string(content) != tc.after {
				t.Fatalf("content=%q err=%v", content, err)
			}
			preview, err := s.PreviewContent(tc.path, false, 1024)
			if err != nil || string(preview) != tc.after {
				t.Fatalf("preview=%q err=%v", preview, err)
			}
			if tc.before != "" {
				preview, err = s.PreviewContent(tc.path, true, 1024)
				if err != nil || string(preview) != tc.before {
					t.Fatalf("before=%q err=%v", preview, err)
				}
			}
			diff, err := s.Diff(tc.path, "")
			if err != nil || diff == nil || !diff.ChangedLines[1] {
				t.Fatalf("diff=%v err=%v", diff, err)
			}
			snapshot, patch, err := s.Snapshot()
			if err != nil || !reflect.DeepEqual(snapshot, s) || !strings.Contains(patch, "+"+strings.TrimSpace(tc.after)) {
				t.Fatalf("snapshot=%+v patch=%q err=%v", snapshot, patch, err)
			}
		})
	}
	for _, ref := range []string{"missing", "--output=bad", "HEAD..HEAD", "HEAD: file.txt", ""} {
		if _, err := ResolveCommit(ref); err == nil {
			t.Errorf("accepted invalid commit %q", ref)
		}
	}
}

func TestReviewSources(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "file 日本語.txt", []byte("base\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "tag", "base")
	writeFile(t, dir, "file 日本語.txt", []byte("committed\n"))
	runGit(t, dir, "commit", "-qam", "head")
	runGit(t, dir, "tag", "tip")
	writeFile(t, dir, "file 日本語.txt", []byte("staged\n"))
	runGit(t, dir, "add", ".")
	writeFile(t, dir, "file 日本語.txt", []byte("working\n"))
	writeFile(t, dir, "untracked.txt", []byte("new\n"))
	for _, tc := range []struct {
		scope, content string
		count          int
	}{
		{"all", "working\n", 2}, {"staged", "staged\n", 1}, {"unstaged", "working\n", 2},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			s := ReviewSource{Scope: tc.scope}
			files, err := s.Files()
			if err != nil || len(files) != tc.count {
				t.Fatalf("files=%v err=%v", files, err)
			}
			content, err := s.Content("file 日本語.txt")
			if err != nil || string(content) != tc.content {
				t.Fatalf("content=%q err=%v", content, err)
			}
			diff, err := s.Diff("file 日本語.txt", "")
			if err != nil || diff == nil || !diff.ChangedLines[1] {
				t.Fatalf("diff=%v err=%v", diff, err)
			}
			if tc.scope == "unstaged" && diff.DeletedAfter[0][0].Content != "staged" {
				t.Fatal("unstaged base is not the index")
			}
		})
	}
	for _, value := range []string{"base..tip", "base...tip", "base..", "base..."} {
		s, err := ResolveRange(value)
		if err != nil {
			t.Fatal(err)
		}
		content, err := s.Content("file 日本語.txt")
		if err != nil || string(content) != "committed\n" {
			t.Fatalf("range content=%q err=%v", content, err)
		}
		files, err := s.Files()
		if err != nil || len(files) != 1 {
			t.Fatalf("range files=%v err=%v", files, err)
		}
	}
	for _, value := range []string{"HEAD", "--output=bad", "missing..HEAD", "base..tip..HEAD", "base....tip"} {
		if _, err := ResolveRange(value); err == nil {
			t.Errorf("accepted invalid range %q", value)
		}
	}
	// A divergent left endpoint distinguishes two-dot and three-dot comparisons.
	runGit(t, dir, "reset", "--hard", "base")
	writeFile(t, dir, "left.txt", []byte("left\n"))
	runGit(t, dir, "add", "left.txt")
	runGit(t, dir, "commit", "-qm", "left")
	runGit(t, dir, "tag", "left")
	two, err := ResolveRange("left..tip")
	if err != nil {
		t.Fatal(err)
	}
	three, err := ResolveRange("left...tip")
	if err != nil {
		t.Fatal(err)
	}
	a, err := two.Files()
	if err != nil {
		t.Fatal(err)
	}
	b, err := three.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || len(b) != 1 || reflect.DeepEqual(a, b) {
		t.Fatalf("two-dot=%v three-dot=%v", a, b)
	}
	for _, source := range []ReviewSource{two, three} {
		out, err := gitCommand("diff", "--name-status", "-z", source.Range, "--")
		if err != nil {
			t.Fatal(err)
		}
		files, err := source.Files()
		if err != nil {
			t.Fatal(err)
		}
		if want := parseNameStatusZ(out); !reflect.DeepEqual(files, want) {
			t.Fatalf("%s: files=%v, git diff=%v", source.Range, files, want)
		}
	}
}

func TestBinaryChangeStatuses(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "deleted.bin", []byte("\x00deleted"))
	writeFile(t, dir, "modified.bin", []byte("\x00before"))
	writeFile(t, dir, "old.bin", []byte("\x00renamed"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "tag", "base")
	runGit(t, dir, "rm", "deleted.bin")
	runGit(t, dir, "mv", "old.bin", "renamed.bin")
	writeFile(t, dir, "added.bin", []byte("\x00added"))
	writeFile(t, dir, "modified.bin", []byte("\x00after"))
	runGit(t, dir, "add", ".")
	check := func(files []FileChange, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]ChangeStatus{"added.bin": StatusAdded, "deleted.bin": StatusDeleted, "modified.bin": StatusModified, "renamed.bin": StatusRenamed}
		if len(files) != len(want) {
			t.Fatalf("files = %+v", files)
		}
		for _, f := range files {
			status, ok := want[f.Path]
			// A pure rename has no numstat content changes to mark as binary.
			if !ok || f.Status != status || (status != StatusRenamed && !f.IsBinary()) {
				t.Errorf("file = %+v", f)
			}
			if f.IsBinary() && f.ReviewStatus() != "binary" {
				t.Errorf("binary review status = %q", f.ReviewStatus())
			}
		}
	}
	check((ReviewSource{Scope: "all"}).Files())
	check((ReviewSource{Scope: "staged"}).Files())
	runGit(t, dir, "commit", "-qm", "changes")
	s, err := ResolveRange("base..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	check(s.Files())
	writeFile(t, dir, "modified.bin", []byte("\x00unstaged"))
	if err := os.Remove("added.bin"); err != nil {
		t.Fatal(err)
	}
	files, err := (ReviewSource{Scope: "unstaged"}).Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Status != StatusDeleted || files[1].Status != StatusModified || !files[0].IsBinary() || !files[1].IsBinary() {
		t.Fatalf("unstaged files = %+v", files)
	}
}

func TestDiffFollowsRenames(t *testing.T) {
	dir := newRepo(t)
	runGit(t, dir, "config", "diff.renames", "false")
	writeFile(t, dir, "old name.txt", []byte("one\ntwo\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "mv", "old name.txt", "new name.txt")
	writeFile(t, dir, "new name.txt", []byte("one\ntwo\nthree\n"))
	runGit(t, dir, "add", ".")

	s := ReviewSource{Scope: "staged"}
	files, err := s.Files()
	if err != nil || len(files) != 1 || files[0].Status != StatusRenamed || files[0].OldPath != "old name.txt" || files[0].Path != "new name.txt" {
		t.Fatalf("files=%+v err=%v, want one rename", files, err)
	}
	for name, diff := range map[string]func() (*DiffInfo, error){
		"source": func() (*DiffInfo, error) { return s.Diff(files[0].Path, files[0].OldPath) },
		"staged": func() (*DiffInfo, error) { return DiffFileStaged(files[0].Path, files[0].OldPath) },
	} {
		info, err := diff()
		if err != nil || info == nil {
			t.Fatalf("%s: diff=%v err=%v", name, info, err)
		}
		if len(info.ChangedLines) != 1 || !info.ChangedLines[3] || len(info.DeletedAfter) != 0 {
			t.Fatalf("%s: changed=%v deleted=%v, want only line 3 added", name, info.ChangedLines, info.DeletedAfter)
		}
	}
	info, err := s.Diff(files[0].Path, "")
	if err != nil || info == nil || len(info.ChangedLines) != 3 {
		t.Fatalf("without the old path: changed=%v err=%v, want the whole file added", info, err)
	}
}
