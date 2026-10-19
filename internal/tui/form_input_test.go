package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/form"
)

func pathCompletionApp(t *testing.T, annotation string) AppModel {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"readme.md", "dir/my file.md", "dir/日本.md", "docs/keep.md"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, _ := formTestAppWithContent(t, "- [ ] Path <!-- form: id=path "+annotation+" -->\n")
	app.session.Meta.CWD = root
	app.toggleForm(1)
	app.refreshCompletion()
	return app
}

func TestPathCompletionKinds(t *testing.T) {
	for _, tt := range []struct {
		kind, want string
	}{
		{"file", "dir/,docs/,readme.md"},
		{"directory", "dir/,docs/"},
		{"path", "dir/,docs/,readme.md"},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			app := pathCompletionApp(t, "type="+tt.kind)
			if got := strings.Join(completionLabels(app), ","); got != tt.want {
				t.Fatalf("candidates=%q, want %q", got, tt.want)
			}
			typeText(&app, "di")
			editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
			if app.modalTextarea.Value() != "dir/" {
				t.Fatal("directory completion added @ or escaped the path")
			}
			if tt.kind == "directory" {
				if app.completion.active() {
					t.Fatal("directory-only field suggested files")
				}
				return
			}
			typeText(&app, "my f")
			editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
			if got := app.modalTextarea.Value(); got != "dir/my file.md" {
				t.Fatalf("plain path completion=%q", got)
			}
			if app.completion.active() {
				t.Fatal("completed file kept capturing Tab")
			}
			editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyEnter})
			answer := app.forms.Documents[0].Answers["path"]
			if app.modal != noModal || answer.Value != "dir/my file.md" || !answer.Human {
				t.Fatalf("saved=%+v modal=%v", answer, app.modal)
			}
		})
	}
}

func TestPathCompletionExistingSuffixAndUnicode(t *testing.T) {
	app := pathCompletionApp(t, `type=file default='dir/my fXX'`)
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyLeft})
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyLeft})
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := app.modalTextarea.Value(); got != "dir/my file.md" {
		t.Fatalf("completion retained old suffix: %q", got)
	}
	editorControl(&app, 'u')
	typeText(&app, "dir/日")
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if app.modalTextarea.Value() != "dir/日本.md" {
		t.Fatal("Unicode completion changed the path")
	}
	app.modalTextarea.SetValue("dirXX/my file.md")
	app.modalTextarea.MoveToBegin()
	app.modalTextarea.SetCursorColumn(2)
	app.refreshCompletion()
	editorMessage(&app, tea.KeyPressMsg{Code: tea.KeyTab})
	if app.modalTextarea.Value() != "dir/my file.md" {
		t.Fatalf("parent completion duplicated its slash: %q", app.modalTextarea.Value())
	}
}

func TestPathDefaultsAndSingleLineEditing(t *testing.T) {
	app := pathCompletionApp(t, `type=path default='new report.md' required`)
	d := &app.forms.Documents[0]
	if app.modalTextarea.Value() != "new report.md" || form.Satisfied(d.Items[0], d.Answers["path"]) {
		t.Fatal("default was hidden or counted as confirmation")
	}
	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal || d.Answers["path"].Human {
		t.Fatal("opening and closing the editor confirmed the default")
	}
	app.toggleForm(1)
	editorControl(&app, 'u')
	editorMessage(&app, tea.PasteMsg{Content: "日本語/my file.md\r\n"})
	if app.modalTextarea.Value() != "日本語/my file.md" {
		t.Fatal("a clipboard line ending became part of the path")
	}
	editorMessage(&app, tea.PasteMsg{Content: "bad\npath"})
	if app.modalTextarea.Value() != "日本語/my file.md" || !strings.Contains(app.clipboardStatus, "single line") {
		t.Fatal("multiline paste silently changed a path")
	}
	app.killRing.entries = []string{"bad\npath", "new file.md"}
	editorControl(&app, 'y')
	if app.modalTextarea.LineCount() != 1 {
		t.Fatal("yank inserted multiple lines")
	}
	editorControl(&app, 's')
	if app.forms.Documents[0].Answers["path"].Value != "日本語/my file.md" || len(app.missingForms()) != 0 {
		t.Fatal("path save lost the value or confirmation")
	}
}

