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

func (m *AppModel) renderCommentContent(author, body, prefix string, width int) string {
	style := m.commentAuthorStyle(author)
	width = max(1, width)
	var refs []markdownImageRef
	if !m.images.hidden && m.modal == noModal {
		if m.imageBodies == nil {
			m.imageBodies = make(map[string][]markdownImageRef)
		}
		var ok bool
		refs, ok = m.imageBodies[body]
		if !ok {
			refs = markdownImageRefs(body)
			if len(m.imageBodies) < 128 {
				m.imageBodies[body] = refs
			}
		}
	}
	var lines []string
	for i, line := range strings.Split(body, "\n") {
		content := style.Render(m.linkFileReferences(line))
		if i == 0 && prefix != "" {
			content = style.Bold(true).Render(prefix) + content
		}
		lines = append(lines, strings.Split(lipgloss.Wrap(expandDisplayTabs(content), width, ""), "\n")...)
		var row []string
		used := 0
		flush := func() {
			if len(row) > 0 {
				lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, row...), "\n")...)
				row = nil
				used = 0
			}
		}
		for _, ref := range refs {
			if ref.line != i+1 {
				continue
			}
			path, ok := m.commentImagePath(ref.destination)
			if !ok {
				continue
			}
			key := commentImageKey{path, width}
			if m.commentImages == nil {
				m.commentImages = make(map[commentImageKey]*previewSide)
			}
			side := m.commentImages[key]
			if side == nil && len(m.commentImages) < 128 {
				side = &previewSide{}
				m.commentImages[key] = side
			}
			if side == nil {
				continue
			}
			block := ""
			if side.failed {
				block = ansi.Truncate("  Image preview unavailable", width, "")
			} else if side.image != nil && side.image.ready {
				block = side.image.placeholder()
			}
			if block == "" {
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
	if m.images.hidden || m.modal != noModal {
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
