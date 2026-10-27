package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestResolveExplicitScopes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"commit", "--allow-empty", "-qm", "base"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
	}
	for _, value := range []string{"committed", "staged", "working"} {
		if err := os.WriteFile("file.txt", []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
		if value != "working" {
			if out, err := exec.Command("git", "add", "file.txt").CombinedOutput(); err != nil {
				t.Fatalf("%s: %v", out, err)
			}
		}
		if value == "committed" {
			if out, err := exec.Command("git", "commit", "-qm", "next").CombinedOutput(); err != nil {
				t.Fatalf("%s: %v", out, err)
			}
		}
	}
	oldScope, oldStaged, oldUnstaged, oldCode := reviewScope, reviewStaged, reviewUnstaged, reviewCode
	t.Cleanup(func() {
		reviewScope, reviewStaged, reviewUnstaged, reviewCode = oldScope, oldStaged, oldUnstaged, oldCode
	})
	for _, tc := range []struct {
		scope, want            string
		staged, unstaged, code bool
	}{
		{want: "all"}, {scope: "all", want: "all"},
		{scope: "staged", want: "staged"}, {scope: "unstaged", want: "unstaged"},
		{staged: true, want: "staged"}, {staged: true, code: true, want: "staged"}, {unstaged: true, want: "unstaged"},
		{scope: "staged", staged: true, want: "staged"},
		{scope: "unstaged", unstaged: true, want: "unstaged"},
	} {
		reviewScope, reviewStaged, reviewUnstaged, reviewCode = tc.scope, tc.staged, tc.unstaged, tc.code
		scope := tc.want
		mode, err := resolveReviewMode(nil)
		if err != nil || mode.source == nil || mode.source.Scope != scope || len(mode.files) != 1 {
			t.Fatalf("%s: mode=%+v err=%v", scope, mode, err)
		}
		if tc.want == "staged" {
			content, err := mode.source.Content("file.txt")
			if err != nil || string(content) != "staged" || !mode.staged || mode.ref != "HEAD" {
				t.Fatalf("staged review did not use the index: %+v, content=%q, error=%v", mode, content, err)
			}
		}
		mode.sessionKey = "0123456789ab"
		cmd, err := buildTUICommand(mode, "tmux")
		if err != nil || !strings.Contains(cmd, "--session '0123456789ab'") {
			t.Fatalf("command=%q err=%v", cmd, err)
		}
	}
	reviewStaged, reviewUnstaged, reviewCode = false, false, false
	for _, scope := range []string{"HEAD~1..", "HEAD"} {
		reviewScope = scope
		mode, err := resolveReviewMode(nil)
		if err != nil || mode.source.Scope != "range" || len(mode.files) != 1 {
			t.Fatalf("%s: mode=%+v err=%v", scope, mode, err)
		}
		if got := mode.persistedCLIArgs(); len(got) != 2 || got[1] != scope {
			t.Fatalf("persisted args=%v", got)
		}
		sess, err := openReviewSession(&config.Config{Output: t.TempDir()}, mode)
		if err != nil {
			t.Fatal(err)
		}
		sess.CJ.ReviewComments = []review.Comment{{ID: "r_commit", Body: "keep this comment"}}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
		loaded, restored, err := loadReviewSession(sess.Key)
		if err != nil {
			t.Fatal(err)
		}
		if restored.source.Range != scope || restored.source.Base != mode.source.Base || restored.source.Head != mode.source.Head || len(restored.files) != 1 || len(loaded.CJ.ReviewComments) != 1 || loaded.CJ.ReviewComments[0].Body != "keep this comment" {
			t.Fatalf("restored mode=%+v comments=%v", restored, loaded.CJ.ReviewComments)
		}
	}
	reviewScope = "HEAD..HEAD"
	if _, err := resolveReviewMode(nil); err == nil {
		t.Fatal("empty range accepted")
	}
	for _, tc := range []struct {
		scope    string
		staged   bool
		unstaged bool
		args     []string
	}{
		{scope: "bad"},
		{scope: "all", staged: true}, {scope: "HEAD..HEAD", staged: true},
		{scope: "staged", args: []string{"README.md"}},
		{staged: true, unstaged: true},
		{scope: "all", unstaged: true}, {scope: "staged", unstaged: true},
		{scope: "HEAD~1..", unstaged: true},
		{unstaged: true, args: []string{"README.md"}},
	} {
		reviewScope, reviewStaged, reviewUnstaged = tc.scope, tc.staged, tc.unstaged
		if _, err := resolveReviewMode(tc.args); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	reviewScope, reviewStaged, reviewUnstaged = "all", false, false
	if out, err := exec.Command("git", "add", "file.txt").CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	if out, err := exec.Command("git", "commit", "-qm", "clean").CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	for _, scope := range []string{"", "all"} {
		reviewScope = scope
		if _, err := resolveReviewMode(nil); err == nil {
			t.Fatal("clean worktree fell back to committed changes")
		}
	}
	oldAttachments := attachmentPaths
	t.Cleanup(func() { attachmentPaths = oldAttachments })
	attachmentPaths = []string{"notice.md"}
	mode, err := resolveReviewMode(nil)
	if err != nil || len(mode.files) != 0 || mode.source == nil {
		t.Fatalf("clean scope with attachment: %+v, %v", mode, err)
	}
}

func TestReviewSourceSessionIsolation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := &config.Config{Output: t.TempDir()}
	modes := []reviewMode{
		{ref: "HEAD", source: &git.ReviewSource{Scope: "all"}},
		{ref: "HEAD", staged: true, source: &git.ReviewSource{Scope: "staged"}},
		{ref: "HEAD", source: &git.ReviewSource{Scope: "unstaged"}},
		{ref: "HEAD~1", source: &git.ReviewSource{Scope: "range", Range: "HEAD~1..HEAD"}},
	}
	keys := map[string]bool{}
	stagedKey := ""
	for i := range modes {
		sess, err := openReviewSession(cfg, &modes[i])
		if err != nil {
			t.Fatal(err)
		}
		if keys[sess.Key] {
			t.Fatal("different scopes share a session")
		}
		keys[sess.Key] = true
		if modes[i].staged {
			stagedKey = sess.Key
		}
		sess.SetFileComments("file.txt", "modified", []review.Comment{{ID: "c_isolated", Body: "separate"}})
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
	}
	alias, err := openReviewSession(cfg, &reviewMode{ref: "HEAD", staged: true})
	if err != nil {
		t.Fatal(err)
	}
	if alias.Key == stagedKey || len(alias.FileComments("file.txt")) != 0 {
		t.Fatal("fresh review reused the previous staged task")
	}

}