func TestAttachedPathInputs(t *testing.T) {
	app, _ := formTestAppWithContent(t, "- [ ] Notes <!-- form: id=where value=notes type=directory default='/notes' -->\n"+
		"- [ ] Project <!-- form: id=where value=project type=directory default='/project' -->\n"+
		"- [ ] Copy <!-- form: id=copy checkbox type=file default='/copy.md' -->\n")
	app.toggleForm(1)
	if app.modalTextarea.Value() != "/notes" {
		t.Fatal("radio did not open its suggested path")
	}
	app.modalTextarea.SetValue("/custom notes")
	editorControl(&app, 's')
	app.toggleForm(2)
	if app.modalTextarea.Value() != "/project" {
		t.Fatal("radio shared a path with another option")
	}
	app = pressKey(app, tea.KeyEscape)
	app.toggleForm(1)
	if app.modalTextarea.Value() != "/custom notes" {
		t.Fatal("radio lost its saved path")
	}
	app = pressKey(app, tea.KeyEscape)
	app.tab().cursorLine = 3
	app = pressKey(app, tea.KeySpace)
	if app.modal != formTextModal || app.modalTextarea.Value() != "/copy.md" {
		t.Fatal("checkbox did not open its associated input")
	}
	app.modalTextarea.SetValue("/custom copy.md")
	editorControl(&app, 's')
	app.tab().cursorLine = 3
	app = pressKey(app, tea.KeySpace)
	stored := app.forms.Documents[0].Answers["copy"]
	exported := app.forms.Result(false, 1).Documents[0].Answers["copy"]
	if stored.Text != "/custom copy.md" || exported.Text != "" || len(exported.Drafts) != 0 {
		t.Fatalf("unchecked input: stored=%+v exported=%+v", stored, exported)
	}
	if app.modal != noModal || app.forms.Documents[0].Answers["copy"].Value != false {
		t.Fatal("clearing the checkbox opened its editor or kept it selected")
	}
	app.tab().cursorLine = 3
	app = pressKey(app, tea.KeySpace)
	if app.modalTextarea.Value() != "/custom copy.md" {
		t.Fatal("checkbox lost its associated input")
	}
	app = pressKey(app, tea.KeyEscape)
}

func TestPathCompletionMouseAfterResize(t *testing.T) {
	app := pathCompletionApp(t, `type=file default='dir/my f'`)
	for _, size := range []tea.WindowSizeMsg{{Width: 90, Height: 24}, {Width: 54, Height: 16}} {
		app, _ = updateApp(app, size)
		rows := strings.Split(ansi.Strip(app.View().Content), "\n")
		var click *tea.MouseClickMsg
		for y, row := range rows {
			if at := strings.Index(row, "│ my file.md"); at >= 0 {
				click = &tea.MouseClickMsg{X: ansi.StringWidth(row[:at]) + 2, Y: y, Button: tea.MouseLeft}
				break
			}
		}
		if click == nil {
			t.Fatalf("completion is not visible after resize to %dx%d", size.Width, size.Height)
		}
		app, _ = updateApp(app, *click)
		if app.modal != formTextModal || app.modalTextarea.Value() != "dir/my file.md" {
			t.Fatalf("click missed the menu: modal=%v text=%q", app.modal, app.modalTextarea.Value())
		}
		app.modalTextarea.SetValue("dir/my f")
		app.refreshCompletion()
	}
}

func TestFileInputRejectsDirectories(t *testing.T) {
	for _, path := range []string{"dir/", "missing/", "dir", "link", "."} {
		t.Run(path, func(t *testing.T) {
			app := pathCompletionApp(t, "type=file")
			if err := os.Symlink(filepath.Join(app.session.Meta.CWD, "dir"), filepath.Join(app.session.Meta.CWD, "link")); err != nil {
				t.Fatal(err)
			}
			app.modalTextarea.SetValue(path)
			app.saveFormText()
			if app.modal != formTextModal || app.forms.Documents[0].Answers["path"].Human || app.clipboardStatus == "" {
				t.Fatalf("invalid file saved: modal=%v answer=%+v error=%q", app.modal, app.forms.Documents[0].Answers["path"], app.clipboardStatus)
			}
			app.modalTextarea.SetValue("missing/new file.md")
			app.saveFormText()
			if app.modal != noModal || app.forms.Documents[0].Answers["path"].Value != "missing/new file.md" {
				t.Fatal("nonexistent file could not be saved")
			}
		})
	}
}

func TestEnterCommentsOnFormRows(t *testing.T) {
	for _, annotation := range []string{"text", "type=file", "value=notes type=directory", "checkbox type=file"} {
		t.Run(annotation, func(t *testing.T) {
			app, _ := formTestAppWithContent(t, "- [ ] Input <!-- form: id=input "+annotation+" -->\n")
			app.tab().cursorLine = 1
			app = pressKey(app, tea.KeyEnter)
			if app.modal != commentModal {
				t.Fatalf("Enter opened modal %v instead of comment", app.modal)
			}
		})
	}
}

