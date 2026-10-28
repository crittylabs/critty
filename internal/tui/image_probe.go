package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

// InteractiveTerminal reports whether the current TERM supports interactive
// terminal UI and escape-sequence probes. Agent tool runners such as
// Antigravity allocate a foreground PTY with TERM=dumb to capture output.
func InteractiveTerminal() bool {
	termName := strings.TrimSpace(os.Getenv("TERM"))
	return termName != "" && !strings.EqualFold(termName, "dumb")
}

// ImageCapability records an actual terminal probe, or why it was skipped.
type ImageCapability struct {
	Checked   bool
	Supported bool
	Reason    string
}

type terminalImageProbe struct {
	display    imageDisplay
	parser     *ansi.Parser
	data       []byte
	cursorSeen bool
}

func newTerminalImageProbe(tmux bool) terminalImageProbe {
	p := new(ansi.Parser)
	p.SetParamsSize(parser.MaxParamsSize)
	p.SetDataSize(maxImageResponseBytes)
	return terminalImageProbe{
		display: imageDisplay{probeID: newImageID(), tmux: tmux, checking: true, attempts: 1},
		parser:  p,
	}
}

// feed uses the same response decoder and acceptance test as the review TUI.
func (p *terminalImageProbe) feed(data []byte) error {
	var decoder uv.EventDecoder
	for _, b := range data {
		if b == 3 {
			return fmt.Errorf("image check interrupted")
		}
		p.data = append(p.data, b)
		if len(p.data) > maxImageResponseBytes {
			return fmt.Errorf("terminal response too large")
		}
		p.parser.Advance(b)
		if p.parser.State() != parser.GroundState {
			continue
		}
		_, event := decoder.Decode(p.data)
		p.data = nil
		switch event := event.(type) {
		case uv.KeyPressEvent:
			msg, _ := p.display.decodeResponse(tea.KeyPressMsg(event))
			p.accept(msg)
		case uv.CursorPositionEvent:
			p.accept(tea.CursorPositionMsg(event))
		case uv.MultiEvent:
			for _, event := range event {
				if cursor, ok := event.(uv.CursorPositionEvent); ok {
					p.accept(tea.CursorPositionMsg(cursor))
				}
			}
		default:
			p.accept(event)
		}
		if p.complete() {
			return nil
		}
	}
	return nil
}

func (p *terminalImageProbe) accept(msg tea.Msg) {
	if _, ok := msg.(tea.CursorPositionMsg); ok && p.display.probeReply != "" {
		p.cursorSeen = true
	}
	p.display.acceptProbeResponse(msg)
}

func (p *terminalImageProbe) complete() bool {
	return p.display.probeReply != "" && p.cursorSeen
}
