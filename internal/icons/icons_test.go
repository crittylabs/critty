package icons

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestNerdGlyphWidth(t *testing.T) {
	for kind := range glyphs {
		glyph := Nerd.Glyph(Kind(kind), "fallback")
		want := 2
		switch Kind(kind) {
		case Close, AddComment, TabLeft, TabRight:
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

func TestNerdLabelStartsAfterTwoCells(t *testing.T) {
	for _, kind := range []Kind{Project, Comment, Close, Unchecked} {
		label := Nerd.Label(kind, "Name")
		if got := ansi.Cut(label, 2, 6); got != "Name" || ansi.StringWidth(label) != 6 {
			t.Errorf("icon %d label starts outside its two-cell field: %q", kind, label)
		}
	}
	if got := Plain.Label(Comment, "Name"); got != "Name" {
		t.Errorf("portable label changed: %q", got)
	}
	styled := "\x1b[31m" + Nerd.Glyph(Comment, "") + "\x1b[m"
	if got := ansi.Strip(ansi.Cut(Nerd.Join(styled, "Name"), 2, 6)); got != "Name" {
		t.Errorf("styled icon shifted the label: %q", got)
	}
}
