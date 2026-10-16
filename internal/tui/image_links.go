package tui

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// imageTarget identifies an image shown in the review: a local file on one
// side of the diff, or a web URL that is linked but never fetched.  line is
// the document line of the reference, when it comes from one, so the other
// images on that line can be found.
type imageTarget struct {
	path    string
	before  bool
	compare bool // a file revision with a Before/After caption
	url     string
	line    int
}

func (t imageTarget) uri() string {
	q := url.Values{}
	if t.compare {
		q.Set("compare", "1")
	}
	if t.url != "" {
		q.Set("url", t.url)
	} else {
		q.Set("path", t.path)
		if t.before {
			q.Set("side", "old")
		}
	}
	if t.line > 0 {
		q.Set("line", strconv.Itoa(t.line))
	}
	u := url.URL{Scheme: "critty", Host: "image", RawQuery: q.Encode()}
	return u.String()
}

func (t imageTarget) label() string {
	if t.url != "" {
		return t.url
	}
	if t.before {
		if t.compare {
			return t.path + " (Before)"
		}
		return t.path + " (before)"
	}
	if t.compare {
		return t.path + " (After)"
	}
	return t.path
}

func imageLink(uri string) (imageTarget, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "critty" || u.Host != "image" {
		return imageTarget{}, false
	}
	q := u.Query()
	line, _ := strconv.Atoi(q.Get("line"))
	t := imageTarget{path: q.Get("path"), before: q.Get("side") == "old", compare: q.Get("compare") == "1", url: q.Get("url"), line: max(0, line)}
	return t, t.path != "" || t.url != ""
}

// webImageURL returns destination when it is an http or https URL.
func webImageURL(destination string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(destination))
	if err != nil || u.Host == "" {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		return u.String(), true
	}
	return "", false
}

func linkedText(uri, text string) string {
	if uri == "" {
		return text
	}
	return ansi.SetHyperlink(uri) + text + ansi.ResetHyperlink()
}

// linkedDisplayLine renders line for display with each span wrapped in a
// hyperlink.  Spans are byte ranges of line sorted by start; display converts
// text for the screen and must not be applied across the escape sequences.
func linkedDisplayLine(line string, spans []linkSpan, display func(string) string) string {
	var b strings.Builder
	end := 0
	for _, span := range spans {
		if span.start < end || span.end > len(line) || span.start >= span.end || span.uri == "" {
			continue
		}
		b.WriteString(display(line[end:span.start]))
		b.WriteString(linkedText(span.uri, display(line[span.start:span.end])))
		end = span.end
	}
	b.WriteString(display(line[end:]))
	return b.String()
}

type linkSpan struct {
	start, end int
	uri        string
}

// imageSpans returns the hyperlink spans of the image references on a
// document line of the active tab.
func (m *AppModel) imageSpans(line int, before bool) []linkSpan {
	var spans []linkSpan
	for _, ref := range m.tab().markdownImages {
		if ref.line == line && ref.before == before && ref.end > ref.start {
			spans = append(spans, linkSpan{start: ref.start, end: ref.end, uri: ref.target().uri()})
		}
	}
	return spans
}

// loadedImage returns a transmitted image for target, if any view holds one.
func (m *AppModel) loadedImage(target imageTarget) *inlineImage {
	if target.url != "" {
		return nil
	}
	loaded := func(side *previewSide) *inlineImage {
		if side != nil && side.image != nil {
			return side.image
		}
		return nil
	}
	if len(m.tabs) > 0 {
		t := m.tab()
		if t.path == target.path || target.before && t.oldPath == target.path {
			if img := loaded(t.preview.side(target.before)); img != nil {
				return img
			}
		}
		// Prefer the reference on the target's own line; the same file on
		// another line shows the same image.
		for _, exact := range []bool{true, false} {
			for i := range t.markdownImages {
				ref := &t.markdownImages[i]
				if ref.path != target.path || ref.before != target.before || exact && ref.line != target.line {
					continue
				}
				if img := loaded(&ref.previewSide); img != nil {
					return img
				}
			}
		}
	}
	if target.before {
		return nil
	}
	for key, side := range m.commentImages {
		if key.path == target.path {
			if img := loaded(side); img != nil {
				return img
			}
		}
	}
	return nil
}

// openImage enlarges a previewed image in the lightbox, or hands a web URL
// or an image without a preview to the system opener.
func (m *AppModel) openImage(target imageTarget) tea.Cmd {
	if len(m.tabs) > 0 && m.tab().isBinary && target.line == 0 {
		for _, candidate := range m.tab().imageTargets() {
			if candidate == target {
				return m.openImageWithSiblings(target, m.tab().imageTargets())
			}
		}
	}
	return m.openImageWithSiblings(target, m.lineImages(target))
}

func (t *FileTab) imageTarget(before bool) imageTarget {
	path := t.path
	if before && t.oldPath != "" {
		path = t.oldPath
	}
	return imageTarget{path: path, before: before, compare: t.status != ""}
}

