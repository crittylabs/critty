package review

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// NewSession allocates independent storage even for identical review arguments.
func NewSession(dataRoot string, meta SessionEntry) (*Session, error) {
	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("creating session ID: %w", err)
	}
	key := hex.EncodeToString(id[:])
	if err := os.MkdirAll(ReviewsRoot(dataRoot), 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(Dir(dataRoot, key), 0o700); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	s, err := OpenSession(dataRoot, key)
	if err != nil {
		return nil, err
	}
	meta.Key = key
	s.Meta = meta
	return s, nil
}

// SnapshotFile preserves the text used to anchor comments across TUI restarts.
type SnapshotFile struct {
	Content string `json:"content"`
	Partial bool   `json:"partial,omitempty"`
}

// RoundState is Critty's saved runtime context, separate from CritJSON comments.
type RoundState struct {
	Files              map[string]SnapshotFile    `json:"files,omitempty"`
	Diff               string                     `json:"diff,omitempty"`
	Finished           bool                       `json:"finished,omitempty"`
	NewFeedback        bool                       `json:"new_feedback,omitempty"`
	Verdict            Verdict                    `json:"verdict,omitempty"`
	ReviewCommentID    string                     `json:"review_comment_id,omitempty"`
	FeedbackComments   map[string]map[string]bool `json:"feedback_comments,omitempty"`
	ReviewCommentDraft string                     `json:"review_comment_draft,omitempty"`
	// Nil means no baseline was recorded; an empty array means no replies
	// existed at submission.  Preserve that distinction in saved sessions.
	SubmittedReplies []string `json:"submitted_replies"`
}

// BeginRound restores the persisted round and remaps anchors onto fresh content.
// A stopped, unfinished round keeps its number and feedback state.
func (s *Session) BeginRound(files map[string]SnapshotFile, rawDiff string) error {
	return s.Update(func(current *Session) error {
		previous := current.CJ.RoundState
		now := Now()
		for path, next := range files {
			file := current.CJ.Files[path]
			prev, exists := previous.Files[path]
			if exists && (previous.Finished || prev != next) {
				if prev.Partial || next.Partial {
					file.Comments = CarryForwardPartial(file.Comments, prev != next, now)
				} else {
					file.Comments = CarryForwardFile(file.Comments, prev.Content, next.Content, now)
				}
			}
			if previous.Diff != "" && previous.Diff != rawDiff {
				for i := range file.Comments {
					if file.Comments[i].Side == "old" {
						file.Comments[i].Drifted = true
					}
				}
			}
			current.CJ.Files[path] = file
		}
		if previous.Finished {
			current.CJ.ReviewRound++
			previous.NewFeedback = false
		}
		// Retain snapshots for files absent from the new scope, so their
		// comments can still be remapped if the files return in a later round.
		for path, prev := range previous.Files {
			if _, exists := files[path]; !exists {
				files[path] = prev
			}
		}
		state := RoundState{Files: files, Diff: rawDiff, NewFeedback: previous.NewFeedback}
		if !previous.Finished {
			state.Verdict = previous.Verdict
			state.ReviewCommentID = previous.ReviewCommentID
			state.ReviewCommentDraft = previous.ReviewCommentDraft
			state.FeedbackComments = previous.FeedbackComments
		}
		current.CJ.RoundState = state
		return nil
	})
}
