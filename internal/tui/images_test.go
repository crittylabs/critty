package tui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/crittylabs/critty/internal/document"
	gitpkg "github.com/crittylabs/critty/internal/git"
)

func previewPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestTextReviewDoesNotProbeImages(t *testing.T) {
	t.Setenv("TMUX", "")
	app := setupAppWithDoc(t, "# Text only\n\nNo image previews.\n")
	out := &previewOutput{changed: make(chan struct{}, 1)}
	p := tea.NewProgram(app, tea.WithInput(strings.NewReader("")), tea.WithOutput(out), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
	timer := time.AfterFunc(350*time.Millisecond, p.Quit)
	defer timer.Stop()
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.output(), "a=T,f=32") {
		t.Fatal("text review sent an image capability query")
	}
}

type previewOutput struct {
	mu sync.Mutex
	bytes.Buffer
	changed chan struct{}
}

func (w *previewOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.Buffer.Write(p)
	w.mu.Unlock()
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (w *previewOutput) output() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.String()
}

func TestPreviewTerminalRoundTrip(t *testing.T) {
	for _, encoded := range []bool{false, true} {
		for _, lost := range []int{0, 2} {
			t.Run(fmt.Sprintf("csi-u=%t/lostProbes=%d", encoded, lost), func(t *testing.T) {
				testPreviewTerminalRoundTrip(t, lost, encoded)
			})
		}
	}
}

func testPreviewTerminalRoundTrip(t *testing.T, lost int, encoded bool) {
	t.Setenv("TMUX", "")
	dir := t.TempDir()
	path := dir + "/image.png"
	if err := os.WriteFile(path, previewPNG(t, 3, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	app := NewApp(path, AppConfig{})
	app.images.tmux = encoded
	out := &previewOutput{changed: make(chan struct{}, 1)}
	input, replies, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := input.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
		if err := replies.Close(); err != nil {
			t.Error(err)
		}
	})
	p := tea.NewProgram(app, tea.WithInput(input), tea.WithOutput(out),
		tea.WithWindowSize(80, 24), tea.WithColorProfile(colorprofile.ANSI256), tea.WithoutSignalHandler())
	type result struct {
		model tea.Model
		err   error
	}
	done := make(chan result, 1)
	go func() { model, err := p.Run(); done <- result{model, err} }()
	t.Cleanup(func() { p.Kill() })
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	seen := make(map[int]bool)
	for {
		select {
		case <-deadline.C:
			t.Fatal("terminal did not display image")
		case <-out.changed:
			output := out.output()
			for _, sequence := range strings.Split(output, "\x1b_G")[1:] {
				header, _, ok := strings.Cut(sequence, ";")
				if !ok {
					continue
				}
				var options kitty.Options
				if err := options.UnmarshalText([]byte(header)); err != nil {
					t.Fatal(err)
				}
				if options.Action != kitty.TransmitAndPut || seen[options.ID] {
					continue
				}
				seen[options.ID] = true
				if len(seen) > lost {
					response := fmt.Sprintf("\x1b[4;8R\x1b_Gi=%d;OK\x1b\\\x1b[2;1R", options.ID)
					if encoded {
						response = fmt.Sprintf("\x1b[95;3uGi=%d;OK\x1b[92;3u\x1b[91;3u2;1R", options.ID)
					}
					if _, err := replies.WriteString(response); err != nil {
						t.Fatal(err)
					}
				}
			}
			if strings.ContainsRune(output, kitty.Placeholder) {
				p.Quit()
				finished := <-done
				if finished.err != nil {
					t.Fatal(finished.err)
				}
				img := finished.model.(AppModel).tabs[0].preview.after.image
				if img == nil || !img.ready {
					t.Fatal("image displayed before transmission completed")
				}
				color := fmt.Sprintf("38;2;%d;%d;%d", img.id>>16&255, img.id>>8&255, img.id&255)
				if !strings.Contains(output, color) {
					t.Fatal("renderer downsampled image ID")
				}
				if !strings.Contains(output, "a=p,U=1") || !strings.Contains(output, ";iVBOR") {
					t.Fatal("terminal did not receive PNG and virtual placement")
				}
				return
			}
		case finished := <-done:
			t.Fatalf("terminal exited early: %v", finished.err)
		}
	}
}

