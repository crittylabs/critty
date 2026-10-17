package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Run this test binary as an editor so help probing is tested without depending
// on an installed editor or starting a GUI.
func TestSourceEditorHelper(t *testing.T) {
	mode := os.Getenv("CRITTY_TEST_EDITOR")
	if mode == "" {
		return
	}
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "--help" {
		os.Exit(2)
	}
	switch mode {
	case "code":
		_, _ = fmt.Fprintln(os.Stdout, "  -g --goto <file:line[:character]>  Open a file at a line")
	case "stderr":
		_, _ = fmt.Fprintln(os.Stderr, "  -g, --goto <file:line>  Open file")
		os.Exit(1)
	case "nvi":
		_, _ = fmt.Fprintln(os.Stderr, "nvi: invalid option -- '-'\nusage: vi [-eFlRrSv] [-c command] [file ...]")
		os.Exit(1)
	case "slow":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

func TestSourceEditorArgumentsAndProbeCache(t *testing.T) {
	for _, mode := range []string{"code", "stderr", "nvi", "plain", "slow"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("CRITTY_TEST_EDITOR", mode)
			if mode == "slow" {
				timeout := editorProbeTimeout
				editorProbeTimeout = 100 * time.Millisecond
				t.Cleanup(func() { editorProbeTimeout = timeout })
			}
			editor := fmt.Sprintf("%q -test.run=TestSourceEditorHelper -- --profile 'Review Profile'", os.Args[0])
			editorGotoCache.Delete(editor)
			t.Cleanup(func() { editorGotoCache.Delete(editor) })
			t.Setenv("EDITOR", editor)
			path := filepath.Join(t.TempDir(), "space $(literal) file.go")
			cmd, err := sourceEditorCommand(path, 40)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{os.Args[0], "-test.run=TestSourceEditorHelper", "--", "--profile", "Review Profile"}
			if mode == "code" || mode == "stderr" {
				want = append(want, "--goto", path+":40")
			} else {
				want = append(want, "+40", path)
			}
			if !reflect.DeepEqual(cmd.Args, want) {
				t.Fatalf("args = %#v, want %#v", cmd.Args, want)
			}
			if _, cached := editorGotoCache.Load(editor); !cached {
				t.Fatal("probe was not cached")
			}
			// A second launch must reuse the result even if the helper changes.
			t.Setenv("CRITTY_TEST_EDITOR", "plain")
			again, err := sourceEditorCommand(path, 40)
			if err != nil || !reflect.DeepEqual(again.Args, want) {
				t.Fatal("probe cache was not reused")
			}
		})
	}
}

func TestSourceEditorWithoutLineSkipsProbe(t *testing.T) {
	t.Setenv("EDITOR", "nonexistent-editor --wait")
	editorGotoCache.Delete("nonexistent-editor --wait")
	cmd, err := sourceEditorCommand("file.go", 0)
	if err != nil || strings.Join(cmd.Args, " ") != "nonexistent-editor --wait file.go" {
		t.Fatalf("command = %v, %v", cmd, err)
	}
	if _, ok := editorGotoCache.Load("nonexistent-editor --wait"); ok {
		t.Fatal("unnecessary help probe")
	}
}

func TestHelpHasGotoRequiresAnOption(t *testing.T) {
	for _, help := range []string{"use --goto to navigate", "--goto-other thing", "app version 1.0"} {
		if helpHasGoto(help) {
			t.Fatalf("not an option: %q", help)
		}
	}
	for _, help := range []string{"--goto FILE", " -g, --goto FILE", "  -g --goto FILE"} {
		if !helpHasGoto(help) {
			t.Fatalf("missing option: %q", help)
		}
	}
}

func TestExternalEditorCommandParsesArguments(t *testing.T) {
	t.Setenv("EDITOR", `code --wait --profile "Review Profile"`)
	cmd, err := externalEditorCommand("comment.md")
	if err != nil {
		t.Fatalf("externalEditorCommand: %v", err)
	}
	want := []string{"code", "--wait", "--profile", "Review Profile", "comment.md"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("editor args = %#v, want %#v", cmd.Args, want)
	}

	t.Setenv("EDITOR", `vim "unterminated`)
	if _, err := externalEditorCommand("comment.md"); err == nil {
		t.Fatal("externalEditorCommand accepted malformed $EDITOR")
	}
}

func TestFinishExternalEditReplacesCommentAndRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(path, []byte("edited externally\n"), 0600); err != nil {
		t.Fatalf("writing editor file: %v", err)
	}
	app := NewApp("test.go", AppConfig{})
	app.modal = commentModal
	app.modalTextarea.SetValue("original")

	updated, _ := app.Update(editorFinishedMsg{path: path})
	app = updated.(AppModel)

	if got := app.modalTextarea.Value(); got != "edited externally\n" {
		t.Fatalf("edited comment = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("editor file was not removed: %v", err)
	}
}
