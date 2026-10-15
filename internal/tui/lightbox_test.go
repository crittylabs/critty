package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

// linkCells returns the screen positions of cells whose hyperlink contains
// needle, keyed by whether the cell is an image placeholder.
func linkCells(view, needle string) (text, image []tea.Mouse) {
	for y, line := range uv.NewStyledString(view).Lines(ansi.GraphemeWidth) {
		x := 0
		for _, cell := range line {
			if strings.Contains(cell.Link.URL, needle) {
				if strings.ContainsRune(cell.Content, kitty.Placeholder) {
					image = append(image, tea.Mouse{X: x, Y: y})
				} else {
					text = append(text, tea.Mouse{X: x, Y: y})
				}
			}
			x += max(1, cell.Width)
		}
	}
	return text, image
}

func placeholderCells(view string) (count int, placements map[int]bool) {
	placements = map[int]bool{}
	for _, line := range uv.NewStyledString(view).Lines(ansi.GraphemeWidth) {
		for _, cell := range line {
			if !strings.ContainsRune(cell.Content, kitty.Placeholder) {
				continue
			}
			count++
			if cell.Style.UnderlineColor != nil {
				r, g, b, _ := cell.Style.UnderlineColor.RGBA()
				placements[int(r>>8)<<16|int(g>>8)<<8|int(b>>8)] = true
			}
		}
	}
	return count, placements
}

// openStatus runs cmd and returns the status of the external open it
// produces, looking inside batches.
func openStatus(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	switch msg := cmd().(type) {
	case externalOpenMsg:
		return msg.status
	case tea.BatchMsg:
		for _, sub := range msg {
			if status := openStatus(sub); status != "" {
				return status
			}
		}
	}
	return ""
}

// rawMessages runs cmd and collects the terminal sequences it emits, in
// order, looking inside batches and sequences.
func rawMessages(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.RawMsg:
		if s, ok := msg.Msg.(string); ok {
			return []string{s}
		}
		return nil
	case tea.BatchMsg:
		var raws []string
		for _, sub := range msg {
			raws = append(raws, rawMessages(sub)...)
		}
		return raws
	}
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice {
		return nil
	}
	var raws []string
	for i := range v.Len() {
		if sub, ok := v.Index(i).Interface().(tea.Cmd); ok {
			raws = append(raws, rawMessages(sub)...)
		}
	}
	return raws
}

func stubOpener(t *testing.T, remote bool) *[]string {
	t.Helper()
	var opened []string
	previousRemote, previousStart := remoteSession, startOpener
	remoteSession = func() bool { return remote }
	startOpener = func(name, arg string) error {
		opened = append(opened, name+" "+arg)
		return nil
	}
	t.Cleanup(func() { remoteSession, startOpener = previousRemote, previousStart })
	return &opened
}

