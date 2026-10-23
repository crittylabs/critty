package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/review"
)

func TestResumeProjectWorkspaces(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	root, _ = os.Getwd()
	origin, next := filepath.Join(t.TempDir(), "origin"), filepath.Join(t.TempDir(), "next")
	runGit := func(directory string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = directory
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(root, "init", "-qb", "main")
	runGit(root, "config", "user.name", "Test")
	runGit(root, "config", "user.email", "test@example.com")
	write("a.md", "Base content\n")
	write("form.md", "- [ ] Note <!-- form: id=note -->\n- [ ] Publish <!-- form: id=publish consent -->\n")
	runGit(root, "add", ".")
	runGit(root, "commit", "-qm", "base")
	runGit(root, "worktree", "add", "-b", "topic", origin)
	runGit(root, "worktree", "add", "-b", "next", next)
	if err := os.Chdir(origin); err != nil {
		t.Fatal(err)
	}
	origin, _ = os.Getwd()
	write("a.md", "Committed topic content\n")
	runGit(origin, "add", "a.md")
	runGit(origin, "commit", "-qm", "topic")
	head := runGit(origin, "rev-parse", "HEAD")
	write("missing.md", "Saved missing document\n")
	write("a.md", "Original staged content\n")
	runGit(origin, "add", "a.md")
	patch, err := git.ParsePatch(strings.NewReader(addedPatch))
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolveSource("staged")
	if err != nil {
		t.Fatal(err)
	}
	files, err := source.Files()
	if err != nil {
		t.Fatal(err)
	}
	rangeSource, err := resolveSource("main..topic")
	if err != nil {
		t.Fatal(err)
	}
	rangeFiles, err := rangeSource.Files()
	if err != nil {
		t.Fatal(err)
	}
	commitSource, err := resolveSource("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	commitFiles, err := commitSource.Files()
	if err != nil {
		t.Fatal(err)
	}
	store := history.New(origin)
	policy, err := (history.Policy{Start: "keep", End: "keep"}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.Start("Portable review", policy)
	if err != nil {
		t.Fatal(err)
	}
	sessions := map[string]*review.Session{}
	for name, mode := range map[string]*reviewMode{
		"files": {docPath: "a.md"}, "missing": {docPath: "missing.md"},
		"diff":   {patch: patch, files: patch.Changes()},
		"git":    {source: source, files: files, ref: source.Base, staged: true},
		"range":  {source: rangeSource, files: rangeFiles, ref: rangeSource.Base},
		"commit": {source: commitSource, files: commitFiles, ref: commitSource.Base},
		"plan":   {docPath: "plan.md", planSlug: "portable", planFile: "plan.md"},
	} {
		sess, err := openReviewSession(&config.Config{}, mode)
		if err != nil {
			t.Fatal(err)
		}
		if name == "plan" {
			sess.Meta.PlanSlug = mode.planSlug
			if _, err := review.SavePlanVersionAt(sess.Dir, []byte("Saved plan content\n")); err != nil {
				t.Fatal(err)
			}
		}
		docs, err := review.ReadAttachedDocuments([]string{"form.md"})
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.ReplaceDocuments(docs); err != nil {
			t.Fatal(err)
		}
		sess.CJ.RoundState.Files = map[string]review.SnapshotFile{"missing.md": {Content: "Saved missing document\n"}}
		sess.CJ.ReviewComments = []review.Comment{{ID: "r_keep", Body: "Keep feedback", Replies: []review.Reply{{ID: "rp_keep", Body: "Keep reply"}}}}
		if name == "git" || name == "range" || name == "commit" {
			sess.SetFileComments("a.md", "modified", []review.Comment{{ID: "c_old", Side: "old", StartLine: 1, Anchor: "Old base content", Body: "Keep old context"}})
		}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
		forms, err := sess.BeginForms(review.ReviewInput{Scope: "document"}, docs)
		if err != nil {
			t.Fatal(err)
		}
		forms.Documents[0].Answers["note"] = form.Answer{Value: true, Human: true, Revision: forms.Revision}
		forms.Documents[0].Answers["publish"] = form.Answer{Value: true, Human: true, Revision: forms.Revision}
		if err := sess.SaveForms(forms); err != nil {
			t.Fatal(err)
		}
		if err := store.Add(task.Task.ID, sess.Key); err != nil {
			t.Fatal(err)
		}
		sessions[name] = sess
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	write("a.md", "Current workspace content\n")
	runGit(root, "add", "a.md")
	resume := func(name, target string) {
		t.Helper()
		sess := sessions[name]
		if err := withSessionDirectory(sess.Key, nil, func([]string) error {
			cwd, err := os.Getwd()
			if err != nil || !sameSessionDirectory(cwd, target) {
				t.Fatalf("resume ignored caller workspace: %s, %v", cwd, err)
			}
			loaded, mode, err := loadReviewSession(sess.Key)
			if err != nil {
				return err
			}
			if loaded.Meta.CWD != origin || !sameSessionDirectory(loaded.Meta.WorkingDirectory(), target) || loaded.CJ.ReviewComments[0].Replies[0].Body != "Keep reply" {
				t.Fatalf("lost ownership or feedback: %+v", loaded.Meta)
			}
			if name == "git" {
				content, err := mode.source.Content("a.md")
				if err != nil || string(content) != "Current workspace content\n" || loaded.Meta.Branch != runGit(target, "branch", "--show-current") {
					t.Fatalf("read another checkout's index: %q, %v", content, err)
				}
			}
			if name == "range" && mode.source.Head != head {
				t.Fatal("committed comparison followed another checkout's HEAD")
			}
			if name == "commit" && mode.source.Head != runGit(target, "rev-parse", "HEAD") {
				t.Fatal("HEAD comparison did not refresh in the selected workspace")
			}
			if name == "missing" && (mode.documentSnapshot == nil || mode.documentSnapshot.Content != "Saved missing document\n") {
				t.Fatal("missing document lost its saved snapshot")
			}
			if name == "plan" {
				content, err := os.ReadFile(mode.docPath)
				if err != nil || string(content) != "Saved plan content\n" {
					t.Fatalf("plan snapshot lost: %q, %v", content, err)
				}
			}
			if mode.documents[0].Source != filepath.Join(target, "form.md") {
				t.Fatalf("attachment source did not move: %+v", mode.documents[0])
			}
			if err := saveReviewMode(loaded, mode); err != nil {
				return err
			}
			if (name == "git" || name == "commit") && !loaded.CJ.Files["a.md"].Comments[0].Drifted {
				t.Fatal("old-side anchor was trusted against a different workspace base")
			}
			if name == "range" && loaded.CJ.Files["a.md"].Comments[0].Drifted {
				t.Fatal("unchanged committed comparison invalidated an old-side anchor")
			}
			forms, err := loaded.BeginForms(review.ReviewInput{Scope: "document"}, mode.documents)
			if err != nil || forms.Documents[0].Answers["note"].Value != true || !forms.Documents[0].Answers["note"].Human || forms.Documents[0].Answers["publish"].Human {
				t.Fatalf("answers lost or consent transferred across workspaces: %+v, %v", forms, err)
			}
			member, err := history.New(origin).ForSession(sess.Key)
			if err != nil || member == nil || member.ID != task.Task.ID {
				t.Fatalf("task membership lost: %+v, %v", member, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	resume("files", root)
	resume("diff", root)
	runGit(root, "worktree", "remove", "--force", origin)
	runGit(root, "branch", "-D", "topic")
	for _, name := range []string{"files", "missing", "plan", "diff", "git", "range", "commit"} {
		t.Run(name+"/removed-origin", func(t *testing.T) { resume(name, root) })
	}
	runGit(root, "branch", "-m", "renamed-main")
	resume("git", root)
	if err := os.Chdir(next); err != nil {
		t.Fatal(err)
	}
	next, _ = os.Getwd()
	write("a.md", "Current workspace content\n")
	runGit(next, "add", "a.md")
	for _, name := range []string{"files", "missing", "plan", "diff", "git", "range", "commit"} {
		t.Run(name+"/another-worktree", func(t *testing.T) { resume(name, next) })
	}
	foreign := t.TempDir()
	runGit(foreign, "init", "-q")
	if err := os.Chdir(foreign); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadReviewSession(sessions["git"].Key); err == nil {
		t.Fatal("accepted a different Git project")
	}
}
