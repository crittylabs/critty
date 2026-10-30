package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestResumedDocumentSnapshotReloadsFromWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := review.NewSession("", review.SessionEntry{CWD: "/removed-worktree", Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess.UseWorkspace(cwd, "main")
	snapshot := review.SnapshotFile{Content: "Original snapshot\n"}
	app := NewApp("missing.md", AppConfig{Session: sess, DocumentSnapshot: &snapshot})
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 25})
	app, _ = updateApp(app, app.loadDocuments()())
	if app.err != nil || !strings.Contains(ansi.Strip(app.contentViewport.GetContent()), "source file unavailable") || app.tab().doc.Content != snapshot.Content {
		t.Fatalf("saved document unavailable: %v, %s", app.err, app.contentViewport.GetContent())
	}
	if err := os.WriteFile("missing.md", []byte("Current workspace document\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadApp(t, &app)
	if app.documentSnapshot != nil || app.tab().doc.Content != "Current workspace document\n" || app.sourcePath("missing.md") != filepath.Join(cwd, "missing.md") {
		t.Fatal("reload did not leave the saved snapshot for the current workspace")
	}
}

func TestResumedCodeShowsSnapshotOutsideCurrentChanges(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := review.NewSession("", review.SessionEntry{CWD: t.TempDir(), Mode: "git"})
	if err != nil {
		t.Fatal(err)
	}
	sess.SetFileComments("old.go", "modified", []review.Comment{{ID: "c_old", Scope: "file", Body: "Keep original discussion"}})
	sess.SetFileComments("plain.go", "modified", nil)
	sess.CJ.RoundState.Files = map[string]review.SnapshotFile{
		"old.go": {Content: "Saved source content\n"}, "plain.go": {Content: "Snapshot without comments\n"},
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(nil, "HEAD", AppConfig{Session: sess})
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 80, Height: 25})
	app, _ = updateApp(app, app.loadDocuments()())
	content := ansi.Strip(app.contentViewport.GetContent())
	if app.err != nil || !strings.Contains(content, "outside current changes") || !strings.Contains(content, "Saved source content") || !strings.Contains(content, "Keep original discussion") {
		t.Fatalf("old review context unavailable: %v, %s", app.err, content)
	}
	if len(app.tabs) != 2 || app.tabs[1].doc.Content != "Snapshot without comments\n" {
		t.Fatal("empty current scope lost unannotated saved source")
	}
}

func TestReplacedAttachmentHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := review.OpenSession(t.TempDir(), "history")
	if err != nil {
		t.Fatal(err)
	}
	old := review.AttachedDocument{ID: "old", Name: "NEXT_ACTIONS.md", Source: "/docs/NEXT_ACTIONS.md", Content: "- [ ] Old action <!-- form: id=old consent -->\n", Removed: true}
	current := review.AttachedDocument{ID: "current", Name: "APPROVAL.md", Source: "/docs/APPROVAL.md", Content: "- [ ] Current action <!-- form: id=current consent -->\n"}
	sess.SetFileComments(old.Path(), "", []review.Comment{{ID: "history-comment", Body: "Keep this discussion"}})
	app := NewCodeReviewApp([]gitpkg.FileChange{{Path: "main.go"}}, "HEAD", AppConfig{Session: sess, Documents: []review.AttachedDocument{old, current}})
	for i := range app.tabs {
		tab := &app.tabs[i]
		tab.state = &fileReview{Comments: sess.FileComments(tab.path)}
		if tab.attachment != nil {
			tab.doc = document.FromContent(tab.path, []byte(tab.attachment.Content))
		}
	}
	if err := app.beginForms(nil, ""); err != nil {
		t.Fatal(err)
	}
	app.width, app.height = 90, 24
	app.recalculateLayout()
	app.syncFileTree()
	if app.tabs[0].path != current.Path() || app.tabs[len(app.tabs)-1].path != old.Path() {
		t.Fatal("current attachments and changes must precede history")
	}
	for _, row := range app.fileTree.rows {
		if row.path == old.Path() {
			t.Fatal("history must start collapsed")
		}
	}
	app.activeTab = len(app.tabs) - 1
	app.syncFileTree()
	if app.fileTree.rows[app.fileTree.cursor].path != old.Path() {
		t.Fatal("navigation to an old comment must reveal its attachment")
	}
	app.rebuildContent()
	content := strings.Join(strings.Fields(ansi.Strip(app.contentViewport.GetContent())), " ")
	for _, want := range []string{"History", "read-only", "not part of this review", "Checkboxes are inactive"} {
		if !strings.Contains(content, want) {
			t.Errorf("history notice missing %q: %s", want, content)
		}
	}
	if app.forms.Document(old.Path()) != nil || len(app.contentLayout.formControls) != 0 || len(app.pendingForms()) != 1 {
		t.Fatal("historical form must remain inactive and excluded from current answers")
	}
	if len(app.tab().state.Comments) != 1 || app.contentLayout.lineRanges[1].end == 0 {
		t.Fatal("history lost its comments or source line mapping")
	}
	app.width = 40
	app.recalculateLayout()
	app.rebuildContent()
	for _, row := range strings.Split(app.contentViewport.GetContent(), "\n") {
		if row == "" {
			break
		}
		if ansi.StringWidth(row) > app.contentViewport.Width() {
			t.Fatalf("history notice overflows narrow content pane: %q", row)
		}
	}
	app.activeTab = 0
	app.rebuildContent()
	if strings.Contains(app.contentViewport.GetContent(), "Checkboxes are inactive") || len(app.contentLayout.formControls) == 0 {
		t.Fatal("current attachment must keep its active form without a history notice")
	}
}

