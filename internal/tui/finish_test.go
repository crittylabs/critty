package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/review"
)

func TestSubmitAttentionColorsAndNotices(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, state := range []string{"required", "threads", "optional", "optional-and-threads", "complete"} {
			app, _ := formTestApp(t)
			if state != "required" {
				app.toggleForm(2)
			}
			if state == "threads" || state == "complete" {
				app.toggleForm(3)
				app.toggleForm(5)
			}
			if state == "threads" || state == "optional-and-threads" {
				app.tab().state.Comments = []review.Comment{testComment()}
			}
			want := success
			switch state {
			case "required":
				want = lipgloss.Red
			case "threads":
				want = lipgloss.Cyan
			case "optional", "optional-and-threads":
				want = warning
			}
			if color.RGBAModel.Convert(app.finishAttentionColor()) != color.RGBAModel.Convert(want) {
				t.Fatalf("%s: wrong border color", state)
			}
			for _, verdict := range []review.Verdict{review.VerdictReject, review.VerdictComment} {
				app.finishVerdict = verdict
				if color.RGBAModel.Convert(app.finishAttentionColor()) != color.RGBAModel.Convert(finishColor(verdict)) {
					t.Fatalf("%s: %s border depends on pending items", state, verdict)
				}
				if notice := app.finishNotices(70); notice != "" {
					t.Fatalf("%s: %s shows pending-item notices: %s", state, verdict, notice)
				}
			}
			app.finishVerdict = review.VerdictApprove
			notice := ansi.Strip(app.finishNotices(70))
			if strings.Contains(notice, "Required items") != (state == "required") || strings.Contains(notice, "Unresolved threads") != (state == "threads" || state == "optional-and-threads") || strings.Contains(notice, "Optional items") != (state == "required" || state == "optional" || state == "optional-and-threads") {
				t.Fatalf("%s: unexpected notices: %s", state, notice)
			}
			app.openFinishModal()
			app.finishVerdict = review.VerdictReject
			content, _ := app.finishModalContent(70)
			greenTitle := modalTitleStyle.MarginBottom(0).Foreground(success).Render("Submit review")
			if !strings.Contains(content, greenTitle) {
				t.Fatal("Submit title changed color with the verdict")
			}
		}
	}
	initAdaptiveStyles(true)
}

func TestFinishPendingSummary(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, _ := formTestApp(t)
		app.tab().state.Comments = []review.Comment{testComment()}
		app.forms.Documents[0].Items[1].Required = true
		app.forms.Documents[0].Items[2].Required = true
		app.openFinishModal()
		content, _ := app.finishModalContent(70)
		plain := strings.Join(strings.Fields(ansi.Strip(content)), " ")
		const status = "0/1 💬 0/1 ☐ 0/1 ○ 0/1 ☐"
		if !strings.Contains(plain, status) || strings.Index(plain, status) > strings.Index(plain, "Abort q") || strings.Contains(plain, "a/r/c: select") {
			t.Fatalf("progress counters are not above the buttons: %s", plain)
		}
		for _, label := range []string{"0/1 ☐", "0/1 ○"} {
			if !hasForeground(content, progressColors.required, label) {
				t.Fatalf("required status is not red: %q", content)
			}
		}
		app.closeFinishModal()
		app.tab().cursorLine = 2
		app.toggleForm(2)
		app.toggleForm(4)
		app.toggleForm(5)
		app.toggleResolve(testComment().ID)
		if status, _ := app.layoutProgressCounts(70, 0, -1); ansi.Strip(status) != "1/1 💬  1/1 ☑  1/1 ●  1/1 ☑" {
			t.Fatalf("completed counts or marks are wrong: %s", status)
		}
	}
	initAdaptiveStyles(true)
}

func TestFinishStatusCompletionColorsAndMarks(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, _ := formTestApp(t)
		app.toggleForm(2)
		app.toggleForm(4)
		app.toggleForm(5)
		status, _ := app.layoutProgressCounts(90, 0, -1)
		if plain := ansi.Strip(status); plain != "0/0 💬  1/1 ☑  1/1 ☑  1/1 ●" {
			t.Fatalf("optional choice counted alternatives instead of groups: %s", plain)
		}
		for _, expected := range []struct {
			color color.Color
			label string
		}{
			{progressColors.complete, "1/1 ☑"},
			{progressColors.pending, "1/1 ☑"},
			{progressColors.pending, "1/1 ●"},
		} {
			if !hasForeground(status, expected.color, expected.label) {
				t.Fatalf("wrong completion color for %s: %q", expected.label, status)
			}
		}
		app.forms.Documents[0].Items[3].Required = true
		app.toggleForm(5)
		status, _ = app.layoutProgressCounts(90, 0, -1)
		if plain := ansi.Strip(status); plain != "0/0 💬  1/2 ☐  1/1 ●" {
			t.Fatalf("partially completed checkboxes used a completed mark: %s", plain)
		}
		if !hasForeground(status, progressColors.required, "1/2 ☐") {
			t.Fatalf("partially completed required checks are not red: %q", status)
		}
	}
	initAdaptiveStyles(true)
}

