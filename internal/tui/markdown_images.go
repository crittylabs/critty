package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	nethtml "golang.org/x/net/html"
)

type markdownImage struct {
	line   int
	before bool
	path   string
	url    string // web URL that is linked but not previewed
	local  bool
	// start and end bound the reference text on its line.
	start, end    int
	columns, rows int // retain layout while pixels are released or reloading
	previewSide
}

func (ref markdownImage) target() imageTarget {
	if ref.url != "" {
		return imageTarget{url: ref.url, line: ref.line}
	}
	return imageTarget{path: ref.path, before: ref.before, line: ref.line}
}

type markdownImagesMsg struct {
	path   string
	images []markdownImage
}

type markdownImageRef struct {
	line        int
	destination string
	// start and end bound the reference text within its line.
	start, end int
}

func markdownImageRefs(source string) []markdownImageRef {
	data := []byte(source)
	root := parser.New().Parse(data)
	var refs []markdownImageRef
	add := func(offset, end int, destination string) {
		if offset < 0 || offset > len(source) {
			return
		}
		lineStart := strings.LastIndex(source[:offset], "\n") + 1
		lineEnd := len(source)
		if i := strings.IndexByte(source[offset:], '\n'); i >= 0 {
			lineEnd = offset + i
		}
		end = max(offset, min(end, lineEnd))
		refs = append(refs, markdownImageRef{strings.Count(source[:offset], "\n") + 1, destination, offset - lineStart, end - lineStart})
	}
	// Inline image syntax carries no end position; the reference closes at
	// the parenthesis after the destination on the same line.
	inlineEnd := func(offset int, destination string) int {
		rest := source[offset:]
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			rest = rest[:i]
		}
		if at := strings.Index(rest, "]("); at >= 0 {
			at += 2
			if i := strings.Index(rest[at:], destination); destination != "" && i >= 0 {
				at += i + len(destination)
			}
			if i := strings.IndexByte(rest[at:], ')'); i >= 0 {
				return offset + at + i + 1
			}
			return offset + len(rest)
		}
		// Reference-style images end at the label's closing bracket.
		at := 0
		if i := strings.Index(rest, "]["); i >= 0 {
			at = i + 2
		}
		if i := strings.IndexByte(rest[at:], ']'); i >= 0 {
			return offset + at + i + 1
		}
		return offset + len(rest)
	}
	readHTML := func(start, end int) {
		if start < 0 || end > len(source) || start > end {
			return
		}
		z := nethtml.NewTokenizer(strings.NewReader(source[start:end]))
		offset := start
		for {
			kind := z.Next()
			if kind == nethtml.ErrorToken {
				break
			}
			raw := z.Raw()
			if kind == nethtml.StartTagToken || kind == nethtml.SelfClosingTagToken {
				token := z.Token()
				if token.Data == "img" {
					for _, attr := range token.Attr {
						if attr.Key == "src" {
							add(offset, offset+len(raw), attr.Val)
							break
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
		case *ast.Image:
			destination := n.Destination.Str(data)
			add(n.Pos(), inlineEnd(n.Pos(), destination), destination)
		case *ast.RawHTML:
			segments := n.Value.Indices()
			if len(segments) > 0 {
				readHTML(segments[0].Start, segments[len(segments)-1].Stop)
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
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].line < refs[j].line })
	return refs
}

func localImagePath(documentPath, destination string) (string, bool) {
	u, err := url.Parse(destination)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" || strings.HasPrefix(destination, "//") {
		return "", false
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(documentPath), path)
	}
	return filepath.Clean(path), imagePath(path)
}

func (t *FileTab) hasLocalMarkdownImages() bool {
	for _, ref := range t.markdownImages {
		if ref.url == "" {
			return true
		}
	}
	return false
}

func (t *FileTab) imageSide(before bool, markdown int) *previewSide {
	if markdown > 0 && markdown <= len(t.markdownImages) {
		return &t.markdownImages[markdown-1].previewSide
	}
	return t.preview.side(before)
}

func (m AppModel) imageSource() gitpkg.ReviewSource {
	if m.source != nil {
		return *m.source
	}
	s := gitpkg.ReviewSource{Base: m.baseRef}
	if m.staged {
		s.Scope = "staged"
	}
	return s
}

func (m AppModel) findMarkdownImages() tea.Cmd {
	t := *m.tab()
	source := m.imageSource()
	return func() tea.Msg {
		msg := markdownImagesMsg{path: t.path, images: []markdownImage{}}
		local := t.attachment != nil || !m.multiFile
		docPath := t.path
		if t.attachment != nil {
			docPath = t.attachment.Source
		}
		appendRefs := func(content, path string, before bool) {
			for _, ref := range markdownImageRefs(content) {
				if len(msg.images) >= 64 {
					break
				}
				img := markdownImage{line: ref.line, before: before, local: local, start: ref.start, end: ref.end}
				if u, ok := webImageURL(ref.destination); ok {
					img.url = u
					msg.images = append(msg.images, img)
					continue
				}
				imagePath, ok := localImagePath(path, ref.destination)
				if !ok {
					continue
				}
				if !local {
					abs, err := filepath.Abs(imagePath)
					cwd, cwdErr := os.Getwd()
					if err != nil || cwdErr != nil {
						continue
					}
					imagePath, err = filepath.Rel(cwd, abs)
					if err != nil || imagePath == ".." || strings.HasPrefix(imagePath, ".."+string(filepath.Separator)) {
						continue
					}
					if !before && source.Scope != "staged" && source.Scope != "range" && !repositoryImagePath(cwd, abs) {
						continue
					}
				}
				img.path = imagePath
				msg.images = append(msg.images, img)
			}
		}
		if t.doc != nil {
			appendRefs(t.doc.Content, docPath, false)
		}
		if !local && len(t.deletedAfter) > 0 {
			if t.oldPath != "" {
				docPath = t.oldPath
			}
			if data, err := source.PreviewContent(docPath, true, maxPreviewBytes); err == nil {
				appendRefs(string(data), docPath, true)
			}
		}
		return msg
	}
}

func repositoryImagePath(root, path string) bool {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if os.IsNotExist(err) {
		// Missing paths are handled by the bounded image reader.
		return true
	}
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, parent)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m AppModel) loadMarkdownImage(index int) tea.Cmd {
	ref := m.tab().markdownImages[index]
	path := m.tab().path
	source := m.imageSource()
	return func() tea.Msg {
		if ref.local {
			source = gitpkg.ReviewSource{}
		}
		data, err := source.PreviewContent(ref.path, ref.before && !ref.local, maxPreviewBytes)
		return m.images.encode(data, imageLoadedMsg{path: path, markdown: index + 1, before: ref.before, err: err})
	}
}