func TestPreviewDecodeLimits(t *testing.T) {
	data := previewPNG(t, 3, 2)
	img, err := decodePreview(data)
	if err != nil || img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("decode: %v, %v", img, err)
	}
	for _, data := range [][]byte{nil, []byte("not an image"), make([]byte, maxPreviewBytes+1), previewPNG(t, 4097, 4096)} {
		if _, err := decodePreview(data); err == nil {
			t.Fatal("accepted invalid or oversized image")
		}
	}
}

func TestImageCapabilityAndPassthrough(t *testing.T) {
	t.Setenv("TMUX", "test")
	d := newImageDisplay()
	sequence := d.probeSequence()
	if !strings.HasPrefix(sequence, "\x1bPtmux;") || !strings.Contains(sequence, "U=1") {
		t.Fatalf("probe not passed through tmux: %q", sequence)
	}
	query := strings.Index(sequence, ansi.RequestCursorPositionReport)
	cleanup := strings.Index(sequence, "a=d,d=I")
	if query < 0 || cleanup < query || !strings.Contains(sequence, ansi.SaveCursor) ||
		!strings.Contains(sequence, ansi.RestoreCursor) || !strings.Contains(sequence, ";AAAAAA==") {
		t.Fatalf("probe does not measure and clean up a transparent placement: %q", sequence)
	}
	for _, tc := range []struct {
		id      int
		payload string
		enabled bool
	}{
		{d.probeID + 1, "OK", false}, {d.probeID, "EINVAL", false}, {d.probeID, "OK", false},
	} {
		app := AppModel{images: d}
		app.images.checking = true
		app.images.attempts = 1
		model, _ := app.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: tc.id}, Payload: []byte(tc.payload)})
		if model.(AppModel).images.enabled != tc.enabled {
			t.Fatalf("capability %+v", tc)
		}
	}
}

func TestImageCapabilityRequiresVirtualPlacement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cursor  tea.CursorPositionMsg
		payload string
		enabled bool
	}{
		{"virtual placement", tea.CursorPositionMsg{X: 0, Y: 1}, "OK", true},
		{"ignored extension", tea.CursorPositionMsg{X: 1, Y: 1}, "OK", false},
		{"unsupported graphics", tea.CursorPositionMsg{X: 0, Y: 1}, "EINVAL", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := AppModel{images: newImageDisplay()}
			app.images.checking = true
			app.images.attempts = 1
			messages := []tea.Msg{
				uv.KittyGraphicsEvent{Options: kitty.Options{ID: app.images.probeID}, Payload: []byte(tc.payload)},
				tc.cursor,
			}
			for i, msg := range messages {
				model, _ := app.Update(msg)
				app = model.(AppModel)
				if app.images.enabled != (i == 1 && tc.enabled) {
					t.Fatalf("message %d: enabled=%t", i, app.images.enabled)
				}
			}
			model, _ := app.Update(tea.CursorPositionMsg{X: 0, Y: 1})
			if model.(AppModel).images.enabled != tc.enabled {
				t.Fatal("later cursor reply changed capability")
			}
		})
	}
}