func TestFinishFormsReviewAndCancelPreserveDraft(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		app, ch := formTestApp(t)
		app.activeTab = 1
		app.openFinishModal()
		app.modalTextarea.SetValue("Keep this overall comment.")
		app.finishVerdict = review.VerdictApprove
		_, cmd := app.doFinish()
		if isQuit(cmd) || app.modal != finishFormsModal || app.modalFocus != finishReviewForms {
			t.Fatal("approval did not default to reviewing pending forms")
		}
		key := tea.KeyEnter
		if cancel {
			key = tea.KeyEscape
		}
		app, cmd = pressKeyCmd(app, key)
		if isQuit(cmd) || app.finishDraft != "Keep this overall comment." || app.session.CJ.RoundState.ReviewCommentDraft != app.finishDraft {
			t.Fatal("pending-form action lost the draft or submitted")
		}
		if cancel {
			if app.modal != finishModal || app.finishVerdict != review.VerdictApprove || app.modalTextarea.Value() != app.finishDraft {
				t.Fatal("Cancel did not return to Submit with the decision and draft")
			}
		} else if app.modal != noModal || app.activeTab != 0 || app.tab().cursorLine != 2 || !app.tab().cursorOnForm {
			t.Fatal("Review did not jump to the first pending item across tabs")
		}
		select {
		case <-ch:
			t.Fatal("review/cancel emitted submission")
		default:
		}
		if err := app.SaveUnsubmittedDraft(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFinishUnviewedFormsByVerdict(t *testing.T) {
	content := "# Form\n" + strings.Repeat("filler\n", 40) + "- [ ] Extra <!-- form: id=extra -->\n"
	for _, verdict := range []review.Verdict{review.VerdictApprove, review.VerdictComment, review.VerdictReject} {
		actions := []rune{'s', 'r'}
		if verdict != review.VerdictApprove {
			actions = []rune{'s'}
		}
		for _, action := range actions {
			t.Run(fmt.Sprintf("%s/%c", verdict, action), func(t *testing.T) {
				document := content
				if verdict != review.VerdictApprove {
					document += "- [ ] Confirm <!-- form: id=confirm required -->\n"
				}
				app, ch := formTestAppWithContent(t, document)
				if verdict != review.VerdictApprove {
					app.tab().state.Comments = []review.Comment{testComment()}
				}
				app.openFinishModal()
				app.modalTextarea.SetValue("Keep this overall comment.")
				app.finishVerdict = verdict
				_, cmd := app.doFinish()
				if verdict != review.VerdictApprove {
					if !isQuit(cmd) {
						t.Fatalf("feedback submission asked for confirmation: modal %v, err %v", app.modal, app.err)
					}
					event := takeEvent(t, ch)
					if event.Approved || event.Result.Verdict != verdict || len(event.Result.Consents) != 0 || app.session.CJ.RoundState.ReviewCommentDraft != "Keep this overall comment." {
						t.Fatalf("unexpected feedback submission: %+v", event)
					}
					return
				}
				if isQuit(cmd) || app.modal != finishFormsModal || !strings.Contains(ansi.Strip(app.View().Content), "not been viewed") {
					t.Fatal("unviewed optional item did not ask for confirmation")
				}
				app, cmd = pressKeyCmd(app, action)
				if action == 'r' {
					if isQuit(cmd) || app.modal != noModal || app.tab().cursorLine != 42 || len(app.unviewedForms()) != 0 {
						t.Fatal("Review did not show the unviewed item")
					}
					app.openFinishModal()
					app.finishVerdict = verdict
					_, cmd = app.doFinish()
				}
				if !isQuit(cmd) {
					t.Fatalf("submission did not continue: modal %v, err %v", app.modal, app.err)
				}
				event := takeEvent(t, ch)
				if event.Approved != (verdict == review.VerdictApprove) || app.session.CJ.RoundState.ReviewCommentDraft != "Keep this overall comment." {
					t.Fatalf("unexpected submission: %+v", event)
				}
			})
		}
	}
}

func TestApproveWithUnansweredOptionalForms(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		app, ch := formTestApp(t)
		app.toggleForm(2)
		if unresolved {
			app.tab().state.Comments = []review.Comment{testComment()}
		}
		app.openFinishModal()
		app.modalTextarea.SetValue("Approve with optional answers empty.")
		app.finishVerdict = review.VerdictApprove
		if notice := app.optionalFormsNotice(70); !strings.Contains(ansi.Strip(notice), "You can still approve") || notice == ansi.Strip(notice) {
			t.Fatal("optional items lack a colored notice")
		}
		_, cmd := app.doFinish()
		if unresolved {
			if app.modal != finishResolveModal {
				t.Fatal("optional items bypassed thread confirmation")
			}
			app, cmd = pressKeyCmd(app, 'y')
		}
		if !isQuit(cmd) {
			t.Fatalf("optional items blocked approval: %v", app.err)
		}
		event := takeEvent(t, ch)
		if !event.Approved || event.Result.Consents["a"]["publish"] || event.Result.Documents[0].Answers["size"].Value != "" {
			t.Fatal("approval changed optional answers or granted consent")
		}
	}
}

func TestFinishFormsCannotProceedPastRequiredItems(t *testing.T) {
	app, ch := formTestApp(t)
	app.openFinishModal()
	app.finishVerdict = review.VerdictApprove
	_, cmd := app.doFinish()
	if isQuit(cmd) || app.modal != finishFormsModal || app.modalFocus != finishReviewForms {
		t.Fatal("approval bypassed required items")
	}
	app.modalFocus = finishCancelForms
	app, cmd = pressKeyCmd(app, tea.KeyEnter)
	if isQuit(cmd) || app.modal != finishModal || app.session.CJ.RoundState.Finished {
		t.Fatal("Cancel submitted unanswered required items")
	}
	select {
	case <-ch:
		t.Fatal("unanswered required item emitted approval")
	default:
	}
}

func TestFinishPendingDialogsFitSmallTerminals(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, size := range [][2]int{{40, 12}, {54, 14}, {80, 24}} {
			for _, required := range []bool{false, true} {
				app, _ := formTestAppWithContent(t, "- [ ] Confirm <!-- form: id=confirm required -->\n- [ ] Required choice <!-- form: id=required-choice value=chosen required -->\n- [ ] Optional choice <!-- form: id=optional-choice value=chosen -->\n- [ ] Consent <!-- form: id=publish consent -->\n- [ ] Optional checkbox <!-- form: id=extra -->\n")
				app.width, app.height = size[0], size[1]
				app.tab().state.Comments = []review.Comment{testComment()}
				if !required {
					app.toggleForm(1)
					app.toggleForm(2)
				}
				app.openFinishModal()
				for _, modal := range []modalType{finishModal, finishFormsModal} {
					app.modal = modal
					for _, region := range app.modalMouseRegions() {
						if region.action.dialog || region.action.textarea || region.action.scrollable {
							continue
						}
						if region.rect.top < 0 || region.rect.bottom > app.height || region.rect.right > app.width {
							t.Fatalf("modal %v has unreachable control in %v: %+v", modal, size, region)
						}
					}
				}
			}
		}
	}
	initAdaptiveStyles(true)
}

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
		{name: "existing unresolved feedback", comments: []review.Comment{testComment()}, approved: true},
		{name: "new feedback", comments: []review.Comment{testComment()}, newFeedback: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, ch := newFinishTestApp(t, tt.comments)
			app.newFeedback = tt.newFeedback
			app, cmd := pressKeyCmd(app, 'q')
			if isQuit(cmd) || app.modal != finishModal {
				t.Fatal("q did not open the Submit dialog")
			}
			wantFocus := finishComment
			wantVerdict := review.VerdictComment
			if tt.approved {
				wantVerdict = review.VerdictApprove
				wantFocus = finishApprove
			}
			if app.finishVerdict != wantVerdict {
				t.Fatalf("default verdict = %s, want %s", app.finishVerdict, wantVerdict)
			}
			if app.modalFocus != wantFocus || app.modalTextarea.Focused() {
				t.Fatalf("default focus = %d, want %d, input focused = %t", app.modalFocus, wantFocus, app.modalTextarea.Focused())
			}
			app, cmd = pressKeyCmd(app, tea.KeyEnter)
			if app.modal == finishResolveModal {
				app, cmd = pressKeyCmd(app, 'y')
			}
			if !isQuit(cmd) || app.modal != noModal {
				t.Fatal("confirmation did not close the modal and quit")
			}
			if event := takeEvent(t, ch); event.Approved != tt.approved {
				t.Errorf("approved = %t, want %t", event.Approved, tt.approved)
			}
		})
	}
}

