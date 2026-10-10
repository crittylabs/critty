package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/knu/tcrit/internal/document"
	"github.com/knu/tcrit/internal/review"
)

func formTestApp(t *testing.T) (AppModel, chan FinishEvent) {
	t.Helper()
	sess, err := review.OpenSession(t.TempDir(), "forms")
	if err != nil {
		t.Fatal(err)
	}
	doc := review.AttachedDocument{ID: "a", Source: "/missing/form.md", Name: "form.md", Content: "# Form\n- [ ] Required confirmation with a long label that wraps <!-- form: id=confirm required -->\n- [ ] Small <!-- form: id=size value=small -->\n- [ ] Large <!-- form: id=size value=large -->\n- [x] Publish <!-- form: id=publish consent -->\n\n```md\n- [ ] Example\n```\n"}
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
	return app, ch
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
	// Labels toggle, but the whitespace after the rendered text does not.
	control := app.contentLayout.formControls[0]
	left, top, _, _ := app.contentBounds()
	app = clickMouse(app, left+control.rect.right+3, top+control.rect.top-app.contentViewport.YOffset())
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
	if app.finishActionLabel() != "Finish Review" || app.resolvesAllOnFinish() {
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

func TestFormApprovalAndResultFailure(t *testing.T) {
	app, ch := formTestApp(t)
	app.toggleForm(2)
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
	app, ch = formTestApp(t)
	app.toggleForm(2)
	if err := os.Mkdir(filepath.Join(app.session.Dir, "result-1.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, cmd = app.doFinish()
	if isQuit(cmd) || app.err == nil {
		t.Fatal("save failure submitted review")
	}
	select {
	case <-ch:
		t.Fatal("save failure emitted approval")
	default:
	}
	if app.forms.Documents[0].Answers["confirm"].Value != true {
		t.Fatal("save failure lost confirmation")
	}
	if app.session.CJ.RoundState.Finished {
		t.Fatal("save failure finished the round")
	}
	if err := os.Remove(filepath.Join(app.session.Dir, "result-1.json")); err != nil {
		t.Fatal(err)
	}
	_, cmd = app.doFinish()
	if !isQuit(cmd) || !takeEvent(t, ch).Approved {
		t.Fatalf("retry failed: %v", app.err)
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
