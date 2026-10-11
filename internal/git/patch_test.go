package git

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestParsePatch(t *testing.T) {
	t.Chdir(t.TempDir())
	tests := []struct {
		name, raw, path, content string
		status                   ChangeStatus
		partial                  bool
	}{
		{"added", "diff --git a/a.txt b/a.txt\nnew file mode 100644\n--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1,2 @@\n+one\n+two\n", "a.txt", "one\ntwo\n", StatusAdded, false},
		{"no newline", "diff --git a/a.txt b/a.txt\nnew file mode 100644\n--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1 @@\n+one\n\\ No newline at end of file\n", "a.txt", "one", StatusAdded, false},
		{"partial", "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -10,2 +10,2 @@\n context\n-old\n+new\n", "a.txt", strings.Repeat("\n", 9) + "context\nnew", StatusModified, true},
		{"deleted", "diff --git a/a.txt b/a.txt\ndeleted file mode 100644\n--- a/a.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n", "a.txt", "", StatusDeleted, false},
		{"renamed", "diff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n", "new.txt", "", StatusRenamed, true},
		{"spaces", "diff --git a/my file.txt b/my file.txt\nnew file mode 100644\n--- /dev/null\n+++ b/my file.txt\n@@ -0,0 +1 @@\n+text\n", "my file.txt", "text\n", StatusAdded, false},
		{"binary", "diff --git a/a.png b/a.png\nindex 1234567..abcdef0 100644\nBinary files a/a.png and b/a.png differ\n", "a.png", "", StatusModified, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePatch(strings.NewReader(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if p.Raw != tt.raw || len(p.Files) != 1 {
				t.Fatalf("unexpected snapshot: %+v", p)
			}
			f := p.Files[0]
			if f.Change.IsBinary() != (tt.name == "binary") {
				t.Fatalf("binary flag = %v", f.Change.IsBinary())
			}
			if f.Change.Path != tt.path || f.Change.Status != tt.status || f.Content != tt.content || (f.Known != nil) != tt.partial {
				t.Fatalf("file = %+v", f)
			}
		})
	}
}

func TestParsePatchRejectsInvalidInput(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, raw := range []string{
		"not a diff\n", "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n-old\n+new\n",
		"diff --git a/../escape b/../escape\nnew file mode 100644\n",
		"diff --git a/a b/a\nnew file mode 100644\ndiff --git a/a b/a\nnew file mode 100644\n",
		"diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1000001 +1000001 @@\n-old\n+new\n",
	} {
		if _, err := ParsePatch(strings.NewReader(raw)); err == nil {
			t.Errorf("accepted invalid diff %q", raw)
		}
	}
}

func TestPatchReconstructsOldBlobWithoutWorktree(t *testing.T) {
	t.Chdir(t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader("before\nold\nafter\n")
	oid, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf("diff --git a/missing.txt b/missing.txt\nindex %s..abcdef0 100644\n--- a/missing.txt\n+++ b/missing.txt\n@@ -2 +2 @@\n-old\n+new\n", strings.TrimSpace(string(oid)))
	p, err := ParsePatch(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if f := p.Files[0]; f.Known != nil || f.Content != "before\nnew\nafter\n" || !f.Diff.ChangedLines[2] {
		t.Fatalf("wrong reconstructed file: %+v", f)
	}
}

func TestPatchZeroContextDeletion(t *testing.T) {
	t.Chdir(t.TempDir())
	p, err := ParsePatch(strings.NewReader("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -10,2 +9,0 @@\n-old\n-more\n"))
	if err != nil {
		t.Fatal(err)
	}
	f := p.Files[0]
	if len(f.Diff.DeletedAfter[9]) != 2 || f.Diff.DeletedAfter[9][0].OldLineNum != 10 {
		t.Fatalf("wrong deletion anchor: %+v", f.Diff.DeletedAfter)
	}
	if len(strings.Split(f.Content, "\n")) != 9 || len(f.Known) != 0 {
		t.Fatalf("fabricated context: %+v", f)
	}
}
