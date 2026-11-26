package tui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/v2/internal/document"
	"github.com/crittylabs/critty/v2/internal/form"
	"github.com/crittylabs/critty/v2/internal/icons"
	"github.com/crittylabs/critty/v2/internal/review"
)

func TestFormNavigationTargets(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		key  rune
		want []string
	}{
		{"changes", 'n', []string{"hunk:2", "confirm:2", "inline", "size:3", "resolved", "size:4", "publish:5", "hunk:1", "main"}},
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
	t.Parallel()
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
			if app.tab().cursorLine != 2 || !app.tab().cursorOnForm {
				t.Fatal("change navigation skipped a required item")
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
	t.Parallel()
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
	t.Parallel()
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
	app := NewApp("target.md", AppConfig{MathRaTeX: "off", Session: sess, Documents: []review.AttachedDocument{doc}, Author: "Human", FinishCh: ch})
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
	t.Parallel()
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

func TestUnannotatedChecklistIsDisplayOnly(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "- [ ] Pending\n- [x] Complete\n- [ ] Answer <!-- form: id=answer -->\n")
	if len(app.contentLayout.formControls) != 1 || len(app.formReviewItems()) != 1 {
		t.Fatal("ordinary checklist entered the form controls or answers")
	}
	for _, line := range []int{1, 2} {
		if app.toggleForm(line) {
			t.Fatalf("ordinary checklist line %d was interactive", line)
		}
	}
	control := app.contentLayout.formControls[0]
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+control.rect.left, top+control.rect.top)
	if app.forms.Documents[0].Answers["answer"].Value != true {
		t.Fatal("annotated checkbox was not clickable")
	}
}

func TestFormKeyboardMouseAndSourcePositions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	app, ch := formTestApp(t)
	app.toggleForm(2)
	app.toggleForm(3)
	app.toggleForm(5)
	_, cmd := app.doFinish()
	if !isQuit(cmd) {
		t.Fatal(app.err)
	}
	ev := takeEvent(t, ch)
	if !ev.Approved || !ev.Result.Consents["a"]["publish"] || ev.Result.ImageBase != app.session.Dir {
		t.Fatalf("human consent lost: %+v", ev)
	}
	if _, err := os.Stat(filepath.Join(app.session.Dir, "result-1.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFormSelectionNeedsMultipleLines(t *testing.T) {
	t.Parallel()
	app, _ := formTestApp(t)
	app.tab().cursorLine = 2
	app = pressKey(app, 'v')
	app = pressKey(app, ' ')
	if len(app.missingForms()) != 1 {
		t.Fatal("single-line selection toggled a form control")
	}
}

func TestToggleFormControlsInSelectedLines(t *testing.T) {
	t.Parallel()
	const source = `# Form
- [x] Outside choice <!-- form: id=size value=outside -->
- [x] Existing check <!-- form: id=existing -->
- [ ] First choice in range <!-- form: id=size value=small -->
- [ ] Second choice in range <!-- form: id=size value=large -->
- [ ] Mode <!-- form: id=mode value=auto -->
- [ ] Consent <!-- form: id=allow consent -->
- [ ] Tagged select-all <!-- form: id=all select-all=outside -->
- [ ] Text input <!-- form: id=note text -->
- [ ] Outside checkbox <!-- form: id=outside tags=outside -->
`
	app, _ := formTestAppWithContent(t, source)
	app.tab().cursorLine = 9
	app = pressKey(app, 'v')
	for range 6 {
		app = pressKey(app, 'k')
	}
	if start, end := app.selectionRange(); start != 3 || end != 9 {
		t.Fatalf("selected lines %d-%d, want 3-9", start, end)
	}
	for _, tc := range []struct {
		name       string
		checked    bool
		size, mode string
	}{
		{"select", true, "small", "auto"},
		{"clear", false, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app = pressKey(app, ' ')
			if app.modal != noModal || !app.tab().selecting || app.err != nil {
				t.Fatalf("range toggle changed selection mode or failed: modal = %v, err = %v", app.modal, app.err)
			}
			saved, err := app.session.LoadForms()
			if err != nil {
				t.Fatal(err)
			}
			answers := saved.Documents[0].Answers
			for id, want := range map[string]any{"existing": tc.checked, "size": tc.size, "mode": tc.mode, "allow": tc.checked} {
				if answer := answers[id]; answer.Value != want || !answer.Human || answer.Via != "" {
					t.Errorf("%s = %+v, want human answer %v", id, answer, want)
				}
			}
			for _, id := range []string{"all", "note", "outside"} {
				if answer := answers[id]; answer.Human {
					t.Errorf("%s changed outside the range targets: %+v", id, answer)
				}
			}
		})
	}
	if app.tab().doc.Content != source {
		t.Fatal("range toggle changed the source")
	}
}

