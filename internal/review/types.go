package review

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// The types in this file mirror the review.json ("CritJSON") format of
// tomasz-tomczyk/crit so that reviews, prompts, and agent tooling written
// for crit work against Critty reviews unchanged.  Fields Critty does not
// produce (forge sync, sharing) are kept so files round-trip losslessly.

// Reply represents a single reply in a comment thread.
type Reply struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	CreatedAt string `json:"created_at"`
	// ReviewRound is the review round during which this reply was authored.
	ReviewRound int `json:"review_round,omitempty"`
	// ResolvedRound mirrors Comment.ResolvedRound for per-reply resolution.
	ResolvedRound int `json:"resolved_round,omitempty"`
}

// Comment represents a single review comment: line-level, file-level, or
// review-level depending on Scope.
type Comment struct {
	ID          string `json:"id"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Side        string `json:"side,omitempty"`
	Body        string `json:"body"`
	Quote       string `json:"quote,omitempty"`
	QuoteOffset *int   `json:"quote_offset,omitempty"`
	// Anchor is the full text of lines StartLine..EndLine at authoring time,
	// used to re-locate the comment when line numbers drift across rounds.
	Anchor  string `json:"anchor,omitempty"`
	Drifted bool   `json:"drifted,omitempty"`
	// DriftedOnRound is the round that newly classified this comment as
	// drifted; preserved for crit compatibility.
	DriftedOnRound int    `json:"drifted_on_round,omitempty"`
	Author         string `json:"author,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	// Scope is "line", "file", or "review".  Empty means "line".
	Scope     string `json:"scope,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Resolved  bool   `json:"resolved,omitempty"`
	// ResolvedRound is the review round during which Resolved transitioned
	// false -> true; cleared to 0 when it transitions back.
	ResolvedRound  int     `json:"resolved_round,omitempty"`
	CarriedForward bool    `json:"carried_forward,omitempty"`
	ReviewRound    int     `json:"review_round,omitempty"`
	Replies        []Reply `json:"replies,omitempty"`

	// Forge-sync fields written by crit; Critty preserves but never sets them.
	GitHubID           int64  `json:"github_id,omitempty"`
	GitLabNoteID       int64  `json:"gitlab_note_id,omitempty"`
	GitLabDiscussionID string `json:"gitlab_discussion_id,omitempty"`
	GitLabResolved     *bool  `json:"gitlab_resolved,omitempty"`
	LastPushedBodyHash string `json:"last_pushed_body_hash,omitempty"`

	HeadSHA string `json:"head_sha,omitempty"`
	// DiffScope is "layer" or "full_stack" for Crit range-focus comments.
	// Working-tree filters such as staged and unstaged intentionally leave it empty.
	DiffScope string `json:"diff_scope,omitempty"`
	FocusKey  string `json:"focus_key,omitempty"`
}

// EndAt returns the effective last line of the comment's range.
func (c *Comment) EndAt() int {
	if c.EndLine > c.StartLine {
		return c.EndLine
	}
	return c.StartLine
}

// CritJSONFile is the per-file section in review files.
type CritJSONFile struct {
	Status   string    `json:"status"`
	FileHash string    `json:"file_hash,omitempty"`
	Comments []Comment `json:"comments"`
}

// CritJSON is the top-level review.json document.
type CritJSON struct {
	RoundState     RoundState              `json:"critty_state,omitempty"`
	Branch         string                  `json:"branch"`
	BaseRef        string                  `json:"base_ref"`
	UpdatedAt      string                  `json:"updated_at"`
	ReviewRound    int                     `json:"review_round"`
	ReviewComments []Comment               `json:"review_comments,omitempty"`
	CliArgs        []string                `json:"cli_args,omitempty"`
	Files          map[string]CritJSONFile `json:"files"`

	// Sharing fields written by crit; preserved but unused by Critty.
	ShareURL        string `json:"share_url,omitempty"`
	DeleteToken     string `json:"delete_token,omitempty"`
	ShareScope      string `json:"share_scope,omitempty"`
	ShareOrg        string `json:"share_org,omitempty"`
	ShareOrgName    string `json:"share_org_name,omitempty"`
	ShareVisibility string `json:"share_visibility,omitempty"`
	LastShareHash   string `json:"last_share_hash,omitempty"`
	// ActiveDiffScope mirrors Crit's current range focus ("layer" or
	// "full_stack").  It is empty for working-tree reviews, including staged.
	ActiveDiffScope string `json:"active_diff_scope,omitempty"`
}

// NewCritJSON returns an empty review document at round 1.
func NewCritJSON() CritJSON {
	return CritJSON{
		ReviewRound: 1,
		Files:       map[string]CritJSONFile{},
	}
}

// TotalComments counts review-level and per-file comments.
func (cj *CritJSON) TotalComments() int {
	n := len(cj.ReviewComments)
	for _, f := range cj.Files {
		n += len(f.Comments)
	}
	return n
}

// Touch stamps UpdatedAt with the current UTC time.
func (cj *CritJSON) Touch() {
	cj.UpdatedAt = Now()
}

// Now returns the current UTC time in the RFC 3339 format used throughout
// review.json.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func randomID(prefix string) string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}

// RandomCommentID returns a random line/file comment ID (e.g. "c_a3f8b2").
func RandomCommentID() string { return randomID("c_") }

// RandomReviewCommentID returns a random review-level comment ID (e.g. "r_b4c9e1").
func RandomReviewCommentID() string { return randomID("r_") }

// RandomReplyID returns a random reply ID (e.g. "rp_d7e2a0").
func RandomReplyID() string { return randomID("rp_") }
