package tui

import (
	"slices"

	"github.com/crittylabs/critty/internal/review"
)

// reviewEditKind names a review action that can be undone.
type reviewEditKind int

const (
	resolveEdit reviewEditKind = iota
	deleteCommentEdit
	deleteReplyEdit
)

// reviewEdit records one resolve or delete so it can be undone and redone.
// comment holds the thread as it was before the edit; for a deleted reply,
// reply and index locate it within that thread.
type reviewEdit struct {
	kind    reviewEditKind
	path    string
	comment review.Comment
	index   int
	reply   review.Reply
	advance bool // `R` rather than `r` or space
}

const reviewUndoLimit = 100

// recordReviewEdit pushes an edit for undo.  A new edit discards the redo
// history unless it is itself a redo.
func (m *AppModel) recordReviewEdit(edit reviewEdit) {
	edit.comment.Replies = slices.Clone(edit.comment.Replies)
	m.reviewUndo = append(m.reviewUndo, edit)
	if len(m.reviewUndo) > reviewUndoLimit {
		m.reviewUndo = slices.Delete(m.reviewUndo, 0, len(m.reviewUndo)-reviewUndoLimit)
	}
	if !m.redoing {
		m.reviewRedo = nil
	}
}

func (m *AppModel) tabIndexForPath(path string) int {
	return slices.IndexFunc(m.tabs, func(t FileTab) bool { return t.path == path })
}

// undoReview reverts the last resolve or delete, restoring the thread and
// moving the cursor back to it so `R` followed by undo returns to the
// thread it left.
func (m *AppModel) undoReview() bool {
	n := len(m.reviewUndo)
	if n == 0 {
		return false
	}
	edit := m.reviewUndo[n-1]
	m.reviewUndo = m.reviewUndo[:n-1]
	tabIndex := m.tabIndexForPath(edit.path)
	if tabIndex < 0 || m.tabs[tabIndex].state == nil {
		return false
	}
	comments := &m.tabs[tabIndex].state.Comments
	switch edit.kind {
	case resolveEdit:
		for i := range *comments {
			c := &(*comments)[i]
			if c.ID == edit.comment.ID {
				c.Resolved, c.ResolvedRound, c.UpdatedAt = edit.comment.Resolved, edit.comment.ResolvedRound, edit.comment.UpdatedAt
				break
			}
		}
	case deleteCommentEdit:
		*comments = slices.Insert(*comments, min(edit.index, len(*comments)), edit.comment)
	case deleteReplyEdit:
		for i := range *comments {
			c := &(*comments)[i]
			if c.ID == edit.comment.ID {
				c.Replies = slices.Insert(c.Replies, min(edit.index, len(c.Replies)), edit.reply)
				c.UpdatedAt = edit.comment.UpdatedAt
				break
			}
		}
	}
	m.reviewRedo = append(m.reviewRedo, edit)
	if m.modal == finishModal {
		m.closeFinishModal()
	}
	m.persist()
	m.focusComment(tabIndex, edit.comment.ID)
	return true
}

// redoReview performs the last undone resolve or delete again, as if its
// key had been pressed.
func (m *AppModel) redoReview() bool {
	n := len(m.reviewRedo)
	if n == 0 {
		return false
	}
	edit := m.reviewRedo[n-1]
	m.reviewRedo = m.reviewRedo[:n-1]
	tabIndex := m.tabIndexForPath(edit.path)
	if tabIndex < 0 {
		return false
	}
	m.activateTab(tabIndex)
	m.redoing = true
	defer func() { m.redoing = false }()
	switch edit.kind {
	case resolveEdit:
		m.resolveThread(edit.comment.ID, edit.advance)
	default:
		m.deleteFromThread(tabIndex, edit.comment.ID, edit.reply.ID)
	}
	return true
}

// focusComment places the cursor on a thread and shows it.
func (m *AppModel) focusComment(tabIndex int, id string) {
	for _, target := range m.reviewTargets(tabIndex) {
		if target.id == id {
			m.selectCommentTarget(tabIndex, target, 0)
			return
		}
	}
	m.rebuildContent()
	m.updateCommentSidebar()
}

// deleteFromThread removes a comment, or one of its replies when replyID
// is set, records the deletion for undo, and returns the cursor to the
// source line.
func (m *AppModel) deleteFromThread(tabIndex int, commentID, replyID string) bool {
	t := &m.tabs[tabIndex]
	if t.state == nil {
		return false
	}
	deleted := false
	for i := range t.state.Comments {
		c := &t.state.Comments[i]
		if c.ID != commentID {
			continue
		}
		if replyID == "" {
			m.recordReviewEdit(reviewEdit{kind: deleteCommentEdit, path: t.path, comment: *c, index: i})
			t.state.Comments = slices.Delete(t.state.Comments, i, i+1)
			deleted = true
			break
		}
		j := slices.IndexFunc(c.Replies, func(r review.Reply) bool { return r.ID == replyID })
		if j < 0 {
			break
		}
		m.recordReviewEdit(reviewEdit{kind: deleteReplyEdit, path: t.path, comment: *c, index: j, reply: c.Replies[j]})
		c.Replies = slices.Delete(c.Replies, j, j+1)
		c.UpdatedAt = review.Now()
		deleted = true
		break
	}
	if !deleted {
		return false
	}
	m.persist()
	t.cursorOnAnnotation = false
	t.cursorAnnoIdx = 0
	if t.cursorLine == 0 {
		m.moveCursorBy(t, 1, 1)
	}
	m.rebuildContent()
	m.updateCommentSidebar()
	return true
}
