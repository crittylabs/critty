package tui

import (
	"bytes"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

const maxImageResponseBytes = 4096

// tmux may encode a terminal reply's ESC prefix as an Alt key when extended
// keys are enabled.  Keep the Alt-key deadline only while the prefix is
// ambiguous; identified terminal traffic ends at its ANSI framing boundary.
type imageResponseInput struct {
	data       []byte
	keys       []tea.KeyPressMsg
	generation uint64
	parser     *ansi.Parser
	confirmed  bool
	discard    bool
}

type imageResponseTimeoutMsg struct{ generation uint64 }
type imageResponseReplayMsg struct{ keys []tea.KeyPressMsg }

func (d *imageDisplay) decodeResponse(msg tea.Msg) (tea.Msg, tea.Cmd) {
	if timeout, ok := msg.(imageResponseTimeoutMsg); ok {
		if timeout.generation == d.input.generation && !d.input.confirmed {
			return d.input.replay(), nil
		}
		return nil, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	input := &d.input
	switch msg.(type) {
	case uv.KittyGraphicsEvent, tea.CursorPositionMsg, tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
		if input.confirmed {
			input.clear()
		}
	}
	if unknown, ok := msg.(uv.UnknownEvent); ok && terminalResponsePrefix([]byte(unknown), d.probeReply != "") {
		input.clear()
		input.start()
		input.data = append(input.data, []byte(unknown)...)
		for _, b := range input.data {
			input.parser.Advance(b)
		}
		if input.parser.State() != parser.GroundState {
			input.confirmed = true
			if len(input.data) > maxImageResponseBytes {
				input.data = nil
				input.discard = true
			}
			return nil, nil
		}
		input.clear()
	}
	if !ok || !d.tmux && input.parser == nil {
		return msg, nil
	}
	if input.confirmed && key.Mod == tea.ModAlt && (key.Code == '_' || key.Code == '[') {
		// A new control sequence replaces a truncated reply; its old bytes are
		// terminal traffic, not keys to replay.
		input.clear()
	}
	var timeout tea.Cmd
	if input.parser == nil {
		if key.Mod != tea.ModAlt || key.Code != '_' && key.Code != '[' {
			return msg, nil
		}
		input.start()
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
		if input.confirmed {
			input.clear()
			return key, nil
		}
		return input.replay(), nil
	}
	for _, b := range input.data[previous:] {
		input.parser.Advance(b)
	}
	if terminalResponsePrefix(input.data, d.probeReply != "") {
		input.confirmed = true
	}
	if len(input.data) > maxImageResponseBytes {
		if !input.confirmed {
			return input.replay(), nil
		}
		input.discard = true
	}
	if input.discard {
		input.data, input.keys = nil, nil
		if input.parser.State() == parser.GroundState {
			input.clear()
		}
		return nil, nil
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
		case uv.MouseMotionEvent:
			input.clear()
			return tea.MouseMotionMsg(event), nil
		case uv.MouseClickEvent:
			input.clear()
			return tea.MouseClickMsg(event), nil
		case uv.MouseReleaseEvent:
			input.clear()
			return tea.MouseReleaseMsg(event), nil
		case uv.MouseWheelEvent:
			input.clear()
			return tea.MouseWheelMsg(event), nil
		case uv.MultiEvent:
			for _, event := range event {
				if cursor, ok := event.(uv.CursorPositionEvent); ok {
					input.clear()
					return tea.CursorPositionMsg(cursor), nil
				}
			}
		}
	}
	if input.confirmed {
		input.clear()
		return nil, nil
	}
	return input.replay(), nil
}

func terminalResponsePrefix(data []byte, cursorExpected bool) bool {
	if bytes.HasPrefix(data, []byte("\x1b_Gi=")) || bytes.HasPrefix(data, []byte("\x1b[<")) {
		return true
	}
	return cursorExpected && bytes.HasPrefix(data, []byte("\x1b[")) && len(data) > 2 && data[2] >= '0' && data[2] <= '9'
}

func (input *imageResponseInput) start() {
	input.generation++
	input.parser = new(ansi.Parser)
	input.parser.SetParamsSize(parser.MaxParamsSize)
	input.parser.SetDataSize(maxImageResponseBytes)
}

func (input *imageResponseInput) clear() {
	input.data = nil
	input.keys = nil
	input.parser = nil
	input.confirmed = false
	input.discard = false
}

func (input *imageResponseInput) replay() tea.Msg {
	if len(input.keys) == 0 {
		return nil
	}
	msg := imageResponseReplayMsg{input.keys}
	input.clear()
	return msg
}