func TestFormSelectionLeavesAnswersUntouchedWhenPathIsInvalid(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "- [ ] Check <!-- form: id=check -->\n- [ ] File <!-- form: id=file checkbox type=file existing default='missing.md' -->\n")
	app.session.Meta.CWD = t.TempDir()
	d := app.forms.Document(app.tab().path)
	before := maps.Clone(d.Answers)
	app = pressKey(app, 'v')
	app = pressKey(app, 'j')
	app = pressKey(app, ' ')
	if app.modal != formTextModal || app.editingFormLine != 2 || !reflect.DeepEqual(d.Answers, before) {
		t.Fatalf("invalid path: modal = %v, line = %d, answers = %+v", app.modal, app.editingFormLine, d.Answers)
	}
}

func TestFormsWithDeletedBinaryTarget(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestSelectAllForm(t *testing.T) {
	t.Parallel()
	for _, symbols := range []icons.Set{icons.Plain, icons.Nerd} {
		t.Run(fmt.Sprintf("nerd=%t", symbols), func(t *testing.T) {
			app, _ := formTestAppWithContent(t, "- [ ] Check all <!-- form: id=all select-all=api -->\n- [ ] Auth <!-- form: id=auth tags=api -->\n- [ ] Errors <!-- form: id=errors tags=api consent -->\n")
			app.icons = symbols
			app.rebuildContent()
			line := func() string {
				return ansi.Strip(strings.Split(app.contentViewport.GetContent(), "\n")[0])
			}
			gap := "  "
			if symbols {
				gap = ""
			}
			if !strings.Contains(line(), symbols.Glyph(icons.Unchecked, "☐")+gap+"All Check all (0/2) 1 consent") {
				t.Fatalf("initial display: %q", line())
			}
			if len(app.formReviewItems()) != 2 {
				t.Fatal("select-all item counted as an answer")
			}
			app.toggleForm(1)
			d := app.forms.Document(app.tab().path)
			if !strings.Contains(line(), symbols.Glyph(icons.Checked, "✓")+gap+"All Check all (2/2)") || d.Answers["errors"].Via != "all" || !d.Answers["errors"].Human {
				t.Fatalf("select all: %q %+v", line(), d.Answers)
			}
			if !app.tab().cursorOnForm || app.tab().cursorLine != 1 {
				t.Fatal("focus left the select-all item")
			}
			app.toggleForm(2)
			if !strings.Contains(line(), symbols.Glyph(icons.Mixed, "⊟")+gap+"All Mixed Check all (1/2)") || d.Answers["auth"].Via != "" {
				t.Fatalf("mixed: %q %+v", line(), d.Answers)
			}
			app.toggleForm(1)
			app.toggleForm(1)
			if !strings.Contains(line(), symbols.Glyph(icons.Unchecked, "☐")+gap+"All Check all (0/2)") {
				t.Fatalf("clear all: %q", line())
			}
			saved, err := app.session.LoadForms()
			if err != nil || saved.Version != 2 || saved.Documents[0].Answers["auth"].Value != false || !saved.Documents[0].Answers["auth"].Human {
				t.Fatalf("saved: %v %+v", err, saved)
			}
		})
	}
}

func TestFormTextAnswers(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "- [ ] Reason <!-- form: id=reason text required -->\n- [ ] Other <!-- form: id=q value=other text -->\n- [ ] Plain <!-- form: id=q value=plain -->\n")
	d := app.forms.Document(app.tab().path)
	content := func() string { return ansi.Strip(app.contentViewport.GetContent()) }
	typeAnswer := func(text string) {
		app.modalTextarea.SetValue(text)
		app = clickModalAction(t, app, "Save")
	}
	if !strings.Contains(content(), "✎  Reason") {
		t.Fatalf("text field badge: %q", content())
	}
	app.tab().cursorLine = 1
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeySpace})
	if app.modal != formTextModal {
		t.Fatal("Space did not open the text field")
	}
	typeAnswer("  Because it is simpler.\nSecond line  ")
	if app.modal != noModal || d.Answers["reason"].Value != "Because it is simpler.\nSecond line" || !d.Answers["reason"].Human || len(app.missingForms()) != 0 {
		t.Fatalf("saved text: %+v", d.Answers["reason"])
	}
	if !strings.Contains(content(), "Because it is simpler.") || !strings.Contains(content(), "Second line") {
		t.Fatalf("text not shown: %q", content())
	}
	app.toggleForm(1)
	typeAnswer("See ![diagram](attachments/diagram.png)")
	if path, _ := app.commentImagePath("attachments/diagram.png"); !slices.ContainsFunc(slices.Collect(maps.Keys(app.commentImages)), func(key commentImageKey) bool { return key.target.path == path }) {
		t.Fatal("answer image was not registered for preview")
	}
	app.toggleForm(1)
	typeAnswer("")
	if d.Answers["reason"].Value != "" || len(app.missingForms()) != 1 {
		t.Fatal("clearing the text did not unanswer the field")
	}

	app.toggleForm(2)
	if app.modal != formTextModal || d.Answers["q"].Value != "other" {
		t.Fatal("choice with text did not open the editor after selection")
	}
	typeAnswer("Use exponential backoff")
	app.toggleForm(3)
	if d.Answers["q"].Value != "plain" || strings.Contains(content(), "exponential") {
		t.Fatalf("note shown for another choice: %+v", d.Answers["q"])
	}
	app.toggleForm(2)
	if app.modalTextarea.Value() != "Use exponential backoff" {
		t.Fatalf("draft note not restored: %q", app.modalTextarea.Value())
	}
	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal || d.Answers["q"].Value != "other" || !strings.Contains(content(), "exponential") {
		t.Fatalf("closing the editor changed the choice: %v %+v", app.modal, d.Answers["q"])
	}
}