// imageTargets follows the same Before/After order as the inline preview.
// A standalone image and an added or deleted file have only one target.
func (t *FileTab) imageTargets() []imageTarget {
	if !t.isBinary || !imagePath(t.path) || t.outsideChanges {
		return nil
	}
	var targets []imageTarget
	for _, before := range t.previewSides() {
		targets = append(targets, t.imageTarget(before))
	}
	return targets
}

func (m *AppModel) openImageWithSiblings(target imageTarget, siblings []imageTarget) tea.Cmd {
	if target.url == "" && m.images.enabled && !m.images.hidden {
		if img := m.loadedImage(target); img != nil {
			return m.openLightbox(target, *img, siblings)
		}
		m.openLightbox(target, inlineImage{}, siblings)
		return m.loadLightboxImage(target)
	}
	return m.openImageExternally(target)
}

// lineImages returns the images referenced on the same document line as
// target, in order, or nil when target is not a document reference.
func (m *AppModel) lineImages(target imageTarget) []imageTarget {
	if len(m.tabs) == 0 || target.line == 0 {
		return nil
	}
	var targets []imageTarget
	for _, ref := range m.tab().markdownImages {
		if ref.line == target.line && ref.before == target.before {
			targets = append(targets, ref.target())
		}
	}
	return targets
}

func (m *AppModel) openImageLink(uri string) (tea.Cmd, bool) {
	target, ok := imageLink(uri)
	if !ok {
		return nil, false
	}
	return m.openImage(target), true
}

// openImageAtCursor opens the image file of the active tab or the first image
// referenced on the cursor line.
func (m *AppModel) openImageAtCursor() tea.Cmd {
	if m.isTextModal() && m.modalFocus == 0 {
		value := m.modalTextarea.Value()
		line, column := m.modalTextarea.Line()+1, m.modalTextarea.Column()
		refs := markdownImageRefs(value)
		best, distance := -1, int(^uint(0)>>1)
		for i, ref := range refs {
			if _, ok := m.commentImageTarget(ref.destination); !ok {
				continue
			}
			if d := referenceDistance(value, line, column, ref); d < distance {
				best, distance = i, d
			}
		}
		if best < 0 {
			return nil
		}
		target, _ := m.commentImageTarget(refs[best].destination)
		var siblings []imageTarget
		for _, ref := range refs {
			if ref.line == refs[best].line {
				if sibling, ok := m.commentImageTarget(ref.destination); ok {
					siblings = append(siblings, sibling)
				}
			}
		}
		return m.openImageWithSiblings(target, siblings)
	}
	t := m.tab()
	if t.isBinary && imagePath(t.path) {
		targets := t.imageTargets()
		if len(targets) > 0 {
			return m.openImageWithSiblings(targets[0], targets)
		}
		return nil
	}
	if t.cursorOnAnnotation || t.cursorLine == 0 {
		return nil
	}
	before := t.cursorSide == "old"
	for _, ref := range t.markdownImages {
		if ref.line == t.cursorLine && ref.before == before {
			return m.openImage(ref.target())
		}
	}
	return nil
}

// referenceDistance measures rune distance to a reference's span, rather
// than screen columns, so wide and combining text keep the same anchors.
func referenceDistance(value string, line, column int, ref markdownImageRef) int {
	lines := strings.Split(value, "\n")
	position := func(row, col int) int {
		n := col
		for _, text := range lines[:min(max(0, row-1), len(lines))] {
			n += utf8.RuneCountInString(text) + 1
		}
		return n
	}
	if ref.line < 1 || ref.line > len(lines) {
		return int(^uint(0) >> 1)
	}
	text := lines[ref.line-1]
	start := position(ref.line, utf8.RuneCountInString(text[:min(ref.start, len(text))]))
	end := position(ref.line, utf8.RuneCountInString(text[:min(ref.end, len(text))]))
	cursor := position(line, column)
	return max(start-cursor, cursor-end, 0)
}

var kittyDiacritics = func() map[rune]bool {
	set := map[rune]bool{}
	for i := 0; ; i++ {
		r := kitty.Diacritic(i)
		if i > 0 && r == kitty.Diacritic(0) {
			return set
		}
		set[r] = true
	}
}()

// hideImagePlaceholders blanks placeholder cells so dimming behind a dialog
// cannot alter the colors that carry image IDs.  Other cells keep their
// content and width.
func hideImagePlaceholders(content string) string {
	if !strings.ContainsRune(content, kitty.Placeholder) {
		return content
	}
	var b strings.Builder
	b.Grow(len(content))
	skipping := false
	for _, r := range content {
		switch {
		case r == kitty.Placeholder:
			b.WriteByte(' ')
			skipping = true
		case skipping && kittyDiacritics[r]:
		default:
			skipping = false
			b.WriteRune(r)
		}
	}
	return b.String()
}
