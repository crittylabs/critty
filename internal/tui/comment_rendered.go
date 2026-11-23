package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
)

type commentBodyKey struct {
	body, language string
}

type commentRenderedBody struct {
	source *markdownSource
	refs   []markdownImage
}

func (m *AppModel) commentSuggestionLexer() chroma.Lexer {
	if len(m.tabs) == 0 {
		return nil
	}
	t := m.tab()
	if t.lexer != nil {
		return t.lexer
	}
	source := ""
	if t.doc != nil {
		source = t.doc.Content
	}
	return t.codeLexer(source)
}

func (m *AppModel) parsedComment(body string, lexer chroma.Lexer) *commentRenderedBody {
	if m.renderedBodies == nil {
		m.renderedBodies = make(map[commentBodyKey]*commentRenderedBody)
	}
	key := commentBodyKey{body: body}
	if lexer != nil {
		key.language = lexer.Config().Name
	}
	cached := m.renderedBodies[key]
	if cached == nil {
		cached = &commentRenderedBody{source: parseMarkdownWithSyntax(strings.Split(body, "\n"), nil, m.syntax, m.icons, lexer)}
		if strings.Contains(body, "$") || strings.Contains(body, "mermaid") {
			cached.refs = m.renderedRefs(body)
		}
		if len(m.renderedBodies) >= 128 {
			clear(m.renderedBodies)
		}
		m.renderedBodies[key] = cached
	}
	return cached
}

func (m *AppModel) commentRenderedDisplay(body string, width int, show bool, suggestion ...chroma.Lexer) *markdownSource {
	lexer := m.commentSuggestionLexer()
	if len(suggestion) > 0 {
		lexer = suggestion[0]
	}
	cached := m.parsedComment(body, lexer)
	cached.source.updateRemoteImageMarks(func(uri string) string {
		url, _ := webImageURL(uri)
		target := imageTarget{url: url}
		side := m.commentImages[commentImageKey{target: target, width: width}]
		return remoteImageMark(m.icons, m.remoteImages.Allows(url), side)
	})
	for i := range cached.source.lines {
		cached.source.lines[i].previewTokens = nil
		cached.source.lines[i].previewHidden = false
		cached.source.lines[i].previewScroll = nil
		cached.source.lines[i].previewWidth = width
	}
	if !show {
		return cached.source
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
		projectRenderedDisplay(cached.source, refs, 0)
	}
	return cached.source
}