func TestAttachedDocumentMixedReview(t *testing.T) {
	d := review.AttachedDocument{ID: "a_notice", Source: "/missing/notice.md", Name: "notice.md", Content: "# Notice\nPlease review\n"}
	app := NewCodeReviewApp([]gitpkg.FileChange{{Path: "notice.md", Status: gitpkg.StatusModified}}, "HEAD", AppConfig{Documents: []review.AttachedDocument{d}, Staged: true, Project: "project"})
	if len(app.tabs) != 2 || app.tab().attachment == nil || app.tabs[1].path != "notice.md" {
		t.Fatalf("wrong order: %+v", app.tabs)
	}
	doc, err := app.loadDocument(d.Path())
	if err != nil || doc.Content != d.Content {
		t.Fatalf("attachment read index or original: %+v, %v", doc, err)
	}
	app.tabs[0].doc, app.tabs[0].state = doc, &fileReview{}
	app.tabs[1].doc, app.tabs[1].state = document.FromContent("notice.md", []byte("Changed\n")), &fileReview{}
	if app.tab().changeSummary(false) != "" || len(app.tab().changedLines) != 0 {
		t.Fatal("document rendered as diff")
	}
	app.syncFileTree()
	rows := app.fileTree.rows
	if len(rows) != 4 || rows[0].name != "attachments" || rows[1].name != "notice.md" || rows[1].tabIndex != 0 || rows[3].tabIndex != 1 {
		t.Fatalf("wrong tree: %+v", rows)
	}
	app.setTreeCursor(3)
	if app.activeTab != 1 {
		t.Fatal("tree lost working-file tab index")
	}
	app.activeTab = 0
	app.syncFileTree()
	app.toggleTreeDir(0)
	if !app.fileTree.collapsed["\x00attachments"] {
		t.Fatal("attachment root did not fold")
	}
	reference := "@" + escapeReferencePath(d.Path()) + " L2"
	if !strings.Contains(app.linkFileReferences(reference), "critty://source") {
		t.Fatal("snapshot reference did not become a link")
	}
	app.activeTab = 1
	if cmd := app.navigateSource(sourceLocation{path: d.Path(), line: 2}); cmd != nil || app.activeTab != 0 || app.tab().cursorLine != 2 {
		t.Fatal("snapshot reference did not navigate")
	}
	app = NewCodeReviewApp(nil, "HEAD", AppConfig{Documents: []review.AttachedDocument{d}})
	if _, subject := app.reviewSubject(); subject != "attachments (no file changes)" {
		t.Fatalf("empty scope not identified: %q", subject)
	}
}

func TestAttachedDocumentCommentsAndResume(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("Plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := review.ReadAttachedDocuments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := review.OpenSession(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(path, AppConfig{Session: sess, Documents: docs})
	if _, subject := app.reviewSubject(); subject != "document" || !app.multiFile {
		t.Fatal("document review lost identity or tab switching")
	}
	for i := range app.tabs {
		app.tabs[i].state = &fileReview{Comments: []review.Comment{{ID: app.tabs[i].path, Body: "comment"}}}
	}
	app.persist()
	if app.err != nil || len(sess.CJ.Files) != 2 || len(sess.FileComments(path)) != 1 || len(sess.FileComments(docs[0].Path())) != 1 {
		t.Fatalf("comments collided: %+v, %v", sess.CJ.Files, app.err)
	}
	docs[0].Removed = true
	resumed := NewApp(path, AppConfig{Session: sess, Documents: docs})
	if resumed.tabs[1].attachment == nil || resumed.tabs[0].path != path {
		t.Fatal("removed document did not retain separate comment tab")
	}
	resumed.syncFileTree()
	last := resumed.fileTree.rows[len(resumed.fileTree.rows)-1]
	if last.name != "attachment history (read-only)" || !last.collapsed {
		t.Fatal("removed attachment history not identified or folded")
	}
}
