package tui

import (
	"net/url"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	nethtml "golang.org/x/net/html"
)

// markdownLinkRefs uses the Markdown and HTML parsers to exclude code and
// resolve links and image references as well as plain web URLs.
func markdownLinkRefs(source string) []markdownImageRef {
	data := []byte(source)
	root := parser.New(parser.WithExtensions(extension.LinkifyParser)).Parse(data)
	var refs []markdownImageRef
	add := func(start, end int, destination string) {
		if start < 0 || end < start || end > len(source) {
			return
		}
		lineStart := strings.LastIndexByte(source[:start], '\n') + 1
		if next := strings.IndexByte(source[start:end], '\n'); next >= 0 {
			end = start + next
		}
		refs = append(refs, markdownImageRef{line: strings.Count(source[:start], "\n") + 1,
			destination: destination, start: start - lineStart, end: end - lineStart})
	}
	readHTML := func(start, end int) {
		z := nethtml.NewTokenizer(strings.NewReader(source[start:end]))
		offset := start
		for {
			kind := z.Next()
			if kind == nethtml.ErrorToken {
				return
			}
			raw := z.Raw()
			if kind == nethtml.StartTagToken {
				token := z.Token()
				if token.Data == "a" {
					for _, attr := range token.Attr {
						if attr.Key == "href" {
							add(offset, offset+len(raw), attr.Val)
						}
					}
				}
			}
			offset += len(raw)
		}
	}
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			start := n.Pos()
			end := start
			if n.Reference != nil {
				end = n.Reference.Value.Index().Stop
			} else {
				end = max(n.Destination.Index().Stop, n.Title.Index().Stop)
			}
			// The parser's source ranges stop before the closing delimiter.
			end = min(max(start+1, end), len(source))
			for end < len(source) && source[end] != '\n' {
				c := source[end]
				end++
				if c == ')' || c == ']' {
					break
				}
			}
			add(start, end, n.Destination.Str(data))
		case *ast.AutoLink:
			i := n.Text.Index()
			add(i.Start, i.Stop, n.Destination.Str(data))
		case *ast.RawHTML:
			indices := n.Value.Indices()
			if len(indices) > 0 {
				readHTML(indices[0].Start, indices[len(indices)-1].Stop)
			}
		case *ast.HTMLBlock:
			segments := n.Value.Segments()
			if len(segments) > 0 {
				readHTML(segments[0].Start, segments[len(segments)-1].Stop)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil
	}
	return append(refs, markdownImageRefs(source)...)
}

func (m *AppModel) openReferenceAtCursor() tea.Cmd {
	if m.isTextModal() && m.modalFocus == 0 {
		value := m.modalTextarea.Value()
		line, column := m.modalTextarea.Line()+1, m.modalTextarea.Column()
		best := int(^uint(0) >> 1)
		var link *markdownImageRef
		// Images win ties, including an image wrapped in a Markdown link.
		for _, ref := range markdownImageRefs(value) {
			if _, ok := m.commentImageTarget(ref.destination); ok {
				best = min(best, referenceDistance(value, line, column, ref))
			}
		}
		for _, ref := range markdownLinkRefs(value) {
			if _, ok := m.linkArgument(ref.destination, true); !ok {
				continue
			}
			if distance := referenceDistance(value, line, column, ref); distance < best {
				best, link = distance, &ref
			}
		}
		if link != nil {
			return m.openLink(link.destination, true)
		}
		return m.openImageAtCursor()
	}
	t := m.tab()
	if t.isBinary || t.cursorOnAnnotation || t.cursorLine == 0 || t.cursorSide == "old" || t.doc == nil {
		return m.openImageAtCursor()
	}
	if t.isCode {
		return nil
	}
	if t.attachment != nil && t.doc.HasLine(t.cursorLine) {
		for _, span := range m.documentLinkSpans(t.cursorLine, false) {
			if cmd, ok := m.activateReference(span.uri); ok {
				return cmd
			}
		}
	}
	for _, ref := range markdownImageRefs(t.doc.Content) {
		if ref.line == t.cursorLine {
			for _, image := range t.markdownImages {
				if image.line == t.cursorLine {
					return m.openImageAtCursor()
				}
			}
			return m.openLink(ref.destination, false)
		}
	}
	for _, ref := range markdownLinkRefs(t.doc.Content) {
		if ref.line == t.cursorLine {
			if _, ok := m.linkArgument(ref.destination, false); ok {
				return m.openLink(ref.destination, false)
			}
		}
	}
	return nil
}

func (m AppModel) linkArgument(destination string, editor bool) (string, bool) {
	if web, ok := webImageURL(destination); ok {
		return web, true
	}
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return "", false
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		base := m.tab().path
		if editor {
			base = "comment.md"
			if m.session != nil {
				base = filepath.Join(m.session.Meta.WorkingDirectory(), "comment.md")
				if strings.HasPrefix(u.Path, "attachments/") {
					base = filepath.Join(m.session.Dir, "comment.md")
				}
			}
		} else if m.tab().attachment != nil {
			base = m.tab().attachment.Source
		}
		path = filepath.Join(filepath.Dir(base), path)
	}
	abs, err := filepath.Abs(path)
	return abs, err == nil && regularFile(abs)
}

func (m AppModel) openLink(destination string, editor bool) tea.Cmd {
	if m.readOnly {
		return nil
	}
	arg, ok := m.linkArgument(destination, editor)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if remoteSession() {
			return externalOpenMsg{"Not opened: this session runs over SSH"}
		}
		opener := externalOpener()
		if opener == "" {
			return externalOpenMsg{"Opening links is not supported on this platform"}
		}
		if err := startOpener(opener, arg); err != nil {
			return externalOpenMsg{"Cannot open " + destination + ": " + err.Error()}
		}
		return externalOpenMsg{"Opened " + destination}
	}
}
