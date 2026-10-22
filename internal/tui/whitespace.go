package tui

import (
	"sort"

	gitpkg "github.com/crittylabs/critty/internal/git"
)

// Keep the status width stable so toggling it does not move neighboring controls.
func (m AppModel) whitespaceButton() string {
	if !m.multiFile {
		return ""
	}
	state := "none"
	if m.ignoreWhitespace {
		state = "WS  "
	}
	style := modalBtnFocusedLabel.Bold(true).Padding(0)
	return style.Render("Ignore: "+state+" ") + modalBtnFocusedKey.Render("W")
}

func (m *AppModel) toggleWhitespace() {
	m.ignoreWhitespace = !m.ignoreWhitespace
	for i := range m.tabs {
		t := &m.tabs[i]
		if t.diff == nil || t.doc == nil {
			continue
		}
		info := t.diff
		if m.ignoreWhitespace {
			info = info.IgnoreWhitespace(t.doc.Lines)
		}
		t.changeChunks = computeChangeChunks(info)
		if m.ignoreWhitespace {
			keepCommentedDeletions(t, info)
		}
		t.changedLines, t.inlineChanges, t.deletedAfter = info.ChangedLines, info.InlineChanges, info.DeletedAfter
		t.commonLines = info.CommonLines
		t.pairedLines = info.PairedLines
		t.chromaLines = nil
		t.deletedLineCache = nil
		t.ensureHighlightCache()
		if t.cursorSide == "old" {
			visible := false
			for _, lines := range t.deletedAfter {
				for _, line := range lines {
					visible = visible || line.OldLineNum == t.cursorLine
				}
			}
			if !visible {
				for anchor, lines := range t.diff.DeletedAfter {
					for _, line := range lines {
						if line.OldLineNum == t.cursorLine {
							t.cursorLine = min(anchor+1, t.doc.LineCount())
							t.cursorSide = ""
							t.cursorOnAnnotation = false
							break
						}
					}
					if t.cursorSide == "" {
						break
					}
				}
			}
		}
		// A selection may include old lines that are no longer displayed.
		if t.selectSide == "old" {
			t.selecting = false
		}
	}
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

// Keep the old-side context for existing threads, even when the replacement
// itself is ignored.  These retained lines are not change-navigation targets.
func keepCommentedDeletions(t *FileTab, info *gitpkg.DiffInfo) {
	if t.state == nil {
		return
	}
	visible := make(map[int]bool)
	for _, lines := range info.DeletedAfter {
		for _, line := range lines {
			visible[line.OldLineNum] = true
		}
	}
	for anchor, lines := range t.diff.DeletedAfter {
		for _, line := range lines {
			if visible[line.OldLineNum] {
				continue
			}
			for _, c := range t.state.Comments {
				if c.Scope != "file" && c.Side == "old" && c.StartLine <= line.OldLineNum && c.EndAt() >= line.OldLineNum {
					line.Inline = nil
					info.DeletedAfter[anchor] = append(info.DeletedAfter[anchor], line)
					break
				}
			}
		}
		sort.Slice(info.DeletedAfter[anchor], func(i, j int) bool {
			return info.DeletedAfter[anchor][i].OldLineNum < info.DeletedAfter[anchor][j].OldLineNum
		})
	}
}
