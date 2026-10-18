package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/review"
)

func TestFormNavigationTargets(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  rune
		want []string
	}{
		{"changes", 'n', []string{"hunk:2", "confirm:2", "inline", "size:3", "publish:5", "hunk:1", "main"}},
		{"all", ']', []string{"confirm:2", "inline", "size:3", "resolved", "size:4", "publish:5", "main"}},
		{"unresolved", '}', []string{"confirm:2", "inline", "size:3", "main"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := formTestApp(t)
			for i := range app.forms.Documents[0].Items {
				if app.forms.Documents[0].Items[i].ID == "size" {
					app.forms.Documents[0].Items[i].Required = true
				}
			}
			app.tabs[0].state.Comments = []review.Comment{
				{ID: "file", Scope: "file", Body: "File"},
				{ID: "inline", StartLine: 2, EndLine: 2, Body: "Inline"},
				{ID: "resolved", StartLine: 3, EndLine: 3, Body: "Resolved", Resolved: true},
			}
			app.tabs[0].changeChunks = []changeChunk{{startLine: 2, endLine: 2}}
			app.tabs[1].state.Comments = []review.Comment{{ID: "main", StartLine: 1, EndLine: 1, Body: "Main"}}
			app.tabs[1].changeChunks = []changeChunk{{startLine: 1, endLine: 1}}
			app.selectComment(0, app.commentTargets(0)[0])
			position := func() string {
				if id := app.selectedCommentID(); id != "" {
					return id
				}
				if app.tab().cursorOnForm {
					return fmt.Sprintf("%s:%d", app.formRows[app.tab().path][app.tab().cursorLine].item.ID, app.tab().cursorLine)
				}
				return fmt.Sprintf("hunk:%d", app.tab().cursorLine)
			}
			for _, want := range tt.want {
				app = pressKey(app, tt.key)
				if got := position(); got != want || app.modal != noModal {
					t.Fatalf("%c reached %s (modal %v), want %s", tt.key, got, app.modal, want)
				}
			}
			if len(app.missingForms()) != 2 {
				t.Fatal("navigation changed the answers")
			}
			app = pressKey(app, tt.key)
			if tt.key == 'n' {
				if position() != "main" || app.modal != noModal {
					t.Fatal("change navigation did not stop at the end")
				}
			} else if app.modal != finishModal {
				t.Fatal("forward navigation did not open Submit at the end")
			} else {
				app = pressKey(app, tea.KeyEscape)
			}
			backward := map[rune]rune{'n': 'N', ']': '[', '}': '{'}[tt.key]
			for i := len(tt.want) - 2; i >= 0; i-- {
				app = pressKey(app, backward)
				if got := position(); got != tt.want[i] {
					t.Fatalf("%c reached %s, want %s", backward, got, tt.want[i])
				}
			}
			app = pressKey(app, backward)
			if position() != "file" {
				t.Fatal("backward navigation skipped the file comment")
			}
			app = pressKey(app, backward)
			want := "main"
			if backward == 'N' {
				want = "file"
			}
			if position() != want {
				t.Fatalf("backward boundary reached %s, want %s", position(), want)
			}
		})
	}
}