func TestFormListModal(t *testing.T) {
	t.Parallel()
	for _, symbols := range []icons.Set{icons.Plain, icons.Nerd} {
		t.Run(fmt.Sprintf("nerd=%t", symbols), func(t *testing.T) {
			app, _ := formTestAppWithContent(t, "# Questions\n\n- [ ] All <!-- form: id=all select-all=r -->\n\n## Q1. Retry?\n\n- [ ] Bounded <!-- form: id=retry value=bounded tags=r -->\n- [ ] Manual <!-- form: id=retry value=manual -->\n\n## Q2. Why?\n\n- [ ] Reason <!-- form: id=reason text required -->\n- [ ] Publish <!-- form: id=publish consent -->\n")
			app.icons = symbols
			app.rebuildContent()
			app.toggleForm(8)
			view := func() string { return ansi.Strip(app.View().Content) }
			if !strings.Contains(app.renderFooter(), "Answers") {
				t.Fatal("no answer list button")
			}
			app = pressKey(app, 'F')
			if app.modal != formListModal {
				t.Fatal("F did not open the answer list")
			}
			for _, want := range []string{"Q1. Retry?", symbols.Join(symbols.Glyph(icons.Check, "✓"), "retry · Manual"), "Q2. Why?", symbols.Join(symbols.Glyph(icons.Warning, "!"), "Reason"), symbols.Join(symbols.Glyph(icons.RadioUnchecked, "○"), "Publish"), "1 of 3 answered"} {
				if !strings.Contains(view(), want) {
					t.Fatalf("missing %q in %q", want, view())
				}
			}
			for _, row := range app.formListRows() {
				if row.item.SelectAll != nil {
					t.Fatal("select-all item listed as an answer")
				}
			}
			app = pressKey(app, 'u')
			if strings.Contains(view(), "retry") || app.formList.selected != 0 {
				t.Fatalf("filter kept answered items: %q", view())
			}
			app = pressKey(app, 'j')
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
			if app.modal != noModal || app.tab().cursorLine != 13 || !app.tab().cursorOnForm || app.focused != contentPane {
				t.Fatalf("Enter did not go to Publish: modal %v line %d", app.modal, app.tab().cursorLine)
			}
			app = pressKey(app, 'F')
			for _, region := range app.modalMouseRegions() {
				if region.action.pick && region.action.pickIndex == 0 {
					app = clickMouse(app, region.rect.left, region.rect.top)
					break
				}
			}
			if app.modal != noModal || app.tab().cursorLine != 12 {
				t.Fatalf("click did not go to Reason: modal %v line %d", app.modal, app.tab().cursorLine)
			}
		})
	}
}

func TestRadioGroupBrackets(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] Review all <!-- form: id=review select-all=review -->
- [ ] Brief <!-- form: id=length value=brief tags=review -->
- [ ] Detailed <!-- form: id=length value=detailed -->
- [ ] Check wording <!-- form: id=wording tags=review -->
`)
	rows := strings.Split(app.contentViewport.GetContent(), "\n")
	for line, want := range []string{" ", "⎛", "⎝", " "} {
		row := rows[app.contentLayout.lineRanges[line+1].start]
		column := app.sourceGutterWidth()
		if got := ansi.Strip(ansi.Cut(row, column, column+1)); got != want {
			t.Errorf("line %d bracket = %q, want %q", line+1, got, want)
		}
	}
}

func TestRankGroupBrackets(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] Alpha <!-- form: id=first rank value=a -->
- [ ] Beta <!-- form: id=first rank value=b -->
- [ ] Gamma <!-- form: id=second rank value=c -->
- [ ] Delta <!-- form: id=second rank value=d -->
- [ ] Epsilon <!-- form: id=first rank value=e -->
- [ ] Zeta <!-- form: id=first rank value=f -->

Unrelated text

- [ ] Eta <!-- form: id=first rank value=g -->
`)
	want := map[int]rune{1: '⎛', 2: '⎝', 3: '⎛', 4: '⎝', 5: '⎛', 6: '⎝', 10: '⎜'}
	for _, numbers := range []bool{true, false} {
		app.hideLineNumbers = !numbers
		app.rebuildContent()
		rows := strings.Split(app.contentViewport.GetContent(), "\n")
		colors := formGroupColors(2, lipgloss.LightDark(terminalIsDark))
		column := app.sourceGutterWidth()
		for line, bracket := range want {
			runes, cells := renderedCells(rows[app.contentLayout.lineRanges[line].start])
			group := 0
			if line == 3 || line == 4 {
				group = 1
			}
			if runes[column] != bracket || !sameColor(cells[column].Fg, colors[group]) {
				t.Errorf("line %d with line numbers %t: got %q in %v, want %q in %v", line, numbers, runes[column], cells[column].Fg, bracket, colors[group])
			}
		}
		if row := ansi.Strip(rows[app.contentLayout.lineRanges[8].start]); strings.ContainsAny(row, "⎛⎜⎝") {
			t.Errorf("unrelated text has a bracket: %q", row)
		}
	}
}

