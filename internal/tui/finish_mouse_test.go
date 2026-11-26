package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/v2/internal/review"
)

func TestFinishMouseSubmitAfterOverallComment(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {60, 18}, {80, 24}, {140, 42}} {
		for _, position := range []string{"left", "center", "right"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], position), func(t *testing.T) {
				comment := testComment()
				app, ch := newFinishTestApp(t, []review.Comment{comment})
				app.width, app.height = size[0], size[1]
				app.recalculateLayout()
				app.toggleResolve(comment.ID)
				app = pressKey(app, 'q')
				click := func(rect mouseRect) tea.Cmd {
					x := rect.left
					switch position {
					case "center":
						x = (rect.left + rect.right - 1) / 2
					case "right":
						x = rect.right - 1
					}
					app = hoverMouse(app, x, rect.top)
					var cmd tea.Cmd
					app, cmd = clickMouseCmd(app, x, rect.top)
					app = releaseMouse(app, x, rect.top)
					return cmd
				}
				click(modalTextareaRect(t, app))
				const body = "どちらの動画もOKです！"
				for _, r := range body {
					app, _ = updateApp(app, tea.KeyPressMsg{Code: r, Text: string(r)})
				}
				if app.finishVerdict != review.VerdictComment {
					t.Fatalf("input verdict = %s", app.finishVerdict)
				}
				click(finishRegion(t, app, finishApprove))
				if app.finishVerdict != review.VerdictApprove {
					t.Fatalf("mouse selection verdict = %s", app.finishVerdict)
				}
				if cmd := click(finishRegion(t, app, finishSubmit)); !isQuit(cmd) {
					t.Fatalf("Submit did not quit: modal=%v error=%v", app.modal, app.err)
				}
				if event := takeEvent(t, ch); !event.Approved {
					t.Fatal("mouse Submit did not approve")
				}
				if got := app.session.CJ.ReviewComments; len(got) != 1 || got[0].Body != body {
					t.Fatalf("submitted comment = %+v", got)
				}
			})
		}
	}
}

func TestFinishMouseSubmitMissingConfirmationKeepsButtonsAligned(t *testing.T) {
	t.Parallel()
	app, ch := formTestApp(t)
	app.width, app.height = 74, 38
	app.recalculateLayout()
	header, ok := app.finishButtonRect()
	if !ok {
		t.Fatal("Submit button missing")
	}
	assertRegionContainsRenderedText(t, app, header, "Submit q")
	app = clickMouse(app, header.left, header.top)
	if app.modal != finishModal {
		t.Fatal("Submit button did not open the dialog")
	}
	input := modalTextareaRect(t, app)
	app = clickMouse(app, input.left, input.top)
	const body = "Review 界e\u0301"
	app, _ = updateApp(app, tea.PasteMsg{Content: body})
	approve := finishRegion(t, app, finishApprove)
	app = clickMouse(app, approve.left, approve.top)
	if app.finishVerdict != review.VerdictApprove {
		t.Fatal("Approve was not selected")
	}
	submit := finishRegion(t, app, finishSubmit)
	app, cmd := clickMouseCmd(app, submit.left, submit.top)
	app = releaseMouse(app, submit.left, submit.top)
	if isQuit(cmd) {
		t.Fatal("missing confirmation allowed submission")
	}
	select {
	case <-ch:
		t.Fatal("missing confirmation emitted a result")
	default:
	}
	if app.modal != finishFormsModal || !strings.Contains(app.View().Content, "Required items must be completed") {
		t.Fatalf("blocked Submit did not show its reason:\n%s", app.View().Content)
	}
	cancel := finishRegion(t, app, finishCancelForms)
	assertRegionContainsRenderedText(t, app, cancel, "Cancel ESC")
	app = clickMouse(app, cancel.left, cancel.top)
	if app.modal != finishModal {
		t.Fatal("Cancel did not return to Submit")
	}
	back := finishRegion(t, app, finishBack)
	assertRegionContainsRenderedText(t, app, back, "Back ESC")
	app = clickMouse(app, back.left, back.top)
	if app.modal != noModal {
		t.Fatal("Back did not close the blocked submission")
	}
	if app.finishDraft != body {
		t.Fatal("Back discarded the overall comment")
	}
}

func TestFinishTabOrderFollowsLayout(t *testing.T) {
	for _, thread := range []bool{false, true} {
		t.Run(fmt.Sprintf("thread=%t", thread), func(t *testing.T) {
			app, _ := newFinishTestApp(t, nil)
			app.width, app.height = 100, 30
			if thread {
				app.session.CJ.ReviewComments = []review.Comment{{ID: "r_prior", Body: "Previous overall feedback", Author: "Reviewer", ReviewRound: 1}}
			}
			app.openFinishModal()
			if thread && !strings.Contains(app.View().Content, "Previous overall feedback") {
				t.Fatal("previous thread is not visible")
			}
			if app.modalFocus != finishApprove || app.modalTextarea.Focused() {
				t.Fatal("initial focus is not on the verdict group")
			}
			var forward []int
			if thread {
				forward = append(forward, finishProgressStart+int(progressThreads))
			}
			forward = append(forward, finishBack, finishAbort, finishSubmit, finishText, finishApprove)
			for _, focus := range forward {
				app = pressKey(app, tea.KeyTab)
				if app.modalFocus != focus || app.modalTextarea.Focused() != (focus == finishText) {
					t.Fatalf("Tab focus = %d, want %d", app.modalFocus, focus)
				}
			}
			app.modalFocus = finishSubmit
			backward := []int{finishAbort, finishBack}
			if thread {
				backward = append(backward, finishProgressStart+int(progressThreads))
			}
			backward = append(backward, finishApprove, finishText, finishSubmit)
			for _, focus := range backward {
				app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
				if app.modalFocus != focus || app.modalTextarea.Focused() != (focus == finishText) {
					t.Fatalf("Shift+Tab focus = %d, want %d", app.modalFocus, focus)
				}
			}
		})
	}
}
