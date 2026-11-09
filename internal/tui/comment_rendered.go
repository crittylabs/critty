package tui

import "strings"

type commentRenderedBody struct {
	source *markdownSource
	refs   []markdownImage
}

func (m *AppModel) commentRenderedDisplay(body string, width int, show bool) *markdownSource {
	if !show || (!m.math.Available() && !m.mermaid.Available()) || (!strings.Contains(body, "$") && !strings.Contains(body, "mermaid")) {
		return nil
	}
	if m.renderedBodies == nil {
		m.renderedBodies = make(map[string]*commentRenderedBody)
	}
	cached := m.renderedBodies[body]
	if cached == nil {
		cached = &commentRenderedBody{source: parseMarkdownSource(strings.Split(body, "\n"), nil), refs: m.renderedRefs(body)}
		if len(m.renderedBodies) < 128 {
			m.renderedBodies[body] = cached
		}
	}
	for i := range cached.source.lines {
		cached.source.lines[i].previewTokens = nil
		cached.source.lines[i].previewHidden = false
		cached.source.lines[i].previewScroll = nil
		cached.source.lines[i].previewWidth = width
	}
	refs := make([]markdownImage, 0, len(cached.refs))
	for _, ref := range cached.refs {
		key := commentImageKey{width: width, renderKind: ref.renderKind, renderBody: ref.renderBody, inline: ref.renderInline, dark: terminalIsDark}
		if side := m.commentImageSide(key); side != nil {
			ref.previewSide = *side
			refs = append(refs, ref)
		}
	}
	if m.images.enabled {
		projectRenderedDisplay(cached.source, refs)
	}
	return cached.source
}
