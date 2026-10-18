package git

import (
	"maps"
	"strings"

	"github.com/aymanbagabas/go-udiff/lcs"
)

// IgnoreWhitespace compares each replacement block without horizontal ASCII
// whitespace.  It preserves source text and coordinates, including for partial
// patches, and never changes the original diff or hides added/deleted lines
// that have no matching line on the other side.
func (info *DiffInfo) IgnoreWhitespace(newLines []string) *DiffInfo {
	if info == nil {
		return nil
	}
	filtered := &DiffInfo{
		CommonLines:  maps.Clone(info.CommonLines),
		ChangedLines: make(map[int]bool), InlineChanges: make(map[int][]InlineSegment),
		DeletedAfter: make(map[int][]DeletedLine),
	}
	if filtered.CommonLines == nil {
		filtered.CommonLines = make(map[int]int)
	}
	for line, changed := range info.ChangedLines {
		filtered.ChangedLines[line] = changed
	}
	for line, segments := range info.InlineChanges {
		filtered.InlineChanges[line] = segments
	}
	for anchor, deleted := range info.DeletedAfter {
		var added []string
		for line := anchor + 1; info.ChangedLines[line] && line <= len(newLines); line++ {
			added = append(added, newLines[line-1])
		}
		oldKeys := make([]string, len(deleted))
		newKeys := make([]string, len(added))
		for i, line := range deleted {
			oldKeys[i] = withoutWhitespace(line.Content)
		}
		for i, line := range added {
			newKeys[i] = withoutWhitespace(line)
			delete(filtered.ChangedLines, anchor+i+1)
			delete(filtered.InlineChanges, anchor+i+1)
		}
		changes := lcs.DiffLines(oldKeys, newKeys)
		for _, change := range changes {
			after := anchor + change.ReplStart
			for i := change.Start; i < change.End; i++ {
				line := deleted[i]
				line.Inline = nil
				filtered.DeletedAfter[after] = append(filtered.DeletedAfter[after], line)
			}
			for i := change.ReplStart; i < change.ReplEnd; i++ {
				filtered.ChangedLines[anchor+i+1] = true
			}
			if change.End-change.Start == change.ReplEnd-change.ReplStart {
				for i := 0; i < change.End-change.Start; i++ {
					oldSegments, newSegments := whitespaceInlineDiff(deleted[change.Start+i].Content, added[change.ReplStart+i])
					filtered.DeletedAfter[after][i].Inline = oldSegments
					filtered.InlineChanges[after+i+1] = newSegments
				}
			}
		}
		// Equal lines still need their original coordinates after filtering.
		old, next := 0, 0
		for _, change := range changes {
			for next < change.ReplStart {
				filtered.CommonLines[anchor+next+1] = deleted[old].OldLineNum
				old++
				next++
			}
			old, next = change.End, change.ReplEnd
		}
		for next < len(added) && old < len(deleted) {
			filtered.CommonLines[anchor+next+1] = deleted[old].OldLineNum
			old++
			next++
		}
	}
	return filtered
}

func horizontalWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\v' || r == '\f'
}

func withoutWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if horizontalWhitespace(r) {
			return -1
		}
		return r
	}, s)
}

// Match non-whitespace runes, then project changes onto the original text so
// indentation and spacing remain visible without receiving word highlights.
func whitespaceInlineDiff(before, after string) ([]InlineSegment, []InlineSegment) {
	oldRunes := []rune(withoutWhitespace(before))
	newRunes := []rune(withoutWhitespace(after))
	oldChanged, newChanged := make(map[int]bool), make(map[int]bool)
	for _, change := range lcs.DiffRunes(oldRunes, newRunes) {
		for i := change.Start; i < change.End; i++ {
			oldChanged[i] = true
		}
		for i := change.ReplStart; i < change.ReplEnd; i++ {
			newChanged[i] = true
		}
	}
	segments := func(text string, changed map[int]bool) []InlineSegment {
		var result []InlineSegment
		i, start := 0, 0
		active := false
		for pos, r := range text {
			space := horizontalWhitespace(r)
			isChanged := !space && changed[i]
			if isChanged != active {
				result = appendInlineSegment(result, text[start:pos], active)
				start, active = pos, isChanged
			}
			if !space {
				i++
			}
		}
		return appendInlineSegment(result, text[start:], active)
	}
	return segments(before, oldChanged), segments(after, newChanged)
}
