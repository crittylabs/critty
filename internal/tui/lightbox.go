package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

// lightboxPlacementID names the enlarged placement, which coexists with an
// image's inline placement until the lightbox closes.
const lightboxPlacementID = 0xfffffe

// lightboxZoomLevels are the zoom steps in percent of the image's pixel
// size; the screen caps the largest and the first is the floor.
var lightboxZoomLevels = []int{25, 50, 75, 100, 150, 200, 300, 400, 600, 800, 1200, 1600}

// nominalCell stands in for the cell size when the terminal does not report
// it, so zoom percentages remain meaningful.
var nominalCell = cellSize{width: 8, height: 16}

type lightboxState struct {
	target      imageTarget
	image       inlineImage
	zoom        int           // requested percent of the pixel size
	siblings    []imageTarget // file revisions or images referenced on one line
	index       int           // position of target in siblings
	returnTo    modalType
	returnFocus int
}

type lightboxReadyMsg struct{ id, columns, rows int }

type lightboxLoadedMsg struct {
	target imageTarget
	imageLoadedMsg
}

func (m AppModel) loadLightboxImage(target imageTarget) tea.Cmd {
	source := m.imageSource()
	if filepath.IsAbs(target.path) {
		source = gitpkg.ReviewSource{}
	}
	return func() tea.Msg {
		data, err := source.PreviewContent(target.path, target.before, maxPreviewBytes)
		return lightboxLoadedMsg{target, m.images.encode(data, imageLoadedMsg{err: err})}
	}
}

func (m *AppModel) finishLightboxLoad(msg lightboxLoadedMsg) tea.Cmd {
	if m.modal != lightboxModal || m.lightbox.target != msg.target || m.lightbox.image.id != 0 {
		return nil
	}
	if msg.err != nil {
		m.locationError = "Cannot preview image: " + msg.err.Error()
		return nil
	}
	img := inlineImage{id: msg.id, width: msg.width, height: msg.height}
	return tea.Sequence(tea.Raw(msg.sequence), m.showLightboxImage(msg.target, img))
}

func lightboxPlacement(id int) int {
	if id == lightboxPlacementID {
		return lightboxPlacementID - 1
	}
	return lightboxPlacementID
}

// openLightbox shows a transmitted image in a dialog at its pixel size, or
// shrunk to fit the screen, returning to the current dialog when it closes.
// siblings lists file revisions or images referenced on the same line.
func (m *AppModel) openLightbox(target imageTarget, img inlineImage, siblings []imageTarget) tea.Cmd {
	m.lightbox = lightboxState{returnTo: m.modal, returnFocus: m.modalFocus, siblings: siblings}
	for i, sibling := range siblings {
		if sibling == target {
			m.lightbox.index = i
		}
	}
	m.modal = lightboxModal
	m.modalFocus = 0
	m.modalTextarea.Blur()
	return m.showLightboxImage(target, img)
}

// showLightboxImage replaces the displayed image.
func (m *AppModel) showLightboxImage(target imageTarget, img inlineImage) tea.Cmd {
	s := &m.lightbox
	img.placementAlias = lightboxPlacement(img.id)
	img.columns, img.rows, img.ready = 0, 0, false
	s.target, s.image, s.zoom = target, img, 100
	return m.prepareLightbox()
}

// cycleLightbox moves to the next or previous image in the group.
func (m *AppModel) cycleLightbox(step int) tea.Cmd {
	s := &m.lightbox
	for range max(0, len(s.siblings)-1) {
		s.index = (s.index + step + len(s.siblings)) % len(s.siblings)
		target := s.siblings[s.index]
		if img := m.loadedImage(target); img != nil {
			return m.showLightboxImage(target, *img)
		}
		if target.url == "" {
			m.showLightboxImage(target, inlineImage{})
			return m.loadLightboxImage(target)
		}
	}
	return nil
}

// closeLightbox returns to the view beneath.  The enlarged placement stays
// in the terminal: virtual placements show only where placeholder cells
// name them, and deleting one by image and placement ID has removed the
// inline placement, or the whole image, in some terminals.
func (m *AppModel) closeLightbox() {
	if m.modal != lightboxModal {
		return
	}
	m.modal = m.lightbox.returnTo
	m.modalFocus = m.lightbox.returnFocus
	m.lightbox = lightboxState{}
	if m.isTextModal() && m.modalFocus == 0 {
		m.modalTextarea.Focus()
	}
}

func (m AppModel) lightboxCell() cellSize {
	if m.images.cell.known() {
		return m.images.cell
	}
	return nominalCell
}

// lightboxCells returns the cell size showing the image at percent of its
// pixel size, shrunk to fit the dialog's room on screen when necessary.
func (m AppModel) lightboxCells(percent int) (int, int) {
	img := m.lightbox.image
	cell := m.lightboxCell()
	width := m.width - modalStyle.GetHorizontalFrameSize() - 2
	height := m.height - modalStyle.GetVerticalFrameSize() - 5
	width = min(width, max(1, (img.width*percent+100*cell.width-1)/(100*cell.width)))
	height = min(height, max(1, (img.height*percent+100*cell.height-1)/(100*cell.height)))
	return img.fitAspect(width, height, cell)
}

