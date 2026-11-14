package icons

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestNerdGlyphWidth(t *testing.T) {
	for kind := range glyphs {
		glyph := Nerd.Glyph(Kind(kind), "fallback")
		want := 2
		if Kind(kind) == Close || Kind(kind) == TabLeft || Kind(kind) == TabRight {
			want = 1
		}
		if width := ansi.StringWidth(glyph); width != want {
			t.Errorf("icon %d reserves %d cells, want %d", kind, width, want)
		}
	}
}

func TestGlyphFallback(t *testing.T) {
	for _, symbols := range []Set{Plain, Nerd} {
		if got := symbols.Glyph(Kind(len(glyphs)), "!"); got != "!" {
			t.Errorf("unknown icon fallback = %q, want !", got)
		}
	}
	if got := Plain.Glyph(Comment, "💬"); got != "💬" {
		t.Errorf("portable icon = %q, want unchanged fallback", got)
	}
}
