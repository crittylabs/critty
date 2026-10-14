package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestTerminalImageProbeResponses(t *testing.T) {
	for _, tt := range []struct {
		name            string
		tmux, supported bool
		reply           func(int) string
	}{
		{"supported", false, true, func(id int) string { return fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\\x1b[2;1R", id) }},
		{"stale cursor", false, true, func(id int) string { return fmt.Sprintf("\x1b[4;8R\x1b_Gi=%d;OK\x1b\\\x1b[2;1R", id) }},
		{"moved cursor", false, false, func(id int) string { return fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\\x1b[2;2R", id) }},
		{"rejected", false, false, func(id int) string { return fmt.Sprintf("\x1b_Gi=%d;ENOTSUP\x1b\\\x1b[2;1R", id) }},
		{"tmux extended keys", true, true, func(id int) string { return fmt.Sprintf("\x1b[95;3uGi=%d;OK\x1b[92;3u\x1b[91;3u2;1R", id) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunks := range []int{1, 7, 4096} {
				p := newTerminalImageProbe(tt.tmux)
				data := []byte(tt.reply(p.display.probeID))
				for len(data) > 0 {
					n := min(chunks, len(data))
					if err := p.feed(data[:n]); err != nil {
						t.Fatal(err)
					}
					data = data[n:]
				}
				if !p.complete() || p.display.enabled != tt.supported {
					t.Fatalf("wrong response state: %+v", p)
				}
			}
		})
	}
}

func TestTerminalImageProbeRejectDrainsCursor(t *testing.T) {
	p := newTerminalImageProbe(false)
	if err := p.feed([]byte(fmt.Sprintf("\x1b_Gi=%d;ENOTSUP\x1b\\", p.display.probeID))); err != nil {
		t.Fatal(err)
	}
	if p.complete() {
		t.Fatal("stopped before the trailing cursor reply")
	}
	if err := p.feed([]byte("\x1b[2;1R")); err != nil || !p.complete() {
		t.Fatalf("cursor not drained: %v", err)
	}
}

func TestTerminalImageProbeIgnoresStaleID(t *testing.T) {
	p := newTerminalImageProbe(false)
	if err := p.feed([]byte(fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\\x1b[2;1R", p.display.probeID+1))); err != nil {
		t.Fatal(err)
	}
	if p.complete() || p.display.enabled {
		t.Fatal("accepted a stale probe response")
	}
}

func TestTerminalImageProbeBoundsAndInterrupt(t *testing.T) {
	for _, input := range []string{"\x03", "\x1b_G" + strings.Repeat("x", maxImageResponseBytes)} {
		p := newTerminalImageProbe(false)
		if err := p.feed([]byte(input)); err == nil {
			t.Fatal("unbounded or interrupted probe accepted")
		}
	}
}
