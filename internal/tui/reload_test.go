package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/form"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func reloadApp(t *testing.T, app *AppModel) {
	t.Helper()
	cmd := app.requestReload(true)
	if cmd == nil {
		t.Fatal("reload was not started")
	}
	msg, ok := cmd().(reloadMsg)
	if !ok {
		t.Fatal("unexpected reload result")
	}
	app.finishReload(msg)
	if app.err != nil || app.reload.err != nil {
		t.Fatalf("reload: %v / %v", app.err, app.reload.err)
	}
}

func TestReloadDocumentAndAttachments(t *testing.T) {
	app := setupAppWithDoc(t, "first\nanchor\n")
	app.width, app.height = 100, 30
	attachment := filepath.Join(t.TempDir(), "notes.md")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(attachment, "# Notes\nold\n")
	docs, err := review.ReadAttachedDocuments([]string{attachment})
	if err != nil {
		t.Fatal(err)
	}
	app.addDocuments(docs)
	app.orderTabsByTree()
	app.tabs[0].state = &fileReview{}
	for i := range app.tabs {
		if app.tabs[i].path == app.filePath {
			app.activeTab = i
		}
	}
	app.tab().state.Comments = []review.Comment{{ID: "anchor", Body: "keep", StartLine: 2, EndLine: 2}}
	app.persist()
	round := app.session.CJ.ReviewRound
	app.split, app.openAll, app.viewMode = true, true, true
	write(app.filePath, "inserted\nfirst\nanchor\n")
	write(attachment, "# Notes\nnew\n")
	reloadApp(t, &app)
	if app.tab().path != app.filePath || !app.split || !app.openAll || !app.viewMode {
		t.Fatal("reload lost active file or display options")
	}
	if app.session.CJ.ReviewRound != round {
		t.Fatal("reload incremented the round")
	}
	comments := app.tab().state.Comments
	if len(comments) != 1 || comments[0].ID != "anchor" || comments[0].StartLine != 3 {
		t.Fatalf("comments: %+v", comments)
	}
	if app.attachments[docs[0].Path()].Content != "# Notes\nnew\n" {
		t.Fatal("attachment did not reload")
	}
	saved, err := app.session.LoadDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Documents[0].Revision == docs[0].Revision {
		t.Fatal("attachment revision was not saved")
	}
}