func TestFinishModalBackReturnsToReview(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		app, ch := newFinishTestApp(t, []review.Comment{testComment()})
		app.width, app.height = 80, 24
		app.recalculateLayout()
		app = pressKey(app, 'q')
		var cmd tea.Cmd
		if mouse {
			rect := finishRegion(t, app, finishBack)
			assertRegionContainsRenderedText(t, app, rect, "Back ESC")
			app = clickMouse(app, rect.left, rect.top)
		} else {
			app, cmd = pressKeyCmd(app, tea.KeyEscape)
		}
		if isQuit(cmd) || app.modal != noModal {
			t.Fatalf("mouse %t: Back did not return to the review: modal %v", mouse, app.modal)
		}
		select {
		case <-ch:
			t.Fatalf("mouse %t: Back emitted a finish event", mouse)
		default:
		}
	}
}

func finishRegion(t *testing.T, app AppModel, focus int) mouseRect {
	t.Helper()
	for _, region := range app.modalMouseRegions() {
		if region.action.focus == focus && !region.action.dialog && !region.action.close && !region.action.textarea && !region.action.scrollable {
			return region.rect
		}
	}
	t.Fatalf("modal action %d not found", focus)
	return mouseRect{}
}

func TestMouseClickFinishModalConfirmActions(t *testing.T) {
	for _, tt := range []struct {
		name        string
		comments    []review.Comment
		newFeedback bool
		label       string
		focus       int
		approved    bool
	}{
		{name: "approve", label: "Approve", focus: finishApprove, approved: true},
		{name: "reject", label: "Reject", focus: finishReject},
		{name: "resolve all and approve", comments: []review.Comment{testComment()}, label: "Approve", focus: finishApprove, approved: true},
		{name: "comment", comments: []review.Comment{testComment()}, newFeedback: true, label: "Comment", focus: finishComment},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, ch := newFinishTestApp(t, tt.comments)
			app.newFeedback = tt.newFeedback
			app.width, app.height = 80, 24
			app.recalculateLayout()
			app = pressKey(app, 'q')
			rect := finishRegion(t, app, tt.focus)
			assertRegionContainsRenderedText(t, app, rect, tt.label)
			app, cmd := clickMouseCmd(app, rect.left, rect.top)
			if isQuit(cmd) || app.modal != finishModal {
				t.Fatal("radio selection submitted review")
			}
			select {
			case <-ch:
				t.Fatal("radio selection emitted a result")
			default:
			}
			rect = finishRegion(t, app, finishSubmit)
			app, cmd = clickMouseCmd(app, rect.right-1, rect.top)
			if app.modal == finishResolveModal {
				rect = finishRegion(t, app, 0)
				assertRegionContainsRenderedText(t, app, rect, "Resolve All")
				app.modalOpenedAt = time.Now().Add(-modalClickGuard)
				app, cmd = clickMouseCmd(app, rect.right-1, rect.top)
			}
			if !isQuit(cmd) || app.modal != noModal {
				t.Fatalf("finish action did not quit: modal %v", app.modal)
			}
			if event := takeEvent(t, ch); event.Approved != tt.approved {
				t.Errorf("approved = %t, want %t", event.Approved, tt.approved)
			}
		})
	}
}

