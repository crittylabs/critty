package tui

import (
	"context"
	"strings"

	"github.com/crittylabs/critty/internal/imageinfo"
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
	return imageinfo.SVGPath(t.path) && !t.isBinary && !t.outsideChanges && t.textconv.Command == "" && (m.patch == nil || t.attachment != nil)
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
