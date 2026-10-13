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
	local  bool
	previewSide
}

type markdownImagesMsg struct {
	path   string
	images []markdownImage
}

type markdownImageRef struct {
	line        int
	destination string
}

func markdownImageRefs(source string) []markdownImageRef {
	data := []byte(source)
	root := parser.New().Parse(data)
	var refs []markdownImageRef
	add := func(offset int, destination string) {
		if offset >= 0 && offset <= len(source) {
			refs = append(refs, markdownImageRef{strings.Count(source[:offset], "\n") + 1, destination})
		}
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
							add(offset, attr.Val)
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
			add(n.Pos(), n.Destination.Str(data))
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
				msg.images = append(msg.images, markdownImage{line: ref.line, before: before, path: imagePath, local: local})
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
	return img.size(min(44, m.contentViewport.Width()-gutterWidth), min(19, max(5, m.contentViewport.Height())))
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
		if img := ref.image; img != nil {
			cols, rows := m.markdownImageSize(img)
			if cols == img.columns && rows == img.rows || cols == 0 || rows == 0 {
				continue
			}
			img.columns, img.rows, img.ready = cols, rows, false
			msg := imageReadyMsg{path: t.path, before: ref.before, columns: cols, rows: rows, markdown: i + 1}
			cmds = append(cmds, tea.Sequence(tea.Raw(m.images.wrap(img.placement())), func() tea.Msg { return msg }))
		} else if !ref.attempted && loading < 2 {
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
	var rows, row []string
	used := 0
	flush := func() {
		if len(row) > 0 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
			used = 0
		}
	}
	for _, ref := range m.tab().markdownImages {
		if ref.line != line || ref.before != before {
			continue
		}
		block := ""
		if ref.failed {
			block = ansi.Truncate("  Image preview unavailable", width, "")
		} else if ref.image != nil && ref.image.ready {
			block = ref.image.placeholder()
		} else {
			continue
		}
		w := lipgloss.Width(block)
		if used > 0 && used+w > width {
			flush()
		}
		row = append(row, block)
		used += w
	}
	flush()
	if len(rows) == 0 {
		return ""
	}
	return lipgloss.NewStyle().PaddingLeft(gutterWidth).Render(strings.Join(rows, "\n"))
}

func (m *AppModel) toggleImages() {
	m.images.hidden = !m.images.hidden
	m.recalculateLayout()
	m.rebuildContent()
	m.updateCommentSidebar()
	m.scrollToCursor()
}

func (m AppModel) imagesButton() string {
	state := "on"
	if m.images.hidden {
		state = "off"
	} else if !m.images.enabled {
		state = "unavailable"
	}
	return renderAccessKeyBarButton(fmt.Sprintf("Images: %s", state))
}