func TestFinishModal_ExplicitApprovalResolvesAll(t *testing.T) {
	comment := testComment()
	app, ch := newFinishTestApp(t, []review.Comment{comment})
	app.session.CJ.ReviewComments = []review.Comment{{
		ID: "r_test01", Body: "overall feedback", CreatedAt: review.Now(),
	}}
	app, _ = pressKeyCmd(app, 'q')
	app.width = 80
	app.height = 24

	app = pressKey(app, 'a')
	app, cmd := pressKeyCmd(app, tea.KeyEnter)
	if app.modal != finishResolveModal || isQuit(cmd) {
		t.Fatal("approval did not request resolve-all confirmation")
	}
	app, cmd = pressKeyCmd(app, 'y')

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

func TestExplicitSubmissionOutcomes(t *testing.T) {
	for _, verdict := range []review.Verdict{review.VerdictApprove, review.VerdictReject, review.VerdictComment} {
		for _, body := range []string{"", "Overall feedback.\nAnother line."} {
			t.Run(string(verdict)+"/"+body, func(t *testing.T) {
				app, ch := newFinishTestApp(t, []review.Comment{testComment()})
				app.newFeedback = true
				app.openFinishModal()
				app.modalTextarea.SetValue(body)
				_, cmd := app.submitReview(verdict)
				if !isQuit(cmd) {
					t.Fatalf("submission failed: %v", app.err)
				}
				if takeEvent(t, ch).Approved != (verdict == review.VerdictApprove) {
					t.Fatal("incorrect approval")
				}
				saved, err := review.OpenSession("", app.session.Key)
				if err != nil {
					t.Fatal(err)
				}
				if !saved.CJ.RoundState.Finished || saved.CJ.RoundState.Verdict != verdict {
					t.Fatalf("incorrect saved submission: %+v", saved.CJ.RoundState)
				}
				comment := saved.SubmissionComment()
				if body == "" {
					if comment != nil {
						t.Fatal("empty comment was created")
					}
				} else if comment == nil || comment.Body != body || comment.Scope != "review" || comment.Resolved != (verdict == review.VerdictApprove) {
					t.Fatalf("wrong overall comment: %+v", comment)
				}
			})
		}
	}
}

func TestSubmissionDraftAndAbort(t *testing.T) {
	app, ch := newFinishTestApp(t, nil)
	app.openFinishModal()
	app.modalTextarea.SetValue("Unsubmitted draft.")
	app.closeFinishModal()
	if len(app.session.CJ.ReviewComments) != 0 {
		t.Fatal("draft was submitted")
	}
	app.openFinishModal()
	if app.modalTextarea.Value() != "Unsubmitted draft." {
		t.Fatal("draft was lost")
	}
	_, cmd := app.requestFinishAbort()
	if isQuit(cmd) || app.modal != finishAbortModal {
		t.Fatal("abort did not request confirmation")
	}
	_, cmd = pressKeyCmd(app, 'y')
	if !isQuit(cmd) || app.session.CJ.RoundState.Finished {
		t.Fatal("abort submitted the review")
	}
	select {
	case <-ch:
		t.Fatal("abort emitted a submission")
	default:
	}
}

func TestSubmissionUnicodeInputAndLayout(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, size := range [][2]int{{100, 24}, {60, 16}, {40, 12}} {
			app, _ := newFinishTestApp(t, nil)
			app.width, app.height = size[0], size[1]
			app.openFinishModal()
			rect := modalTextareaRect(t, app)
			app = clickMouse(app, rect.left, rect.top)
			app, _ = updateApp(app, tea.PasteMsg{Content: "日本語e\u0301\n" + strings.Repeat("長文", 30)})
			view := app.View()
			if view.Cursor == nil || !rect.contains(tea.Mouse{X: view.Cursor.X, Y: view.Cursor.Y}) {
				t.Fatalf("cursor outside textarea: %+v %+v", view.Cursor, rect)
			}
			if strings.Count(view.Content, "\n")+1 > size[1] {
				t.Fatalf("dialog exceeds terminal: %v", size)
			}
			for _, region := range app.modalMouseRegions() {
				if region.action.focus >= finishApprove && !region.action.dialog && !region.action.close && !region.action.scrollable {
					if region.rect.top < 0 || region.rect.bottom > size[1] {
						t.Fatalf("button unreachable: %+v in %v", region, size)
					}
				}
			}
			before := app.modalTextarea.Value()
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'q', Text: "q"})
			if app.modalTextarea.Value() != before+"q" {
				t.Fatal("input shortcut swallowed q")
			}
		}
	}
	initAdaptiveStyles(true)
}

