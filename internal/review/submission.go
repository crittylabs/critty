package review

import "strings"

type Verdict string

const (
	VerdictApprove Verdict = "approve"
	VerdictReject  Verdict = "reject"
	VerdictComment Verdict = "comment"
)

func (v Verdict) Valid() bool {
	return v == VerdictApprove || v == VerdictReject || v == VerdictComment
}

// SetSubmission records a round's decision and its optional review-level
// comment.  Reusing the comment ID makes failed saves safe to retry.
func (s *Session) SetSubmission(verdict Verdict, body, author string) {
	body = strings.TrimSpace(body)
	state := &s.CJ.RoundState
	state.Verdict = verdict
	for i := range s.CJ.ReviewComments {
		c := &s.CJ.ReviewComments[i]
		if c.ID != state.ReviewCommentID {
			continue
		}
		if body == "" {
			s.CJ.ReviewComments = append(s.CJ.ReviewComments[:i], s.CJ.ReviewComments[i+1:]...)
			state.ReviewCommentID = ""
			return
		}
		c.Body, c.UpdatedAt = body, Now()
		c.Resolved = verdict == VerdictApprove
		c.ResolvedRound = 0
		if c.Resolved {
			c.ResolvedRound = s.CJ.ReviewRound
		}
		return
	}
	if body == "" {
		return
	}
	s.AppendReviewComment(body, author, "")
	c := &s.CJ.ReviewComments[len(s.CJ.ReviewComments)-1]
	state.ReviewCommentID = c.ID
	c.Resolved = verdict == VerdictApprove
	if c.Resolved {
		c.ResolvedRound = s.CJ.ReviewRound
	}
}

func (s *Session) SubmissionComment() *Comment {
	for _, c := range s.CJ.ReviewComments {
		if c.ID == s.CJ.RoundState.ReviewCommentID {
			return &c
		}
	}
	return nil
}
