package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knu/tcrit/internal/document"
	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

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
	if app.tab().changeSummary() != "" || len(app.tab().changedLines) != 0 {
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
	if !strings.Contains(app.linkFileReferences(reference), "tcrit://source") {
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
	if resumed.tabs[0].path != path || resumed.tabs[1].attachment == nil {
		t.Fatal("removed document did not retain separate comment tab")
	}
	resumed.syncFileTree()
	if !strings.Contains(resumed.fileTree.rows[1].name, "removed") {
		t.Fatal("removed attachment not identified")
	}
}