func assertRankAnswer(t *testing.T, doc *review.FormDocument, id string, order []string, parents map[string]string) {
	t.Helper()
	answer := doc.Answers[id]
	nodes, ranked := form.RankTree(doc.Items, id, answer)
	if !ranked || !answer.Human || !maps.Equal(answer.RankParents, parents) || len(nodes) != len(order) {
		t.Fatalf("%s answer = %+v, want human order %v with parents %v", id, answer, order, parents)
	}
	for i, node := range nodes {
		if node.Value != order[i] {
			t.Fatalf("%s item %d = %q, want %q", id, i+1, node.Value, order[i])
		}
	}
}

func rankControlRect(t *testing.T, app AppModel, line int) mouseRect {
	t.Helper()
	for _, control := range app.contentLayout.formControls {
		if control.line == line {
			return control.rect
		}
	}
	t.Fatalf("no ranking control at line %d", line)
	return mouseRect{}
}

func dragRankUnder(t *testing.T, app AppModel, from, to int) AppModel {
	t.Helper()
	start, target := rankControlRect(t, app, from), rankControlRect(t, app, to)
	x, y := contentScreenPoint(app, start.left, start.top)
	tx, ty := contentScreenPoint(app, max(start.left+rankIndentWidth, target.left), target.top)
	app = pressMouse(app, x, y)
	app = moveMouse(app, tx, ty)
	return releaseMouse(app, tx, ty)
}

func TestRankFieldReordersInPlace(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "# Lead\n1. [ ] Alpha <!-- form: id=lead rank value=a -->\n2. [ ] Beta <!-- form: id=lead rank value=b -->\n3. [ ] Gamma <!-- form: id=lead rank value=c -->\n\nAfter\n")
	d := app.forms.Document(app.tab().path)
	rows := func() []string {
		var labels []string
		for _, line := range strings.Split(ansi.Strip(app.contentViewport.GetContent()), "\n") {
			for _, label := range []string{"Alpha", "Beta", "Gamma", "After"} {
				if strings.Contains(line, label) {
					labels = append(labels, label)
				}
			}
		}
		return labels
	}
	assertPendingDot := func(want bool) {
		t.Helper()
		rows := strings.Split(app.contentViewport.GetContent(), "\n")
		row := rows[app.contentLayout.lineRanges[2].start]
		runes, styles := renderedCells(row)
		if got := runes[0] == '●'; got != want {
			t.Fatalf("unanswered dot present = %t, want %t in %q", got, want, row)
		}
		if want && !sameColor(styles[0].Fg, unreadDotStyle.GetForeground()) {
			t.Fatalf("unanswered dot color = %v, want %v", styles[0].Fg, unreadDotStyle.GetForeground())
		}
	}
	assertPendingDot(true)
	if len(app.missingForms()) != 0 {
		t.Fatal("optional ranking requires an answer")
	}
	if content := ansi.Strip(app.contentViewport.GetContent()); !strings.Contains(content, " 1  Alpha") || !strings.Contains(content, " 3  Gamma") {
		t.Fatalf("unranked items lack numbers: %q", content)
	}
	app.tab().cursorLine, app.tab().cursorOnForm = 2, true
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt})
	assertRankAnswer(t, d, "lead", []string{"a", "b", "c"}, nil)
	assertPendingDot(false)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	if got := rows(); !slices.Equal(got, []string{"Beta", "Alpha", "Gamma", "After"}) {
		t.Fatalf("display order: %v", got)
	}
	assertRankAnswer(t, d, "lead", []string{"b", "a", "c"}, nil)
	if app.tab().cursorLine != 2 || app.rankMove(app.tab(), 2) != 1 || app.rankMove(app.tab(), 3) != -1 || app.rankMove(app.tab(), 4) != 0 {
		t.Fatal("moved rows are not tracked")
	}
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 4 {
		t.Fatalf("cursor did not follow display order: line %d", app.tab().cursorLine)
	}
	start, target := rankControlRect(t, app, 4), rankControlRect(t, app, 2)
	x, y := contentScreenPoint(app, gutterWidth+4, start.top)
	app = pressMouse(app, x, y)
	_, y = contentScreenPoint(app, gutterWidth+4, target.top)
	app = moveMouse(app, x, y)
	assertRankAnswer(t, d, "lead", []string{"b", "a", "c"}, nil)
	app = releaseMouse(app, x, y)
	if got := rows(); !slices.Equal(got, []string{"Beta", "Gamma", "Alpha", "After"}) {
		t.Fatalf("dragging did not move Gamma to Alpha: %v", got)
	}
	assertRankAnswer(t, d, "lead", []string{"b", "c", "a"}, nil)
}

