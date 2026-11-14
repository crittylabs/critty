package config

import "testing"

func TestNerdFontsConfig(t *testing.T) {
	for _, tc := range []struct {
		name, global, project, want string
	}{
		{"default", "", "", "auto"},
		{"inherit", `nerd_fonts = "on"`, "", "on"},
		{"disable", `nerd_fonts = "on"`, `nerd_fonts = "off"`, "off"},
		{"enable", `nerd_fonts = "off"`, `nerd_fonts = "on"`, "on"},
		{"restore auto", `nerd_fonts = "off"`, `nerd_fonts = "auto"`, "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			project := t.TempDir()
			writeConfig(t, GlobalPath(), tc.global)
			writeConfig(t, ProjectPath(project), tc.project)
			cfg, err := Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.NerdFonts != tc.want {
				t.Fatalf("NerdFonts = %q, want %q", cfg.NerdFonts, tc.want)
			}
		})
	}
	for _, value := range []string{`""`, `"enabled"`, `true`, `1`} {
		t.Run("invalid "+value, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			writeConfig(t, GlobalPath(), "nerd_fonts = "+value)
			if _, err := Load(""); err == nil {
				t.Fatalf("invalid setting error = %v", err)
			}
		})
	}
}
