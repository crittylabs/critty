package tui

import "github.com/crittylabs/critty/internal/review"

func (m *AppModel) replyIDs() []string {
	ids := []string{}
	for _, tab := range m.tabs {
		if tab.state == nil {
			continue
		}
		for _, comment := range tab.state.Comments {
			for _, reply := range comment.Replies {
				ids = append(ids, reply.ID)
			}
		}
	}
	return ids
}

func (m *AppModel) restoreRound() error {
	if m.session == nil {
		return nil
	}
	if m.session.CJ.RoundState.Finished {
		m.previousReplyIDs = m.session.CJ.RoundState.SubmittedReplies
	}
	files := make(map[string]review.SnapshotFile, len(m.tabs))
	for _, tab := range m.tabs {
		if tab.loadErr != nil {
			return tab.loadErr
		}
		if tab.outsideChanges || tab.path == overallCommentsPath {
			continue
		}
		if tab.isBinary {
			files[tab.path] = review.SnapshotFile{Partial: true}
			continue
		}
		if tab.isDeleted {
			files[tab.path] = review.SnapshotFile{Textconv: tab.textconv}
			continue
		}
		doc, err := m.loadDocument(tab.path)
		if err != nil {
			return err
		}
		files[tab.path] = review.SnapshotFile{Content: doc.Content, Partial: doc.Known != nil, Textconv: tab.textconv}
	}
	rawDiff := ""
	if m.patch != nil {
		rawDiff = m.patch.Raw
	}
	if err := m.session.BeginRound(files, rawDiff); err != nil {
		return err
	}
	if err := m.beginForms(files, rawDiff); err != nil {
		return err
	}
	m.newFeedback = m.session.CJ.RoundState.NewFeedback
	return nil
}

func (m *AppModel) focusNewReply() {
	if m.initialFocus != nil || m.previousReplyIDs == nil || m.width <= 0 || m.height <= 0 {
		return
	}
	seen := make(map[string]bool, len(m.previousReplyIDs))
	for _, id := range m.previousReplyIDs {
		seen[id] = true
	}
	m.previousReplyIDs = nil
	for i := range m.tabs {
		tab := &m.tabs[i]
		comments := make(map[string]review.Comment)
		if tab.state != nil {
			for _, comment := range tab.state.Comments {
				comments[comment.ID] = comment
			}
		}
		for _, target := range m.commentTargets(i) {
			comment := comments[target.id]
			replies := comment.Replies
			for j := len(replies) - 1; j >= 0; j-- {
				reply := replies[j]
				if seen[reply.ID] || reply.Author == m.author {
					continue
				}
				if !isGeneralCommentScope(target.scope) && m.visualLineIndex(tab, lineRef{line: target.line, side: target.side}) < 0 {
					break
				}
				if m.threadScrolls == nil {
					m.threadScrolls = make(map[threadViewKey]threadScroll)
				}
				if !comment.Resolved && commentDrifted(tab, comment) {
					if tab.expandedDrifted == nil {
						tab.expandedDrifted = make(map[string]bool)
					}
					tab.expandedDrifted[comment.ID] = true
				}
				m.threadScrolls[threadViewKey{path: tab.path, id: target.id}] = threadScroll{revealReply: reply.ID}
				m.hideComments = false
				m.recalculateLayout()
				m.selectComment(i, target)
				return
			}
		}
	}
}
