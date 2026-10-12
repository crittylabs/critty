package tui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	_ "golang.org/x/image/webp"
)

const maxPreviewBytes = 5 << 20
const maxPreviewPixels = 16 << 20

type imageDisplay struct {
	probeID int
	enabled bool
	hidden  bool
	tmux    bool
}

type inlineImage struct {
	id, width, height int
	columns, rows     int
	ready             bool
}

type previewSide struct {
	image     *inlineImage
	attempted bool
	failed    bool
}

type imagePreview struct {
	before, after previewSide
}

func (p *imagePreview) side(before bool) *previewSide {
	if before {
		return &p.before
	}
	return &p.after
}

func (t *FileTab) compareImages() bool {
	return t.status == "modified" || t.status == "renamed"
}

func (t *FileTab) previewSides() []bool {
	if t.compareImages() {
		return []bool{true, false}
	}
	return []bool{t.isDeleted}
}

type imageLoadedMsg struct {
	path          string
	comment       *commentImageKey
	markdown      int
	before        bool
	sequence      string
	id            int
	width, height int
	err           error
}

type imageTransmittedMsg struct{ imageLoadedMsg }

type imageReadyMsg struct {
	path          string
	comment       *commentImageKey
	before        bool
	columns, rows int
	markdown      int
}

func newImageDisplay() imageDisplay {
	return imageDisplay{probeID: newImageID(), tmux: os.Getenv("TMUX") != ""}
}

func newImageID() int {
	return rand.IntN(0xffffff) + 1
}

func (d imageDisplay) wrap(sequence string) string {
	if d.tmux {
		return ansi.TmuxPassthrough(sequence)
	}
	return sequence
}

func (d imageDisplay) probe() tea.Cmd {
	// An invisible placement checks the extension used by the renderer as
	// well as image transmission.  Unsupported terminals never enable previews.
	if d.probeID == 0 || strings.HasPrefix(os.Getenv("TERM"), "screen") && !d.tmux {
		return nil
	}
	return tea.Raw(d.wrap(ansi.KittyGraphics([]byte("AAAA/w=="),
		"a=T", "f=32", "s=1", "v=1", "U=1", "c=1", "r=1",
		fmt.Sprintf("i=%d", d.probeID))))
}

func imagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

