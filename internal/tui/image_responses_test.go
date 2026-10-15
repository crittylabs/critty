package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestImageResponseReplayPreservesTextInput(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"\x1b[95;3uabc", "abc"},
		{"\x1b[95;3uG", "G"},
		{"\x1b[91;3uA", "A"},
		{"\x1b[91;3u日本語", "日本語"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.images.tmux = true
			app.modal = commentModal
			app.modalTextarea.Focus()
			out := &previewOutput{changed: make(chan struct{}, 1)}
			p := tea.NewProgram(app, tea.WithInput(strings.NewReader(tc.input)), tea.WithOutput(out),
				tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
			timer := time.AfterFunc(250*time.Millisecond, p.Quit)
			defer timer.Stop()
			model, err := p.Run()
			if err != nil {
				t.Fatalf("buffered keys were not restored to the editor: %v", err)
			}
			var got string
			switch app := model.(type) {
			case AppModel:
				got = app.modalTextarea.Value()
			case *AppModel:
				got = app.modalTextarea.Value()
			}
			if got != tc.want {
				t.Fatalf("editor input = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTerminalTrafficDoesNotEnterEditor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunks []string
		tmux   bool
	}{
		{"mouse", []string{"\x1b[91;3u<35;20;10M"}, true},
		{"direct delayed mouse", []string{"\x1b[<35;", "20;10M"}, false},
		{"direct delayed graphics", []string{"\x1b_Gi=123;", "OK\x1b\\"}, false},
		{"delayed mouse", []string{"\x1b[91;3u<35;", "20;10M"}, true},
		{"delayed graphics", []string{"\x1b[95;3uGi=123;", "OK\x1b[92;3u\x1b[91;3u2;1R"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupAppWithDoc(t, "source\n")
			app.images.tmux = tc.tmux
			app.modal = commentModal
			app.modalTextarea.Focus()
			reader, writer := io.Pipe()
			defer reader.Close()
			out := &previewOutput{changed: make(chan struct{}, 1)}
			p := tea.NewProgram(app, tea.WithInput(reader), tea.WithOutput(out), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
			writes := make(chan error, 1)
			go func() {
				defer writer.Close()
				for _, chunk := range tc.chunks {
					if _, err := io.WriteString(writer, chunk); err != nil {
						writes <- err
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				_, err := io.WriteString(writer, "after")
				writes <- err
				// Let the input reader deliver the final ordinary text before quitting.
				time.Sleep(100 * time.Millisecond)
				p.Quit()
			}()
			timer := time.AfterFunc(3*time.Second, p.Kill)
			defer timer.Stop()
			model, err := p.Run()
			if err != nil {
				t.Fatal(err)
			}
			if err := <-writes; err != nil {
				t.Fatal(err)
			}
			var got string
			switch app := model.(type) {
			case AppModel:
				got = app.modalTextarea.Value()
			case *AppModel:
				got = app.modalTextarea.Value()
			}
			if got != "after" {
				t.Fatalf("terminal traffic changed editor input: %q", got)
			}
		})
	}
}

func TestIdentifiedResponseIgnoresKeyDeadline(t *testing.T) {
	d := imageDisplay{tmux: true}
	feed := func(key tea.KeyPressMsg) tea.Msg {
		msg, _ := d.decodeResponse(key)
		return msg
	}
	feed(tea.KeyPressMsg{Code: '_', Mod: tea.ModAlt})
	feed(tea.KeyPressMsg{Code: 'G', Text: "Gi=123;"})
	if msg, _ := d.decodeResponse(imageResponseTimeoutMsg{d.input.generation}); msg != nil || !d.input.confirmed {
		t.Fatal("identified response was replayed at the Alt-key deadline")
	}
	feed(tea.KeyPressMsg{Code: 'O', Text: "OK"})
	if _, ok := feed(tea.KeyPressMsg{Code: '\\', Mod: tea.ModAlt}).(uv.KittyGraphicsEvent); !ok {
		t.Fatal("delayed response was not decoded")
	}
}

func TestOversizedResponseDrainsAndPreservesInterrupt(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		d := imageDisplay{tmux: true}
		d.decodeResponse(tea.KeyPressMsg{Code: '_', Mod: tea.ModAlt})
		d.decodeResponse(tea.KeyPressMsg{Code: 'G', Text: "Gi=123;"})
		if msg, _ := d.decodeResponse(tea.KeyPressMsg{Code: 'E', Text: strings.Repeat("E", maxImageResponseBytes)}); msg != nil {
			t.Fatal("oversized response was replayed as keys")
		}
		if len(d.input.data) != 0 || len(d.input.keys) != 0 {
			t.Fatal("oversized response retained buffered input")
		}
		if interrupt {
			key := tea.KeyPressMsg{Code: tea.KeyEscape}
			if msg, _ := d.decodeResponse(key); msg != key {
				t.Fatal("escape was swallowed by an incomplete response")
			}
		} else {
			d.decodeResponse(tea.KeyPressMsg{Code: '\\', Mod: tea.ModAlt})
		}
		key := tea.KeyPressMsg{Code: 'x', Text: "x"}
		if msg, _ := d.decodeResponse(key); msg != key {
			t.Fatal("response drain swallowed later text")
		}
	}
}

func TestImageResponseTimeoutAndSizeLimit(t *testing.T) {
	d := imageDisplay{tmux: true}
	prefix := tea.KeyPressMsg{Code: '_', Mod: tea.ModAlt}
	msg, cmd := d.decodeResponse(prefix)
	if msg != nil || cmd == nil {
		t.Fatal("ambiguous Alt key was not buffered with a deadline")
	}
	old := d.input.generation
	msg, cmd = d.decodeResponse(imageResponseTimeoutMsg{old})
	if cmd != nil || msg.(imageResponseReplayMsg).keys[0] != prefix {
		t.Fatal("timeout did not preserve the original Alt key")
	}
	d.decodeResponse(prefix)
	d.decodeResponse(imageResponseTimeoutMsg{old})
	if len(d.input.keys) != 1 {
		t.Fatal("stale timeout flushed a later sequence")
	}
	msg, _ = d.decodeResponse(tea.KeyPressMsg{Code: 'a', Text: strings.Repeat("a", 4096)})
	if msg == nil || len(d.input.keys) != 0 || len(d.input.data) != 0 {
		t.Fatal("oversized sequence retained buffered keys")
	}
}

func TestImageResponseDoesNotInterceptDirectTerminalKeys(t *testing.T) {
	d := imageDisplay{}
	key := tea.KeyPressMsg{Code: '_', Mod: tea.ModAlt}
	msg, cmd := d.decodeResponse(key)
	if msg != key || cmd != nil {
		t.Fatal("direct terminal Alt key was intercepted")
	}
}
