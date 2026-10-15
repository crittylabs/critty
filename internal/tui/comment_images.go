package tui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

type commentImageKey struct {
	path  string
	width int
}

func (m AppModel) commentImagePath(destination string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	if m.session != nil {
		if m.session.Meta.CWD != "" {
			cwd = m.session.Meta.CWD
		}
		u, err := url.Parse(destination)
		if err == nil && strings.HasPrefix(u.Path, "attachments/") {
			cwd = m.session.Dir
		}
	}
	return localImagePath(filepath.Join(cwd, "comment.md"), destination)
}

// commentImageTarget resolves an image destination in a comment body to a
// local image file or a web URL.

func (m *AppModel) commentImageSide(key commentImageKey) *previewSide {
	if m.commentImages == nil {
		m.commentImages = make(map[commentImageKey]*previewSide)
	}
	side := m.commentImages[key]
	if side == nil && len(m.commentImages) < 128 {
		side = &previewSide{}
		m.commentImages[key] = side
	}
	return side
}

func (m *AppModel) commentImageRefs(body string) []markdownImageRef {
	if m.imageBodies == nil {
		m.imageBodies = make(map[string][]markdownImageRef)
	}
	refs, ok := m.imageBodies[body]
	if !ok {
		refs = markdownImageRefs(body)
		if len(m.imageBodies) < 128 {
			m.imageBodies[body] = refs
		}
	}
	return refs
}

// linkCommentLine links file references and the image references that refs
// places on line number lineNumber.

func (m *AppModel) renderCommentContent(author, body, prefix string, width int) string {
	style := m.commentAuthorStyle(author)
	width = max(1, width)
	refs := m.commentImageRefs(body)
	showImages := !m.images.hidden && m.modal == noModal
	var lines []string
	for i, line := range strings.Split(body, "\n") {
		content := style.Render(m.linkFileReferences(line))
		if i == 0 && prefix != "" {
			content = style.Bold(true).Render(prefix) + content
		}
		lines = append(lines, strings.Split(lipgloss.Wrap(expandDisplayTabs(content), width, ""), "\n")...)
		if !showImages {
			continue
		}
		var blocks []string
		for _, ref := range refs {
			if ref.line != i+1 {
				continue
			}
			path, ok := m.commentImagePath(ref.destination)
			if !ok {
				continue
			}
			side := m.commentImageSide(commentImageKey{path, width})
			if side == nil {
				continue
			}
			if side.failed {
				blocks = append(blocks, ansi.Truncate("  Image preview unavailable", width, ""))
			} else if side.image != nil && side.image.ready {
				blocks = append(blocks, side.image.placeholder())
			}
		}
		for _, row := range packImageBlocks(blocks, width) {
			lines = append(lines, strings.Split(row, "\n")...)
		}
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) loadCommentImage(key commentImageKey) tea.Cmd {
	return func() tea.Msg {
		data, err := (gitpkg.ReviewSource{}).PreviewContent(key.path, false, maxPreviewBytes)
		return m.images.encode(data, imageLoadedMsg{comment: &key, err: err})
	}
}

func (m *AppModel) prepareCommentImages() tea.Cmd {
	if m.session != nil && m.session.CJ.RoundState.Finished {
		return nil
	}
	if m.images.hidden || m.modal != noModal && !m.editorImagesActive() {
		return nil
	}
	if !m.images.enabled {
		if len(m.commentImages) > 0 {
			return m.requestImageProbe()
		}
		return nil
	}
	loading := 0
	for _, side := range m.commentImages {
		if side.image == nil && side.attempted && !side.failed {
			loading++
		}
	}
	var cmds []tea.Cmd
	for key, side := range m.commentImages {
		if img := side.image; img != nil {
			cols, rows := img.size(min(44, key.width), 11)
			if cols == img.columns && rows == img.rows || cols == 0 || rows == 0 {
				continue
			}
			img.columns, img.rows, img.ready = cols, rows, false
			msg := imageReadyMsg{comment: &key, columns: cols, rows: rows}
			cmds = append(cmds, tea.Sequence(tea.Raw(m.images.wrap(img.placement())), func() tea.Msg { return msg }))
		} else if !side.attempted && loading < 2 {
			side.attempted = true
			loading++
			cmds = append(cmds, m.loadCommentImage(key))
		}
	}
	return tea.Batch(cmds...)
}