func (m AppModel) markdownImageSize(img *inlineImage) (int, int) {
	return img.size(min(44, m.contentViewport.Width()-gutterWidth), min(19, max(5, m.contentViewport.Height())), m.images.cell)
}

func (m AppModel) markdownImageVisible(ref markdownImage) bool {
	ranges := m.contentLayout.lineRanges
	if ref.before {
		ranges = m.contentLayout.oldRanges
	}
	r, ok := ranges[ref.line]
	top := m.contentViewport.YOffset()
	return ok && r.start < top+m.contentViewport.Height() && r.end > top
}

// Keep dimensions when releasing pixels so scrolling back can reload the
// image without changing the document's layout.
func (m *AppModel) releaseHiddenMarkdownImages() tea.Cmd {
	if m.modal != noModal {
		return nil
	}
	var sequence strings.Builder
	for i := range m.tabs {
		for j := range m.tabs[i].markdownImages {
			ref := &m.tabs[i].markdownImages[j]
			img := ref.image
			if img == nil || img.id == 0 || i == m.activeTab && !m.images.hidden && m.markdownImageVisible(*ref) {
				continue
			}
			fmt.Fprintf(&sequence, "\x1b_Ga=d,d=I,q=2,i=%d\x1b\\", img.id)
			ref.columns, ref.rows = img.columns, img.rows
			ref.image, ref.attempted = nil, false
		}
	}
	if sequence.Len() == 0 {
		return nil
	}
	return tea.Raw(m.images.wrap(sequence.String()))
}

func (m *AppModel) prepareMarkdownImages() tea.Cmd {
	t := m.tab()
	if !t.markdownImagesPending {
		t.markdownImagesPending = true
		return m.findMarkdownImages()
	}
	var cmds []tea.Cmd
	loading := 0
	for i := range t.markdownImages {
		ref := &t.markdownImages[i]
		if ref.image == nil && ref.attempted && !ref.failed {
			loading++
		}
	}
	for i := range t.markdownImages {
		ref := &t.markdownImages[i]
		if !m.markdownImageVisible(*ref) {
			continue
		}
		if img := ref.image; img != nil {
			cols, rows := m.markdownImageSize(img)
			if cols == img.columns && rows == img.rows || cols == 0 || rows == 0 {
				continue
			}
			img.columns, img.rows, img.ready = cols, rows, false
			ref.columns, ref.rows = cols, rows
			msg := imageReadyMsg{path: t.path, before: ref.before, columns: cols, rows: rows, markdown: i + 1}
			cmds = append(cmds, tea.Sequence(tea.Raw(m.images.wrap(img.placement())), func() tea.Msg { return msg }))
		} else if ref.url == "" && !ref.attempted && loading < 2 {
			ref.attempted = true
			loading++
			cmds = append(cmds, m.loadMarkdownImage(i))
		}
	}
	if len(cmds) > 0 {
		m.rebuildContent()
	}
	return tea.Batch(cmds...)
}

func (m AppModel) renderMarkdownImages(line int, before bool) string {
	if !m.images.enabled || m.images.hidden {
		return ""
	}
	width := max(1, m.contentViewport.Width()-gutterWidth)
	var blocks []string
	for _, ref := range m.tab().markdownImages {
		if ref.line != line || ref.before != before {
			continue
		}
		if ref.failed {
			blocks = append(blocks, ansi.Truncate("  Image preview unavailable", width, ""))
		} else if ref.image != nil && ref.image.ready {
			blocks = append(blocks, ref.image.linkedPlaceholder(ref.target().uri()))
		} else if ref.columns > 0 && ref.rows > 0 {
			row := strings.Repeat(" ", min(width, ref.columns+2))
			blocks = append(blocks, strings.TrimSuffix(strings.Repeat(row+"\n", ref.rows), "\n"))
		}
	}
	rows := packImageBlocks(blocks, width)
	if len(rows) == 0 {
		return ""
	}
	return lipgloss.NewStyle().PaddingLeft(gutterWidth).Render(strings.Join(rows, "\n"))
}

func (m *AppModel) toggleImages() tea.Cmd {
	if !m.images.enabled {
		m.images.hidden = false
		return m.images.restartProbe()
	}
	m.images.hidden = !m.images.hidden
	m.recalculateLayout()
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
	return nil
}

func (m AppModel) imagesButton() string {
	state := "on"
	if m.images.hidden {
		state = "off"
	} else if !m.images.enabled && m.images.checking {
		state = "checking"
	} else if !m.images.enabled {
		state = "N/A"
	}
	return renderAccessKeyBarButton(fmt.Sprintf("Images: %s", state))
}
