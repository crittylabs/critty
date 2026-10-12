package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/internal/review"
	"strings"
	"testing"
)

func TestOverallCommentNavigationReplyAndResolution(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	app.width, app.height = 100, 30
	root := review.Comment{ID: "r_overall", Scope: "review", Body: "Overall concern", Author: "Reviewer", ReviewRound: 1, CreatedAt: review.Now()}
	app.session.CJ.ReviewComments = []review.Comment{root}
	app.persist()
	if app.err != nil {
		t.Fatal(app.err)
	}
	if app.unresolvedTotal() != 1 {
		t.Fatal("overall comment counted twice")
	}
	app.recalculateLayout()
	app = pressKey(app, '}')
	if app.tab().path != overallCommentsPath || app.selectedCommentID() != root.ID || commentDrifted(app.tab(), root) {
		t.Fatal("unresolved overall comment not reachable")
	}
	if !strings.Contains(app.View().Content, "Overall concern") || !strings.Contains(app.View().Content, "Overall comments") {
		t.Fatalf("overall comment not visible: %s", app.View().Content)
	}
	cli, err := review.OpenSession("", app.session.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Update(func(s *review.Session) error { return s.AppendReply(root.ID, "Agent reply", "Agent", "", false, "") }); err != nil {
		t.Fatal(err)
	}
	app = pressKey(app, tea.KeyEnter)
	if app.modal != replyModal {
		t.Fatal("overall reply editor missing")
	}
	app.modalTextarea.SetValue("Reviewer reply")
	app.modalSubmit()
	if app.err != nil {
		t.Fatal(app.err)
	}
	if len(app.session.CJ.ReviewComments[0].Replies) != 2 {
		t.Fatal("concurrent overall reply lost")
	}
	app = pressKey(app, 'r')
	if app.unresolvedTotal() != 0 || !app.session.CJ.ReviewComments[0].Resolved {
		t.Fatal("overall comment not resolved")
	}
	if _, ok := app.session.CJ.Files[overallCommentsPath]; ok {
		t.Fatal("overall comment saved as a file")
	}
	app.openFinishModal()
	app.modalTextarea.SetValue("Approval summary")
	_, cmd := app.doFinish()
	if !isQuit(cmd) || len(app.session.CJ.ReviewComments) != 2 || app.session.SubmissionComment().Body != "Approval summary" {
		t.Fatal("submission lost overall comments")
	}
}

func TestOverallCommentsRemainAvailableWithoutSourceChanges(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	app.session.CJ.ReviewComments = []review.Comment{{ID: "r_only", Scope: "review", Body: "Only remaining concern", Author: "Reviewer", CreatedAt: review.Now()}}
	app.persist()
	fresh := NewCodeReviewApp(nil, "HEAD", AppConfig{Session: app.session, Author: app.author})
	fresh, _ = updateApp(fresh, docRenderedMsg{})
	fresh, _ = updateApp(fresh, tea.WindowSizeMsg{Width: 80, Height: 24})
	if fresh.err != nil {
		t.Fatal(fresh.err)
	}
	fresh = pressKey(fresh, '}')
	if fresh.selectedCommentID() != "r_only" || !strings.Contains(fresh.View().Content, "Only remaining concern") {
		t.Fatal("empty diff lost overall comment navigation")
	}
	if _, ok := fresh.session.CJ.RoundState.Files[overallCommentsPath]; ok {
		t.Fatal("display tab included in reviewed source")
	}
}
