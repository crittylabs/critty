package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestRestoreSavedDocumentAndDiffWithoutProcess(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.WriteFile("a.md", []byte("document\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	patch, err := git.ParsePatch(strings.NewReader(addedPatch))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []*reviewMode{{docPath: "a.md"}, {patch: patch, files: patch.Changes()}} {
		sess, err := openReviewSession(&config.Config{}, mode)
		if err != nil {
			t.Fatal(err)
		}
		sess.CJ.ReviewComments = []review.Comment{{ID: "r_keep", Body: "retain this"}}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
		if err := stopReview(sess.Key); err != nil {
			t.Fatal(err)
		}
		loaded, restored, err := loadReviewSession(sess.Key)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.CJ.ReviewComments[0].Body != "retain this" || restored.internalMode() != mode.internalMode() {
			t.Fatal("saved review changed during stop and resume")
		}
		if restored.patch != nil && restored.patch.Raw != patch.Raw {
			t.Fatal("diff snapshot was not restored")
		}
		other, err := openReviewSession(&config.Config{}, mode)
		if err != nil {
			t.Fatal(err)
		}
		if other.Key == sess.Key {
			t.Fatal("fresh task reused saved session")
		}
	}
}

func TestPlanNamesDoNotShareStorageAndRequireSessionWhenAmbiguous(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd, _ := os.Getwd()
	for _, content := range []string{"first", "second"} {
		sess, err := review.NewSession("", review.SessionEntry{CWD: cwd, Mode: "plan", PlanSlug: "same-name"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := review.SavePlanVersionAt(sess.Dir, []byte(content)); err != nil {
			t.Fatal(err)
		}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
		_, mode, err := loadReviewSession(sess.Key)
		if err != nil {
			t.Fatal(err)
		}
		if mode.docPath != filepath.Join(sess.Dir, "current.md") {
			t.Fatal("plan storage escaped session")
		}
		stored, err := os.ReadFile(mode.docPath)
		if err != nil || string(stored) != content {
			t.Fatalf("plan content = %q, %v", stored, err)
		}
	}
	if _, err := review.ResolvePlan("same-name"); err == nil {
		t.Fatal("ambiguous plan name was silently selected")
	}
}

func TestResumeRejectsDifferentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := openReviewSession(&config.Config{}, &reviewMode{docPath: "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if _, _, err := loadReviewSession(sess.Key); err == nil {
		t.Fatal("resumed against another working directory")
	}
}
