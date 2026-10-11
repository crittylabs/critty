package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/review"
)

func TestMouseClickSubmitButtonOpensModal(t *testing.T) {
	app := setupAppWithDoc(t, "first\nsecond\n")
	app.width = 100
	app.height = 24
	app.recalculateLayout()
	rect, ok := app.finishButtonRect()
	if !ok {
		t.Fatal("Submit button not found")
	}
	if actualY := renderedLineY(t, app, "Submit q"); actualY != rect.top {
		t.Fatalf("button region row = %d, rendered button row = %d", rect.top, actualY)
	}
	assertRegionContainsRenderedText(t, app, rect, "Submit q")

	app = clickMouse(app, rect.left, rect.top)

	if app.modal != finishModal {
		t.Fatalf("modal = %v, want finish modal", app.modal)
	}
}

func TestMouseClickFooterHelpButtonOpensHelp(t *testing.T) {
	for _, width := range []int{60, 120} {
		app := setupAppWithDoc(t, "first\nsecond\n")
		app.width = width
		app.height = 24
		app.recalculateLayout()
		rect, ok := app.footerHelpRect()
		if !ok {
			t.Fatalf("width %d: footer Help button not found", width)
		}
		assertRegionContainsRenderedText(t, app, rect, "Help M-?")

		app = clickMouse(app, rect.left, rect.top)

		if app.modal != helpModal {
			t.Fatalf("width %d: modal = %v, want help modal", width, app.modal)
		}
	}
}

func TestQuit_OpensFinishModal(t *testing.T) {
	app, _ := newFinishTestApp(t, []review.Comment{testComment()})

	app, cmd := pressKeyCmd(app, 'q')

	if isQuit(cmd) {
		t.Fatal("expected quit to be intercepted by the finish modal")
	}
	if app.modal != finishModal {
		t.Fatalf("expected finishModal, got %v", app.modal)
	}
}

func TestFinishModalConfirmation(t *testing.T) {
	resolved := testComment()
	resolved.Resolved = true
	for _, tt := range []struct {
		name                  string
		comments              []review.Comment
		newFeedback, approved bool
	}{
		{name: "no comments", approved: true},
		{name: "resolved comments", comments: []review.Comment{resolved}, approved: true},
		{name: "new feedback", comments: []review.Comment{testComment()}, newFeedback: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, ch := newFinishTestApp(t, tt.comments)
			app.newFeedback = tt.newFeedback
			app = pressKey(app, 'q')
			app, cmd := pressKeyCmd(app, 'y')
			if !isQuit(cmd) || app.modal != noModal {
				t.Fatal("confirmation did not close the modal and quit")
			}
			if event := takeEvent(t, ch); event.Approved != tt.approved {
				t.Errorf("approved = %t, want %t", event.Approved, tt.approved)
			}
		})
	}
}

func TestFinishModal_EscReturnsToReview(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, tea.KeyEscape)

	if isQuit(cmd) {
		t.Fatal("expected esc to stay in the review")
	}
	if app.modal != noModal {
		t.Fatalf("expected modal dismissed, got %v", app.modal)
	}
	select {
	case <-ch:
		t.Error("expected no finish event on cancel")
	default:
	}
}

func TestMouseClickFinishModalCloseReturnsToReview(t *testing.T) {
	app, ch := newFinishTestApp(t, nil)
	app.width = 80
	app.height = 24
	app.recalculateLayout()
	app, _ = pressKeyCmd(app, 'q')
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == 1 {
			assertRegionContainsRenderedText(t, app, region.rect, "Close n")
			break
		}
	}

	app = clickModalAction(t, app, "Close n")

	if app.modal != noModal {
		t.Fatalf("modal = %v, want no modal", app.modal)
	}
	select {
	case <-ch:
		t.Error("close emitted a finish event")
	default:
	}
}

func TestMouseClickFinishModalConfirmActions(t *testing.T) {
	tests := []struct {
		name        string
		comments    []review.Comment
		newFeedback bool
		label       string
		approved    bool
	}{
		{name: "approve", label: "Approve", approved: true},
		{
			name: "resolve all and approve", comments: []review.Comment{testComment()},
			label: "Resolve All & Approve", approved: true,
		},
		{
			name: "finish review", comments: []review.Comment{testComment()}, newFeedback: true,
			label: "Finish Review", approved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, ch := newFinishTestApp(t, tt.comments)
			app.newFeedback = tt.newFeedback
			app.width = 80
			app.height = 24
			app.recalculateLayout()
			app, _ = pressKeyCmd(app, 'q')
			var actionRect mouseRect
			found := false
			for _, region := range app.modalMouseRegions() {
				if region.action.focus == 0 {
					actionRect = region.rect
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("finish action %q not found", tt.label)
			}
			assertRegionContainsRenderedText(t, app, actionRect, tt.label+" y")

			app, cmd := clickMouseCmd(app, actionRect.left, actionRect.top)

			if !isQuit(cmd) {
				t.Error("finish action did not quit inline review")
			}
			if app.modal != noModal {
				t.Fatalf("modal = %v, want no modal", app.modal)
			}
			if event := takeEvent(t, ch); event.Approved != tt.approved {
				t.Errorf("approved = %t, want %t", event.Approved, tt.approved)
			}
		})
	}
}

func TestFinishModal_NoNewFeedbackResolvesAllAndApproves(t *testing.T) {
	comment := testComment()
	app, ch := newFinishTestApp(t, []review.Comment{comment})
	app.session.CJ.ReviewComments = []review.Comment{{
		ID: "r_test01", Body: "overall feedback", CreatedAt: review.Now(),
	}}
	app, _ = pressKeyCmd(app, 'q')
	app.width = 80
	app.height = 24

	rendered := app.renderWithModal(strings.Repeat(" ", 80))
	if !strings.Contains(rendered, "Resolve all & Approve?") {
		t.Fatalf("expected resolve-all prompt, got:\n%s", rendered)
	}

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected approval to quit")
	}
	if ev := takeEvent(t, ch); !ev.Approved {
		t.Error("expected approved finish event")
	}
	if !app.tabs[0].state.Comments[0].Resolved {
		t.Error("expected file comment resolved")
	}
	if !app.session.CJ.ReviewComments[0].Resolved {
		t.Error("expected review comment resolved")
	}
	if got := app.tabs[0].state.Comments[0].ResolvedRound; got != 1 {
		t.Errorf("resolved round = %d, want 1", got)
	}
}

func TestFinishModal_QuitWithoutFinishing(t *testing.T) {
	app, ch := newFinishTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	_, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected q in the modal to quit without finishing")
	}
	select {
	case <-ch:
		t.Error("expected no finish event when quitting without finishing")
	default:
	}
}
