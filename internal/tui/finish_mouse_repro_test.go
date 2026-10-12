package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/review"
)

func TestFinishMouseSubmitAfterOverallComment(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {110, 40}, {140, 42}, {60, 18}, {40, 16}, {40, 24}, {50, 24}, {60, 24}, {65, 24}, {80, 18}} {
		for _, position := range []string{"left", "center", "right"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], position), func(t *testing.T) {
				comment := testComment()
				app, ch := newFinishTestApp(t, []review.Comment{comment})
				app.width, app.height = size[0], size[1]
				app.recalculateLayout()
				app.toggleResolve(comment.ID)
				app = pressKey(app, 'q')
				click := func(focus int, textarea bool) tea.Cmd {
					t.Helper()
					for _, region := range app.modalMouseRegions() {
						if region.action.dialog || region.action.close || region.action.scrollable || region.action.textarea != textarea || (!textarea && region.action.focus != focus) {
							continue
						}
						x := region.rect.left
						switch position {
						case "center":
							x = (region.rect.left + region.rect.right - 1) / 2
						case "right":
							x = region.rect.right - 1
						}
						app = hoverMouse(app, x, region.rect.top)
						var cmd tea.Cmd
						app, cmd = clickMouseCmd(app, x, region.rect.top)
						app = releaseMouse(app, x, region.rect.top)
						return cmd
					}
					t.Fatalf("missing mouse action: focus=%d textarea=%t", focus, textarea)
					return nil
				}
				click(finishText, true)
				const body = "どちらの動画もOKです！"
				for _, r := range body {
					app, _ = updateApp(app, tea.KeyPressMsg{Code: r, Text: string(r)})
				}
				if app.finishVerdict != review.VerdictComment {
					t.Fatalf("input verdict = %s", app.finishVerdict)
				}
				click(finishApprove, false)
				if app.finishVerdict != review.VerdictApprove {
					t.Fatalf("mouse selection verdict = %s", app.finishVerdict)
				}
				if cmd := click(finishSubmit, false); !isQuit(cmd) {
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	app, ch := formTestApp(t)
	app.width, app.height = 74, 38
	app.recalculateLayout()
	app.openFinishModal()
	for _, region := range app.modalMouseRegions() {
		if region.action.textarea {
			app = clickMouse(app, region.rect.left, region.rect.top)
			break
		}
	}
	app, _ = updateApp(app, tea.PasteMsg{Content: "どちらの動画もOKです！"})
	for _, region := range app.modalMouseRegions() {
		if region.action.pick && region.action.focus == finishApprove {
			app = clickMouse(app, region.rect.left, region.rect.top)
			break
		}
	}
	if app.finishVerdict != review.VerdictApprove {
		t.Fatal("Approve was not selected")
	}
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == finishSubmit && !region.action.dialog && !region.action.textarea {
			var cmd tea.Cmd
			app, cmd = clickMouseCmd(app, region.rect.left, region.rect.top)
			app = releaseMouse(app, region.rect.left, region.rect.top)
			if isQuit(cmd) {
				t.Fatal("missing confirmation allowed submission")
			}
			break
		}
	}
	select {
	case <-ch:
		t.Fatal("missing confirmation emitted a result")
	default:
	}
	if !strings.Contains(app.View().Content, "Complete required confirmations") {
		t.Fatalf("blocked Submit did not show its reason:\n%s", app.View().Content)
	}
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == finishBack && !region.action.dialog && !region.action.textarea {
			assertRegionContainsRenderedText(t, app, region.rect, "Back ESC")
			app = clickMouse(app, region.rect.left, region.rect.top)
			break
		}
	}
	if app.modal != noModal {
		t.Fatal("Back did not close the blocked submission")
	}
	if app.finishDraft != "どちらの動画もOKです！" {
		t.Fatal("Back discarded the overall comment")
	}
}

func TestFinishTabOrderPlacesInputBeforeSubmit(t *testing.T) {
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
			app.modalFocus = finishApprove
			for _, focus := range []int{finishReject, finishComment, finishText, finishSubmit, finishBack, finishAbort, finishApprove} {
				app = pressKey(app, tea.KeyTab)
				if app.modalFocus != focus || app.modalTextarea.Focused() != (focus == finishText) {
					t.Fatalf("Tab focus = %d, want %d", app.modalFocus, focus)
				}
			}
			app.modalFocus = finishSubmit
			for _, focus := range []int{finishText, finishComment, finishReject, finishApprove, finishAbort, finishBack, finishSubmit} {
				app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
				if app.modalFocus != focus || app.modalTextarea.Focused() != (focus == finishText) {
					t.Fatalf("Shift+Tab focus = %d, want %d", app.modalFocus, focus)
				}
			}
		})
	}
}
