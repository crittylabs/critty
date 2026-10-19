package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/review"
)

func progressTestApp(t *testing.T, complete bool) AppModel {
	t.Helper()
	app, _ := formTestAppWithContent(t, "- [ ] First check <!-- form: id=first required -->\n- [ ] Second check <!-- form: id=second required -->\n- [ ] Small <!-- form: id=required-choice value=small required -->\n- [ ] Large <!-- form: id=required-choice value=large required -->\n- [ ] Consent <!-- form: id=consent consent -->\n- [ ] Extra <!-- form: id=extra -->\n- [ ] Left <!-- form: id=optional-choice value=left -->\n- [ ] Right <!-- form: id=optional-choice value=right -->\n")
	app.tabs[0].state.Comments = []review.Comment{
		{ID: "done", Scope: "file", Body: "Completed", Resolved: true},
		{ID: "pending", StartLine: 3, EndLine: 3, Body: "Pending", Resolved: complete},
	}
	app.toggleForm(1)
	if complete {
		for _, line := range []int{2, 4, 5, 6, 8} {
			app.toggleForm(line)
		}
	}
	app.recalculateLayout()
	return app
}

func TestSubmitProgressCountersFocusAndNavigate(t *testing.T) {
	for _, complete := range []bool{false, true} {
		for _, kind := range []progressKind{progressThreads, progressRequiredChecks, progressRequiredChoices, progressOptionalChecks, progressOptionalChoices} {
			t.Run(fmt.Sprintf("complete=%t/kind=%d", complete, kind), func(t *testing.T) {
				app := progressTestApp(t, complete)
				app.activeTab = 1
				app.openFinishModal()
				app.modalTextarea.SetValue("Keep this overall draft.")
				app.modalFocus = app.finishVerdictFocus()
				for count := progressThreads; count <= kind; count++ {
					app = pressKey(app, tea.KeyTab)
				}
				if app.modalFocus != finishProgressStart+int(kind) || app.modalTextarea.Focused() {
					t.Fatal("Tab did not focus the requested progress counter")
				}
				app, cmd := pressKeyCmd(app, tea.KeyEnter)
				if isQuit(cmd) || app.modal != noModal || app.activeTab != 0 || app.finishDraft != "Keep this overall draft." {
					t.Fatal("progress navigation submitted or lost the draft")
				}
				if kind == progressThreads {
					want := "pending"
					if complete {
						want = "done"
					}
					if app.selectedCommentID() != want {
						t.Fatalf("thread target = %s, want %s", app.selectedCommentID(), want)
					}
				} else {
					want := map[progressKind]int{progressRequiredChecks: 2, progressRequiredChoices: 3, progressOptionalChecks: 5, progressOptionalChoices: 7}[kind]
					if complete && kind == progressRequiredChecks {
						want = 1
					}
					if app.tab().cursorLine != want || !app.tab().cursorOnForm {
						t.Fatalf("form target = %d, want %d", app.tab().cursorLine, want)
					}
				}
			})
		}
	}
}