func TestRequiredFormNavigationUsesValidHumanAnswers(t *testing.T) {
	for _, human := range []bool{false, true} {
		t.Run(fmt.Sprintf("human=%t", human), func(t *testing.T) {
			app, _ := formTestApp(t)
			d := &app.forms.Documents[0]
			for i := range d.Items {
				d.Items[i].Required = true
			}
			d.Answers["confirm"] = form.Answer{Value: true, Human: human}
			d.Answers["size"] = form.Answer{Value: "large", Human: human}
			d.Answers["publish"] = form.Answer{Value: true, Human: human}
			for _, target := range app.reviewTargets(0) {
				if target.resolved != human {
					t.Fatalf("%s resolved=%t, want %t", target.formID, target.resolved, human)
				}
			}
			app.tabs[1].state.Comments = []review.Comment{{ID: "main", StartLine: 1, EndLine: 1}}
			app.tab().cursorLine = 1
			app = pressKey(app, '}')
			if human {
				if app.selectedCommentID() != "main" {
					t.Fatal("answered required items were not skipped")
				}
			} else if app.tab().cursorLine != 2 || !app.tab().cursorOnForm {
				t.Fatal("nonhuman answer resolved a required item")
			}
			app.activeTab = 0
			app.tab().cursorLine, app.tab().cursorOnForm = 1, false
			app = pressKey(app, 'n')
			if human && app.selectedCommentID() != "main" {
				t.Fatal("change navigation included answered required items")
			}
			if !human && (app.tab().cursorLine != 2 || !app.tab().cursorOnForm) {
				t.Fatal("change navigation skipped an unanswered required item")
			}
			app.activeTab = 0
			app.tab().cursorLine, app.tab().cursorOnForm = 1, false
			for _, line := range []int{2, 3, 4, 5} {
				app = pressKey(app, ']')
				if app.tab().cursorLine != line || !app.tab().cursorOnForm {
					t.Fatalf("all-item navigation reached %d, want %d", app.tab().cursorLine, line)
				}
			}
			app.selectComment(0, app.reviewTargets(0)[0])
			app = pressKey(app, ' ')
			if !app.reviewTargets(0)[0].unresolved() {
				t.Fatal("unchecking a required checkbox did not reopen it")
			}
		})
	}
}

func TestOptionalChoiceNavigationAndFormFocus(t *testing.T) {
	app, _ := formTestApp(t)
	app.tab().cursorLine = 2
	app = pressKey(app, ' ')
	for _, line := range []int{3, 4, 5} {
		app = pressKey(app, 'n')
		if app.tab().cursorLine != line || !app.tab().cursorOnForm {
			t.Fatalf("optional navigation reached %d, want %d", app.tab().cursorLine, line)
		}
	}
	app = pressKey(app, '?')
	app = typeSearch(app, "Small")
	if app.tab().cursorOnForm {
		t.Fatal("search kept form focus")
	}
	app = pressKey(app, tea.KeyEscape)
	if app.tab().cursorLine != 5 || !app.tab().cursorOnForm {
		t.Fatal("search cancellation did not restore form focus")
	}
	app = pressKey(app, 'N')
	if app.tab().cursorLine != 4 || !app.tab().cursorOnForm {
		t.Fatal("restored focus skipped an optional choice")
	}
	app = pressKey(app, 'k')
	if app.tab().cursorOnForm {
		t.Fatal("line movement kept form navigation focus")
	}
	app.tabs[1].state.Comments = []review.Comment{{ID: "main", StartLine: 1, EndLine: 1}}
	app = pressKey(app, '}')
	if app.selectedCommentID() != "main" {
		t.Fatal("unresolved navigation included optional form items")
	}
}

func TestFormNavigationFinishAndWrap(t *testing.T) {
	for _, key := range []rune{']', '}'} {
		t.Run(string(key), func(t *testing.T) {
			app, _ := formTestApp(t)
			app.tabs = app.tabs[:1]
			app.multiFile = false
			app.tab().cursorLine = 1
			app = pressKey(app, key)
			first := app.tab().cursorLine
			if first != 2 || !app.tab().cursorOnForm {
				t.Fatal("did not focus the first form control")
			}
			if key == ']' {
				for range 3 {
					app = pressKey(app, key)
				}
			}
			last := app.tab().cursorLine
			app = pressKey(app, key)
			if app.modal != finishModal || app.tab().cursorLine != last {
				t.Fatal("last form control did not open Submit without moving")
			}
			app = pressKey(app, tea.KeyEscape)
			app.selectComment(0, app.reviewTargets(0)[0])
			backward := '['
			if key == '}' {
				backward = '{'
			}
			app = pressKey(app, backward)
			if app.tab().cursorLine != last || !app.tab().cursorOnForm {
				t.Fatal("backward navigation did not wrap to the last matching control")
			}
		})
	}
}

