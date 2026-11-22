package tui

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	gitpkg "github.com/crittylabs/critty/v2/internal/git"
)

type commentImageKey struct {
	target               imageTarget
	width                int
	renderKind           renderedKind
	renderBody           string
	inline, dark, editor bool
}

func (m AppModel) commentImagePath(destination string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	if m.session != nil {
		if m.session.Meta.WorkingDirectory() != "" {
			cwd = m.session.Meta.WorkingDirectory()
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
func (m AppModel) commentImageTarget(destination string) (imageTarget, bool) {
	if u, ok := webImageURL(destination); ok {
		return imageTarget{url: u}, true
	}
	path, ok := m.commentImagePath(destination)
	return imageTarget{path: path}, ok
}

// commentImageSide returns the preview state for a comment image at
// a width, registering it for loading when first seen.
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

func (m *AppModel) renderCommentContent(author, body, prefix string, width int, suggestion ...chroma.Lexer) string {
	style := m.commentAuthorStyle(author)
	width = max(1, width)
	refs := m.commentImageRefs(body)
	showImages := !m.images.hidden && m.modal == noModal
	renderSource := m.commentRenderedDisplay(body, width, showImages, suggestion...)
	tables := commentTableLayout(renderSource, width)
	var lines []string
	prefixed := false
	for i := range renderSource.lines {
		parsed := &renderSource.lines[i]
		if parsed.previewHidden {
			continue
		}
		links := m.referenceSpans(parsed.source)
		for _, ref := range refs {
			if ref.line == i+1 {
				if target, ok := m.commentImageTarget(ref.destination); ok {
					links = append(links, linkSpan{start: ref.start, end: ref.end, uri: target.uri()})
				}
			}
		}
		var rows []string
		if table, ok := tables[i+1]; ok {
			if !prefixed && prefix != "" {
				header := style.Bold(true).Render(strings.TrimRight(prefix, " "))
				lines = append(lines, strings.Split(lipgloss.Wrap(header, width, ""), "\n")...)
			}
			rows = commentTableRows(parsed, table.colWidths, i+1 == table.startLine, style, links)
		} else {
			prefixWidth := 0
			if !prefixed && prefix != "" {
				prefixWidth = ansi.StringWidth(prefix)
				if parsed.thematicBreak {
					parsed.previewWidth = max(1, width-prefixWidth)
				}
			}
			content := style.Render(parsed.render(nil, style, style, links))
			if prefixWidth > 0 {
				content = style.Bold(true).Render(prefix) + content
			}
			rows = parsed.wrapRows(expandDisplayTabs(content), width, prefixWidth)
			if parsed.codeBlock && parsed.previewTokens == nil {
				rows[0] = shadeCodeRow(rows[0], prefixWidth, width)
				for row := 1; row < len(rows); row++ {
					rows[row] = shadeCodeRow(rows[row], 0, width)
				}
			}
		}
		prefixed = true
		lines = append(lines, rows...)
		if !showImages {
			continue
		}
		var blocks []string
		for _, ref := range refs {
			if ref.line != i+1 {
				continue
			}
			target, ok := m.commentImageTarget(ref.destination)
			if !ok || !m.canPreviewImage(target) {
				continue
			}
			side := m.commentImageSide(commentImageKey{target: target, width: width})
			if side == nil {
				continue
			}
			if side.failed {
				blocks = append(blocks, ansi.Truncate("  Image preview unavailable", width, ""))
			} else if side.image != nil && side.image.ready {
				blocks = append(blocks, side.image.linkedPlaceholder(target.uri()))
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
		if key.renderBody != "" {
			ctx := m.reload.context
			if ctx == nil {
				ctx = context.Background()
			}
			data, err := m.renderSource(ctx, key.renderKind, key.renderBody, key.inline, key.dark)
			return m.images.encode(data, imageLoadedMsg{comment: &key, rendered: true, renderDark: key.dark, renderKind: key.renderKind, renderInline: key.inline && !key.editor, err: err})
		}
		msg := imageLoadedMsg{comment: &key}
		if key.target.url != "" {
			return m.loadRemotePreview(key.target.url, msg)
		}
		data, err := (gitpkg.ReviewSource{}).PreviewContent(key.target.path, false, maxPreviewBytes)
		msg.err = err
		return m.encodeImage(data, key.target.path, msg)
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
	for key, side := range m.commentImages {
		if key.renderBody != "" && key.dark != terminalIsDark {
			continue
		}
		if side.image == nil && side.attempted && !side.failed {
			loading++
		}
	}
	var cmds []tea.Cmd
	for key, side := range m.commentImages {
		if key.renderBody != "" && key.dark != terminalIsDark {
			continue
		}
		if img := side.image; img != nil {
			var cols, rows int
			switch {
			case key.renderBody == "":
				cols, rows = img.size(min(44, key.width), 11, m.images.cell)
			case key.renderKind == renderedMermaid:
				maxRows := 19
				if key.editor {
					maxRows = min(11, max(1, m.height-20))
				}
				cols, rows = img.fit(key.width, maxRows, m.images.cell)
			default:
				maxRows := 8
				if key.inline && !key.editor {
					maxRows = 5
				}
				cols, rows = img.mathFit(min(44, key.width), maxRows, m.images.cell)
			}
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
