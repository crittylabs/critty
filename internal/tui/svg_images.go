package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/crittylabs/critty/v2/internal/imageinfo"
)

type svgRenderer interface {
	Render(context.Context, []byte) ([]byte, error)
}

func (m AppModel) encodeImage(data []byte, path string, msg imageLoadedMsg) imageLoadedMsg {
	if msg.err == nil && imageinfo.SVGPath(path) {
		ctx := m.reload.context
		if ctx == nil {
			ctx = context.Background()
		}
		data, msg.err = m.svg.Render(ctx, data)
		msg = m.images.encode(data, msg)
		if msg.err == nil {
			msg.info.Format = "SVG"
		}
		return msg
	}
	return m.images.encode(data, msg)
}

func (m AppModel) svgFilePreview() bool {
	t := m.tab()
	return imageinfo.SVGPath(t.path) && !t.isBinary && !t.outsideChanges && t.textconv.Command == ""
}

// Patch previews use only the saved source and diff, never worktree files.
func (m AppModel) svgPatchContent(path string, before bool) ([]byte, error) {
	file := m.patch.File(path)
	if before {
		for i := range m.patch.Files {
			if m.patch.Files[i].Change.OldPath == path {
				file = &m.patch.Files[i]
				break
			}
		}
	}
	if file == nil || file.Known != nil {
		return nil, fmt.Errorf("complete SVG source unavailable in patch")
	}
	content := file.Content
	if before {
		if file.Diff == nil {
			return nil, fmt.Errorf("SVG diff unavailable in patch")
		}
		var b strings.Builder
		appendDeleted := func(line int) {
			for _, deleted := range file.Diff.DeletedAfter[line] {
				b.WriteString(deleted.Content)
				b.WriteByte('\n')
			}
		}
		appendDeleted(0)
		if content != "" {
			for i, line := range strings.SplitAfter(content, "\n") {
				if !file.Diff.ChangedLines[i+1] {
					b.WriteString(line)
				}
				appendDeleted(i + 1)
			}
		}
		content = b.String()
	}
	if len(content) > maxPreviewBytes {
		return nil, fmt.Errorf("SVG exceeds 5 MiB")
	}
	return []byte(content), nil
}

// SVG remains a text document, so previews do not replace source or anchors.
func (m AppModel) appendSVGPreview(b *strings.Builder, layout *renderedContentLayout) {
	if !m.svgFilePreview() {
		return
	}
	if preview := m.renderImagePreview(); preview != "" {
		layout.appendBlock(b, preview, contentMouseTarget{})
		layout.appendBlock(b, "", contentMouseTarget{})
	}
}