func TestPathValidationLayout(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 90, Height: 24}, {Width: 54, Height: 16}} {
		app := pathCompletionApp(t, `type=file default='dir/'`)
		app, _ = updateApp(app, size)
		before := finishRegion(t, app, 1)
		var titleRow, inputRow int
		for _, region := range app.modalMouseRegions() {
			if region.action.close {
				titleRow = region.rect.top
			}
			if region.action.textarea {
				inputRow = region.rect.top
			}
		}
		if inputRow != titleRow+4 {
			t.Fatalf("extra caption spacing: title=%d input=%d", titleRow, inputRow)
		}
		app.saveFormText()
		if after := finishRegion(t, app, 1); after != before {
			t.Fatalf("error moved Save: before=%+v after=%+v", before, after)
		}
		view := ansi.Strip(app.View().Content)
		if !strings.Contains(view, "File path must not end with /") || strings.Contains(view, "Path (file)") {
			t.Fatalf("missing error or incorrect title:\n%s", view)
		}
		if menu, _ := app.completionLayout(app.modalMouseRegions(), app.width, app.height); menu != "" {
			t.Fatal("completion obscures the validation error")
		}
		var errorEnd, buttonsEnd, errorRow, buttonsRow int
		for row, line := range strings.Split(view, "\n") {
			if at := strings.Index(line, "File path must not end with /"); at >= 0 {
				errorEnd = ansi.StringWidth(line[:at]) + len("File path must not end with /")
				errorRow = row
			}
			if at := strings.Index(line, "Save C-s"); at >= 0 {
				buttonsEnd = ansi.StringWidth(line[:at]) + len("Save C-s")
				buttonsRow = row
			}
		}
		if errorEnd == 0 || buttonsEnd != errorEnd || errorRow <= buttonsRow {
			t.Fatalf("error or actions are not visible:\n%s", view)
		}
		app.modalTextarea.SetValue("new.md")
		app = clickModalAction(t, app, "Save")
		if app.modal != noModal || app.forms.Documents[0].Answers["path"].Value != "new.md" {
			t.Fatal("right-aligned Save mouse target is incorrect")
		}
	}
}

func TestPathExistenceNotices(t *testing.T) {
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		for _, tt := range []struct {
			annotation, path, notice string
			blocked, parentMissing   bool
		}{
			{"type=file", "new.md", "File does not exist", false, false},
			{"type=directory", "new-dir/", "Directory does not exist", false, false},
			{"type=path", "new/child", "Parent directory does not exist", false, true},
			{"type=file existing", "new.md", "File does not exist", true, false},
			{"type=directory existing", "new-dir/", "Directory does not exist", true, false},
			{"type=path existing", "new/child", "Parent directory does not exist", true, true},
			{"type=file existing", "readme.md", "", false, false},
			{"type=directory existing", "dir", "", false, false},
			{"type=directory", "readme.md", "", true, false},
		} {
			t.Run(fmt.Sprintf("dark=%t/%s/%s", dark, tt.annotation, tt.path), func(t *testing.T) {
				app := pathCompletionApp(t, tt.annotation)
				app.modalTextarea.SetValue(tt.path)
				app.refreshCompletion()
				text, tint := app.pathNotice()
				if text != tt.notice {
					t.Fatalf("notice=%q, want %q", text, tt.notice)
				}
				if tt.notice != "" {
					want := textColors.cyan
					if tt.parentMissing {
						want = warning
					}
					if tt.blocked {
						want = lipgloss.Red
					}
					if !sameColor(tint, want) {
						t.Fatalf("notice tint=%v, want %v", tint, want)
					}
				}
				app.saveFormText()
				if (app.modal == formTextModal) != tt.blocked || app.forms.Documents[0].Answers["path"].Human == tt.blocked {
					t.Fatalf("save blocked=%t, modal=%v answer=%+v", tt.blocked, app.modal, app.forms.Documents[0].Answers["path"])
				}
			})
		}
	}
}

func TestExistingPathSelectionsRequireValidInput(t *testing.T) {
	for _, control := range []string{"checkbox", "value=selected", "bulk"} {
		t.Run(control, func(t *testing.T) {
			annotation := control
			prefix := ""
			line := 1
			if control == "bulk" {
				prefix = "- [ ] All <!-- form: id=all select-all=inputs -->\n"
				annotation = "checkbox tags=inputs"
				line = 2
			}
			app, _ := formTestAppWithContent(t, prefix+"- [ ] Path <!-- form: id=path "+annotation+" type=file existing default='missing.md' -->\n")
			root := t.TempDir()
			app.session.Meta.CWD = root
			if err := os.WriteFile(filepath.Join(root, "valid.md"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			app.toggleForm(1)
			if app.modal != formTextModal || app.pendingFormAnswer == nil || form.Selected(app.forms.Documents[0].Items[line-1], app.forms.Documents[0].Answers["path"]) {
				t.Fatal("invalid path was confirmed before editing")
			}
			app = pressKey(app, tea.KeyEscape)
			if app.modal != noModal || app.pendingFormAnswer != nil {
				t.Fatal("closing invalid draft changed the control")
			}
			app.toggleForm(1)
			app.modalTextarea.SetValue("valid.md")
			app.saveFormText()
			answer := app.forms.Result(false, 1).Documents[0].Answers["path"]
			if app.modal != noModal || !answer.Human || answer.Text != "valid.md" || !form.Selected(app.forms.Documents[0].Items[line-1], answer) {
				t.Fatalf("valid selected path was not saved: %+v", answer)
			}
		})
	}
}

func TestPathNoticeDoesNotKeepPreviousError(t *testing.T) {
	app := pathCompletionApp(t, "type=file existing")
	app.modalTextarea.SetValue("missing.md")
	app.saveFormText()
	if app.clipboardStatus == "" {
		t.Fatal("missing path was accepted")
	}
	app.discardTextModal()
	app.toggleForm(1)
	if notice, _ := app.pathNotice(); notice != "" {
		t.Fatalf("reopened input kept the previous error: %q", notice)
	}
}
