package review

import (
	"strings"
	"testing"
)

func TestSessionSaveMergesConcurrentThreadChanges(t *testing.T) {
	setupStateDir(t)
	seed, err := OpenSession("", "merge")
	if err != nil {
		t.Fatal(err)
	}
	seed.SetFileComments("a.go", "", []Comment{
		{ID: "c_deleted", Body: "remove remotely"},
		{ID: "c_edit", Body: "original", GitHubID: 9223372036854775807, Replies: []Reply{{ID: "rp_deleted", Body: "remove locally"}}},
	})
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}
	local, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Update(func(s *Session) error {
		s.SetFileComments("a.go", "", s.FileComments("a.go")[1:])
		s.AppendFileComment("a.go", "new CLI comment", "Agent", "")
		s.AppendReviewComment("new review comment", "Agent", "")
		return s.AppendReply("c_edit", "CLI reply", "Agent", "", false, "")
	}); err != nil {
		t.Fatal(err)
	}
	comments := local.FileComments("a.go")
	comments[1].Body, comments[1].Resolved = "reviewer edit", true
	comments[1].Replies = []Reply{{ID: "rp_local", Body: "reviewer reply"}}
	local.SetFileComments("a.go", "", comments)
	local.AppendFileComment("a.go", "new TUI comment", "Reviewer", "")
	if err := local.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	comments = saved.FileComments("a.go")
	if len(comments) != 3 || len(saved.CJ.ReviewComments) != 1 {
		t.Fatalf("lost or resurrected comments: %+v", saved.CJ)
	}
	c := comments[0]
	if c.ID != "c_edit" || c.Body != "reviewer edit" || !c.Resolved || c.GitHubID != 9223372036854775807 || len(c.Replies) != 2 {
		t.Fatalf("lost thread edits or integer precision: %+v", c)
	}
	if c.Replies[0].Body != "CLI reply" || c.Replies[1].Body != "reviewer reply" {
		t.Fatalf("lost or resurrected replies: %+v", c.Replies)
	}
}

func TestSessionSaveRejectsConflictingEdits(t *testing.T) {
	setupStateDir(t)
	seed, err := OpenSession("", "conflict")
	if err != nil {
		t.Fatal(err)
	}
	seed.SetFileComments("a.go", "", []Comment{{ID: "c_edit", Body: "original"}})
	if err := seed.Save(); err != nil {
		t.Fatal(err)
	}
	local, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.Update(func(s *Session) error {
		s.FileComments("a.go")[0].Body = "CLI edit"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	local.FileComments("a.go")[0].Body = "TUI edit"
	if err := local.Save(); err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("conflicting edit succeeded: %v", err)
	}
	if local.FileComments("a.go")[0].Body != "TUI edit" {
		t.Fatal("failed save lost the local edit")
	}
	saved, err := OpenSession("", seed.Key)
	if err != nil {
		t.Fatal(err)
	}
	if saved.FileComments("a.go")[0].Body != "CLI edit" {
		t.Fatal("failed save overwrote the concurrent edit")
	}
}
