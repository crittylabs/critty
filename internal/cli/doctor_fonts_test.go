package cli

import (
	"strings"
	"testing"
)

func TestDoctorNerdFonts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("NERDFONT", "0")
	var r doctorReport
	r.checkNerdFonts()
	d := diagnosticFor(t, r, "fonts/nerd")
	if d.Status != "info" || !strings.Contains(d.Message, "off (nerd_fonts=auto): NERDFONT=0") || !strings.Contains(d.Advice, "selected font") {
		t.Fatalf("unexpected diagnostic: %+v", d)
	}
}
