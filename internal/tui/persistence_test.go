package tui

import (
	"testing"

	"github.com/crittylabs/critty/internal/review"
)

func TestCommentEditPreservesConcurrentCLIReply(t *testing.T) {
	app := setupAppWithDoc(t, "# Plan\n")
	c := testComment()
	app.tab().state.Comments = []review.Comment{c}
	app.persist()
	if app.err != nil {
		t.Fatal(app.err)
	}

	cli, err := review.OpenSession("", app.session.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Update(func(s *review.Session) error {
		return s.AppendReply(c.ID, "Updated by CLI", "Agent", "", false, "")
	}); err != nil {
		t.Fatal(err)
	}

	app.modal, app.editingID, app.modalInitial = editModal, c.ID, c.Body
	app.modalTextarea.SetValue("Clarified by reviewer")
	app.modalSubmit()
	if app.err != nil {
		t.Fatal(app.err)
	}
	saved, err := review.OpenSession("", app.session.Key)
	if err != nil {
		t.Fatal(err)
	}
	comments := saved.FileComments(app.tab().path)
	if len(comments) != 1 || comments[0].Body != "Clarified by reviewer" || len(comments[0].Replies) != 1 || comments[0].Replies[0].Body != "Updated by CLI" {
		t.Fatalf("comment edit lost concurrent reply: %+v", comments)
	}
	if len(app.tab().state.Comments[0].Replies) != 1 {
		t.Fatal("TUI did not refresh the merged replies")
	}
}
