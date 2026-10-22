package git

import (
	"strings"

	"github.com/rivo/uniseg"
)

const maxLineDistance = 0.6
const maxAlignmentComparisons = 4096
const maxAlignmentWork = 1 << 20
const maxInlineBytes = 64 << 10

type replacementLine struct {
	line    int
	content string
}

// annotateReplacement greedily matches similar lines in source order.  Skipped
// additions and unmatched deletions stay independent instead of shifting pairs.
func (info *DiffInfo) annotateReplacement(deleted []*DeletedLine, added []replacementLine, ignoreWhitespace bool) {
	if len(added) == 0 {
		return
	}
	if info.PairedLines == nil {
		info.PairedLines = make(map[int]int)
	}
	tokens := make([][]string, len(added))
	for i, line := range added {
		info.PairedLines[line.line] = 0
		if len(line.content) <= maxInlineBytes {
			tokens[i] = splitInlineTokens(line.content)
		}
	}
	next, comparisons, work := 0, 0, maxAlignmentWork
	for _, old := range deleted {
		if len(old.Content) > maxInlineBytes {
			continue
		}
		oldTokens := splitInlineTokens(old.Content)
		words := make(map[string]bool, len(oldTokens))
		for _, token := range oldTokens {
			if strings.TrimSpace(token) != "" {
				words[token] = true
			}
		}
		for j := next; j < len(added); j++ {
			if comparisons >= maxAlignmentComparisons {
				return
			}
			comparisons++
			if len(added[j].content) > maxInlineBytes || len(oldTokens)+1 > work/(len(tokens[j])+1) {
				continue
			}
			shared := false
			for _, token := range tokens[j] {
				if words[token] {
					shared = true
					break
				}
			}
			if !shared && (len(words) > 0 || strings.TrimSpace(added[j].content) != "") {
				continue
			}
			work -= (len(oldTokens) + 1) * (len(tokens[j]) + 1)
			oldSegments, newSegments := inlineDiffTokens(oldTokens, tokens[j])
			if lineDistance(oldSegments, newSegments) > maxLineDistance {
				continue
			}
			if ignoreWhitespace {
				oldSegments, newSegments = whitespaceInlineDiff(old.Content, added[j].content)
			}
			info.PairedLines[added[j].line] = old.OldLineNum
			old.Inline = oldSegments
			info.InlineChanges[added[j].line] = newSegments
			next = j + 1
			break
		}
	}
}

// Measure changed non-whitespace width relative to the total width on both
// sides, so a shared prefix or suffix makes a replacement a plausible pair.
func lineDistance(before, after []InlineSegment) float64 {
	changed, total := 0, 0
	for _, segments := range [][]InlineSegment{before, after} {
		for _, segment := range segments {
			width := uniseg.StringWidth(strings.TrimSpace(segment.Content))
			total += width
			if segment.Changed {
				changed += width
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(changed) / float64(total)
}