func TestImageProbeRetriesAndIgnoresStaleResponses(t *testing.T) {
	app := AppModel{images: newImageDisplay()}
	app.images.checking = true
	var previous int
	var cmd tea.Cmd
	for attempt := 1; attempt <= maxImageProbeAttempts; attempt++ {
		id := app.images.probeID
		if id == previous {
			t.Fatal("retry reused its image ID")
		}
		app, cmd = updateApp(app, imageProbeMsg{id})
		if cmd == nil || app.images.attempts != attempt {
			t.Fatalf("attempt %d: no probe or timeout", attempt)
		}
		if previous != 0 {
			app, _ = updateApp(app, uv.KittyGraphicsEvent{Options: kitty.Options{ID: previous}, Payload: []byte("OK")})
			app, _ = updateApp(app, tea.CursorPositionMsg{X: 7, Y: 3})
			app, _ = updateApp(app, imageProbeTimeoutMsg{previous})
			app, _ = updateApp(app, imageProbeMsg{previous})
			if app.images.probeReply != "" || app.images.probeCursor != nil || !app.images.checking || app.images.attempts != attempt {
				t.Fatal("old response or timer contaminated the retry")
			}
		}
		if attempt < maxImageProbeAttempts {
			app, cmd = updateApp(app, imageProbeTimeoutMsg{id})
			if cmd == nil || !app.images.checking {
				t.Fatal("missing retry after lost probe")
			}
			previous = id
			continue
		}
		app, _ = updateApp(app, uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
		app, _ = updateApp(app, tea.CursorPositionMsg{X: 0, Y: 1})
		app, _ = updateApp(app, imageProbeTimeoutMsg{id})
		if !app.images.enabled || app.images.checking {
			t.Fatal("retry did not recover supported terminal")
		}
	}
}

func TestImageProbeExhaustionAndManualRecheck(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(fmt.Sprintf("mouse=%t", mouse), func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.images = newImageDisplay()
			app.images.checking = true
			for attempt := 0; attempt < maxImageProbeAttempts; attempt++ {
				id := app.images.probeID
				app, _ = updateApp(app, imageProbeMsg{id})
				app, _ = updateApp(app, imageProbeTimeoutMsg{id})
			}
			if app.images.checking || !strings.Contains(app.imagesButton(), "unavailable") {
				t.Fatal("unanswered probes did not finish")
			}
			oldID := app.images.probeID
			var cmd tea.Cmd
			if mouse {
				app.width, app.height = 100, 40
				app.recalculateLayout()
				_, layout := app.renderReviewScreen()
				app, cmd = updateApp(app, tea.MouseClickMsg{X: layout.imagesButton.left, Y: layout.imagesButton.top, Button: tea.MouseLeft})
			} else {
				app, cmd = updateApp(app, tea.KeyPressMsg{Code: 'I', Text: "I"})
			}
			if cmd == nil || !app.images.checking || app.images.hidden || app.images.probeID == oldID || app.images.attempts != 0 {
				t.Fatal("manual recheck did not start a fresh probe cycle")
			}
			previous := app.images.probeID
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'I', Text: "I"})
			if app.images.probeID == previous || app.images.hidden {
				t.Fatal("repeated recheck did not restart the running probe")
			}
			id := app.images.probeID
			app, _ = updateApp(app, imageProbeMsg{id})
			app, _ = updateApp(app, uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
			app, _ = updateApp(app, tea.CursorPositionMsg{X: 0, Y: 1})
			if !app.images.enabled || app.images.hidden {
				t.Fatal("manual recheck did not restore previews")
			}
		})
	}
}

func TestImageManualRecheckHasNoAttemptLimit(t *testing.T) {
	app := setupAppWithDoc(t, "source\n")
	for cycle := 0; cycle < 5; cycle++ {
		previous := app.images.probeID
		app, _ = updateApp(app, tea.KeyPressMsg{Code: 'I', Text: "I"})
		if !app.images.checking || app.images.attempts != 0 || app.images.probeID == previous {
			t.Fatalf("cycle %d: manual check did not restart", cycle)
		}
		app, _ = updateApp(app, imageProbeTimeoutMsg{previous})
		if !app.images.checking || app.images.attempts != 0 {
			t.Fatal("old timeout ended the manual check")
		}
		for attempt := 0; attempt < maxImageProbeAttempts; attempt++ {
			id := app.images.probeID
			app, _ = updateApp(app, imageProbeMsg{id})
			app, _ = updateApp(app, imageProbeTimeoutMsg{id})
		}
		if app.images.checking {
			t.Fatal("automatic retries did not stop")
		}
	}
}

func TestImageProbeIgnoresEarlierCursorReports(t *testing.T) {
	app := AppModel{images: newImageDisplay()}
	app.images.checking = true
	id := app.images.probeID
	app, _ = updateApp(app, imageProbeMsg{id})
	app, _ = updateApp(app, tea.CursorPositionMsg{X: 7, Y: 3})
	app, _ = updateApp(app, uv.KittyGraphicsEvent{Options: kitty.Options{ID: id}, Payload: []byte("OK")})
	app, _ = updateApp(app, tea.CursorPositionMsg{X: 0, Y: 1})
	if !app.images.enabled {
		t.Fatal("earlier cursor report blocked a supported terminal")
	}
}