// lightboxPercent returns the displayed size as a percentage of the image's
// pixel size.
func (m AppModel) lightboxPercent() int {
	img := m.lightbox.image
	if img.columns == 0 || img.width == 0 {
		return 0
	}
	return max(1, img.columns*m.lightboxCell().width*100/img.width)
}

// zoomLightbox steps to the nearest zoom level that changes the displayed
// size, so the screen limit ends zooming in.
func (m *AppModel) zoomLightbox(in bool) {
	current := m.lightboxPercent()
	cols, rows := m.lightboxCells(m.lightbox.zoom)
	levels := lightboxZoomLevels
	for i := range levels {
		level := levels[i]
		if !in {
			level = levels[len(levels)-1-i]
		}
		if in && level <= current || !in && level >= current {
			continue
		}
		if c, r := m.lightboxCells(level); c != cols || r != rows {
			m.lightbox.zoom = level
		}
		return
	}
}

// prepareLightbox places the image at the current size, again after a
// resize or zoom.
func (m *AppModel) prepareLightbox() tea.Cmd {
	if m.modal != lightboxModal || m.width == 0 || m.height == 0 {
		return nil
	}
	img := &m.lightbox.image
	cols, rows := m.lightboxCells(m.lightbox.zoom)
	if cols == img.columns && rows == img.rows || cols == 0 || rows == 0 {
		return nil
	}
	img.columns, img.rows, img.ready = cols, rows, false
	msg := lightboxReadyMsg{id: img.id, columns: cols, rows: rows}
	return tea.Sequence(tea.Raw(m.images.wrap(img.placement())), func() tea.Msg { return msg })
}

func (m *AppModel) finishLightboxPlacement(msg lightboxReadyMsg) {
	img := &m.lightbox.image
	if m.modal == lightboxModal && img.id == msg.id && img.columns == msg.columns && img.rows == msg.rows {
		img.ready = true
	}
}

func (m *AppModel) handleLightboxKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeLightbox()
		return m, nil
	case "enter":
		return m, m.openImageExternally(m.lightbox.target)
	case "space", " ", "+", "=":
		m.zoomLightbox(true)
	case "backspace", "-":
		m.zoomLightbox(false)
	case "alt+o", "right", "tab":
		return m, m.cycleLightbox(1)
	case "left", "shift+tab":
		return m, m.cycleLightbox(-1)
	}
	return m, nil
}

func (m *AppModel) handleLightboxMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	for _, region := range m.modalMouseRegions() {
		if region.action.openImage && region.rect.contains(mouse) {
			return m, m.openImageExternally(m.lightbox.target)
		}
	}
	return m, nil
}

func (m *AppModel) handleLightboxWheel(mouse tea.Mouse) {
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.zoomLightbox(true)
	case tea.MouseWheelDown:
		m.zoomLightbox(false)
	}
}

// lightboxContent renders the dialog body and returns its width with the
// regions inside it.
func (m AppModel) lightboxContent() (string, int, []modalMouseRegion) {
	img := m.lightbox.image
	frame := modalStyle.GetHorizontalFrameSize()
	innerWidth := max(20, min(m.width-frame, max(40, img.columns+2)))
	size := fmt.Sprintf(" (%d×%d)", img.width, img.height)
	label := m.lightbox.target.label()
	if room := innerWidth - lipgloss.Width(size) - 3; lipgloss.Width(label) > room && room > 1 {
		label = ansi.TruncateLeft(label, lipgloss.Width(label)-room+1, "…")
	}
	title, closeRegion := modalTitle(label+size, innerWidth)
	body := "  Loading image…"
	var regions []modalMouseRegion
	if img.ready {
		body = img.placeholder()
		top := strings.Count(title, "\n")
		regions = append(regions, modalMouseRegion{
			rect:   mouseRect{left: 2, top: top, right: 2 + img.columns, bottom: top + img.rows},
			action: modalMouseAction{openImage: true},
		})
	}
	guide := fmt.Sprintf("%d%% · SPC/+: zoom in · BS/-: zoom out · RET/click: open with the system viewer · ESC: close", m.lightboxPercent())
	if n := len(m.lightbox.siblings); n > 1 {
		group := fmt.Sprintf("image %d of %d", m.lightbox.index+1, n)
		if !m.lightbox.target.compare {
			group += " on the line"
		}
		guide = fmt.Sprintf("%d%% · %s · M-o/←/→: other images · SPC/+: zoom in · BS/-: zoom out · RET/click: open with the system viewer · ESC: close",
			m.lightboxPercent(), group)
	}
	if m.locationError != "" {
		guide += "\n" + m.locationError
	}
	guide = lipgloss.Wrap(footerStyle.Render(guide), innerWidth, "")
	content := title + "\n" + body + "\n\n" + guide
	regions = append(regions, closeRegion)
	return lipgloss.NewStyle().Width(innerWidth).Render(content), innerWidth + frame, regions
}
