package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func patchApp(t *testing.T, raw string) AppModel {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	p, err := gitpkg.ParsePatch(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenDiffSession("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.SaveDiff(p); err != nil {
		t.Fatal(err)
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	m := NewCodeReviewApp(p.Changes(), "", AppConfig{Session: sess, Patch: p})
	m.width, m.height = 120, 40
	updated, _ := m.Update(docRenderedMsg{})
	return updated.(AppModel)
}

func TestBinaryChangeLabels(t *testing.T) {
	for _, tc := range []struct {
		status, marker, header, before, after string
	}{
		{"added", "A", "new file mode 100644\n", "/dev/null", "b/image.png"},
		{"deleted", "D", "deleted file mode 100644\n", "a/image.png", "/dev/null"},
		{"modified", "M", "", "a/image.png", "b/image.png"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			m := patchApp(t, "diff --git a/image.png b/image.png\n"+tc.header+"Binary files "+tc.before+" and "+tc.after+" differ\n")
			m.syncFileTree()
			label := "(" + tc.marker + ")"
			for name, view := range map[string]string{
				"content": m.contentViewport.GetContent(),
				"tree":    m.fileTree.viewport.GetContent(),
			} {
				if !strings.Contains(ansi.Strip(view), label) {
					t.Errorf("%s missing %s: %s", name, label, ansi.Strip(view))
				}
			}
		})
	}
}

func TestTextChangeSummaryKeepsLineCounts(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n")
	if got := ansi.Strip(m.tab().changeSummary()); got != "(+1 -1)" {
		t.Fatalf("text summary = %q", got)
	}
	for _, tc := range []struct{ status, want string }{
		{"added", "(A +1)"}, {"untracked", "(A +1)"}, {"deleted", "(D -1)"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			tab := m.tabs[0]
			tab.status = tc.status
			if tc.status == "deleted" {
				tab.changedLines = nil
			} else {
				tab.deletedAfter = nil
			}
			if got := ansi.Strip(tab.changeSummary()); got != tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
			tab.changedLines, tab.deletedAfter = nil, nil
			if got := ansi.Strip(tab.changeSummary()); got != tc.want[:2]+")" {
				t.Fatalf("empty file summary = %q", got)
			}
		})
	}
}

func TestPatchPartialContext(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -10,2 +10,2 @@\n context\n-old\n+new\n@@ -20 +20 @@\n-last\n+final\n")
	if err := os.WriteFile("a.txt", []byte("unrelated worktree"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m.tab().cursorLine != 10 || m.reviewScopeLabel() != "Supplied diff" {
		t.Fatalf("initial cursor = %d, scope = %s", m.tab().cursorLine, m.reviewScopeLabel())
	}
	for _, ref := range m.visualLines(m.tab()) {
		if ref.side == "" && ref.line != 10 && ref.line != 11 && ref.line != 20 {
			t.Errorf("unknown line selectable: %+v", ref)
		}
	}
	m.rebuildContent()
	view := ansi.Strip(m.contentViewport.GetContent())
	if !strings.Contains(view, "context not included") || !strings.Contains(view, "final") || strings.Contains(view, "unrelated") {
		t.Fatalf("incorrect partial rendering: %s", view)
	}
	m.modal = commentModal
	m.tab().cursorLine = 20
	m.tab().selecting, m.tab().selectAnchor = true, 11
	if m.canSuggest() || m.anchorText(m.tab(), "", 11, 20) != "" {
		t.Fatal("suggestion or anchor spans unknown context")
	}
}

func TestPatchZeroContextDeletionNavigation(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -10,2 +9,0 @@\n-old\n-more\n")
	lines := m.visualLines(m.tab())
	if len(lines) != 2 || lines[0].side != "old" || lines[0].line != 10 {
		t.Fatalf("selectable lines = %+v", lines)
	}
	m.selectChange(0, m.tab().changeChunks[0])
	if m.tab().cursorLine != 10 || m.tab().cursorSide != "old" {
		t.Fatalf("change jump = %d, %s", m.tab().cursorLine, m.tab().cursorSide)
	}
}

func TestChangeNavigationVisitsDeletedLineComments(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -10,2 +9,0 @@\n-old\n-more\n")
	m.tab().state.Comments = []review.Comment{
		{ID: "old-first", Side: "old", StartLine: 10, EndLine: 10},
		{ID: "old-second", Side: "old", StartLine: 11, EndLine: 11},
	}
	m.selectChange(0, m.tab().changeChunks[0])
	for _, id := range []string{"old-first", "old-second", "old-second"} {
		m = pressKey(m, 'n')
		if m.selectedCommentID() != id || m.tab().cursorSide != "old" {
			t.Fatalf("next comment = %q, side = %q; want %q on old side", m.selectedCommentID(), m.tab().cursorSide, id)
		}
	}
	m = pressKey(m, 'N')
	if m.selectedCommentID() != "old-first" {
		t.Fatal("previous should visit the first deleted-line comment")
	}
	m = pressKey(m, 'N')
	if m.tab().cursorOnAnnotation || m.tab().cursorSide != "old" || m.tab().cursorLine != 10 {
		t.Fatal("previous should return to the deletion hunk")
	}
}

func TestPatchRoundRefreshesSnapshot(t *testing.T) {
	m := patchApp(t, "diff --git a/a.txt b/a.txt\nnew file mode 100644\n--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1 @@\n+first\n")
	m.tab().state.Comments = []review.Comment{{ID: "c1", Scope: "file", Body: "keep this thread"}}
	m.persist()
	p, err := gitpkg.ParsePatch(strings.NewReader("diff --git a/b.txt b/b.txt\nnew file mode 100644\n--- /dev/null\n+++ b/b.txt\n@@ -0,0 +1 @@\n+second\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.session.SaveDiff(p); err != nil {
		t.Fatal(err)
	}
	m = restartRound(t, m)
	if m.err != nil || m.session.CJ.ReviewRound != 2 || len(m.tabs) != 2 {
		t.Fatalf("round failed: err=%v round=%d tabs=%d", m.err, m.session.CJ.ReviewRound, len(m.tabs))
	}
	if !m.tabs[0].outsideChanges || len(m.tabs[0].state.Comments) != 1 || m.tabs[1].doc.Content != "second\n" || !m.tabs[1].changedLines[1] {
		t.Fatalf("incorrect refreshed tabs: %+v", m.tabs)
	}
}