func TestProgressCounterMouseRegionsSurviveWrappingAndResize(t *testing.T) {
	sizes := []tea.WindowSizeMsg{{Width: 110, Height: 30}, {Width: 40, Height: 16}, {Width: 80, Height: 24}}
	kinds := []progressKind{progressThreads, progressRequiredChecks, progressRequiredChoices, progressOptionalChecks, progressOptionalChoices}
	// Pairwise: every pair of kind, size, color scheme, and modal values occurs.
	for k, kind := range kinds {
		for i, size := range sizes {
			dark, modal := (k+i)%2 == 0, (k/2+i)%2 == 1
			initAdaptiveStyles(dark)
			app := progressTestApp(t, false)
			app.activeTab = 1
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 24})
			if modal {
				app.openFinishModal()
				app.modalTextarea.SetValue("Mouse navigation draft.")
			}
			app, _ = updateApp(app, size)
			var rect mouseRect
			found := false
			if modal {
				for _, region := range app.modalMouseRegions() {
					if !region.action.dialog && !region.action.close && region.action.focus == finishProgressStart+int(kind) {
						rect, found = region.rect, true
					}
				}
			} else {
				_, layout := app.renderReviewScreen()
				for _, region := range layout.progressRegions {
					if region.kind == kind {
						rect, found = region.rect, true
					}
					if region.rect.bottom > app.headerHeight() {
						t.Fatal("header counter overlaps the content tabs")
					}
				}
			}
			if !found || rect.left < 0 || rect.right > app.width || rect.top < 0 || rect.bottom > app.height {
				t.Fatalf("counter %d unreachable in %v, modal=%t: %+v", kind, size, modal, rect)
			}
			for _, count := range app.progressCounts() {
				if count.kind == kind {
					row := strings.Split(app.View().Content, "\n")[rect.top]
					if text := ansi.Strip(ansi.Cut(row, rect.left, rect.right)); text != ansi.Strip(count.render(false)) {
						t.Fatalf("counter mouse region contains %q, want %q", text, ansi.Strip(count.render(false)))
					}
				}
			}
			app, cmd := clickMouseCmd(app, (rect.left+rect.right)/2, rect.top)
			if isQuit(cmd) || app.modal != noModal || app.activeTab != 0 {
				t.Fatal("clicking a progress counter did not navigate")
			}
			if modal && app.finishDraft != "Mouse navigation draft." {
				t.Fatal("clicking a counter discarded the overall draft")
			}
			app = pressKey(app, tea.KeyTab)
			if app.activeTab != 1 {
				t.Fatal("header counters intercepted file-tab navigation")
			}
		}
	}
	initAdaptiveStyles(true)
}

func TestProgressIncludesOverallThreadsOnce(t *testing.T) {
	app := progressTestApp(t, true)
	app.session.CJ.ReviewComments = []review.Comment{{ID: "overall", Scope: "review", Body: "Overall", Resolved: false}}
	if !app.jumpToProgress(progressThreads) || app.selectedCommentID() != "overall" || app.tab().path != overallCommentsPath {
		t.Fatal("thread counter did not navigate to the unresolved overall thread")
	}
	for _, count := range app.progressCounts() {
		if count.kind == progressThreads && (count.complete != 2 || count.total != 3) {
			t.Fatalf("overall threads counted twice: %+v", count)
		}
	}
}

func TestEmptyProgressDoesNotDismissSubmit(t *testing.T) {
	app, _ := newFinishTestApp(t, nil)
	app.openFinishModal()
	if app.jumpToProgress(progressThreads) || app.modal != finishModal {
		t.Fatal("empty counter dismissed Submit")
	}
	if !strings.Contains(ansi.Strip(app.renderHeader()), "0/0 💬") {
		t.Fatal("empty review lost its comment count")
	}
}

func TestHeaderProgressPositionDoesNotDependOnPathLength(t *testing.T) {
	for _, width := range []int{40, 80, 110} {
		app := progressTestApp(t, false)
		app.width = width
		app.tabs[0].attachment.Source = "a.md"
		_, short := app.renderHeaderWithProgress()
		app.tabs[0].attachment.Source = strings.Repeat("long-directory/", 12) + "review.md"
		_, long := app.renderHeaderWithProgress()
		if len(short) != len(long) || len(short) == 0 {
			t.Fatal("path length changed the counter layout")
		}
		for i := range short {
			if short[i] != long[i] {
				t.Fatalf("width %d: path length moved counter %d", width, i)
			}
			if i == len(short)-1 || short[i+1].rect.top != short[i].rect.top {
				want := width - headerStyle.GetHorizontalFrameSize() + headerStyle.GetPaddingLeft()
				if short[i].rect.right != want {
					t.Fatalf("width %d: right edge = %d, want %d", width, short[i].rect.right, want)
				}
			}
		}
	}
}

