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
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	_ "golang.org/x/image/webp"
)

const maxPreviewBytes = 5 << 20
const maxPreviewPixels = 16 << 20

const imageProbeDelay = 150 * time.Millisecond
const imageProbeTimeout = 500 * time.Millisecond
const maxImageProbeAttempts = 3
const imageProbeRow = 2

type imageProbeMsg struct{ id int }
type imageProbeTimeoutMsg struct{ id int }

// imageProbeSkippedMsg reports a query withheld because the tmux pane was
// not the selected one; the probe's timeout retries it.
type imageProbeSkippedMsg struct{ id int }

type imageDisplay struct {
	cell        cellSize
	probeID     int
	probeReply  string
	probeCursor *tea.CursorPositionMsg
	checking    bool
	attempts    int
	// skipped records that the last check ended with queries withheld
	// because the tmux pane was not the client's selected pane.
	skipped bool
	input   imageResponseInput
	enabled bool
	hidden  bool
	tmux    bool
}

type inlineImage struct {
	id, width, height int
	columns, rows     int
	ready             bool
	// placementAlias names a second virtual placement for the lightbox; zero
	// selects the inline placement, which shares the image ID.
	placementAlias int
}

func (img inlineImage) placementID() int {
	if img.placementAlias != 0 {
		return img.placementAlias
	}
	return img.id
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
	return imageDisplay{probeID: newImageID(), tmux: os.Getenv("TMUX") != "", cell: cellPixelSize()}
}

// cellSize is the terminal's cell size in pixels; zero means unknown.
type cellSize struct{ width, height int }

func (c cellSize) known() bool {
	return c.width > 0 && c.height > 0
}

func (d imageDisplay) scheduleProbe() tea.Cmd {
	return tea.Tick(imageProbeDelay, func(time.Time) tea.Msg { return imageProbeMsg{d.probeID} })
}

// recheckImagesOnSelection restarts an unconfirmed check when a key or click
// selects a tab holding a local image, or arrives after a check whose queries
// were withheld: input proves the pane is now the one in view.
func (m *AppModel) recheckImagesOnSelection(previousTab int, msg tea.Msg) tea.Cmd {
	if m.images.enabled || m.images.checking || m.images.hidden || len(m.tabs) == 0 {
		return nil
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg:
	default:
		return nil
	}
	if m.images.skipped {
		return m.images.restartProbe()
	}
	if m.activeTab == previousTab {
		return nil
	}
	t := m.tab()
	if t.outsideChanges {
		return nil
	}
	if t.isBinary && imagePath(t.path) && m.patch == nil {
		return m.images.restartProbe()
	}
	if t.isMarkdown && t.doc != nil && (m.patch == nil || t.attachment != nil) {
		for _, ref := range markdownImageRefs(t.doc.Content) {
			if _, ok := localImagePath(t.path, ref.destination); ok {
				return m.images.restartProbe()
			}
		}
	}
	return nil
}

func (d *imageDisplay) restartProbe() tea.Cmd {
	d.attempts = 0
	d.skipped = false
	return d.nextProbe()
}

func (d *imageDisplay) nextProbe() tea.Cmd {
	previous := d.probeID
	for d.probeID == previous {
		d.probeID = newImageID()
	}
	d.probeReply = ""
	d.probeCursor = nil
	d.checking = true
	return d.scheduleProbe()
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
	sequence := d.probeSequence()
	if sequence == "" {
		return nil
	}
	return func() tea.Msg {
		if !imageProbeAllowed() {
			return imageProbeSkippedMsg{d.probeID}
		}
		return tea.RawMsg{Msg: sequence}
	}
}

func (d imageDisplay) probeSequence() string {
	// A virtual placement leaves the cursor unchanged.  Terminals that ignore
	// U=1 instead display the image and advance the cursor, even when replying OK.
	// Row two avoids the ambiguous F3 input event produced by row-one reports.
	if d.probeID == 0 || strings.HasPrefix(os.Getenv("TERM"), "screen") && !d.tmux {
		return ""
	}
	sequence := ansi.SaveCursor + ansi.CursorPosition(1, imageProbeRow) + ansi.KittyGraphics([]byte("AAAAAA=="),
		"a=T", "f=32", "s=1", "v=1", "U=1", "c=1", "r=1",
		fmt.Sprintf("i=%d", d.probeID)) + ansi.RequestCursorPositionReport +
		ansi.KittyGraphics(nil, "a=d", "d=I", "q=2", fmt.Sprintf("i=%d", d.probeID)) + ansi.RestoreCursor
	return d.wrap(sequence)
}