func TestExplicitApprovalRequiresConfirmations(t *testing.T) {
	app, ch := formTestApp(t)
	app.openFinishModal()
	app.modalTextarea.SetValue("Approval comment")
	_, cmd := app.submitReview(review.VerdictApprove)
	if isQuit(cmd) || app.session.CJ.RoundState.Finished {
		t.Fatal("approved missing confirmations")
	}
	select {
	case <-ch:
		t.Fatal("approval event emitted")
	default:
	}
	app.modalTextarea.SetValue("Please complete the required confirmations.")
	_, cmd = app.submitReview(review.VerdictReject)
	if !isQuit(cmd) {
		t.Fatalf("reject failed: %v", app.err)
	}
	ev := takeEvent(t, ch)
	if ev.Approved || ev.Result.Verdict != review.VerdictReject || ev.Result.ReviewComment == nil || len(ev.Result.Consents) != 0 {
		t.Fatalf("wrong result: %+v", ev.Result)
	}
}

func TestFinishRadioSelectionChangesFrameColorWithoutSubmitting(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, ch := newFinishTestApp(t, nil)
		app.width, app.height = 100, 24
		app.openFinishModal()
		background := lipgloss.NewStyle().Width(app.width).Height(app.height).Render("")
		frames := make(map[string]bool)
		plainFrame := ""
		for _, key := range []rune{'a', 'A', 'r', 'R', 'c', 'C'} {
			var cmd tea.Cmd
			app, cmd = pressKeyCmd(app, key)
			if isQuit(cmd) || app.modal != finishModal {
				t.Fatal("selection submitted the review")
			}
			select {
			case <-ch:
				t.Fatal("selection emitted a result")
			default:
			}
			view := app.renderWithModal(background)
			for _, line := range strings.Split(view, "\n") {
				plain := ansi.Strip(line)
				if !strings.Contains(plain, "╭") {
					continue
				}
				if plainFrame != "" && plain != plainFrame {
					t.Fatal("selection changed frame geometry")
				}
				plainFrame = plain
				frames[line] = true
				break
			}
		}
		if len(frames) != 3 {
			t.Fatal("radio selections did not produce three frame colors")
		}
		app.modalFocus = finishReject
		app, cmd := pressKeyCmd(app, ' ')
		if isQuit(cmd) || app.finishVerdict != review.VerdictReject {
			t.Fatal("Space did not select the focused radio")
		}
		app, cmd = pressKeyCmd(app, tea.KeyEnter)
		if !isQuit(cmd) || takeEvent(t, ch).Approved || app.session.CJ.RoundState.Verdict != review.VerdictReject {
			t.Fatal("Enter did not submit the selected verdict")
		}
	}
	initAdaptiveStyles(true)
}

