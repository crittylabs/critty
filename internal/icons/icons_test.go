package icons

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestNerdGlyphWidth(t *testing.T) {
	for kind := range glyphs {
		glyph := Nerd.Glyph(Kind(kind), "fallback")
		if width := ansi.StringWidth(glyph); width != 2 {
			t.Errorf("icon %d reserves %d cells, want 2", kind, width)
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
