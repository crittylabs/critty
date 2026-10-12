package review

import "testing"

func TestSubmissionCommentPersistsAcrossRetryAndRounds(t *testing.T) {
	setupStateDir(t)
	s, err := OpenSession("", "submission")
	if err != nil {
		t.Fatal(err)
	}
	s.SetSubmission(VerdictReject, " Please revise this.\nHere is why. ", "Human")
	id := s.CJ.RoundState.ReviewCommentID
	s.SetSubmission(VerdictComment, "Check the explanation.", "Human")
	if len(s.CJ.ReviewComments) != 1 || s.CJ.ReviewComments[0].ID != id {
		t.Fatal("retry duplicated the review comment")
	}
	s.SetSubmission(VerdictApprove, "Approved.", "Human")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSession("", s.Key)
	if err != nil {
		t.Fatal(err)
	}
	c := s.SubmissionComment()
	if c == nil || c.Scope != "review" || c.Body != "Approved." || !c.Resolved || c.ResolvedRound != 1 || s.CJ.RoundState.Verdict != VerdictApprove {
		t.Fatalf("wrong submission: %+v %+v", c, s.CJ.RoundState)
	}
	s.CJ.RoundState.Finished = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(nil, ""); err != nil {
		t.Fatal(err)
	}
	s.SetSubmission(VerdictComment, "A different round.", "Human")
	if len(s.CJ.ReviewComments) != 2 || s.CJ.ReviewComments[1].ID == id || s.CJ.ReviewComments[1].ReviewRound != 2 || s.CJ.ReviewComments[1].Resolved {
		t.Fatalf("round history lost: %+v", s.CJ.ReviewComments)
	}
}

func TestUnfinishedRoundPreservesSubmissionState(t *testing.T) {
	setupStateDir(t)
	s, err := OpenSession("", "draft")
	if err != nil {
		t.Fatal(err)
	}
	s.CJ.RoundState.ReviewCommentDraft = "Not submitted yet."
	s.CJ.RoundState.FeedbackComments = map[string]map[string]bool{"a.md": {"c_changed": true}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(nil, ""); err != nil {
		t.Fatal(err)
	}
	if s.CJ.RoundState.ReviewCommentDraft != "Not submitted yet." || !s.CJ.RoundState.FeedbackComments["a.md"]["c_changed"] || len(s.CJ.ReviewComments) != 0 {
		t.Fatal("submission state was lost or submitted")
	}
	s.CJ.RoundState.Finished = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(s.CJ.RoundState.FeedbackComments) != 0 {
		t.Fatal("feedback markers carried into a new round")
	}
}
