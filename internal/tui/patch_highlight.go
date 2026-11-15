package tui

import (
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func (t *FileTab) highlightPatchDocument() {
	patch := t.documentPatch()
	if patch == nil {
		return
	}
	patch.before = make(map[int]string)
	for _, hunk := range patch.hunks {
		t.highlightPatchSide(patch, hunk, true)
		t.highlightPatchSide(patch, hunk, false)
	}
}

func (t *FileTab) highlightPatchSide(patch *patchDocumentCache, hunk patchDocumentHunk, before bool) {
	var refs []patchDocumentSourceLine
	var lines []string
	for _, line := range hunk.lines {
		if before && line.op == gitdiff.OpAdd || !before && line.op == gitdiff.OpDelete {
			continue
		}
		refs = append(refs, line)
		lines = append(lines, strings.TrimPrefix(t.doc.LineAt(line.line), line.op.String()))
	}
	if len(lines) == 0 {
		return
	}
	name := hunk.newName
	if before {
		name = hunk.oldName
	}
	source := strings.Join(lines, "\n") + "\n"
	lexer, configured := t.syntax.Match(name)
	if !configured {
		lexer = codeLexer(name, source)
	}
	if lexer == nil || lexer.Config().Name == "plaintext" || strings.EqualFold(lexer.Config().Name, "diff") {
		return
	}
	// Each hunk starts a fresh lexer pass; missing context must not leak state
	// into another hunk, file, or the opposite side of a replacement.
	var highlighted []string
	if strings.EqualFold(lexer.Config().Name, "markdown") {
		highlighted = t.highlightMarkdownSource(lines)
	} else {
		highlighted = newCodeHighlighter(lexer)(source)
	}
	if len(highlighted) != len(refs) {
		return
	}
	for i, ref := range refs {
		index := ref.line - 1
		if index >= len(t.chromaLines) {
			continue
		}
		prefix := ""
		if raw := t.doc.LineAt(ref.line); raw != "" {
			prefix = raw[:1]
		}
		styled := prefix + highlighted[i]
		if before {
			patch.before[ref.line] = displaySourceText(styled)
		}
		if !before || ref.op == gitdiff.OpDelete {
			t.chromaLines[index] = styled
		}
	}
}

// Backgrounds depend on terminal detection and belong to rendering, not the
// source cache built before the terminal's first background response.
func (t *FileTab) patchDisplayLine(n int, text string) string {
	patch := t.documentPatch()
	if patch == nil {
		return text
	}
	switch patch.kinds[n] {
	case gitdiff.OpAdd:
		return inlineBackground(diffChangedLineBg, text)
	case gitdiff.OpDelete:
		return inlineBackground(diffDeletedLineBg, text)
	default:
		return text
	}
}
