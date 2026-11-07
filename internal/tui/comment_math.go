package tui

import "strings"

type commentMathBody struct {
	source *markdownSource
	refs   []markdownImage
}

func (m *AppModel) commentMathDisplay(body string, width int, show bool) *markdownSource {
	if !show || !m.math.Available() || !strings.Contains(body, "$") {
		return nil
	}
	if m.mathBodies == nil {
		m.mathBodies = make(map[string]*commentMathBody)
	}
	cached := m.mathBodies[body]
	if cached == nil {
		cached = &commentMathBody{source: parseMarkdownSource(strings.Split(body, "\n"), nil), refs: markdownMathRefs(body)}
		if len(m.mathBodies) < 128 {
			m.mathBodies[body] = cached
		}
	}
	for i := range cached.source.lines {
		cached.source.lines[i].mathTokens = nil
		cached.source.lines[i].mathHidden = false
		cached.source.lines[i].mathWidth = width
	}
	refs := make([]markdownImage, 0, len(cached.refs))
	for _, ref := range cached.refs {
		key := commentImageKey{width: width, math: ref.math, inline: ref.mathInline, dark: terminalIsDark}
		if side := m.commentImageSide(key); side != nil {
			ref.previewSide = *side
			refs = append(refs, ref)
		}
	}
	if m.images.enabled {
		projectMathDisplay(cached.source, refs)
	}
	return cached.source
}