func TestReloadPreservesPerFileViewport(t *testing.T) {
	app := setupAppWithDoc(t, strings.Repeat("source line\n", 100))
	attachment := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(attachment, []byte(strings.Repeat("note line\n", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := review.ReadAttachedDocuments([]string{attachment})
	if err != nil {
		t.Fatal(err)
	}
	app.addDocuments(docs)
	app, _ = updateApp(app, docRenderedMsg{})
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 120, Height: 24})
	positions := []struct {
		path         string
		line, offset int
	}{{app.filePath, 40, 32}, {docs[0].Path(), 70, 62}}
	selectFile := func(path string) {
		t.Helper()
		for i, tab := range app.tabs {
			if tab.path == path {
				app.selectTab(i)
				return
			}
		}
		t.Fatalf("missing tab %s", path)
	}
	for _, position := range positions {
		selectFile(position.path)
		app.tab().cursorLine = position.line
		app.rebuildContent()
		app.contentViewport.SetYOffset(position.offset)
	}
	reloadApp(t, &app)
	for _, position := range positions {
		selectFile(position.path)
		if app.tab().cursorLine != position.line || app.contentViewport.YOffset() != position.offset {
			t.Fatalf("%s restored line/offset %d/%d, want %d/%d", position.path,
				app.tab().cursorLine, app.contentViewport.YOffset(), position.line, position.offset)
		}
	}
}

func TestReloadDeferralAndFailure(t *testing.T) {
	app := setupAppWithDoc(t, "original\n")
	app.modal = commentModal
	if cmd := app.requestReload(true); cmd != nil || !app.reload.pending {
		t.Fatal("editing did not defer reload")
	}
	app.modal = noModal
	cmd := app.resumeReload()
	if cmd == nil {
		t.Fatal("reload did not resume")
	}
	msg := cmd().(reloadMsg)
	app.modal = finishModal
	app.finishReload(msg)
	if !app.reload.pending || app.tab().doc.Content != "original\n" {
		t.Fatal("submission did not defer reload")
	}
	app.modal = noModal
	if err := os.Remove(app.filePath); err != nil {
		t.Fatal(err)
	}
	msg = app.resumeReload()().(reloadMsg)
	app.finishReload(msg)
	if app.reload.err == nil || app.err != nil || app.tab().doc.Content != "original\n" {
		t.Fatal("failed reload did not retain the displayed snapshot")
	}
	if !strings.Contains(ansi.Strip(app.renderFooter()), "Reload failed:") {
		t.Fatal("reload error is not visible")
	}
}

func TestReloadDefersTextSelection(t *testing.T) {
	for _, dragging := range []bool{true, false} {
		t.Run(fmt.Sprintf("dragging=%t", dragging), func(t *testing.T) {
			app := setupAppWithDoc(t, "original\n")
			app.width, app.height = 100, 30
			app.recalculateLayout()
			app.rebuildContent()
			left, top, _, _ := app.contentBounds()
			app = pressMouse(app, left+gutterWidth, top)
			if !dragging {
				app = releaseMouse(app, left+gutterWidth+3, top)
			}
			if err := os.WriteFile(app.filePath, []byte("updated\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if cmd := app.requestReload(true); cmd != nil || !app.reload.pending {
				t.Fatal("text selection did not defer reload")
			}
			app.clearTextSelection()
			cmd := app.resumeReload()
			if cmd == nil {
				t.Fatal("clearing text selection did not resume reload")
			}
			app.finishReload(cmd().(reloadMsg))
			if app.tab().doc.Content != "updated\n" {
				t.Fatal("deferred reload did not load the new text")
			}
		})
	}
}

func TestWatchResultAfterDisable(t *testing.T) {
	app := setupAppWithDoc(t, "original\n")
	app.reload.watch = true
	cmd := app.requestReload(false)
	if err := os.WriteFile(app.filePath, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	msg := cmd().(reloadMsg)
	app.toggleWatch()
	app.finishReload(msg)
	if app.tab().doc.Content != "original\n" {
		t.Fatal("disabled watch installed an in-flight result")
	}
}

func TestWatchIndicator(t *testing.T) {
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := setupAppWithDoc(t, "text\n")
		for _, width := range []int{18, 40, 80, 120} {
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: width, Height: 30})
			app.reload.watch = false
			if bar := ansi.Strip(app.renderReviewBar()); strings.Contains(bar, "Reload") || strings.Contains(bar, "Watch") {
				t.Fatalf("idle controls are visible: %q", bar)
			}
			app.reload.watch = true
			_, layout := app.renderReviewScreen()
			if !layout.hasWatchIndicator || !layout.hasFinishButton {
				t.Fatalf("width %d: missing indicator or Submit", width)
			}
			for _, r := range []mouseRect{layout.watchIndicator, layout.finishButton} {
				if r.left < 0 || r.right > width {
					t.Fatalf("width %d: control outside terminal: %+v", width, r)
				}
			}
			indicator := app.watchIndicator()
			runes, styles := renderedCells(indicator)
			if string(runes) != "Watch: on" || styles[0].Bg == nil || sameColor(styles[0].Bg, styles[1].Bg) {
				t.Fatalf("missing Watch label or key highlight: %q", indicator)
			}
			for _, style := range styles[1:] {
				if !sameColor(style.Bg, modalBtnFocusedLabel.GetBackground()) {
					t.Fatalf("Watch label does not use the Split button background: %q", indicator)
				}
			}
			app = clickMouse(app, layout.watchIndicator.left, layout.watchIndicator.top)
			if app.reload.watch || app.watchIndicator() != "" {
				t.Fatal("click did not disable Watch and hide its indicator")
			}
		}
		app.tabs = nil
		app.reload.watch = true
		body, _ := app.renderEmptyReview()
		if !strings.Contains(ansi.Strip(body), "Watch: on") {
			t.Fatal("empty review lost Watch status")
		}
	}
}

func TestReloadWatchKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		key    tea.KeyPressMsg
		reload bool
	}{
		{"F5", tea.KeyPressMsg{Code: tea.KeyF5}, true},
		{"M-r", tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt}, true},
		{"W", tea.KeyPressMsg{Code: 'W'}, false},
		{"S-F5", tea.KeyPressMsg{Code: tea.KeyF5, Mod: tea.ModShift}, false},
		{"M-R", tea.KeyPressMsg{Code: 'R', Mod: tea.ModAlt}, false},
		{"M-S-r", tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt | tea.ModShift}, false},
		{"M-S-R", tea.KeyPressMsg{Code: 'R', Mod: tea.ModAlt | tea.ModShift}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "text\n")
			app, cmd := updateApp(app, tc.key)
			if cmd == nil || app.reload.busy != tc.reload || app.reload.watch == tc.reload || app.ignoreWhitespace {
				t.Fatal("key did not invoke the intended action")
			}
		})
	}
	app := setupAppWithDoc(t, "text\n")
	app.multiFile = true
	app = pressKey(app, 'w')
	if !app.ignoreWhitespace || app.reload.watch {
		t.Fatal("lowercase w no longer controls whitespace")
	}
	app.openLineComment()
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'W', Text: "W"})
	if app.reload.watch || app.modalTextarea.Value() != "W" {
		t.Fatal("W in an editor toggled Watch")
	}
}