func TestResolveAllConfirmationCanBeDeclined(t *testing.T) {
	for _, cancel := range []rune{'n', tea.KeyEscape} {
		app, ch := newFinishTestApp(t, []review.Comment{testComment()})
		app.width, app.height = 80, 24
		app.openFinishModal()
		app.modalTextarea.SetValue("Preserved draft.")
		_, cmd := app.doFinish()
		if isQuit(cmd) || app.modal != finishResolveModal {
			t.Fatal("missing resolve-all confirmation")
		}
		app, cmd = pressKeyCmd(app, cancel)
		if isQuit(cmd) || app.modal != finishModal || app.modalTextarea.Value() != "Preserved draft." {
			t.Fatal("declining did not return with the draft")
		}
		if app.tab().state.Comments[0].Resolved || app.session.CJ.RoundState.Finished || len(app.session.CJ.ReviewComments) != 0 {
			t.Fatal("declining resolved or submitted feedback")
		}
		select {
		case <-ch:
			t.Fatal("declining emitted a finish event")
		default:
		}
		_, cmd = app.doFinish()
		if isQuit(cmd) || app.modal != finishResolveModal {
			t.Fatal("retry skipped confirmation")
		}
		app, cmd = pressKeyCmd(app, 'y')
		if !isQuit(cmd) || !takeEvent(t, ch).Approved || !app.tab().state.Comments[0].Resolved {
			t.Fatal("confirmation did not resolve and approve")
		}
	}
}

func TestDefaultVerdictConsidersOnlyUnresolvedFeedback(t *testing.T) {
	for _, scenario := range []string{"untouched", "resolve and unresolve", "resolved new comment", "new unresolved comment", "new reply", "edited comment"} {
		t.Run(scenario, func(t *testing.T) {
			old := testComment()
			old.Author = "Tester"
			app, _ := newFinishTestApp(t, []review.Comment{old})
			want := review.VerdictApprove
			switch scenario {
			case "resolve and unresolve":
				app.toggleResolve(old.ID)
				app.toggleResolve(old.ID)
			case "resolved new comment", "new unresolved comment":
				fresh := review.Comment{ID: "c_fresh", Author: app.author, Body: "New feedback.", ReviewRound: app.reviewRound(), Resolved: scenario == "resolved new comment"}
				app.tab().state.Comments = append(app.tab().state.Comments, fresh)
				app.noteFinishFeedback(app.tab().path, fresh.ID)
				app.newFeedback = true
				if !fresh.Resolved {
					want = review.VerdictComment
				}
			case "new reply":
				app.tab().state.Comments[0].Replies = []review.Reply{{ID: "rp_new", Author: app.author, Body: "More feedback.", ReviewRound: app.reviewRound()}}
				want = review.VerdictComment
			case "edited comment":
				app.modal, app.editingID = editModal, old.ID
				app.modalInitial = old.Body
				app.modalTextarea.SetValue("Changed feedback.")
				app.modalSubmit()
				want = review.VerdictComment
			}
			if got := app.defaultFinishVerdict(); got != want {
				t.Fatalf("default = %s, want %s", got, want)
			}
		})
	}
}

func TestResolveAllConfirmationFitsAndBackWorksByMouse(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, ch := newFinishTestApp(t, []review.Comment{testComment()})
		app.width, app.height = 40, 12
		app.openFinishModal()
		app.doFinish()
		if app.modal != finishResolveModal {
			t.Fatal("missing confirmation")
		}
		for _, region := range app.modalMouseRegions() {
			if region.action.dialog || region.action.close {
				continue
			}
			if region.rect.top < 0 || region.rect.bottom > app.height {
				t.Fatalf("unreachable confirmation action: %+v", region)
			}
			if region.action.focus == 1 {
				app = clickMouse(app, region.rect.left, region.rect.top)
				break
			}
		}
		if app.modal != finishModal {
			t.Fatal("Back did not return to Submit")
		}
		select {
		case <-ch:
			t.Fatal("Back emitted approval")
		default:
		}
	}
	initAdaptiveStyles(true)
}

func TestFinishAbortDeclineKeepsReviewOpen(t *testing.T) {
	for _, key := range []rune{'n', tea.KeyEscape, tea.KeyEnter, 'q'} {
		app, ch := newFinishTestApp(t, []review.Comment{testComment()})
		app.openFinishModal()
		app.modalTextarea.SetValue("Preserve this draft.")
		app, cmd := pressKeyCmd(app, 'q')
		if isQuit(cmd) || app.modal != finishAbortModal {
			t.Fatal("abort did not ask")
		}
		app, cmd = pressKeyCmd(app, key)
		if isQuit(cmd) || app.modal != finishModal || app.modalTextarea.Value() != "Preserve this draft." || app.tab().state.Comments[0].Resolved || app.session.CJ.RoundState.Finished {
			t.Fatal("declining abort changed the review")
		}
		select {
		case <-ch:
			t.Fatal("declining abort emitted submission")
		default:
		}
	}
}

