package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseNameStatusZ(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []FileChange
	}{
		{
			name: "empty",
			out:  "",
			want: nil,
		},
		{
			name: "modified with spaces",
			out:  "M\x00docs/user guide.md\x00",
			want: []FileChange{
				{Path: "docs/user guide.md", Status: StatusModified},
			},
		},
		{
			name: "added and deleted",
			out:  "A\x00new file.txt\x00D\x00old file.txt\x00",
			want: []FileChange{
				{Path: "new file.txt", Status: StatusAdded},
				{Path: "old file.txt", Status: StatusDeleted},
			},
		},
		{
			name: "rename with spaces",
			out:  "R100\x00old name.txt\x00new name.txt\x00M\x00other.txt\x00",
			want: []FileChange{
				{Path: "new name.txt", OldPath: "old name.txt", Status: StatusRenamed},
				{Path: "other.txt", Status: StatusModified},
			},
		},
		{
			name: "copy consumes both paths",
			out:  "C75\x00src file.txt\x00copy file.txt\x00A\x00added.txt\x00",
			want: []FileChange{
				{Path: "copy file.txt", OldPath: "src file.txt", Status: StatusRenamed},
				{Path: "added.txt", Status: StatusAdded},
			},
		},
		{
			name: "non-ASCII path",
			out:  "M\x00\u30c6\u30b9\u30c8 \u30e1\u30e2.txt\x00",
			want: []FileChange{
				{Path: "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", Status: StatusModified},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNameStatusZ(tt.out)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseNumstatZ(t *testing.T) {
	out := "3\t1\ttext file.txt\x00" +
		"-\t-\tbin file.dat\x00" +
		"-\t-\t\x00old bin.dat\x00new bin.dat\x00" +
		"2\t2\t\x00old name.txt\x00new name.txt\x00"

	got := parseNumstatZ(out)
	want := map[string]bool{
		"bin file.dat": true,
		"new bin.dat":  true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for path := range want {
		if !got[path] {
			t.Errorf("missing binary path %q in %v", path, got)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates a repository with isolated Git configuration and makes it
// the working directory.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Chdir(dir)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func TestFilesWithSpecialPaths(t *testing.T) {
	dir := newRepo(t)

	writeFile(t, dir, "docs/user guide.md", []byte("# Guide\n"))
	writeFile(t, dir, "old name.txt", []byte("hello\nworld\n"))
	writeFile(t, dir, "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", []byte("hello\n"))
	writeFile(t, dir, "bin file.dat", []byte{0x00, 0x01, 0x02})
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "docs/user guide.md", []byte("# User Guide\n"))
	writeFile(t, dir, "\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", []byte("goodbye\n"))
	writeFile(t, dir, "bin file.dat", []byte{0x03, 0x04, 0x05, 0x00})
	runGit(t, dir, "mv", "old name.txt", "new name.txt")
	writeFile(t, dir, "untracked file.txt", []byte("new\n"))

	files, err := (ReviewSource{Scope: "all"}).Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	byPath := make(map[string]FileChange, len(files))
	for _, fc := range files {
		byPath[fc.Path] = fc
	}

	assertStatus := func(path string, status ChangeStatus) {
		t.Helper()
		fc, ok := byPath[path]
		if !ok {
			t.Errorf("missing %q in %+v", path, files)
			return
		}
		if fc.Status != status {
			t.Errorf("%q: got status %v, want %v", path, fc.Status, status)
		}
	}

	assertStatus("docs/user guide.md", StatusModified)
	assertStatus("\u30c6\u30b9\u30c8 \u30e1\u30e2.txt", StatusModified)
	assertStatus("bin file.dat", StatusModified)
	if !byPath["bin file.dat"].IsBinary() {
		t.Error("binary file lost its binary flag")
	}
	assertStatus("untracked file.txt", StatusUntracked)

	fc, ok := byPath["new name.txt"]
	if !ok {
		t.Fatalf("missing rename target in %+v", files)
	}
	if fc.Status != StatusRenamed || fc.OldPath != "old name.txt" {
		t.Errorf("rename: got %+v, want Status=StatusRenamed OldPath=%q", fc, "old name.txt")
	}
}

func TestStagedFilesBeforeInitialCommit(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "first.txt", []byte("first\n"))
	runGit(t, dir, "add", "first.txt")

	files, err := (ReviewSource{Scope: "staged"}).Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "first.txt" {
		t.Fatalf("staged files = %+v, want only first.txt", files)
	}
	diff, err := DiffFileStaged("first.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || !diff.ChangedLines[1] {
		t.Fatalf("staged diff = %+v, want changed line 1", diff)
	}
}

func TestInlineDiff(t *testing.T) {
	before := "prefix.  `tfil` maintains a screen model of the output and enables mouse reporting.  Hovering continues."
	after := "prefix.  When Codex starts, `tfil` begins maintaining a screen model of the output and enables mouse reporting.  Non-interactive commands stay quiet.  Hovering continues."
	oldSegments, newSegments := inlineDiffTokens(splitInlineTokens(before), splitInlineTokens(after))

	assertSegments := func(name, want string, segments []InlineSegment) string {
		t.Helper()
		var got strings.Builder
		var changedText strings.Builder
		changed := false
		unchanged := false
		for _, segment := range segments {
			got.WriteString(segment.Content)
			if segment.Changed {
				changed = true
				changedText.WriteString(segment.Content)
			} else {
				unchanged = true
			}
		}
		if got.String() != want {
			t.Errorf("%s reconstructed as %q, want %q", name, got.String(), want)
		}
		if !changed || !unchanged {
			t.Errorf("%s segments did not separate changed and common text: %+v", name, segments)
		}
		return changedText.String()
	}

	oldChanged := assertSegments("old", before, oldSegments)
	newChanged := assertSegments("new", after, newSegments)
	if oldChanged != "maintains" {
		t.Errorf("old changed text = %q, want %q", oldChanged, "maintains")
	}
	if strings.Contains(newChanged, "screen model") || strings.Contains(newChanged, "Hovering") {
		t.Errorf("new changed text contains common words: %q", newChanged)
	}
	for _, want := range []string{"When Codex starts", "begins maintaining", "Non-interactive commands"} {
		if !strings.Contains(newChanged, want) {
			t.Errorf("new changed text %q does not contain %q", newChanged, want)
		}
	}
}

func TestInlineDiffWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		old, new            []InlineSegment
	}{
		{
			name:   "changed phrase",
			before: "foo bar quux", after: "foo baz quuz",
			old: []InlineSegment{{"foo ", false}, {"bar quux", true}},
			new: []InlineSegment{{"foo ", false}, {"baz quuz", true}},
		},
		{
			name:   "shared space after a replacement",
			before: "foo bar quux", after: "foo baz quux",
			old: []InlineSegment{{"foo ", false}, {"bar", true}, {" quux", false}},
			new: []InlineSegment{{"foo ", false}, {"baz", true}, {" quux", false}},
		},
		{
			name:   "phrases separated by a shared word",
			before: "a old words middle old phrase z", after: "a new text middle new wording z",
			old: []InlineSegment{{"a ", false}, {"old words", true}, {" middle ", false}, {"old phrase", true}, {" z", false}},
			new: []InlineSegment{{"a ", false}, {"new text", true}, {" middle ", false}, {"new wording", true}, {" z", false}},
		},
		{
			name:   "different word counts around shared words",
			before: "// splitInlineTokens preserves each whitespace run with the word before it, so",
			after:  "// splitInlineTokens separates words and whitespace runs so shared whitespace",
			old:    []InlineSegment{{"// splitInlineTokens ", false}, {"preserves each", true}, {" whitespace ", false}, {"run with the word before it, so", true}},
			new:    []InlineSegment{{"// splitInlineTokens ", false}, {"separates words and", true}, {" whitespace ", false}, {"runs so shared whitespace", true}},
		},
		{
			name:   "shared unicode whitespace",
			before: "前	古い\u3000末尾  ", after: "前	新しい\u3000末尾  ",
			old: []InlineSegment{{"前	", false}, {"古い", true}, {"\u3000末尾  ", false}},
			new: []InlineSegment{{"前	", false}, {"新しい", true}, {"\u3000末尾  ", false}},
		},
		{
			name:   "changed whitespace",
			before: "foo bar", after: "foo	bar",
			old: []InlineSegment{{"foo", false}, {" ", true}, {"bar", false}},
			new: []InlineSegment{{"foo", false}, {"	", true}, {"bar", false}},
		},
		{
			name:   "inserted word and space",
			before: "foo bar", after: "foo baz bar",
			old: []InlineSegment{{"foo bar", false}},
			new: []InlineSegment{{"foo", false}, {" baz", true}, {" bar", false}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, new := inlineDiffTokens(splitInlineTokens(tc.before), splitInlineTokens(tc.after))
			if !reflect.DeepEqual(old, tc.old) || !reflect.DeepEqual(new, tc.new) {
				t.Fatalf("inlineDiff = %#v, %#v; want %#v, %#v", old, new, tc.old, tc.new)
			}
		})
	}
}

func TestDiffFileAnnotatesReplacementLines(t *testing.T) {
	dir := newRepo(t)

	writeFile(t, dir, "README.md", []byte("prefix old suffix\n"))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	writeFile(t, dir, "README.md", []byte("prefix new suffix\n"))

	info, err := DiffFile("README.md", "", "HEAD")
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if len(info.InlineChanges[1]) == 0 {
		t.Fatal("expected inline changes for added replacement line")
	}
	deleted := info.DeletedAfter[0]
	if len(deleted) != 1 || len(deleted[0].Inline) == 0 {
		t.Fatalf("expected inline changes for deleted replacement line, got %+v", deleted)
	}
}

func TestDiffFileDeletedFileAnchorsBeforeFirstLine(t *testing.T) {
	dir := newRepo(t)

	writeFile(t, dir, "gone.txt", []byte("one\ntwo\nthree\n"))
	runGit(t, dir, "add", "gone.txt")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	runGit(t, dir, "rm", "-q", "gone.txt")

	info, err := DiffFile("gone.txt", "", "HEAD")
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if len(info.DeletedAfter) != 1 {
		t.Fatalf("DeletedAfter keys = %v, want only 0", info.DeletedAfter)
	}
	deleted := info.DeletedAfter[0]
	if len(deleted) != 3 {
		t.Fatalf("DeletedAfter[0] = %+v, want the three deleted lines", deleted)
	}
	for i, want := range []string{"one", "two", "three"} {
		if deleted[i].OldLineNum != i+1 || deleted[i].Content != want {
			t.Errorf("deleted[%d] = %+v, want line %d %q", i, deleted[i], i+1, want)
		}
	}
}
