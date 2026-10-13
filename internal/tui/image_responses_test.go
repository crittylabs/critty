package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
				tea.WithWindowSize(80, 24), tea.WithoutSignalHandler(),
				tea.WithFilter(func(model tea.Model, msg tea.Msg) tea.Msg {
					var current AppModel
					switch model := model.(type) {
					case AppModel:
						current = model
					case *AppModel:
						current = *model
					}
					if current.modalTextarea.Value() == tc.want {
						return tea.Quit()
					}
					return msg
				}))
			timer := time.AfterFunc(2*time.Second, p.Kill)
			defer timer.Stop()
			if _, err := p.Run(); err != nil {
				t.Fatalf("buffered keys were not restored to the editor: %v", err)
			}
		})
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
