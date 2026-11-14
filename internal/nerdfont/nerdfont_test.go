package nerdfont

import (
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name, setting, env string
		want               bool
		reason             string
	}{
		{"force on", "on", "0", true, "nerd_fonts=on"},
		{"force off", "off", "1", false, "nerd_fonts=off"},
		{"auto environment on", "auto", "1", true, "NERDFONT=1"},
		{"default environment off", "", "0", false, "NERDFONT=0"},
		{"bundled symbols", "auto", "auto", true, "Ghostty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NERDFONT", tc.env)
			t.Setenv("KITTY_WINDOW_ID", "")
			t.Setenv("GHOSTTY_RESOURCES_DIR", "/resources")
			got := Detect(tc.setting)
			if got.Supported != tc.want || !strings.Contains(strings.ToLower(got.Reason), strings.ToLower(tc.reason)) {
				t.Fatalf("Detect(%q) = %+v", tc.setting, got)
			}
		})
	}
}
