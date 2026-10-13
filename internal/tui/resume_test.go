package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestNewRoundReplyFocusFromEmptyBaseline(t *testing.T) {
	for _, author := range []string{"Agent", "Tester"} {
		t.Run(author, func(t *testing.T) {
			app := setupAppWithDoc(t, "first\nsecond\n")
			app.width, app.height = 120, 40
			app.tab().state.Comments = []review.Comment{{ID: "thread", StartLine: 2, EndLine: 2, Body: "please fix"}}
			app.newFeedback = true
			if _, cmd := app.doFinish(); !isQuit(cmd) {
				t.Fatal("finish failed")
			}
			other, err := review.OpenSessionAt(app.session.Key, app.session.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if other.CJ.RoundState.SubmittedReplies == nil {
				t.Fatal("empty baseline was lost during persistence")
			}
			if err := other.AppendReply("thread", "response", author, "", false, ""); err != nil {
				t.Fatal(err)
			}
			if err := other.Save(); err != nil {
				t.Fatal(err)
			}
			app = restartRound(t, app)
			if app.images.retriedOnKey {
				t.Fatal("round focus consumed the first-key image retry")
			}
			if got, want := app.tab().cursorOnAnnotation, author != app.author; got != want {
				t.Fatalf("focused = %t, want %t", got, want)
			}
		})
	}
}

func TestNewRoundFocusesFreshReplies(t *testing.T) {
	for _, tt := range []struct {
		name, scope, side             string
		resolved, lateWindow, drifted bool
	}{
		{name: "line"},
		{name: "drifted line", drifted: true},
		{name: "drifted resolved line", drifted: true, resolved: true},
		{name: "drifted late window", drifted: true, lateWindow: true},
		{name: "file resolved", scope: "file", resolved: true},
		{name: "deleted line", side: "old"},
		{name: "window arrives after documents", lateWindow: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := patchApp(t, "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n first\n-old\n+new\n"+
				"diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1,2 +1,2 @@\n first\n-old\n+new\n")
			app.author = "Reviewer"
			app.tabs[0].state.Comments = []review.Comment{
				{ID: "unanswered", StartLine: 1, EndLine: 1, Body: "no response"},
				{ID: "old-reply", StartLine: 2, EndLine: 2, Body: "already seen", Replies: []review.Reply{{ID: "seen", Author: "Agent", Body: "old answer"}}},
			}
			target := review.Comment{ID: "target", StartLine: 2, EndLine: 2, Scope: tt.scope, Side: tt.side,
				Body: strings.Repeat("long original comment\n", 25), Drifted: tt.drifted}
			if tt.scope == "file" {
				target.StartLine, target.EndLine = 0, 0
			}
			app.tabs[1].state.Comments = []review.Comment{target}
			app.newFeedback = true
			updated, cmd := app.doFinish()
			app = *updated.(*AppModel)
			if !isQuit(cmd) || app.err != nil {
				t.Fatalf("finish failed: %v", app.err)
			}
			other, err := review.OpenSessionAt(app.session.Key, app.session.Dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, reply := range []struct{ id, author, body string }{
				{"unanswered", "Reviewer", "reviewer follow-up"},
				{"target", "Agent", "NEW AGENT RESPONSE"},
				{"target", "Reviewer", strings.Repeat("later reviewer note\n", 25)},
			} {
				if err := other.AppendReply(reply.id, reply.body, reply.author, "", tt.resolved, ""); err != nil {
					t.Fatal(err)
				}
			}
			if err := other.Save(); err != nil {
				t.Fatal(err)
			}
			if tt.lateWindow {
				app.width, app.height = 0, 0
			}
			app = restartRound(t, app)
			if tt.lateWindow {
				if app.activeTab != 0 {
					t.Fatal("focused before window dimensions arrived")
				}
				updated, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
				app = updated.(AppModel)
			}
			if app.activeTab != 1 || app.focused != contentPane || !app.tab().cursorOnAnnotation {
				t.Fatalf("focus = tab %d pane %v annotation %t", app.activeTab, app.focused, app.tab().cursorOnAnnotation)
			}
			if app.images.retriedOnKey {
				t.Fatal("round focus consumed the first-key image retry")
			}
			if app.tab().cursorSide != tt.side || app.selectedCommentID() != app.tab().state.Comments[0].ID {
				t.Fatal("wrong thread selected after carry-forward")
			}
			if got := ansi.Strip(app.contentViewport.View()); !strings.Contains(got, "NEW AGENT RESPONSE") {
				t.Fatalf("focused thread hides the fresh reply:\n%s", got)
			}
			// This is an unfinished round: reopening must not replay its initial jump.
			loaded, err := review.OpenSessionAt(app.session.Key, app.session.Dir)
			if err != nil {
				t.Fatal(err)
			}
			next := NewCodeReviewApp(app.patch.Changes(), "", AppConfig{Session: loaded, Patch: app.patch, Author: app.author})
			next.width, next.height = 120, 40
			updated, _ = next.Update(docRenderedMsg{})
			next = updated.(AppModel)
			if next.activeTab != 0 || next.tab().cursorOnAnnotation {
				t.Fatal("unfinished round repeated the jump")
			}
			// Submitting again without another reply must not revisit old answers.
			app.newFeedback = true
			if _, cmd := app.doFinish(); !isQuit(cmd) {
				t.Fatal("second finish failed")
			}
			app = restartRound(t, app)
			if app.activeTab != 0 || app.tab().cursorOnAnnotation {
				t.Fatal("old replies triggered another jump")
			}
		})
	}
}

// restartRound constructs a fresh model from disk, as the next TUI process does.
func restartRound(t *testing.T, old AppModel) AppModel {
	t.Helper()
	cfg := AppConfig{Author: old.author, Staged: old.staged, Source: old.source}
	if old.session != nil {
		if err := old.session.Update(func(s *review.Session) error { s.CJ.RoundState.Finished = true; return nil }); err != nil {
			t.Fatal(err)
		}
		loaded, err := review.OpenSessionAt(old.session.Key, old.session.Dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Session = loaded
	}
	var next AppModel
	if !old.multiFile {
		next = NewApp(old.filePath, cfg)
	} else {
		var files []gitpkg.FileChange
		var err error
		ref := old.baseRef
		switch {
		case old.patch != nil:
			cfg.Patch, err = gitpkg.LoadPatch(old.session.DiffPath())
			if err == nil {
				files = cfg.Patch.Changes()
			}
		case old.source != nil:
			if old.source.Scope == "range" {
				source, resolveErr := gitpkg.ResolveRange(old.source.Range)
				if resolveErr != nil {
					t.Fatal(resolveErr)
				}
				cfg.Source = &source
				ref = source.Base
			}
			files, err = cfg.Source.Files()
		case old.staged:
			files, err = gitpkg.ChangedFilesStaged()
		default:
			files, err = gitpkg.ChangedFilesFrom(ref)
		}
		if err != nil {
			t.Fatal(err)
		}
		next = NewCodeReviewApp(files, ref, cfg)
	}
	next.width, next.height = old.width, old.height
	updated, _ := next.Update(docRenderedMsg{})
	next = updated.(AppModel)
	if next.err != nil {
		t.Fatal(next.err)
	}
	return next
}
