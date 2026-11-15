package tui

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	nethtml "golang.org/x/net/html"
)

var markdownLinkParser = parser.New(parser.WithExtensions(extension.LinkifyParser))

// markdownLinkRefs uses the Markdown and HTML parsers to exclude code and
// resolve links and plain web URLs.  Images are collected separately.
func markdownLinkRefs(source string) []markdownImageRef {
	data := []byte(source)
	root := markdownLinkParser.Parse(data)
	lines := sourceLineCursor{text: source}
	var refs []markdownImageRef
	add := func(start, end int, destination string) {
		if start < 0 || end < start || end > len(source) {
			return
		}
		line, column := lines.position(start)
		if next := strings.IndexByte(source[start:end], '\n'); next >= 0 {
			end = start + next
		}
		refs = append(refs, markdownImageRef{line: line,
			destination: destination, start: column, end: column + end - start})
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
	return refs
}

func (m *AppModel) openReferenceAtCursor() tea.Cmd {
	if m.isTextModal() && m.modalFocus == 0 {
		return m.openCommentReference(true)
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
			cmd, _ := m.followLink(ref.destination)
			return cmd
		}
	}
	for _, ref := range markdownLinkRefs(t.doc.Content) {
		if ref.line == t.cursorLine {
			if cmd, ok := m.followLink(ref.destination); ok {
				return cmd
			}
		}
	}
	return nil
}

func (m *AppModel) openCommentReference(includeLinks bool) tea.Cmd {
	value := m.modalTextarea.Value()
	images := markdownImageRefs(value)
	distance := referenceDistance(value, m.modalTextarea.Line()+1, m.modalTextarea.Column())
	best, image := int(^uint(0)>>1), -1
	// Images win ties, including an image wrapped in a Markdown link.
	for i, ref := range images {
		if _, ok := m.commentImageTarget(ref.destination); ok {
			if d := distance(ref); d < best {
				best, image = d, i
			}
		}
	}
	if includeLinks {
		var link *markdownImageRef
		for _, ref := range append(markdownLinkRefs(value), images...) {
			if _, ok := m.linkArgument(ref.destination, true); !ok {
				continue
			}
			if d := distance(ref); d < best {
				best, link = d, &ref
			}
		}
		if link != nil {
			return m.openLink(link.destination, true)
		}
	}
	if image < 0 {
		return nil
	}
	target, _ := m.commentImageTarget(images[image].destination)
	var siblings []imageTarget
	for _, ref := range images {
		if ref.line == images[image].line {
			if sibling, ok := m.commentImageTarget(ref.destination); ok {
				siblings = append(siblings, sibling)
			}
		}
	}
	return m.openImageWithSiblings(target, siblings)
}

// referenceDistance prepares rune distances from one cursor to reference
// spans.  Wide and combining text use rune positions, not screen columns.
func referenceDistance(value string, line, column int) func(markdownImageRef) int {
	lines := strings.Split(value, "\n")
	starts := make([]int, len(lines)+1)
	for i, text := range lines {
		starts[i+1] = starts[i] + utf8.RuneCountInString(text) + 1
	}
	cursor := starts[min(max(0, line-1), len(lines))] + column
	return func(ref markdownImageRef) int {
		if ref.line < 1 || ref.line > len(lines) {
			return int(^uint(0) >> 1)
		}
		text := lines[ref.line-1]
		start := starts[ref.line-1] + utf8.RuneCountInString(text[:min(ref.start, len(text))])
		end := starts[ref.line-1] + utf8.RuneCountInString(text[:min(ref.end, len(text))])
		return max(start-cursor, cursor-end, 0)
	}
}

func (m AppModel) linkArgument(destination string, editor bool) (string, bool) {
	if web, ok := webImageURL(destination); ok {
		return web, true
	}
	path, ok := m.localLinkPath(destination, editor)
	return path, ok && regularFile(path)
}

func (m AppModel) localLinkPath(destination string, editor bool) (string, bool) {
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return "", false
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		base := m.sourcePath(m.tab().path)
		if editor {
			base = "comment.md"
			if m.session != nil {
				base = filepath.Join(m.session.Meta.WorkingDirectory(), "comment.md")
				if strings.HasPrefix(u.Path, "attachments/") {
					base = filepath.Join(m.session.Dir, "comment.md")
				}
			}
		} else if m.tab().attachment != nil {
			base = m.sourcePath(m.tab().attachment.Source)
		}
		path = filepath.Join(filepath.Dir(base), path)
	}
	abs, err := filepath.Abs(path)
	return abs, err == nil
}

func (m *AppModel) followLink(destination string) (tea.Cmd, bool) {
	if m.readOnly {
		return m.followViewLink(destination)
	}
	if path, ok := m.localLinkPath(destination, false); ok {
		if m.focusSource(sourceLocation{path: path}) {
			return nil, true
		}
		if !regularFile(path) {
			m.locationError = "File does not exist on disk."
			return nil, true
		}
		m.locationError = ""
		return m.openUnreviewedLink(destination, path), true
	}
	cmd := m.openLink(destination, false)
	return cmd, cmd != nil
}

func (m *AppModel) openUnreviewedLink(destination, path string) tea.Cmd {
	editor := m.openSourceEditor(sourceLocation{path: path})
	external := m.openLink(destination, false)
	tab := FileTab{path: path, syntax: m.syntax}
	return func() tea.Msg {
		file, err := os.Open(path)
		if err != nil {
			return externalOpenMsg{"Cannot open " + destination + ": " + err.Error()}
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, maxShebangLineBytes+1))
		if err != nil {
			return externalOpenMsg{"Cannot read " + destination + ": " + err.Error()}
		}
		cmd := external
		if lexer := tab.codeLexer(string(data)); markdownPath(path) ||
			lexer != nil && lexer.Config().Name != "plaintext" ||
			strings.HasPrefix(http.DetectContentType(data), "text/") {
			cmd = editor
		}
		if cmd == nil {
			return nil
		}
		return cmd()
	}
}

func (m AppModel) openLink(destination string, editor bool) tea.Cmd {
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