func TestFinishAbortConfirmationMouseAndSmallTerminal(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, ch := newFinishTestApp(t, nil)
		app.width, app.height = 40, 12
		app.openFinishModal()
		app.requestFinishAbort()
		found := false
		for _, region := range app.modalMouseRegions() {
			if region.action.close || region.action.dialog {
				continue
			}
			if region.rect.top < 0 || region.rect.bottom > app.height {
				t.Fatal("unreachable abort action")
			}
			if region.action.focus == 0 {
				_, cmd := clickMouseCmd(app, region.rect.right-1, region.rect.top)
				if !isQuit(cmd) || app.session.CJ.RoundState.Finished {
					t.Fatal("Abort click submitted or did not quit")
				}
				found = true
			}
		}
		if !found {
			t.Fatal("Abort button missing")
		}
		select {
		case <-ch:
			t.Fatal("abort emitted submission")
		default:
		}
	}
	initAdaptiveStyles(true)
}

func TestFinishOverallHistoryStartsAtLatestComment(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	app.width, app.height = 80, 30
	app.session.CJ.ReviewComments = []review.Comment{
		{ID: "r_first", Scope: "review", Body: strings.Repeat("Older feedback.\n", 20)},
		{ID: "r_latest", Scope: "review", Body: "Latest overall feedback.\n" + strings.Repeat("More latest feedback.\n", 20)},
	}
	app.openFinishModal()
	content, regions := app.finishModalContent(50)
	if !strings.Contains(content, "Latest overall feedback.") {
		t.Fatal("latest overall feedback not visible on opening")
	}
	plain := ansi.Strip(content)
	previous := -1
	for _, text := range []string{"Latest overall feedback.", "Overall comment (optional)", "Approve", "💬", "Back"} {
		position := strings.Index(plain, text)
		if position <= previous {
			t.Fatalf("%q is missing or out of order: %s", text, plain)
		}
		previous = position
	}
	for _, region := range regions {
		if region.action.scrollable && (region.action.scrollOffset == 0 || region.action.scrollOffset == region.action.scrollMaxOffset) {
			t.Fatal("history did not start at the latest comment")
		}
	}
	app.scrollFinishReference(-1)
	if app.modalReferenceOffset < 0 {
		t.Fatal("history did not scroll from actual initial offset")
	}
}

func TestFinishCtrlSReturnsToSelectedVerdict(t *testing.T) {
	app, ch := newFinishTestApp(t, nil)
	app.openFinishModal()
	for _, focus := range []int{finishApprove, finishReject, finishComment, finishSubmit, finishBack, finishAbort} {
		app.modalFocus = focus
		var cmd tea.Cmd
		app, cmd = updateApp(app, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if isQuit(cmd) || app.modal != finishModal || app.modalFocus != focus {
			t.Fatal("C-s acted outside the overall editor")
		}
	}
	app.modalFocus = finishApprove
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if app.modalFocus != finishText || !app.modalTextarea.Focused() {
		t.Fatal("Shift+Tab did not focus the overall editor")
	}
	app, _ = updateApp(app, tea.PasteMsg{Content: "Overall summary"})
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if isQuit(cmd) || app.modal != finishModal || app.modalFocus != finishComment || app.modalTextarea.Focused() || app.modalTextarea.Value() != "Overall summary" || app.session.CJ.RoundState.ReviewCommentDraft != "Overall summary" {
		t.Fatal("C-s did not save the draft and focus Comment without sending")
	}
	select {
	case <-ch:
		t.Fatal("C-s emitted submission")
	default:
	}
	_, cmd = pressKeyCmd(app, tea.KeyEnter)
	if !isQuit(cmd) || takeEvent(t, ch).Approved || app.session.CJ.RoundState.Verdict != review.VerdictComment {
		t.Fatal("Enter did not send the automatic Comment verdict after C-s")
	}
}

func TestFinishVerdictGroupNavigation(t *testing.T) {
	for _, initial := range []review.Verdict{review.VerdictApprove, review.VerdictComment} {
		t.Run(string(initial), func(t *testing.T) {
			app, ch := newFinishTestApp(t, nil)
			if initial == review.VerdictComment {
				app.tab().state.Comments = []review.Comment{{ID: "new", Author: app.author, Body: "Feedback", ReviewRound: app.reviewRound()}}
			}
			app.openFinishModal()
			if app.finishVerdict != initial || app.modalFocus != app.finishVerdictFocus() {
				t.Fatal("initial focus does not match the default verdict")
			}
			content, _ := app.finishChoices(80, 0)
			plain := ansi.Strip(content)
			if strings.Index(plain, "Approve") > strings.Index(plain, "Comment") || strings.Index(plain, "Comment") > strings.Index(plain, "Reject") {
				t.Fatalf("wrong choice order: %s", plain)
			}
			for _, step := range []struct {
				key     rune
				verdict review.Verdict
			}{
				{'a', review.VerdictApprove},
				{tea.KeyRight, review.VerdictComment},
				{tea.KeyRight, review.VerdictReject},
				{tea.KeyRight, review.VerdictApprove},
				{tea.KeyLeft, review.VerdictReject},
				{tea.KeyLeft, review.VerdictComment},
				{tea.KeyLeft, review.VerdictApprove},
				{'c', review.VerdictComment},
				{'r', review.VerdictReject},
			} {
				app = pressKey(app, step.key)
				if app.finishVerdict != step.verdict || app.modalFocus != app.finishVerdictFocus() {
					t.Fatalf("key %d: verdict = %s, focus = %d", step.key, app.finishVerdict, app.modalFocus)
				}
				for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab, Mod: tea.ModShift}, {Code: tea.KeyUp}} {
					app, _ = updateApp(app, key)
					if app.modalFocus != finishText || !app.modalTextarea.Focused() {
						t.Fatal("Shift+Tab/Up did not focus the editor")
					}
					app = pressKey(app, tea.KeyTab)
					if app.modalFocus != app.finishVerdictFocus() || app.modalTextarea.Focused() {
						t.Fatal("Tab did not return to the selected verdict")
					}
				}
			}
			select {
			case <-ch:
				t.Fatal("navigation submitted the review")
			default:
			}
		})
	}
}