func TestImageProbeDoesNotRetryOnTextInput(t *testing.T) {
	for _, checking := range []bool{false, true} {
		t.Run(fmt.Sprintf("checking=%t", checking), func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.images = newImageDisplay()
			app.images.checking = checking
			app.modal = commentModal
			app.modalTextarea.Focus()
			previous := app.images.probeID
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'a', Text: "a"})
			if app.images.checking != checking || app.images.probeID != previous {
				t.Fatal("text input restarted image detection")
			}
			id := app.images.probeID
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'b', Text: "b"})
			if app.images.probeID != id || app.modalTextarea.Value() != "ab" {
				t.Fatal("retry repeated or intercepted normal input")
			}
			for attempt := 0; attempt < maxImageProbeAttempts; attempt++ {
				id := app.images.probeID
				app, _ = updateApp(app, imageProbeMsg{id})
				app, _ = updateApp(app, imageProbeTimeoutMsg{id})
			}
			id = app.images.probeID
			app, _ = updateApp(app, tea.KeyPressMsg{Code: 'c', Text: "c"})
			if app.images.checking || app.images.probeID != id || app.modalTextarea.Value() != "abc" {
				t.Fatal("later key retried unavailable detection")
			}
		})
	}
	app := setupAppWithDoc(t, "source\n")
	app.images = newImageDisplay()
	app.images.enabled = true
	id := app.images.probeID
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab})
	if app.images.probeID != id {
		t.Fatal("confirmed terminal was checked again")
	}
}

func TestImageProbeRetriesWhenSelectingPreview(t *testing.T) {
	for _, tc := range []struct {
		name, path, content string
		binary, want        bool
	}{
		{"image", "image.png", "", true, true},
		{"document image", "images.md", "![preview](image.png)", false, true},
		{"document text", "text.md", "plain text", false, false},
		{"remote image", "remote.md", "![remote](https://example.com/image.png)", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "text\n")
			app.multiFile = true
			app.images.checking = false
			app.tabs = append(app.tabs, FileTab{path: tc.path, isBinary: tc.binary, isMarkdown: !tc.binary, doc: document.FromContent(tc.path, []byte(tc.content)), state: &fileReview{}})
			id := app.images.probeID
			app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyTab})
			if got := app.images.probeID != id; got != tc.want {
				t.Fatalf("selection retry = %t, want %t", got, tc.want)
			}
			if tc.want {
				id = app.images.probeID
				app, _ = updateApp(app, tea.KeyPressMsg{Code: 'j', Text: "j"})
				if app.images.probeID != id {
					t.Fatal("navigation restarted the pending preview check")
				}
			}
		})
	}
}

func TestImagePlaceholderSurvivesRendering(t *testing.T) {
	app := setupAppWithDoc(t, "text\n")
	app.width, app.height = 100, 30
	app.recalculateLayout()
	img := &inlineImage{id: 0x123456, width: 30, height: 20, ready: true}
	img.columns, img.rows = img.size(app.contentViewport.Width(), app.contentViewport.Height())
	app.tab().isBinary, app.tab().doc, app.tab().preview.after.image = true, nil, img
	app.rebuildContent()
	view := app.View().Content
	lines := uv.NewStyledString(view).Lines(ansi.GraphemeWidth)
	count := 0
	for _, line := range lines {
		for _, cell := range line {
			if !strings.ContainsRune(cell.Content, kitty.Placeholder) {
				continue
			}
			count++
			if cell.Width != 1 || cell.Style.Fg == nil {
				t.Fatalf("invalid placeholder cell: %+v", cell)
			}
			r, g, b, _ := cell.Style.Fg.RGBA()
			if r>>8 != 0x12 || g>>8 != 0x34 || b>>8 != 0x56 {
				t.Fatalf("image ID changed during rendering: %+v", cell)
			}
		}
	}
	if count != img.columns*img.rows {
		t.Fatalf("placeholder count=%d want=%d", count, img.columns*img.rows)
	}
	app.modal = helpModal
	if strings.ContainsRune(app.View().Content, kitty.Placeholder) {
		t.Fatal("image remains behind the dialog")
	}
	app.modal = noModal
	if !strings.ContainsRune(app.View().Content, kitty.Placeholder) {
		t.Fatal("closing dialog did not restore image")
	}
	masked := hideImagePlaceholders(img.placeholder())
	if lipgloss.Width(masked) != img.columns+2 || lipgloss.Height(masked) != img.rows {
		t.Fatal("hiding image changed layout")
	}
}

