package tui

import (
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

const maxImageResponseBytes = 4096

// tmux may encode a terminal reply's ESC prefix as an Alt key when extended
// keys are enabled.  Reassemble only graphics and cursor replies; replay other
// keys unchanged when the sequence is invalid or incomplete.
type imageResponseInput struct {
	data       []byte
	keys       []tea.KeyPressMsg
	generation uint64
	parser     *ansi.Parser
}

type imageResponseTimeoutMsg struct{ generation uint64 }
type imageResponseReplayMsg struct{ keys []tea.KeyPressMsg }

func (d *imageDisplay) decodeResponse(msg tea.Msg) (tea.Msg, tea.Cmd) {
	if timeout, ok := msg.(imageResponseTimeoutMsg); ok {
		if timeout.generation == d.input.generation {
			return d.input.replay(), nil
		}
		return nil, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !d.tmux || !ok {
		return msg, nil
	}
	input := &d.input
	var timeout tea.Cmd
	if len(input.keys) == 0 {
		if key.Mod != tea.ModAlt || key.Code != '_' && key.Code != '[' {
			return msg, nil
		}
		input.generation++
		input.parser = new(ansi.Parser)
		input.parser.SetParamsSize(parser.MaxParamsSize)
		input.parser.SetDataSize(maxImageResponseBytes)
		generation := input.generation
		timeout = tea.Tick(uv.DefaultEscTimeout,
			func(time.Time) tea.Msg { return imageResponseTimeoutMsg{generation} })
	}
	input.keys = append(input.keys, key)
	previous := len(input.data)
	switch {
	case key.Mod == tea.ModAlt && (key.Code == '_' || key.Code == '[' || key.Code == '\\'):
		input.data = append(input.data, '\x1b', byte(key.Code))
	case key.Mod&^tea.ModShift == 0 && key.Text != "":
		input.data = append(input.data, key.Text...)
	case key.Mod == 0 && unicode.IsPrint(key.Code):
		input.data = append(input.data, string(key.Code)...)
	default:
		return input.replay(), nil
	}
	if len(input.data) > maxImageResponseBytes {
		return input.replay(), nil
	}
	for _, b := range input.data[previous:] {
		input.parser.Advance(b)
	}
	if input.parser.State() != parser.GroundState {
		return nil, timeout
	}
	var decoder uv.EventDecoder
	n, event := decoder.Decode(input.data)
	if n == len(input.data) {
		switch event := event.(type) {
		case uv.KittyGraphicsEvent:
			input.clear()
			return event, nil
		case uv.CursorPositionEvent:
			input.clear()
			return tea.CursorPositionMsg(event), nil
		case uv.MultiEvent:
			for _, event := range event {
				if cursor, ok := event.(uv.CursorPositionEvent); ok {
					input.clear()
					return tea.CursorPositionMsg(cursor), nil
				}
			}
		}
	}
	return input.replay(), nil
}

func (input *imageResponseInput) clear() {
	input.data = nil
	input.keys = nil
	input.parser = nil
}

func (input *imageResponseInput) replay() tea.Msg {
	if len(input.keys) == 0 {
		return nil
	}
	msg := imageResponseReplayMsg{input.keys}
	input.clear()
	return msg
}