func TestReloadGitFilesAndConcurrentComments(t *testing.T) {
	dir := newGitRepo(t)
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("kept.txt", "base\n")
	write("gone.txt", "base\n")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-qm", "base")
	write("kept.txt", "modified\n")
	write("gone.txt", "modified\n")
	source := gitpkg.ReviewSource{Scope: "all", Base: "HEAD"}
	files, err := source.Files()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenCodeSessionWithArgs("", []string{"--scope", "all"})
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Source: &source})
	app, _ = updateApp(app, docRenderedMsg{})
	app.width, app.height = 100, 30
	for i := range app.tabs {
		if app.tabs[i].path == "gone.txt" {
			app.tabs[i].state.Comments = []review.Comment{{ID: "kept-comment", Scope: "file", Body: "Keep context"}}
		}
	}
	app.persist()
	// Simulate a CLI reply arriving while the TUI is open.
	other, err := review.OpenSession("", sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Update(func(current *review.Session) error {
		file := current.CJ.Files["gone.txt"]
		file.Comments[0].Replies = append(file.Comments[0].Replies, review.Reply{ID: "cli-reply", Body: "CLI reply"})
		current.CJ.Files["gone.txt"] = file
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	write("gone.txt", "base\n")
	write("added.txt", "new\n")
	reloadApp(t, &app)
	found := false
	for _, tab := range app.tabs {
		if tab.path == "gone.txt" {
			found = tab.outsideChanges && len(tab.state.Comments) == 1 && len(tab.state.Comments[0].Replies) == 1 && tab.doc.Content == "modified\n"
		}
	}
	if !found {
		t.Fatal("file leaving the diff lost its comments, CLI reply, or context")
	}
	if len(app.tabs) != 3 {
		t.Fatalf("tabs=%d, want new file plus retained file", len(app.tabs))
	}
}

func TestReloadFormDraftsAndConsent(t *testing.T) {
	app := setupAppWithDoc(t, "target\n")
	path := filepath.Join(t.TempDir(), "form.md")
	content := "- [ ] Publish <!-- form: id=publish consent -->\n- [ ] Notes <!-- form: id=notes text -->\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := review.ReadAttachedDocuments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	app.addDocuments(docs)
	app.orderTabsByTree()
	app, _ = updateApp(app, docRenderedMsg{})
	if app.err != nil {
		t.Fatal(app.err)
	}
	app.forms.Documents[0].Answers["publish"] = form.Answer{Value: true}
	app.forms.Documents[0].Answers["notes"] = form.Answer{Value: "keep draft"}
	if err := app.session.SaveForms(*app.forms); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.filePath, []byte("changed target\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reloadApp(t, &app)
	answers := app.forms.Documents[0].Answers
	if answers["publish"].Value == true || answers["notes"].Value != "keep draft" {
		t.Fatalf("answers: %+v", answers)
	}
}

func TestWatchQueuedStateSurvivesUpdate(t *testing.T) {
	app := setupAppWithDoc(t, "content\n")
	app.reload.watch, app.reload.dirty = true, true
	updated, cmd := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = updated.(AppModel)
	if cmd == nil || !app.reload.busy || app.reload.request != 1 || app.reload.dirty {
		t.Fatal("queued reload did not survive the model update")
	}
}

func TestReloadBranchWithoutContentChanges(t *testing.T) {
	dir := newGitRepo(t)
	if err := os.WriteFile("notes.md", []byte("# Notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := NewApp("notes.md", AppConfig{Project: "sample"})
	app, _ = updateApp(app, docRenderedMsg{})
	app.width, app.height = 100, 30
	reloadApp(t, &app)
	before := app.branch
	runGitIn(t, dir, "symbolic-ref", "HEAD", "refs/heads/feature/sidebar")
	reloadApp(t, &app)
	if before == app.branch || app.branch != "feature/sidebar" {
		t.Fatalf("branch after reload = %q, before %q", app.branch, before)
	}
}
