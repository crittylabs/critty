package cli

import (
	"os"
	"testing"

	"github.com/crittylabs/critty/internal/review"
)

func TestRunClearAll(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := runClearAll(); err != nil {
		t.Fatalf("runClearAll without saved state: %v", err)
	}
	if err := os.WriteFile("plan.md", []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess, err := review.OpenDocSession("", "plan.md")
	if err != nil {
		t.Fatal(err)
	}
	sess.SetFileComments("plan.md", "", []review.Comment{
		{ID: "c_abc123", StartLine: 1, EndLine: 1, Body: "x"},
	})
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}

	// A session registered for another directory must survive clear --all.
	otherDir := t.TempDir()
	other, err := review.OpenSession("", review.SessionKey(otherDir, "main", nil))
	if err != nil {
		t.Fatal(err)
	}
	other.Meta = review.SessionEntry{Key: other.Key, CWD: otherDir, Branch: "main"}
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	if err := runClearAll(); err != nil {
		t.Fatalf("runClearAll: %v", err)
	}

	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Errorf("expected %s removed", sess.Dir)
	}
	if _, err := os.Stat(other.Dir); err != nil {
		t.Errorf("expected other directory's session preserved: %v", err)
	}
}
