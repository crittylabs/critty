package tui

import "github.com/crittylabs/critty/internal/document"

const overallCommentsPath = "\x00review"

func isGeneralCommentScope(scope string) bool {
	return scope == "file" || scope == "review"
}

func (m *AppModel) ensureOverallCommentsTab() {
	if m.session == nil || len(m.session.CJ.ReviewComments) == 0 {
		return
	}
	for _, tab := range m.tabs {
		if tab.path == overallCommentsPath {
			return
		}
	}
	tab := newFileTab(overallCommentsPath, nil)
	tab.state = &fileReview{Comments: m.session.CJ.ReviewComments}
	tab.doc = overallCommentsDocument()
	tab.ensureHighlightCache()
	m.tabs = append(m.tabs, tab)
	m.multiFile = true
}

func overallCommentsDocument() *document.Document {
	return document.FromContent("review.md", []byte("# Overall review comments\n\nUse `r` to resolve a thread or `enter` to reply.\n"))
}