func formTestApp(t *testing.T) (AppModel, chan FinishEvent) {
	t.Helper()
	return formTestAppWithContent(t, "# Form\n- [ ] Required confirmation with a long label that wraps <!-- form: id=confirm required -->\n- [ ] Small <!-- form: id=size value=small -->\n- [ ] Large <!-- form: id=size value=large -->\n- [x] Publish <!-- form: id=publish consent -->\n\n```md\n- [ ] Example\n```\n")
}

func formTestAppWithContent(t *testing.T, content string) (AppModel, chan FinishEvent) {
	t.Helper()
	sess, err := review.OpenSession(t.TempDir(), "forms")
	if err != nil {
		t.Fatal(err)
	}
	doc := review.AttachedDocument{ID: "a", Source: "/missing/form.md", Name: "form.md", Content: content}
	ch := make(chan FinishEvent, 1)
	app := NewApp("target.md", AppConfig{Session: sess, Documents: []review.AttachedDocument{doc}, Author: "Human", FinishCh: ch})
	for i := range app.tabs {
		app.tabs[i].state = &fileReview{}
		if app.tabs[i].attachment != nil {
			app.tabs[i].doc = document.FromContent(app.tabs[i].path, []byte(doc.Content))
		} else {
			app.tabs[i].doc = document.FromContent("target.md", []byte("target\n"))
		}
	}
	if err := app.beginForms(map[string]review.SnapshotFile{"target.md": {Content: "target\n"}}, ""); err != nil {
		t.Fatal(err)
	}
	app.width, app.height = 90, 24
	app.focused = contentPane
	app.recalculateLayout()
	app.tab().ensureHighlightCache()
	app.rebuildContent()
	app.markVisibleForms()
	return app, ch
}

func TestFormLinksTakePriorityOverSelection(t *testing.T) {
	app, _ := formTestAppWithContent(t, "- [ ] [Example](https://example.org) https://example.net <!-- form: id=link -->\n")
	app.width = 70
	app.recalculateLayout()
	app.rebuildContent()
	for _, uri := range []string{"https://example.org", "https://example.net"} {
		cells, _ := linkCells(app.contentViewport.GetContent(), uri)
		if len(cells) == 0 {
			t.Fatalf("no clickable cells for %s", uri)
		}
		left, top, _, _ := app.contentBounds()
		for _, point := range []tea.Mouse{cells[0], cells[len(cells)-1]} {
			_, cmd := app.handleMouseClick(tea.MouseClickMsg(tea.Mouse{X: left + point.X, Y: top + point.Y, Button: tea.MouseLeft}))
			if cmd == nil || app.forms.Documents[0].Answers["link"].Value == true {
				t.Fatalf("link %q at %+v (viewport %dx%d) resolved %q; cmd=%v answer=%+v", uri, point, app.contentViewport.Width(), app.contentViewport.Height(), app.referenceAt(left+point.X, top+point.Y), cmd != nil, app.forms.Documents[0].Answers["link"])
			}
		}
	}
	control := app.contentLayout.formControls[0]
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+app.contentViewport.Width()-1, top+control.rect.top)
	if app.forms.Documents[0].Answers["link"].Value != true {
		t.Fatal("trailing space did not toggle selection")
	}
	app = clickMouse(app, left+control.rect.left, top+control.rect.top)
	if app.forms.Documents[0].Answers["link"].Value == true {
		t.Fatal("checkbox did not toggle selection")
	}
	cells, _ := linkCells(app.contentViewport.GetContent(), "https://example.org")
	gap := cells[len(cells)-1]
	app = clickMouse(app, left+gap.X+1, top+gap.Y)
	if app.forms.Documents[0].Answers["link"].Value != true {
		t.Fatal("space between links did not toggle selection")
	}
}