func TestImageLinksOpenLightboxAndExternally(t *testing.T) {
	opened := stubOpener(t, false)
	app := setupAppWithDoc(t, "![pic](a.png)\n<img src=\"https://example.org/b.png\">\n")
	imagePath := filepath.Join(filepath.Dir(app.tab().path), "a.png")
	if err := os.WriteFile(imagePath, previewPNG(t, 160, 80), 0o644); err != nil {
		t.Fatal(err)
	}
	app.images.enabled = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	refs := app.findMarkdownImages()().(markdownImagesMsg)
	if len(refs.images) != 2 || refs.images[1].url != "https://example.org/b.png" || refs.images[0].end != 13 {
		t.Fatalf("refs=%+v", refs.images)
	}
	app.tab().markdownImagesPending = true
	app.tab().markdownImages = refs.images
	img := &inlineImage{id: 0x123456, width: 160, height: 80}
	img.columns, img.rows = app.markdownImageSize(img)
	img.ready = true
	app.tab().markdownImages[0].image = img
	app.rebuildContent()

	view := app.View().Content
	text, image := linkCells(view, "critty://image?line=1&path=")
	if len(text) == 0 || len(image) != img.columns*img.rows {
		t.Fatalf("local image links: text=%d image=%d want image=%d", len(text), len(image), img.columns*img.rows)
	}
	urlText, urlImage := linkCells(view, "critty://image?line=2&url=https%3A%2F%2Fexample.org%2Fb.png")
	if len(urlText) == 0 || len(urlImage) != 0 {
		t.Fatalf("web image links: text=%d image=%d", len(urlText), len(urlImage))
	}

	app, cmd := clickMouseCmd(app, urlText[0].X, urlText[0].Y)
	if app.modal != noModal || cmd == nil {
		t.Fatal("web URL click should open externally")
	}
	if msg, ok := cmd().(externalOpenMsg); !ok || msg.status != "Opened https://example.org/b.png" {
		t.Fatalf("web open: %+v", cmd())
	}
	if len(*opened) != 1 || !strings.HasSuffix((*opened)[0], " https://example.org/b.png") {
		t.Fatalf("opened=%v", *opened)
	}

	app, cmd = clickMouseCmd(app, image[0].X, image[0].Y)
	if app.modal != lightboxModal || cmd == nil || app.lightbox.image.id != img.id {
		t.Fatalf("placeholder click: modal=%v lightbox=%+v", app.modal, app.lightbox)
	}
	cols, rows := app.lightbox.image.columns, app.lightbox.image.rows
	if cols != 20 || rows != 5 {
		t.Fatalf("lightbox opens at %dx%d, want the pixel size at nominal 8×16 cells (20x5)", cols, rows)
	}
	app, _ = updateApp(app, lightboxReadyMsg{id: img.id, columns: cols, rows: rows})
	view = app.View().Content
	count, placements := placeholderCells(view)
	if count != cols*rows || !placements[lightboxPlacementID] || placements[img.id] {
		t.Fatalf("lightbox placeholders=%d want=%d placements=%v", count, cols*rows, placements)
	}
	if plain := ansi.Strip(view); !strings.Contains(plain, "a.png (160×80)") || !strings.Contains(plain, "100% · SPC/+") {
		t.Fatalf("lightbox title or guide missing in %q", plain)
	}
	// Update places the zoomed image; the terminal's reply marks it ready.
	zoomed := func(app AppModel) (AppModel, int, int, int) {
		app, _ = updateApp(app, lightboxReadyMsg{id: img.id, columns: app.lightbox.image.columns, rows: app.lightbox.image.rows})
		return app, app.lightbox.image.columns, app.lightbox.image.rows, app.lightboxPercent()
	}
	for _, step := range []struct {
		key                 rune
		cols, rows, percent int
	}{
		{' ', 30, 8, 150}, {'+', 40, 10, 200}, {' ', 60, 15, 300}, {' ', 80, 20, 400},
		{' ', 92, 23, 460}, {' ', 92, 23, 460}, // the screen caps zooming in
		{tea.KeyBackspace, 80, 20, 400}, {'-', 60, 15, 300},
	} {
		app = pressKey(app, step.key)
		var c, r, percent int
		app, c, r, percent = zoomed(app)
		if c != step.cols || r != step.rows || percent != step.percent {
			t.Fatalf("after %q: %dx%d at %d%%, want %dx%d at %d%%", step.key, c, r, percent, step.cols, step.rows, step.percent)
		}
	}
	for range 10 {
		app = pressKey(app, tea.KeyBackspace)
	}
	var percent int
	app, _, _, percent = zoomed(app)
	if percent != 25 {
		t.Fatalf("zooming out stops at %d%%, want 25%%", percent)
	}
	if !strings.Contains(ansi.Strip(app.View().Content), "25% · ") {
		t.Fatal("guide does not show the zoom percentage")
	}
	app, _ = updateApp(app, tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelUp})
	app, _, _, percent = zoomed(app)
	if percent != 50 {
		t.Fatalf("wheel zoom: %d%%", percent)
	}
	regions := app.modalMouseRegions()
	var imageRegion *modalMouseRegion
	for i := range regions {
		if regions[i].action.openImage {
			imageRegion = &regions[i]
		}
	}
	if imageRegion == nil {
		t.Fatal("lightbox image has no click region")
	}
	app, cmd = clickMouseCmd(app, imageRegion.rect.left+1, imageRegion.rect.top+1)
	if app.modal != lightboxModal || cmd == nil {
		t.Fatal("clicking the enlarged image should keep the lightbox and open externally")
	}
	status := cmd().(externalOpenMsg).status
	if status != "Opened "+imagePath || len(*opened) != 2 || !strings.HasSuffix((*opened)[1], " "+imagePath) {
		t.Fatalf("status=%q opened=%v", status, *opened)
	}
	app, _ = updateApp(app, externalOpenMsg{status})
	if !strings.Contains(ansi.Strip(app.View().Content), "Opened ") {
		t.Fatal("lightbox does not show the open status")
	}

	app, cmd = pressKeyCmd(app, tea.KeyEscape)
	if app.modal != noModal || len(rawMessages(cmd)) != 0 {
		t.Fatalf("esc should close the lightbox without terminal commands: %q", rawMessages(cmd))
	}
	if _, placements := placeholderCells(app.View().Content); !placements[img.id] || placements[lightboxPlacementID] {
		t.Fatalf("inline image not restored: %v", placements)
	}

	app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.modal != noModal || cmd != nil {
		t.Fatal("M-o above the first line has no image to enlarge")
	}
	app = pressKey(app, 'j')
	app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.modal != lightboxModal || cmd == nil {
		t.Fatalf("M-o should enlarge the image on cursor line %d", app.tab().cursorLine)
	}
	app, _ = pressKeyCmd(app, tea.KeyEnter)
	if app.modal != lightboxModal {
		t.Fatal("enter keeps the lightbox open while opening externally")
	}
	app, _ = pressKeyCmd(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatal("esc should close the lightbox")
	}

	app.images.enabled = false
	app, cmd = clickMouseCmd(app, text[0].X, text[0].Y)
	if app.modal != noModal || openStatus(cmd) != "Opened "+imagePath {
		t.Fatal("without previews the reference should open externally")
	}
}

func TestExternalOpenRefusesRemoteAndUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("a.png", previewPNG(t, 2, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	opened := stubOpener(t, true)
	if status := openImageTarget(gitpkg.ReviewSource{}, imageTarget{path: "a.png"}); !strings.HasPrefix(status, "Not opened") || len(*opened) != 0 {
		t.Fatalf("remote: %q %v", status, *opened)
	}
	remoteSession = func() bool { return false }
	for _, tc := range []struct {
		target imageTarget
		want   string
	}{
		{imageTarget{path: "missing.png"}, "no such file"},
		{imageTarget{path: "a.txt"}, "not an image file"},
		{imageTarget{url: "ftp://example.org/a.png"}, "not a web URL"},
	} {
		if status := openImageTarget(gitpkg.ReviewSource{}, tc.target); !strings.Contains(status, tc.want) {
			t.Errorf("%+v: %q", tc.target, status)
		}
	}
	if len(*opened) != 0 {
		t.Fatalf("opened=%v", *opened)
	}

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "add", "a.png")
	runGitIn(t, dir, "commit", "-qm", "base")
	if err := os.WriteFile("a.png", previewPNG(t, 6, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	status := openImageTarget(gitpkg.ReviewSource{Scope: "staged"}, imageTarget{path: "a.png", before: true})
	if status != "Opened a.png (before)" || len(*opened) != 1 {
		t.Fatalf("before side: %q %v", status, *opened)
	}
	temp := strings.TrimPrefix((*opened)[0], externalOpener()+" ")
	t.Cleanup(func() { os.Remove(temp) })
	data, err := os.ReadFile(temp)
	if err != nil || filepath.Ext(temp) != ".png" || string(data) != string(previewPNG(t, 2, 2)) {
		t.Fatalf("temp copy %s: %v", temp, err)
	}
}

func TestLightboxCyclesImagesOnTheLine(t *testing.T) {
	app := setupAppWithDoc(t, "![a](a.png)\n![a](a.png) ![b](b.png) ![web](https://example.org/c.png)\nsecond ![c](c.png)\n")
	app.images.enabled = true
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	refs := app.findMarkdownImages()().(markdownImagesMsg)
	if len(refs.images) != 5 {
		t.Fatalf("refs=%+v", refs.images)
	}
	app.tab().markdownImagesPending = true
	app.tab().markdownImages = refs.images
	for i := range app.tab().markdownImages {
		ref := &app.tab().markdownImages[i]
		if ref.url == "" {
			ref.image = &inlineImage{id: 0x100 + i, width: 160, height: 80, columns: 20, rows: 5, ready: true}
		}
	}
	app = pressKey(app, 'j')
	app = pressKey(app, 'j')
	app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.modal != lightboxModal || cmd == nil || app.lightbox.image.id != 0x101 || len(app.lightbox.siblings) != 3 {
		t.Fatalf("first image: modal=%v lightbox=%+v", app.modal, app.lightbox)
	}
	app, _ = updateApp(app, lightboxReadyMsg{id: 0x101, columns: app.lightbox.image.columns, rows: app.lightbox.image.rows})
	if !strings.Contains(ansi.Strip(app.View().Content), "image 1 of 3 on the line") {
		t.Fatal("guide does not count the line's images")
	}
	app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if cmd == nil || app.lightbox.image.id != 0x102 || app.lightbox.index != 1 || app.lightbox.image.ready {
		t.Fatalf("second image: %+v", app.lightbox)
	}
	if raws := rawMessages(cmd); len(raws) != 1 || !strings.Contains(raws[0], "a=p") || strings.Contains(raws[0], "a=d") {
		t.Fatalf("switching should only place the next image: %q", raws)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.lightbox.image.id != 0x101 || app.lightbox.index != 0 {
		t.Fatalf("the web image has no preview and is skipped: %+v", app.lightbox)
	}
	app, _ = pressKeyCmd(app, tea.KeyLeft)
	if app.lightbox.image.id != 0x102 {
		t.Fatalf("left arrow should go back: %+v", app.lightbox)
	}
	app, _ = pressKeyCmd(app, tea.KeyEscape)
	app = pressKey(app, 'j')
	app, _ = updateApp(app, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	if app.modal != lightboxModal || app.lightbox.image.id != 0x104 || len(app.lightbox.siblings) != 1 {
		t.Fatalf("single image line: %+v", app.lightbox)
	}
	if strings.Contains(ansi.Strip(app.View().Content), "on the line") {
		t.Fatal("single image shows no cycling guide")
	}
}

func TestImageFitStopsAtPixelSize(t *testing.T) {
	img := inlineImage{width: 100, height: 50}
	if cols, rows := img.fit(80, 40, cellSize{}); cols != 80 || rows != 20 {
		t.Fatalf("unknown cell size: %dx%d", cols, rows)
	}
	cell := cellSize{width: 10, height: 20}
	if cols, rows := img.fit(80, 40, cell); cols != 10 || rows != 3 {
		t.Fatalf("native cap: %dx%d", cols, rows)
	}
	if cols, rows := img.fit(4, 40, cell); cols != 4 || rows != 1 {
		t.Fatalf("narrow box keeps the cell proportions: %dx%d", cols, rows)
	}
	icon := inlineImage{width: 8, height: 8}
	if cols, rows := icon.size(60, 20, cell); cols != 1 || rows != 1 {
		t.Fatalf("icon preview: %dx%d", cols, rows)
	}
}
