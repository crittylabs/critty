package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/git"
	"github.com/mattn/go-shellwords"
)

func TestParseFocus(t *testing.T) {
	for _, tt := range []struct {
		value, path string
		line        int
		invalid     bool
	}{
		{value: "a.go", path: "a.go"},
		{value: "dir/a.go:42", path: "dir/a.go", line: 42},
		{value: "dir/a:b.go:2", path: "dir/a:b.go", line: 2},
		{value: "a:b.go", path: "a:b.go"},
		{value: "日本語 file.go:3", path: "日本語 file.go", line: 3},
		{value: "", invalid: true},
		{value: ":2", invalid: true},
		{value: "a.go:0", invalid: true},
		{value: "a.go:-1", invalid: true},
		{value: "a.go:999999999999999999999999", invalid: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			path, line, err := parseFocus(tt.value)
			if (err != nil) != tt.invalid || err == nil && (path != tt.path || line != tt.line) {
				t.Fatalf("parseFocus(%q) = %q, %d, %v", tt.value, path, line, err)
			}
		})
	}
}

func TestResolveFocusPatchAndCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	originalFocus, originalExec := reviewFocus, resolveExec
	t.Cleanup(func() { reviewFocus, resolveExec = originalFocus, originalExec })
	resolveExec = func() (string, error) { return "/a path/critty", nil }
	patch, err := git.ParsePatch(strings.NewReader("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -10 +10 @@\n-old\n+new\n"))
	if err != nil {
		t.Fatal(err)
	}
	// Disk content must not supply lines missing from the diff.
	if err := os.WriteFile("a.txt", []byte(strings.Repeat("disk\n", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, focus := range []string{"a.txt", filepath.Join(t.TempDir(), "../missing.txt"), "./a.txt:10", "a.txt:1", "a.txt:30"} {
		reviewFocus = focus
		mode := &reviewMode{patch: patch, files: patch.Changes(), sessionKey: "test"}
		err := resolveFocus(mode)
		valid := focus == "a.txt" || focus == "./a.txt:10"
		if (err == nil) != valid {
			t.Fatalf("resolveFocus(%q): %v", focus, err)
		}
		if !valid {
			continue
		}
		command, err := buildTUICommand(mode, "tmux")
		if err != nil {
			t.Fatal(err)
		}
		args, err := shellwords.Parse(command)
		if err != nil || args[len(args)-2] != "--focus" {
			t.Fatalf("command = %q, error = %v", command, err)
		}
		path, line, err := parseFocus(args[len(args)-1])
		if err != nil || path != mode.focusPath || line != mode.focusLine {
			t.Fatalf("forwarded focus = %q, %d, %v", path, line, err)
		}
	}
}

func TestResolveFocusPlan(t *testing.T) {
	t.Chdir(t.TempDir())
	original := reviewFocus
	t.Cleanup(func() { reviewFocus = original })
	mode := &reviewMode{docPath: "saved/current.md", planFile: "plan.md", planContent: []byte("one\ntwo"), planSlug: "plan"}
	path, err := filepath.Abs("plan.md")
	if err != nil {
		t.Fatal(err)
	}
	reviewFocus = path + ":2"
	if err := resolveFocus(mode); err != nil {
		t.Fatal(err)
	}
	if mode.focusPath != mode.docPath || mode.focusLine != 2 {
		t.Fatalf("focus = %q:%d", mode.focusPath, mode.focusLine)
	}
}

func TestDocumentLineArgument(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("notes.md", []byte("one\ntwo\nthree"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalFocus := reviewFocus
	t.Cleanup(func() { reviewFocus = originalFocus })
	for _, arg := range []string{"notes.md:2", "notes.md:0", "notes.md:-1", "notes.md:4"} {
		mode, err := resolveReviewMode([]string{arg})
		if arg != "notes.md:2" {
			if err == nil {
				t.Fatalf("accepted invalid line: %s", arg)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if mode.docPath != "notes.md" || mode.focusPath != "notes.md" || mode.focusLine != 2 {
			t.Fatalf("document mode = %+v", mode)
		}
		reviewFocus = "notes.md:3"
		if err := resolveFocus(mode); err != nil || mode.focusLine != 3 {
			t.Fatalf("explicit focus did not override document line: %+v, %v", mode, err)
		}
	}
}