func TestFormKeyboardMouseAndSourcePositions(t *testing.T) {
	app, _ := formTestApp(t)
	content := app.tab().doc.Content
	if len(app.contentLayout.formControls) < 4 {
		t.Fatalf("controls: %+v", app.contentLayout.formControls)
	}
	if strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "form:") {
		t.Fatal("metadata displayed")
	}
	if r := app.contentLayout.lineRanges[2]; r.end-r.start < 2 {
		t.Fatal("label did not wrap")
	}
	app.tab().cursorLine = 2
	app = pressKey(app, ' ')
	if len(app.missingForms()) != 0 {
		t.Fatalf("Space did not confirm: %+v", app.forms)
	}
	app.tab().cursorLine = 3
	app = pressKey(app, ' ')
	app.tab().cursorLine = 4
	app = pressKey(app, ' ')
	if answer := app.forms.Documents[0].Answers["size"]; answer.Value != "large" || !answer.Human {
		t.Fatalf("choice: %+v", answer)
	}
	for _, control := range app.contentLayout.formControls {
		if control.line != 5 {
			continue
		}
		left, top, _, _ := app.contentBounds()
		app = clickMouse(app, left+control.rect.left, top+control.rect.top-app.contentViewport.YOffset())
	}
	if app.forms.Documents[0].Answers["publish"].Value != true {
		t.Fatal("click did not consent")
	}
	if app.tab().doc.Content != content {
		t.Fatal("answer changed source snapshot")
	}
	app.tab().cursorLine = 8
	app = pressKey(app, ' ')
	if app.toggleForm(8) {
		t.Fatal("code example interactive")
	}
	// Labels and trailing space toggle; the gutter does not.
	control := app.contentLayout.formControls[0]
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+control.rect.left-1, top+control.rect.top-app.contentViewport.YOffset())
	if len(app.missingForms()) != 0 {
		t.Fatal("trailing whitespace toggled confirmation")
	}
	app = clickMouse(app, left+control.rect.right-1, top+control.rect.top-app.contentViewport.YOffset())
	if len(app.missingForms()) != 1 {
		t.Fatal("label click did not toggle")
	}
	app = clickMouse(app, left+control.rect.left, top+control.rect.top-app.contentViewport.YOffset())
	for _, wrapped := range app.contentLayout.formControls {
		if wrapped.line == 2 && wrapped.rect.top > control.rect.top {
			app = clickMouse(app, left+wrapped.rect.left, top+wrapped.rect.top-app.contentViewport.YOffset())
			break
		}
	}
	if len(app.missingForms()) != 1 {
		t.Fatal("wrapped label did not toggle")
	}
}

func TestMissingFormFinishesAsFeedbackWithoutResolving(t *testing.T) {
	app, ch := formTestApp(t)
	app.tab().state.Comments = []review.Comment{testComment()}
	app.newFeedback = false
	app.toggleForm(5)
	if app.defaultFinishVerdict() != review.VerdictComment {
		t.Fatal("missing confirmation allows approval")
	}
	_, cmd := app.doFinish()
	if !isQuit(cmd) {
		t.Fatalf("finish failed: %v", app.err)
	}
	ev := takeEvent(t, ch)
	if ev.Approved || ev.Result == nil || len(ev.Result.Consents) != 0 || app.tab().state.Comments[0].Resolved {
		t.Fatalf("feedback granted approval: %+v", ev)
	}
}

