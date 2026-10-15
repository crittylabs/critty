//go:build darwin || linux

package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAcceptBackgroundReply(t *testing.T) {
	for _, tt := range []struct {
		name  string
		reply string
		dark  bool
	}{
		{"light", "\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?62;c", false},
		{"dark", "\x1b]11;rgb:1e1e/1e1e/1e1e\x07\x1b[?62;c", true},
		{"unsupported", "\x1b[?62;c", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parser := ansi.GetParser()
			defer ansi.PutParser(parser)
			dark, done := true, false
			var pending []byte
			var state byte
			data := []byte(tt.reply)
			for len(data) > 0 && !done {
				seq, _, read, next := ansi.DecodeSequence(data, state, parser)
				pending = append(pending, seq...)
				if next == ansi.NormalState {
					done = acceptBackgroundReply(string(pending), parser, &dark)
					pending = pending[:0]
				}
				state = next
				data = data[read:]
			}
			if !done || dark != tt.dark {
				t.Fatalf("done = %t, dark = %t, want done with dark = %t", done, dark, tt.dark)
			}
		})
	}
}