func TestImagesUseDistinctPlacementIDs(t *testing.T) {
	images := []inlineImage{
		{id: 0x123456, columns: 32, rows: 16},
		{id: 0x654321, columns: 40, rows: 6},
	}
	for _, img := range images {
		placement := fmt.Sprintf("p=%d", img.id)
		if !strings.Contains(img.placement(), placement+",") {
			t.Fatalf("image %d has no distinct placement ID: %q", img.id, img.placement())
		}
		for _, line := range uv.NewStyledString(img.placeholder()).Lines(ansi.GraphemeWidth) {
			for _, cell := range line {
				if !strings.ContainsRune(cell.Content, kitty.Placeholder) {
					continue
				}
				if cell.Style.UnderlineColor == nil {
					t.Fatal("placeholder has no placement ID")
				}
				r, g, b, _ := cell.Style.UnderlineColor.RGBA()
				id := int(r>>8)<<16 | int(g>>8)<<8 | int(b>>8)
				if id != img.id {
					t.Fatalf("placeholder placement ID=%d want=%d", id, img.id)
				}
			}
		}
	}
}

func TestPreviewReadsReviewedImage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGitIn(t, dir, "init", "-q")
	runGitIn(t, dir, "config", "user.name", "Test")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	write := func(width int) {
		t.Helper()
		if err := os.WriteFile("画像.png", previewPNG(t, width, 2), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(3)
	runGitIn(t, dir, "add", "画像.png")
	runGitIn(t, dir, "commit", "-qm", "base")
	write(4)
	runGitIn(t, dir, "add", "画像.png")
	runGitIn(t, dir, "commit", "-qm", "head")
	write(5)
	runGitIn(t, dir, "add", "画像.png")
	write(6)
	for _, tc := range []struct {
		source  gitpkg.ReviewSource
		deleted bool
		width   int
	}{
		{gitpkg.ReviewSource{Scope: "all"}, false, 6},
		{gitpkg.ReviewSource{Scope: "staged"}, false, 5},
		{gitpkg.ReviewSource{Scope: "range", Base: "HEAD~1", Head: "HEAD"}, false, 4},
		{gitpkg.ReviewSource{Scope: "all"}, true, 4},
		{gitpkg.ReviewSource{Scope: "unstaged"}, true, 5},
		{gitpkg.ReviewSource{Scope: "range", Base: "HEAD~1", Head: "HEAD"}, true, 3},
	} {
		t.Run(fmt.Sprintf("%s/deleted=%t", tc.source.Scope, tc.deleted), func(t *testing.T) {
			app := AppModel{images: imageDisplay{enabled: true}, source: &tc.source,
				tabs: []FileTab{{path: "画像.png", isBinary: true, isDeleted: tc.deleted, state: &fileReview{}}}}
			msg := app.prepareImage()().(imageLoadedMsg)
			if msg.err != nil || msg.width != tc.width || msg.height != 2 || !strings.Contains(msg.sequence, "f=100") {
				t.Fatalf("loaded wrong snapshot: %+v", msg)
			}
			_, payload, ok := strings.Cut(msg.sequence, ";")
			if !ok {
				t.Fatal("image payload missing")
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(payload, "\x1b\\"))
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil || img.Bounds().Dx() != tc.width {
				t.Fatalf("transmitted image: %v", err)
			}
			if app.prepareImage() != nil {
				t.Fatal("reloaded cached image")
			}
		})
	}
	for _, source := range []gitpkg.ReviewSource{{Scope: "all"}, {Scope: "staged"}, {Scope: "range", Head: "HEAD"}} {
		if _, err := source.PreviewContent("画像.png", false, 1); err == nil {
			t.Fatalf("%s accepted image beyond read limit", source.Scope)
		}
	}
	for _, tc := range []struct {
		source        gitpkg.ReviewSource
		before, after int
	}{
		{gitpkg.ReviewSource{Scope: "all"}, 4, 6},
		{gitpkg.ReviewSource{Scope: "staged"}, 4, 5},
		{gitpkg.ReviewSource{Scope: "unstaged"}, 5, 6},
		{gitpkg.ReviewSource{Scope: "range", Base: "HEAD~1", Head: "HEAD"}, 3, 4},
	} {
		app := AppModel{images: imageDisplay{enabled: true}, source: &tc.source,
			tabs: []FileTab{{path: "画像.png", status: "modified", isBinary: true, state: &fileReview{}}}}
		batch := app.prepareImage()().(tea.BatchMsg)
		if len(batch) != 2 {
			t.Fatal("comparison did not load both sides")
		}
		for _, cmd := range batch {
			msg := cmd().(imageLoadedMsg)
			want := tc.after
			if msg.before {
				want = tc.before
			}
			if msg.err != nil || msg.width != want {
				t.Fatalf("%s comparison: %+v", tc.source.Scope, msg)
			}
		}
	}
	if err := os.WriteFile("renamed.png", previewPNG(t, 7, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	app := AppModel{images: imageDisplay{enabled: true},
		tabs: []FileTab{{path: "renamed.png", oldPath: "画像.png", status: "renamed", isBinary: true, state: &fileReview{}}}}
	before := app.loadImage(true)().(imageLoadedMsg)
	after := app.loadImage(false)().(imageLoadedMsg)
	if before.err != nil || before.width != 4 || after.err != nil || after.width != 7 {
		t.Fatal("renamed comparison did not read the old path")
	}
}

func TestPreviewComparisonLayout(t *testing.T) {
	app := setupAppWithDoc(t, "text\n")
	app.images.enabled = true
	app.tab().path, app.tab().status, app.tab().isBinary = "file.png", "modified", true
	app.tab().preview = imagePreview{
		before: previewSide{image: &inlineImage{id: 0x123456, width: 12, height: 12}},
		after:  previewSide{image: &inlineImage{id: 0x654321, width: 12, height: 12}},
	}
	for _, width := range []int{100, 40, 100} {
		app.contentViewport.SetWidth(width)
		app.contentViewport.SetHeight(30)
		app.prepareImage()
		for _, old := range []bool{true, false} {
			app.tab().preview.side(old).image.ready = true
		}
		preview := app.renderImagePreview()
		if lipgloss.Width(preview) > width {
			t.Fatal("comparison exceeds pane width")
		}
		labelsTogether := false
		for _, line := range strings.Split(ansi.Strip(preview), "\n") {
			if strings.Contains(line, "Before (12×12)") && strings.Contains(line, "After (12×12)") {
				labelsTogether = true
			}
		}
		if labelsTogether != (width >= 64) {
			t.Fatal("comparison did not adapt to width")
		}
		app.rebuildContent()
		for _, row := range app.contentLayout.rows {
			if row.line != 0 {
				t.Fatal("preview created commentable source lines")
			}
		}
	}
	app.tab().preview.before = previewSide{failed: true, attempted: true}
	preview := app.renderImagePreview()
	if !strings.Contains(preview, "Preview unavailable") || !strings.ContainsRune(preview, kitty.Placeholder) {
		t.Fatalf("one unavailable side hid the other image: %q", preview)
	}
}

func TestPreviewFallbackAndResize(t *testing.T) {
	app := setupAppWithDoc(t, "text\n")
	app.tab().path, app.tab().isBinary = "file.png", true
	if app.prepareImage() == nil || !app.images.checking || app.tab().preview.after.attempted {
		t.Fatal("image must request detection before loading")
	}
	app.images.enabled = true
	app.patch = &gitpkg.Patch{}
	if app.prepareImage() != nil {
		t.Fatal("read local image for supplied diff")
	}
	app.patch = nil
	app.tab().preview.after.image = &inlineImage{id: 17, width: 30, height: 20}
	app.contentViewport.SetWidth(50)
	app.contentViewport.SetHeight(20)
	if app.prepareImage() == nil {
		t.Fatal("missing initial placement")
	}
	cols, rows := app.tab().preview.after.image.columns, app.tab().preview.after.image.rows
	model, _ := app.Update(imageReadyMsg{path: "file.png", columns: cols - 1, rows: rows})
	app = model.(AppModel)
	if app.tab().preview.after.image.ready {
		t.Fatal("stale resize completion accepted")
	}
	model, _ = app.Update(imageReadyMsg{path: "file.png", columns: cols, rows: rows})
	app = model.(AppModel)
	if !app.tab().preview.after.image.ready {
		t.Fatal("placement did not become visible")
	}
	app.contentViewport.SetWidth(30)
	if app.prepareImage() == nil || app.tab().preview.after.image.ready {
		t.Fatal("resize did not update placement")
	}
	if _, err := (gitpkg.ReviewSource{}).PreviewContent("missing.png", false, maxPreviewBytes); err == nil {
		t.Fatal("missing image accepted")
	}
}