func TestFormApprovalGrantsHumanConsent(t *testing.T) {
	app, ch := formTestApp(t)
	app.toggleForm(2)
	app.toggleForm(3)
	app.toggleForm(5)
	_, cmd := app.doFinish()
	if !isQuit(cmd) {
		t.Fatal(app.err)
	}
	ev := takeEvent(t, ch)
	if !ev.Approved || !ev.Result.Consents["a"]["publish"] {
		t.Fatalf("human consent lost: %+v", ev)
	}
	if _, err := os.Stat(filepath.Join(app.session.Dir, "result-1.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFormsDoNotToggleWhileSelecting(t *testing.T) {
	app, _ := formTestApp(t)
	app.tab().cursorLine, app.tab().selecting = 2, true
	app.handleKeyPress(tea.KeyPressMsg{Code: ' '})
	if len(app.missingForms()) != 1 {
		t.Fatal("selection toggled form")
	}
}

func TestFormsWithDeletedBinaryTarget(t *testing.T) {
	app, _ := formTestApp(t)
	app.tabs = append(app.tabs, FileTab{path: "missing-deleted.bin", status: "deleted", isBinary: true, isDeleted: true})
	if err := app.beginForms(map[string]review.SnapshotFile{"target.md": {Content: "target\n"}}, ""); err != nil {
		t.Fatal(err)
	}
	if target := app.forms.Input.Files["missing-deleted.bin"]; !target.Binary || target.Status != "deleted" {
		t.Fatalf("lost deleted binary target: %+v", target)
	}
}

func TestFormAppearanceAndFinishDetailsFit(t *testing.T) {
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		app, _ := formTestApp(t)
		for _, selected := range []bool{false, true} {
			app.tab().selecting = selected
			app.tab().cursorLine, app.tab().selectAnchor = 2, 2
			app.rebuildContent()
			if !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "Required") {
				t.Fatal("selection hid required marker")
			}
		}
		app.tab().selecting = false
		app.toggleForm(2)
		if !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "Required") {
			t.Fatal("confirmation hid required marker")
		}
		app.forms.Documents[0].Items[3].Label = strings.Repeat("Long operation and exact target. ", 30)
		app.openFinishModal()
		view := app.View().Content
		if height := strings.Count(view, "\n") + 1; height > app.height {
			t.Fatalf("finish details do not fit: %d > %d", height, app.height)
		}
		if !strings.Contains(ansi.Strip(view), "details") {
			t.Fatal("long details not scrollable")
		}
	}
	initAdaptiveStyles(true)
}

func TestSubmissionSaveRetryPreservesDecisionAndComment(t *testing.T) {
	for _, verdict := range []review.Verdict{review.VerdictApprove, review.VerdictReject, review.VerdictComment} {
		t.Run(string(verdict), func(t *testing.T) {
			app, ch := formTestApp(t)
			app.toggleForm(2)
			app.toggleForm(3)
			app.toggleForm(5)
			app.openFinishModal()
			app.modalTextarea.SetValue("Overall comment retained on retry.")
			blocker := filepath.Join(app.session.Dir, "result-1.json")
			if err := os.Mkdir(blocker, 0o700); err != nil {
				t.Fatal(err)
			}
			_, cmd := app.submitReview(verdict)
			if isQuit(cmd) || !app.finishSaveFailed || app.session.CJ.RoundState.Finished {
				t.Fatal("save failure submitted review")
			}
			select {
			case <-ch:
				t.Fatal("save failure emitted a result")
			default:
			}
			if app.forms.Documents[0].Answers["confirm"].Value != true {
				t.Fatal("save failure lost confirmation")
			}
			id := app.session.CJ.RoundState.ReviewCommentID
			if err := os.Remove(blocker); err != nil {
				t.Fatal(err)
			}
			_, cmd = app.doFinish()
			if !isQuit(cmd) {
				t.Fatal(app.err)
			}
			ev := takeEvent(t, ch)
			if ev.Result.Verdict != verdict || ev.Result.ReviewComment.ID != id || len(app.session.CJ.ReviewComments) != 1 {
				t.Fatalf("retry changed submission: %+v", ev.Result)
			}
			if ev.Approved != (verdict == review.VerdictApprove) || ev.Result.Consents["a"]["publish"] != ev.Approved {
				t.Fatalf("incorrect approval or consent: %+v", ev.Result)
			}
		})
	}
}
