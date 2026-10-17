package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

const addedPatch = "diff --git a/a.txt b/a.txt\nnew file mode 100644\n--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1 @@\n+snapshot\n"

func TestAttachmentOnlyReview(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.WriteFile("notice.md", []byte("Review this notice.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("empty.diff", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	oldDiff, oldNull, oldAttachments, oldClear, oldStdin := reviewDiff, reviewNull, attachmentPaths, clearAttachments, os.Stdin
	t.Cleanup(func() {
		reviewDiff, reviewNull, attachmentPaths, clearAttachments, os.Stdin = oldDiff, oldNull, oldAttachments, oldClear, oldStdin
	})
	for _, input := range []string{"null", os.DevNull, "empty.diff", "-"} {
		t.Run(input, func(t *testing.T) {
			reviewNull, reviewDiff = input == "null", input
			if reviewNull {
				reviewDiff = ""
			}
			if input == "-" {
				f, err := os.Open("empty.diff")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := f.Close(); err != nil {
						t.Error(err)
					}
				})
				os.Stdin = f
			}
			attachmentPaths, clearAttachments = nil, false
			mode, err := resolveReviewMode(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := mode.validateTarget(); err == nil {
				t.Fatal("accepted empty review without attachments")
			}
			attachmentPaths = []string{"notice.md"}
			if err := prepareDocuments(mode); err != nil {
				t.Fatal(err)
			}
			if err := mode.validateTarget(); err != nil {
				t.Fatal(err)
			}
			sess, err := openReviewSession(&config.Config{}, mode)
			if err != nil {
				t.Fatal(err)
			}
			attachmentPaths = nil
			_, resumed, err := loadReviewSession(sess.Key)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.patch == nil || len(resumed.files) != 0 || len(resumed.documents) != 1 {
				t.Fatalf("resumed review = %+v", resumed)
			}
			if err := resumed.validateTarget(); err != nil {
				t.Fatal(err)
			}
			resumed.documents[0].Removed = true
			if err := resumed.validateTarget(); err == nil {
				t.Fatal("accepted removed attachments as review targets")
			}
			resumed.documents[0].Removed = false
			if got := nextRoundCommand(sess, resumed); got != "critty --session "+sess.Key {
				t.Fatalf("continuation = %q", got)
			}
			clearAttachments = true
			if err := prepareDocuments(resumed); err != nil {
				t.Fatal(err)
			}
			if err := resumed.validateTarget(); err == nil {
				t.Fatal("accepted removal of all review targets")
			}
		})
	}
}

func TestExclusiveInputsRejectConflicts(t *testing.T) {
	oldNull, oldDiff, oldScope, oldCode, oldStaged, oldUnstaged := reviewNull, reviewDiff, reviewScope, reviewCode, reviewStaged, reviewUnstaged
	t.Cleanup(func() {
		reviewNull, reviewDiff, reviewScope, reviewCode, reviewStaged, reviewUnstaged = oldNull, oldDiff, oldScope, oldCode, oldStaged, oldUnstaged
	})
	// TestDiffInputArguments covers file arguments with --diff.
	for _, primary := range []string{"null", "diff"} {
		for _, input := range []string{"diff", "scope", "code", "staged", "unstaged", "file"} {
			if primary == "diff" && (input == "diff" || input == "file") {
				continue
			}
			t.Run(primary+"/"+input, func(t *testing.T) {
				reviewNull, reviewDiff = primary == "null", ""
				if primary == "diff" {
					reviewDiff = "-"
				}
				reviewScope, reviewCode, reviewStaged, reviewUnstaged = "", false, false, false
				var args []string
				switch input {
				case "diff":
					reviewDiff = os.DevNull
				case "scope":
					reviewScope = "all"
				case "code":
					reviewCode = true
				case "staged":
					reviewStaged = true
				case "unstaged":
					reviewUnstaged = true
				case "file":
					args = []string{"notice.md"}
				}
				if _, err := resolveReviewMode(args); err == nil {
					t.Fatal("accepted conflicting input")
				}
			})
		}
	}
}

func TestDiffReviewSessionAndDetachedCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	p, err := git.ParsePatch(strings.NewReader(addedPatch))
	if err != nil {
		t.Fatal(err)
	}
	mode := &reviewMode{patch: p, files: p.Changes()}
	sess, err := openReviewSession(&config.Config{}, mode)
	if err != nil {
		t.Fatal(err)
	}
	code, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Key == code.Key || mode.sessionKey != sess.Key || mode.internalMode() != "diff" || len(sess.CJ.CliArgs) != 1 || sess.CJ.CliArgs[0] != "--diff" {
		t.Fatalf("incorrect diff session: %+v", sess)
	}
	entry, err := review.ReadSessionEntry(sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := review.OpenSessionFromEntry(*entry)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := git.LoadPatch(reopened.DiffPath())
	if err != nil || loaded.Files[0].Content != "snapshot\n" {
		t.Fatalf("snapshot = %+v, error = %v", loaded, err)
	}
	original := resolveExec
	resolveExec = func() (string, error) { return "/a path/critty", nil }
	t.Cleanup(func() { resolveExec = original })
	cmd, err := buildTUICommand(mode, "tmux")
	if err != nil || !strings.Contains(cmd, "'/a path/critty' _tui --session '"+sess.Key+"'") {
		t.Fatalf("detached command = %q, %v", cmd, err)
	}
	if got := nextRoundCommand(sess, mode); !strings.Contains(got, "--diff") {
		t.Fatalf("next round does not request updated diff: %q", got)
	}
}

func TestDiffInputArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	path := "changes 日本語.diff"
	if err := os.WriteFile(path, []byte(addedPatch), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, diff := os.Stdin, reviewDiff
	t.Cleanup(func() { os.Stdin, reviewDiff = stdin, diff })
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		args []string
		fail bool
	}{
		{[]string{"--diff"}, false},
		{[]string{"--diff=-"}, false},
		{[]string{"--diff", "-"}, false},
		{[]string{"--diff=" + path}, false},
		{[]string{"--diff", "./" + path}, false},
		{[]string{"--diff=" + abs}, false},
		{[]string{"--diff="}, true},
		{[]string{"--diff=missing.diff"}, true},
		{[]string{"--diff=" + path, path}, true},
		{[]string{"--diff", path, path}, true},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := f.Close(); err != nil {
					t.Error(err)
				}
			})
			os.Stdin = f
			cmd := &cobra.Command{
				Args: cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					mode, err := resolveReviewMode(args)
					if err == nil && (mode.patch == nil || len(mode.files) != 1) {
						t.Fatalf("incorrect diff mode: %+v", mode)
					}
					return err
				},
			}
			addDiffFlag(cmd)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); (err != nil) != tt.fail {
				t.Fatalf("error = %v, want failure %v", err, tt.fail)
			}
		})
	}
}