func TestCommentOnRankedItemNamesTheItem(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, "Intro\n1. [ ] Alpha <!-- form: id=lead rank value=a -->\n2. [ ] Beta <!-- form: id=lead rank value=b -->\n3. [ ] Gamma <!-- form: id=lead rank value=c -->\n\nAfter\n")
	t1 := app.tab()
	t1.cursorLine, t1.cursorOnForm = 2, true
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	// Display order is now Beta, Alpha, Gamma.
	t1 = app.tab()
	t1.cursorOnForm = false
	t1.selecting, t1.selectAnchor, t1.cursorLine = true, 3, 2
	if start, end := app.selectionRange(); start != 2 || end != 2 {
		t.Fatalf("selection inside a reordered ranking: %d-%d", start, end)
	}
	// Intro through Alpha would skip Beta, displayed between them.
	t1.selectAnchor, t1.cursorLine = 1, 2
	if start, end := app.selectionRange(); start != 1 || end != 3 {
		t.Fatalf("selection entering a reordered ranking: %d-%d", start, end)
	}
	t1.selecting, t1.cursorLine = false, 2
	app.openLineComment()
	app.modalTextarea.SetValue("Alpha belongs lower")
	app.modalSubmit()
	comments := app.tab().state.Comments
	if len(comments) != 1 || comments[0].StartLine != 2 || !strings.Contains(comments[0].Anchor, "value=a") {
		t.Fatalf("comment does not name Alpha: %+v", comments)
	}
}

func TestReorderedRegions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		order []int
		want  [][2]int
	}{
		{[]int{0, 1, 2}, nil},
		{[]int{0, 2, 1, 3}, [][2]int{{2, 3}}},
		{[]int{1, 0, 2, 4, 5, 3}, [][2]int{{1, 2}, {4, 6}}},
	} {
		if got := reorderedRegions(tt.order); !slices.Equal(got, tt.want) {
			t.Errorf("%v: got %v, want %v", tt.order, got, tt.want)
		}
	}
}

func TestNestedRankingMovesSubtrees(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `Overview
- [ ] Alpha <!-- form: id=lead rank value=a -->
  - [ ] Child <!-- form: id=lead rank value=child -->
    - [ ] Grandchild <!-- form: id=lead rank value=grandchild -->
  - [ ] Sibling <!-- form: id=lead rank value=sibling -->
- [ ] Beta <!-- form: id=lead rank value=b -->
  - [ ] Other child <!-- form: id=lead rank value=other -->
`)
	doc := app.forms.Document(app.tab().path)
	move := func(line int, key rune, mod tea.KeyMod) {
		t.Helper()
		app.tab().cursorLine = line
		app, _ = updateApp(app, tea.KeyPressMsg{Code: key, Mod: mod})
		if app.tab().cursorLine != line {
			t.Fatalf("cursor moved away from source line %d", line)
		}
	}
	move(2, tea.KeyDown, tea.ModAlt)
	assertRankAnswer(t, doc, "lead", []string{"b", "other", "a", "child", "grandchild", "sibling"},
		map[string]string{"other": "b", "child": "a", "grandchild": "child", "sibling": "a"})
	move(3, tea.KeyLeft, tea.ModAlt)
	assertRankAnswer(t, doc, "lead", []string{"b", "other", "a", "sibling", "child", "grandchild"},
		map[string]string{"other": "b", "grandchild": "child", "sibling": "a"})
	move(3, tea.KeyUp, tea.ModAlt)
	move(3, tea.KeyRight, tea.ModAlt)
	assertRankAnswer(t, doc, "lead", []string{"b", "other", "child", "grandchild", "a", "sibling"},
		map[string]string{"other": "b", "child": "b", "grandchild": "child", "sibling": "a"})

	app = dragRankUnder(t, app, 6, 2)
	values := []string{"a", "sibling", "b", "other", "child", "grandchild"}
	parents := map[string]string{"b": "a", "other": "b", "child": "b", "grandchild": "child", "sibling": "a"}
	assertRankAnswer(t, doc, "lead", values, parents)
	if got := app.lineOrder(app.tab()); !slices.Equal(got[:7], []int{0, 1, 4, 5, 6, 2, 3}) {
		t.Fatalf("visible source lines: %v", got)
	}
	root, grandchild := rankControlRect(t, app, 2), rankControlRect(t, app, 4)
	if grandchild.left-root.left != 3*rankIndentWidth {
		t.Fatalf("grandchild indentation: root %v, grandchild %v", root, grandchild)
	}
	app = dragRankUnder(t, app, 2, 4) // A branch cannot be dropped onto its own descendant.
	assertRankAnswer(t, doc, "lead", values, parents)

	saved, err := app.session.LoadForms()
	if err != nil {
		t.Fatal(err)
	}
	result := saved.Result(true, 1)
	if result.Version != 4 {
		t.Fatalf("nested result version = %d, want 4", result.Version)
	}
	assertRankAnswer(t, result.Document(doc.Path()), "lead", values, parents)
	app.tab().markdownSourceView = true
	if got := app.lineOrder(app.tab()); !slices.Equal(got[:7], []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Fatalf("source view order changed: %v", got)
	}
}

