package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type terminalInputApp struct {
	AppModel
	want string
}

func (m terminalInputApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	app, cmd := updateApp(m.AppModel, msg)
	m.AppModel = app
	if app.modalTextarea.Value() == m.want {
		return m, tea.Batch(cmd, tea.Quit)
	}
	return m, cmd
}

func TestImageResponseReplayPreservesTextInput(t *testing.T) {
	t.Parallel()
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
			app := setupParallelAppWithDoc(t, "source\n")
			app.images.tmux = true
			app.modal = commentModal
			app.modalTextarea.Focus()
			out := &previewOutput{changed: make(chan struct{}, 1)}
			p := tea.NewProgram(terminalInputApp{AppModel: app, want: tc.want}, tea.WithInput(strings.NewReader(tc.input)), tea.WithOutput(out),
				tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
			timer := time.AfterFunc(3*time.Second, p.Kill)
			defer timer.Stop()
			model, err := p.Run()
			if err != nil {
				t.Fatalf("buffered keys were not restored to the editor: %v", err)
			}
			got := model.(terminalInputApp).modalTextarea.Value()
			if got != tc.want {
				t.Fatalf("editor input = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTerminalTrafficDoesNotEnterEditor(t *testing.T) {
	// Escape decoding uses a real clock deadline; keep this program test serial.
	for _, tc := range []struct {
		name   string
		chunks []string
		tmux   bool
	}{
		{"mouse", []string{"\x1b[91;3u<35;20;10M"}, true},
		{"direct delayed mouse", []string{"\x1b[<35;", "20;10M"}, false},
		{"direct delayed graphics", []string{"\x1b_Gi=123;", "OK\x1b\\"}, false},
		{"delayed mouse", []string{"\x1b[91;3u<35;", "20;10M"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := setupPrivateAppWithDoc(t, "source\n")
			app.images.tmux = tc.tmux
			app.modal = commentModal
			app.modalTextarea.Focus()
			reader, writer := io.Pipe()
			defer reader.Close()
			out := &previewOutput{changed: make(chan struct{}, 1)}
			p := tea.NewProgram(terminalInputApp{AppModel: app, want: "after"}, tea.WithInput(reader), tea.WithOutput(out), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
			writes := make(chan error, 1)
			go func() {
				defer writer.Close()
				for _, chunk := range tc.chunks {
					if _, err := io.WriteString(writer, chunk); err != nil {
						writes <- err
						return
					}
					// Cross the Alt-key deadline and separate traffic from editor text.
					time.Sleep(100 * time.Millisecond)
				}
				_, err := io.WriteString(writer, "after")
				writes <- err
			}()
			timer := time.AfterFunc(3*time.Second, p.Kill)
			defer timer.Stop()
			model, err := p.Run()
			if err != nil {
				if state, ok := model.(terminalInputApp); ok {
					t.Fatalf("%v; editor input = %q", err, state.modalTextarea.Value())
				}
				t.Fatal(err)
			}
			if err := <-writes; err != nil {
				t.Fatal(err)
			}
			got := model.(terminalInputApp).modalTextarea.Value()
			if got != "after" {
				t.Fatalf("terminal traffic changed editor input: %q", got)
			}
		})
	}
}

func TestIdentifiedResponseIgnoresKeyDeadline(t *testing.T) {
	app := setupPrivateAppWithDoc(t, "source\n")
	app.images = imageDisplay{tmux: true, probeID: 123, checking: true, attempts: 1}
	app.modal = commentModal
	app.modalTextarea.Focus()
	feed := func(msg tea.Msg) { app, _ = updateApp(app, msg) }
	feed(tea.KeyPressMsg{Code: '_', Mod: tea.ModAlt})
	feed(tea.KeyPressMsg{Code: 'G', Text: "Gi=123;"})
	feed(imageResponseTimeoutMsg{app.images.input.generation})
	if !app.images.input.confirmed || app.modalTextarea.Value() != "" {
		t.Fatal("identified graphics response was replayed at the Alt-key deadline")
	}
	feed(tea.KeyPressMsg{Code: 'O', Text: "OK"})
	feed(tea.KeyPressMsg{Code: '\\', Mod: tea.ModAlt})
	if app.images.probeReply != "OK" {
		t.Fatal("delayed graphics response was not decoded")
	}
	feed(tea.KeyPressMsg{Code: '[', Mod: tea.ModAlt})
	feed(tea.KeyPressMsg{Code: '2', Text: "2;"})
	feed(imageResponseTimeoutMsg{app.images.input.generation})
	if !app.images.input.confirmed {
		t.Fatal("identified cursor response expired at the Alt-key deadline")
	}
	feed(tea.KeyPressMsg{Code: '1', Text: "1R"})
	if app.images.probeCursor == nil || app.modalTextarea.Value() != "" {
		t.Fatal("terminal traffic entered the editor instead of completing the probe")
	}
	for _, r := range "after" {
		feed(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := app.modalTextarea.Value(); got != "after" {
		t.Fatalf("terminal traffic changed subsequent editor input: %q", got)
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