func decodePreview(data []byte) (image.Image, error) {
	if len(data) > maxPreviewBytes {
		return nil, fmt.Errorf("image exceeds 5 MiB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxPreviewPixels/cfg.Height {
		return nil, fmt.Errorf("image exceeds 16 megapixels")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (m *AppModel) prepareImage() tea.Cmd {
	if !m.images.enabled || m.images.hidden || len(m.tabs) == 0 || m.tab().state == nil {
		return nil
	}
	t := m.tab()
	// Supplied diffs carry no files to resolve against, but attachments
	// keep their original location.
	if t.isMarkdown && !t.isBinary && !t.outsideChanges && (m.patch == nil || t.attachment != nil) {
		return m.prepareMarkdownImages()
	}
	if !t.isBinary || !imagePath(t.path) || t.outsideChanges || m.patch != nil {
		return nil
	}
	width, height, _ := m.imagePreviewLayout()
	var cmds []tea.Cmd
	for _, before := range t.previewSides() {
		side := t.preview.side(before)
		if img := side.image; img != nil {
			cols, rows := img.size(width, height)
			if cols == img.columns && rows == img.rows || cols == 0 || rows == 0 {
				continue
			}
			img.columns, img.rows, img.ready = cols, rows, false
			path := t.path
			cmds = append(cmds, tea.Sequence(tea.Raw(m.images.wrap(img.placement())),
				func() tea.Msg { return imageReadyMsg{path: path, before: before, columns: cols, rows: rows} }))
		} else if !side.attempted {
			side.attempted = true
			cmds = append(cmds, m.loadImage(before))
		}
	}
	if len(cmds) > 0 {
		m.rebuildContent()
	}
	return tea.Batch(cmds...)
}

func (m AppModel) loadImage(before bool) tea.Cmd {
	t := m.tab()
	path := t.path
	sourcePath := path
	if before && t.oldPath != "" {
		sourcePath = t.oldPath
	}
	return func() tea.Msg {
		source := m.imageSource()
		data, err := source.PreviewContent(sourcePath, before, maxPreviewBytes)
		return m.images.encode(data, imageLoadedMsg{path: path, before: before, err: err})
	}
}

func (d imageDisplay) encode(data []byte, msg imageLoadedMsg) imageLoadedMsg {
	if msg.err != nil {
		return msg
	}
	img, err := decodePreview(data)
	msg.err = err
	if err != nil {
		return msg
	}
	msg.id = newImageID()
	msg.width, msg.height = img.Bounds().Dx(), img.Bounds().Dy()
	var out bytes.Buffer
	msg.err = kitty.EncodeGraphics(&out, img, &kitty.Options{ID: msg.id, Action: kitty.Transmit, Format: kitty.PNG, Transmission: kitty.Direct, Quiet: 2, Chunk: true, ChunkFormatter: d.wrap})
	msg.sequence = out.String()
	return msg
}

func (m AppModel) finishImageLoad(msg imageLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if msg.comment != nil {
			if side := m.commentImages[*msg.comment]; side != nil {
				side.failed = true
			}
			m.rebuildContent()
			m.updateCommentSidebar()
			return m, nil
		}
		for i := range m.tabs {
			if m.tabs[i].path == msg.path {
				m.tabs[i].imageSide(msg.before, msg.markdown).failed = true
			}
		}
		if len(m.tabs) > 0 && m.tab().state != nil {
			m.rebuildContent()
		}
		return m, nil
	}
	sequence := msg.sequence
	msg.sequence = ""
	return m, tea.Sequence(tea.Raw(sequence), func() tea.Msg { return imageTransmittedMsg{msg} })
}

func (m AppModel) imagePreviewLayout() (width, height int, horizontal bool) {
	width, height = m.contentViewport.Width(), m.contentViewport.Height()
	if !m.tab().compareImages() {
		return width, height, false
	}
	if width >= 64 {
		return (width - 2) / 2, height - 1, true
	}
	return width, max(5, (height-2)/2), false
}

func (m AppModel) renderImagePreview() string {
	if m.images.hidden {
		return ""
	}
	t := m.tab()
	ready := func(side previewSide) bool { return side.image != nil && side.image.ready }
	if !ready(t.preview.before) && !ready(t.preview.after) {
		return ""
	}
	if !t.compareImages() {
		side := t.preview.side(t.isDeleted)
		if ready(*side) {
			return side.image.placeholder()
		}
		return ""
	}
	width, _, horizontal := m.imagePreviewLayout()
	column := func(label string, side previewSide) string {
		content := "  Loading image…"
		if side.failed {
			content = "  Preview unavailable"
		} else if ready(side) {
			label += fmt.Sprintf(" (%d×%d)", side.image.width, side.image.height)
			content = side.image.placeholder()
		}
		return lipgloss.NewStyle().Width(width).Render("  " + label + "\n" + content)
	}
	before, after := column("Before", t.preview.before), column("After", t.preview.after)
	if horizontal {
		return lipgloss.JoinHorizontal(lipgloss.Top, before, "  ", after)
	}
	return before + "\n\n" + after
}

func (img inlineImage) size(width, height int) (int, int) {
	if width < 6 || height < 5 {
		return 0, 0
	}
	cols := min(width-4, 128)
	rows := max(1, min(height-3, 64, (cols*img.height+2*img.width-1)/(2*img.width)))
	cols = max(1, min(cols, rows*2*img.width/img.height))
	return cols, rows
}

func (img inlineImage) placement() string {
	return ansi.KittyGraphics(nil, "a=p", "U=1", "q=2", fmt.Sprintf("p=%d", img.id),
		fmt.Sprintf("i=%d", img.id), fmt.Sprintf("c=%d", img.columns), fmt.Sprintf("r=%d", img.rows))
}

func (img inlineImage) placeholder() string {
	var b strings.Builder
	for row := range img.rows {
		fmt.Fprintf(&b, "  \x1b[38;2;%d;%d;%dm\x1b[58;2;%d;%d;%dm",
			img.id>>16&255, img.id>>8&255, img.id&255,
			img.id>>16&255, img.id>>8&255, img.id&255)
		for col := range img.columns {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(row))
			b.WriteRune(kitty.Diacritic(col))
			b.WriteRune(kitty.Diacritic(img.id >> 24))
		}
		b.WriteString("\x1b[39;59m\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func hideImagePlaceholders(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.ContainsRune(line, kitty.Placeholder) {
			lines[i] = strings.Repeat(" ", lipgloss.Width(line))
		}
	}
	return strings.Join(lines, "\n")
}