func TestFlatRankingCanGainAndLoseChildren(t *testing.T) {
	t.Parallel()
	for _, keys := range []struct {
		name            string
		indent, outdent rune
	}{
		{"Alt arrows", tea.KeyRight, tea.KeyLeft},
		{"Ghostty Option arrows", 'f', 'b'},
	} {
		t.Run(keys.name, func(t *testing.T) {
			app, _ := formTestAppWithContent(t, `- [ ] Alpha <!-- form: id=lead rank value=a -->

- [ ] Beta <!-- form: id=lead rank value=b -->
  with a continuation
`)
			doc := app.forms.Document(app.tab().path)
			app.tab().cursorLine = 3
			app, _ = updateApp(app, tea.KeyPressMsg{Code: keys.indent, Mod: tea.ModAlt})
			if got := doc.Answers["lead"].RankParents["b"]; got != "a" {
				t.Fatalf("indent with %v: Beta's parent = %q", keys.indent, got)
			}
			if got := app.forms.Result(true, 1).Version; got != 4 {
				t.Fatalf("nested result version = %d", got)
			}
			app, _ = updateApp(app, tea.KeyPressMsg{Code: keys.outdent, Mod: tea.ModAlt})
			if got := doc.Answers["lead"].RankParents; len(got) != 0 {
				t.Fatalf("outdent with %v: unexpected hierarchy %v", keys.outdent, got)
			}
			if got := app.forms.Result(true, 1).Version; got != 2 {
				t.Fatalf("flat result version = %d", got)
			}
		})
	}
}

func TestRankMovesPastParentAtSiblingBoundary(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		key  rune
		line int
		want []string
	}{
		{"first child up", tea.KeyUp, 3, []string{"root", "first", "leaf", "parent", "last", "next"}},
		{"last child down", tea.KeyDown, 5, []string{"root", "parent", "first", "leaf", "last", "next"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := formTestAppWithContent(t, `- [ ] Root <!-- form: id=r rank value=root -->
  - [ ] Parent <!-- form: id=r rank value=parent -->
    - [ ] First <!-- form: id=r rank value=first -->
      - [ ] Leaf <!-- form: id=r rank value=leaf -->
    - [ ] Last <!-- form: id=r rank value=last -->
  - [ ] Next <!-- form: id=r rank value=next -->
`)
			app.tab().cursorLine = tt.line
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tt.key, Mod: tea.ModAlt})
			d := app.forms.Document(app.tab().path)
			parents := map[string]string{"parent": "root", "first": "parent", "leaf": "first", "last": "parent", "next": "root"}
			if tt.key == tea.KeyUp {
				parents["first"] = "root"
			} else {
				parents["last"] = "root"
			}
			assertRankAnswer(t, d, "r", tt.want, parents)
		})
	}
}

func TestRankHorizontalDragChangesDepth(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] First <!-- form: id=r rank value=first -->
  - [ ] Child <!-- form: id=r rank value=child -->
  - [ ] Sibling <!-- form: id=r rank value=sibling -->
- [ ] Second <!-- form: id=r rank value=second -->
  - [ ] Leaf <!-- form: id=r rank value=leaf -->