func (d *imageDisplay) acceptProbeResponse(msg tea.Msg) {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if d.attempts == 0 || msg.Options.ID != d.probeID || d.probeReply != "" {
			return
		}
		d.probeReply = string(msg.Payload)
		if d.probeReply != "OK" {
			d.checking = false
		}
	case tea.CursorPositionMsg:
		// Only a cursor report after this placement's reply belongs to the probe.
		if d.probeReply != "OK" || d.probeCursor != nil {
			return
		}
		d.probeCursor = &msg
		d.checking = false
	}
	d.enabled = d.probeReply == "OK" && d.probeCursor != nil &&
		d.probeCursor.X == 0 && d.probeCursor.Y == imageProbeRow-1
}

func (m AppModel) updateImageCapability(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.images.probeID == 0 || m.images.enabled || !m.images.checking {
		return m, nil
	}
	switch msg := msg.(type) {
	case imageProbeMsg:
		if msg.id != m.images.probeID {
			return m, nil
		}
		m.images.attempts++
		id := m.images.probeID
		return m, tea.Batch(m.images.probe(), tea.Tick(imageProbeTimeout,
			func(time.Time) tea.Msg { return imageProbeTimeoutMsg{id} }))
	case imageProbeTimeoutMsg:
		if msg.id != m.images.probeID {
			return m, nil
		}
		m.images.checking = false
		if m.images.attempts < maxImageProbeAttempts {
			cmd := m.images.nextProbe()
			return m, cmd
		}
		return m, nil
	case uv.KittyGraphicsEvent, tea.CursorPositionMsg:
		m.images.acceptProbeResponse(msg)
	}
	if !m.images.enabled {
		return m, nil
	}
	if len(m.tabs) > 0 && m.tab().state != nil {
		m.rebuildContent()
		m.updateCommentSidebar()
	}
	// Placeholder colors encode image IDs and must not be downsampled.
	return m, func() tea.Msg { return tea.ColorProfileMsg{Profile: colorprofile.TrueColor} }
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
	if m.session != nil && m.session.CJ.RoundState.Finished {
		return nil
	}
	if m.images.hidden || len(m.tabs) == 0 || m.tab().state == nil {
		return nil
	}
	t := m.tab()
	// Supplied diffs carry no files to resolve against, but attachments
	// keep their original location.
	if t.isMarkdown && !t.isBinary && !t.outsideChanges && (m.patch == nil || t.attachment != nil) {
		if !t.markdownImagesPending {
			t.markdownImagesPending = true
			return m.findMarkdownImages()
		}
		if !m.images.enabled {
			if t.hasLocalMarkdownImages() {
				return m.requestImageProbe()
			}
			return nil
		}
		return m.prepareMarkdownImages()
	}
	if !t.isBinary || !imagePath(t.path) || t.outsideChanges || m.patch != nil {
		return nil
	}
	if !m.images.enabled {
		return m.requestImageProbe()
	}
	width, height, _ := m.imagePreviewLayout()
	var cmds []tea.Cmd
	for _, before := range t.previewSides() {
		side := t.preview.side(before)
		if img := side.image; img != nil {
			cols, rows := img.size(width, height, m.images.cell)
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

func (m *AppModel) requestImageProbe() tea.Cmd {
	if m.images.checking || m.images.attempts != 0 {
		return nil
	}
	m.images.checking = true
	return m.images.scheduleProbe()
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
	link := func(before bool) string { return imageTarget{path: t.path, before: before}.uri() }
	if !t.compareImages() {
		side := t.preview.side(t.isDeleted)
		if ready(*side) {
			return side.image.linkedPlaceholder(link(t.isDeleted))
		}
		return ""
	}
	width, _, horizontal := m.imagePreviewLayout()
	column := func(label string, before bool) string {
		side := *t.preview.side(before)
		content := "  Loading image…"
		if side.failed {
			content = "  Preview unavailable"
		} else if ready(side) {
			label += fmt.Sprintf(" (%d×%d)", side.image.width, side.image.height)
			content = side.image.linkedPlaceholder(link(before))
		}
		return lipgloss.NewStyle().Width(width).Render("  " + label + "\n" + content)
	}
	before, after := column("Before", true), column("After", false)
	if horizontal {
		return lipgloss.JoinHorizontal(lipgloss.Top, before, "  ", after)
	}
	return before + "\n\n" + after
}

func (img inlineImage) size(width, height int, cell cellSize) (int, int) {
	if width < 6 || height < 5 {
		return 0, 0
	}
	return img.fit(min(width-4, 128), min(height-3, 64), cell)
}

// nativeCells returns the cells the image covers at its pixel size, or no
// limit when the cell size is unknown.
func (img inlineImage) nativeCells(cell cellSize) (int, int) {
	if !cell.known() {
		return maxPlaceholderCells, maxPlaceholderCells
	}
	return max(1, (img.width+cell.width-1)/cell.width), max(1, (img.height+cell.height-1)/cell.height)
}

// maxPlaceholderCells is the number of rows or columns the Unicode
// placeholder diacritics can address.
const maxPlaceholderCells = 297

// fit returns the largest cell size within width×height that keeps the image
// proportions without enlarging it past its pixel size.  Cells are assumed
// twice as tall as wide when their pixel size is unknown.
func (img inlineImage) fit(width, height int, cell cellSize) (int, int) {
	nativeCols, nativeRows := img.nativeCells(cell)
	return img.fitAspect(min(width, nativeCols), min(height, nativeRows), cell)
}

// fitAspect returns the largest cell size within width×height that keeps the
// image proportions for the given cell proportions.
func (img inlineImage) fitAspect(width, height int, cell cellSize) (int, int) {
	if width < 1 || height < 1 || img.width <= 0 || img.height <= 0 {
		return 0, 0
	}
	cellWidth, cellHeight := 1, 2
	if cell.known() {
		cellWidth, cellHeight = cell.width, cell.height
	}
	cols := min(width, maxPlaceholderCells)
	rows := max(1, min(height, maxPlaceholderCells, (cols*cellWidth*img.height+cellHeight*img.width-1)/(cellHeight*img.width)))
	cols = max(1, min(cols, rows*cellHeight*img.width/(cellWidth*img.height)))
	return cols, rows
}

func (img inlineImage) placement() string {
	return ansi.KittyGraphics(nil, "a=p", "U=1", "q=2", fmt.Sprintf("p=%d", img.placementID()),
		fmt.Sprintf("i=%d", img.id), fmt.Sprintf("c=%d", img.columns), fmt.Sprintf("r=%d", img.rows))
}

func (img inlineImage) placeholder() string {
	return img.linkedPlaceholder("")
}

// linkedPlaceholder renders the placeholder cells, each row wrapped in a
// hyperlink to uri when given so clicks on the image can be located.
func (img inlineImage) linkedPlaceholder(uri string) string {
	placement := img.placementID()
	var b strings.Builder
	for row := range img.rows {
		b.WriteString("  ")
		if uri != "" {
			b.WriteString(ansi.SetHyperlink(uri))
		}
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[58;2;%d;%d;%dm",
			img.id>>16&255, img.id>>8&255, img.id&255,
			placement>>16&255, placement>>8&255, placement&255)
		for col := range img.columns {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(row))
			b.WriteRune(kitty.Diacritic(col))
			b.WriteRune(kitty.Diacritic(img.id >> 24))
		}
		b.WriteString("\x1b[39;59m")
		if uri != "" {
			b.WriteString(ansi.ResetHyperlink())
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// packImageBlocks lays blocks out left to right, starting a new row when the
// next block would not fit in width.
func packImageBlocks(blocks []string, width int) []string {
	var rows, row []string
	used := 0
	flush := func() {
		if len(row) > 0 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
			used = 0
		}
	}
	for _, block := range blocks {
		w := lipgloss.Width(block)
		if used > 0 && used+w > width {
			flush()
		}
		row = append(row, block)
		used += w
	}
	flush()
	return rows
}