func TestUnansweredFormsCountersNavigate(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		app := progressTestApp(t, false)
		app, _ = updateApp(app, tea.WindowSizeMsg{Width: 110, Height: 30})
		app.openFinishModal()
		app.modalTextarea.SetValue("Keep this draft.")
		app.finishVerdict = review.VerdictApprove
		app.doFinish()
		content, _ := app.finishFormsContent(70)
		plain := ansi.Strip(content)
		if strings.Index(plain, "Required items") >= strings.Index(plain, "Review r") || strings.Index(plain, "Cancel esc") >= strings.Index(plain, "1/2") {
			t.Fatalf("unexpected confirmation order: %s", plain)
		}
		if mouse {
			for _, region := range app.modalMouseRegions() {
				if region.action.focus == finishProgressStart+int(progressRequiredChecks) && !region.action.dialog && !region.action.close {
					app, _ = clickMouseCmd(app, region.rect.left, region.rect.top)
					break
				}
			}
		} else {
			app = pressKey(app, tea.KeyTab)
			app = pressKey(app, tea.KeyTab)
			app = pressKey(app, tea.KeyEnter)
		}
		if app.modal != noModal || app.tab().cursorLine != 2 || !app.tab().cursorOnForm || app.finishDraft != "Keep this draft." {
			t.Fatal("confirmation counter did not navigate while preserving the draft")
		}
	}
}

func TestResolveConfirmationCountersNavigateWithoutApproving(t *testing.T) {
	for _, width := range []int{40, 110} {
		for _, mouse := range []bool{false, true} {
			for _, kind := range []progressKind{progressThreads, progressRequiredChecks, progressRequiredChoices, progressOptionalChecks, progressOptionalChoices} {
				app := progressTestApp(t, false)
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: width, Height: 24})
				app.toggleForm(2)
				app.toggleForm(3)
				app.openFinishModal()
				app.modalTextarea.SetValue("Keep resolve confirmation draft.")
				app.finishVerdict = review.VerdictApprove
				app.doFinish()
				if app.modal != finishResolveModal {
					t.Fatal("approval did not open Resolve All")
				}
				if mouse {
					for _, region := range app.modalMouseRegions() {
						if !region.action.dialog && !region.action.close && region.action.focus == finishProgressStart+int(kind) {
							app, _ = clickMouseCmd(app, region.rect.left, region.rect.top)
							break
						}
					}
				} else {
					for i := 0; i < int(kind)+1; i++ {
						app = pressKey(app, tea.KeyTab)
					}
					app = pressKey(app, tea.KeyEnter)
				}
				if app.modal != noModal || app.finishDraft != "Keep resolve confirmation draft." || app.session.CJ.RoundState.Finished || app.unresolvedTotal() != 1 {
					t.Fatalf("width=%d mouse=%t kind=%d: counter submitted, resolved, or lost draft", width, mouse, kind)
				}
			}
		}
	}
}

func TestTabFormIndicatorsShowOnlyPendingKinds(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app := progressTestApp(t, false)
		app, _ = updateApp(app, tea.WindowSizeMsg{Width: 40, Height: 16})
		if row := app.renderTabBar(); lipgloss.Width(row) > app.contentPaneWidth() || !strings.Contains(ansi.Strip(row), "☐○☐○") {
			t.Fatalf("small terminal lost the indicators or overflowed: %q", row)
		}
		app, _ = updateApp(app, tea.WindowSizeMsg{Width: 110, Height: 30})
		for _, step := range []struct {
			lines []int
			want  string
		}{
			{want: "☐○☐○"},
			{lines: []int{2}, want: "○☐○"},
			{lines: []int{3}, want: "☐○"},
			{lines: []int{5, 6}, want: "○"},
			{lines: []int{7}, want: ""},
		} {
			for _, line := range step.lines {
				app.toggleForm(line)
			}
			indicators := app.tabFormIndicators()
			if got := ansi.Strip(indicators[0]); got != step.want || indicators[1] != "" {
				t.Fatalf("indicators = %q, want %q on the form tab only", indicators, step.want)
			}
			label := ansi.Strip(app.tabLabels()[0].text)
			if step.want != "" && !strings.HasSuffix(label, step.want) {
				t.Fatalf("indicators are not at the title's right end: %q", label)
			}
		}
	}
	initAdaptiveStyles(true)
}