`)
	d := app.forms.Document(app.tab().path)
	drag := func(line, delta int, moving ...string) {
		t.Helper()
		rect := rankControlRect(t, app, line)
		x, y := contentScreenPoint(app, rect.left, rect.top)
		before := ansi.Strip(app.contentViewport.GetContent())
		checkLabels := func(dragging bool) {
			t.Helper()
			runes, cells := renderedCells(app.contentViewport.GetContent())
			for _, label := range []string{"Child", "Second", "Leaf", "Sibling"} {
				prefix, _, found := strings.Cut(string(runes), label)
				if !found {
					t.Fatalf("missing rank label %q", label)
				}
				want := dragging && slices.Contains(moving, label)
				at := len([]rune(prefix))
				style := cells[at]
				if got := sameColor(style.Fg, formChip.rankDragForeground); got != want {
					t.Fatalf("dimmed %q = %t, want %t", label, got, want)
				}
				background := formChip.rankBackground
				if label == moving[0] {
					background = buttonBackground
				}
				if want {
					background = tintedBackground(background, 60)
				}
				if !sameColor(style.Bg, background) {
					t.Fatalf("%q background = %v, want %v", label, style.Bg, background)
				}
				badge := formChip.rankPendingBadge
				if d.Answers["r"].Human {
					badge = formChip.rankSavedBadge
				}
				if want {
					badge = tintedBackground(badge, 75)
				}
				number := cells[at-3]
				if !sameColor(number.Bg, badge) || !sameColor(number.Fg, formChip.rankBadgeSymbol) {
					t.Fatalf("%q number style = %v, want background %v and foreground %v", label, number, badge, formChip.rankBadgeSymbol)
				}
			}
		}
		app = pressMouse(app, x, y)
		app = moveMouse(app, x+delta, y)
		checkLabels(true)
		if line == 4 {
			during := ansi.Strip(app.contentViewport.GetContent())
			for _, pair := range []string{" 2  Second", " 3  Second", " 1  Leaf"} {
				if strings.Contains(during, pair) != strings.Contains(before, pair) {
					t.Fatalf("number changed during drag: %q", pair)
				}
			}
		}
		app = moveMouse(app, x+2*delta, y) // Repeat motion changes depth only once.
		app = releaseMouse(app, x+2*delta, y)
		checkLabels(false)
	}
	check := func(parent string, want []int) {
		t.Helper()
		answer := d.Answers["r"]
		if answer.RankParents["second"] != parent || answer.RankParents["leaf"] != "second" {
			t.Fatalf("wrong drag answer: %+v", answer)
		}
		if order := app.lineOrder(app.tab()); !slices.Equal(order[:len(want)], want) {
			t.Fatalf("display order: %v, want %v", order, want)
		}
	}
	drag(2, rankIndentWidth, "Child") // First child cannot indent.
	if d.Answers["r"].Human {
		t.Fatal("an invalid depth change confirmed the ranking")
	}
	drag(4, rankIndentWidth, "Second", "Leaf")
	check("first", []int{0, 1, 2, 3, 4})
	if content := ansi.Strip(app.contentViewport.GetContent()); !strings.Contains(content, " 3  Second") || !strings.Contains(content, " 1  Leaf") {
		t.Fatalf("drop did not update hierarchy numbers: %q", content)
	}
	drag(4, -rankIndentWidth, "Second", "Leaf")
	check("", []int{0, 1, 2, 3, 4})
}

func TestRankHorizontalDragOutdentsPastRemainingSiblings(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] Parent <!-- form: id=r rank value=parent -->
  - [ ] Branch <!-- form: id=r rank value=branch -->
    - [ ] Leaf <!-- form: id=r rank value=leaf -->
  - [ ] Sibling <!-- form: id=r rank value=sibling -->
- [ ] Next <!-- form: id=r rank value=next -->
`)
	rect := rankControlRect(t, app, 2)
	x, y := contentScreenPoint(app, rect.left, rect.top)
	app = pressMouse(app, x, y)
	app = moveMouse(app, x-2, y)
	app = moveMouse(app, x-3, y)
	app = releaseMouse(app, x-3, y)
	d := app.forms.Document(app.tab().path)
	assertRankAnswer(t, d, "r", []string{"parent", "sibling", "branch", "leaf", "next"},
		map[string]string{"leaf": "branch", "sibling": "parent"})
}

func TestRankFixedAndDepthLimits(t *testing.T) {
	t.Parallel()
	app, _ := formTestAppWithContent(t, `- [ ] Inbox <!-- form: id=r rank value=inbox fixed max-depth=1 -->
- [ ] Review <!-- form: id=r rank value=review -->
  - [ ] Verify <!-- form: id=r rank value=verify -->
- [ ] Draft <!-- form: id=r rank value=draft -->
`)
	d := app.forms.Document(app.tab().path)
	app.tab().cursorLine = 1
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	if d.Answers["r"].Human || !strings.Contains(app.locationError, "fixed") {
		t.Fatalf("fixed keyboard move: %+v, %s", d.Answers["r"], app.locationError)
	}
	app.tab().cursorLine = 2
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	if d.Answers["r"].Human || !strings.Contains(app.locationError, "Inbox") {
		t.Fatalf("over-depth keyboard move: %+v, %s", d.Answers["r"], app.locationError)
	}
	app = dragRankUnder(t, app, 1, 4)
	if d.Answers["r"].Human {
		t.Fatal("drag moved fixed item")
	}
	app = dragRankUnder(t, app, 2, 1)
	if d.Answers["r"].Human || !strings.Contains(app.locationError, "Inbox") {
		t.Fatalf("over-depth drop: %+v, %s", d.Answers["r"], app.locationError)
	}
	app = dragRankUnder(t, app, 4, 1)
	if parent := d.Answers["r"].RankParents["draft"]; parent != "inbox" {
		t.Fatalf("valid drop set parent to %q", parent)
	}
}

