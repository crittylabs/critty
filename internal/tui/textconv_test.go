package tui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestTextconvReviewRoundTrip(t *testing.T) {
	dir := newGitRepo(t)
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitattributes", "*.bin diff=records\n")
	write("convert.sh", "tr '\\000' '\\n' < \"$1\"\n")
	runGitIn(t, dir, "config", "diff.records.textconv", "sh convert.sh")
	write("data.bin", "old\x00target\x00")
	runGitIn(t, dir, "add", ".")
	runGitIn(t, dir, "commit", "-qm", "base")
	write("data.bin", "new\x00target\x00")
	source := gitpkg.ReviewSource{Scope: "all", Base: "HEAD", TextconvDrivers: []string{"records"}}
	sess, err := review.OpenCodeSessionWithArgs("", nil)
	if err != nil {
		t.Fatal(err)
	}
	start := func() AppModel {
		t.Helper()
		files, err := source.Files()
		if err != nil {
			t.Fatal(err)
		}
		docs := []review.AttachedDocument{{ID: "check", Source: "/check.md", Content: "- [ ] Check <!-- form: id=check -->\n"}}
		app := NewCodeReviewApp(files, "HEAD", AppConfig{Session: sess, Source: &source, Documents: docs})
		updated, _ := app.Update(docRenderedMsg{})
		app = updated.(AppModel)
		if app.err != nil {
			t.Fatal(app.err)
		}
		for i, tab := range app.tabs {
			if tab.path == "data.bin" {
				app.activeTab = i
			}
		}
		return app
	}
	app := start()
	if app.tab().isBinary || app.tab().doc.Content != "new\ntarget\n" || !app.tab().changedLines[1] {
		t.Fatal("display and diff do not use converted text")
	}
	app.tab().state.Comments = []review.Comment{{ID: "c_target", Scope: "line", StartLine: 2, EndLine: 2, Anchor: "target", Body: "change this"}}
	app.persist()
	conversion := gitpkg.Textconv{Driver: "records", Command: "sh convert.sh"}
	if sess.CJ.Files["data.bin"].Textconv != conversion || sess.CJ.ListComments(false)[0].Textconv != conversion {
		t.Fatal("review or comment JSON lost conversion metadata")
	}
	forms, err := sess.LoadForms()
	if err != nil || forms.Input.Files["data.bin"].Textconv != conversion || forms.Input.Files["data.bin"].Digest == "" {
		t.Fatalf("review target lost conversion or original identity: %+v, %v", forms, err)
	}
	data, err := os.ReadFile(sess.Path())
	if err != nil {
		t.Fatal(err)
	}
	var saved review.CritJSON
	if err := json.Unmarshal(data, &saved); err != nil || saved.Files["data.bin"].Textconv != conversion {
		t.Fatalf("saved metadata=%+v, err=%v", saved.Files, err)
	}
	sess, err = review.ResolveTarget("", app.session.Key)
	if err != nil {
		t.Fatal(err)
	}
	write("data.bin", "inserted\x00new\x00target\x00")
	app = start()
	comments := app.tab().state.Comments
	if len(comments) != 1 || comments[0].StartLine != 3 || comments[0].Drifted {
		t.Fatalf("converted anchor did not survive resume: %+v", comments)
	}
	// Removing the driver must not leave stale conversion metadata behind.
	write(".gitattributes", "")
	app = start()
	if sess.CJ.Files["data.bin"].Textconv.Command != "" {
		t.Fatal("removed driver still recorded")
	}
	if strings.Contains(string(data), "\\u0000") {
		t.Fatal("review text snapshot contains original binary bytes")
	}
}

func TestSuppliedDiffDoesNotInferTextconv(t *testing.T) {
	dir := newGitRepo(t)
	if err := os.WriteFile(".gitattributes", []byte("*.bin diff=records\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, dir, "config", "diff.records.textconv", "false")
	patch, err := gitpkg.ParsePatch(strings.NewReader("diff --git a/data.bin b/data.bin\n--- a/data.bin\n+++ b/data.bin\n@@ -1 +1 @@\n-old\n+converted\n"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewCodeReviewApp(patch.Changes(), "", AppConfig{Patch: patch, ReadOnly: true})
	updated, _ := app.Update(docRenderedMsg{})
	app = updated.(AppModel)
	if app.err != nil || app.tab().doc.Content != "converted" || app.tab().textconv.Command != "" {
		t.Fatalf("supplied diff used local conversion: %+v, %v", app.tab(), app.err)
	}
}