func TestFinishButtonsUpFocusesVerdict(t *testing.T) {
	for _, focus := range []int{finishSubmit, finishBack, finishAbort} {
		app, _ := newFinishTestApp(t, nil)
		app.openFinishModal()
		app.modalTextarea.SetValue("Preserved draft")
		app.modalFocus = focus
		app = pressKey(app, tea.KeyUp)
		if app.modalFocus != app.finishVerdictFocus() || app.modalTextarea.Focused() || app.modalTextarea.Value() != "Preserved draft" {
			t.Fatalf("Up from %d did not focus the verdict or preserve the draft", focus)
		}
	}
}

func TestFinishInputAutoCommentPreservesExplicitChoices(t *testing.T) {
	for _, scenario := range []string{"first character", "paste", "reject", "approve again", "existing draft", "delete"} {
		app, _ := newFinishTestApp(t, nil)
		if scenario == "existing draft" {
			app.session.CJ.RoundState.ReviewCommentDraft = "Existing draft"
		}
		app.openFinishModal()
		if scenario == "reject" {
			app.finishVerdict = review.VerdictReject
		}
		app.modalFocus = finishText
		app.modalTextarea.Focus()
		if scenario == "paste" {
			app, _ = updateApp(app, tea.PasteMsg{Content: "Pasted overall feedback"})
		} else {
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'x', Text: "x"})
		}
		want := review.VerdictComment
		if scenario == "reject" {
			want = review.VerdictReject
		}
		if scenario == "existing draft" {
			want = review.VerdictApprove
		}
		if scenario == "approve again" || scenario == "delete" {
			app.finishVerdict = review.VerdictApprove
			if scenario == "delete" {
				app = pressKey(app, tea.KeyBackspace)
			} else {
				app, _ = updateApp(app, tea.KeyPressMsg{Code: 'y', Text: "y"})
			}
			want = review.VerdictApprove
		}
		if app.finishVerdict != want {
			t.Fatalf("%s: verdict after input = %s, want %s", scenario, app.finishVerdict, want)
		}
		app, _ = updateApp(app, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if app.finishVerdict != want || app.modalFocus != app.finishVerdictFocus() || app.modalTextarea.Focused() {
			t.Fatalf("%s: C-s did not return to the selected verdict", scenario)
		}
	}
}

func TestAnswerModeSubmitLabel(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	app.answerMode = true
	app.width, app.height = 100, 30
	app.recalculateLayout()
	app = pressKey(app, 'q')
	if app.modal != finishModal || app.finishVerdict != review.VerdictComment {
		t.Fatalf("answer rounds should default to Confirm answers: %v %v", app.modal, app.finishVerdict)
	}
	if view := ansi.Strip(app.View().Content); !strings.Contains(view, "● Confirm answers") || strings.Contains(view, "● Comment") || strings.Contains(view, "○ Comment") {
		t.Fatalf("label: %q", view)
	}
	app = pressKey(app, 'a')
	app = pressKey(app, 'c')
	if app.finishVerdict != review.VerdictComment {
		t.Fatal("c did not select Confirm answers")
	}
}