func TestVerticalRankMovesSkipParentsThatCannotAcceptBranch(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, source string
		line         int
		key          rune
		want         []string
	}{
		{
			name: "down to first child",
			source: `- [ ] Source <!-- form: id=r rank value=source fixed -->
  - [ ] Branch <!-- form: id=r rank value=branch min-level=2 -->
    - [ ] Leaf <!-- form: id=r rank value=leaf -->
- [ ] Shallow <!-- form: id=r rank value=shallow fixed max-depth=1 -->
- [ ] Destination <!-- form: id=r rank value=destination fixed max-depth=2 -->
  - [ ] Peer <!-- form: id=r rank value=peer -->
`,
			line: 2,
			key:  tea.KeyDown,
			want: []string{"source", "shallow", "destination", "branch", "leaf", "peer"},
		},
		{
			name: "up to last child when descendant requires its depth",
			source: `- [ ] Destination <!-- form: id=r rank value=destination fixed max-depth=2 -->
  - [ ] Peer <!-- form: id=r rank value=peer -->
- [ ] Shallow <!-- form: id=r rank value=shallow fixed max-depth=1 -->
- [ ] Source <!-- form: id=r rank value=source fixed -->
  - [ ] Branch <!-- form: id=r rank value=branch -->
    - [ ] Leaf <!-- form: id=r rank value=leaf min-level=3 -->
`,
			line: 5,
			key:  tea.KeyUp,
			want: []string{"destination", "peer", "branch", "leaf", "shallow", "source"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := formTestAppWithContent(t, tt.source)
			d := app.forms.Document(app.tab().path)
			app.tab().cursorLine = tt.line
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
			if d.Answers["r"].Human || !strings.Contains(app.locationError, "must stay") {
				t.Fatalf("disallowed outdent: %+v, %s", d.Answers["r"], app.locationError)
			}
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tt.key, Mod: tea.ModAlt})
			wantParents := map[string]string{"branch": "destination", "leaf": "branch", "peer": "destination"}
			assertRankAnswer(t, d, "r", tt.want, wantParents)
			if app.tab().cursorLine != tt.line || app.locationError != "" {
				t.Fatalf("cursor = %d, error = %q", app.tab().cursorLine, app.locationError)
			}
		})
	}
}

func TestRankHandlesAndGutter(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		font icons.Set
	}{
		{"plain", icons.Plain},
		{"nerd", icons.Nerd},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := formTestAppWithContent(t, `- [ ] Inbox <!-- form: id=r rank value=inbox fixed -->
  - [ ] Pinned <!-- form: id=r rank value=pinned min-level=2 max-level=2 -->
  - [ ] Movable <!-- form: id=r rank value=movable -->
- [ ] Archive <!-- form: id=r rank value=archive fixed max-depth=0 -->
- [ ] Review <!-- form: id=r rank value=review -->
- [ ] Draft <!-- form: id=r rank value=draft -->
`)
			app.icons = tt.font
			check := func(wantDot bool) {
				t.Helper()
				app.rebuildContent()
				rows := strings.Split(app.contentViewport.GetContent(), "\n")
				row := rows[app.contentLayout.lineRanges[1].start]
				if dot := ansi.Strip(ansi.Cut(row, 0, 1)) == "●"; dot != wantDot {
					t.Fatalf("left gutter dot = %t, want %t", dot, wantDot)
				}
				if strings.Contains(ansi.Strip(row), "Fixed") {
					t.Fatal("lock handle is accompanied by a redundant Fixed label")
				}
				for i, item := range []struct {
					depth  int
					handle string
					number string
				}{
					{0, tt.font.Glyph(icons.Lock, "▣"), "1"},
					{1, tt.font.Glyph(icons.MoveVertical, "↕"), "1"},
					{1, tt.font.Glyph(icons.MoveFourDirections, "✥"), "2"},
					{0, tt.font.Glyph(icons.Lock, "▣"), "2"},
					{0, tt.font.Glyph(icons.MoveVertical, "↕"), "3"},
					{0, tt.font.Glyph(icons.MoveFourDirections, "✥"), "4"},
				} {
					start := app.sourceGutterWidth() + 1 + item.depth*rankIndentWidth
					row := rows[app.contentLayout.lineRanges[i+1].start]
					if got := ansi.Strip(ansi.Cut(row, start, start+1)); got != item.handle {
						t.Errorf("line %d handle = %q, want %q", i+1, got, item.handle)
					}
					if got := ansi.Strip(ansi.Cut(row, start+2, start+5)); got != " "+item.number+" " {
						t.Errorf("line %d rank badge shifted: %q", i+1, got)
					}
				}
			}
			check(true)
			app.toggleLineNumbers()
			check(true)
			app.toggleForm(1)
			check(false)
		})
	}
}
