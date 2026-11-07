package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/crittylabs/critty/internal/config"
)

func TestDoctorMermaid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	oldFind, oldLook := findMermaid, lookPath
	t.Cleanup(func() { findMermaid, lookPath = oldFind, oldLook })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	for _, tc := range []struct {
		name, cfg         string
		available         bool
		primary, fallback string
	}{
		{"missing", "", false, "info", "info"},
		{"auto", "", true, "ok", "ok"},
		{"off", "mermaid_merman='off'\nmermaid_mmdr='off'", true, "info", "info"},
		{"missing override", "mermaid_merman='wrapper'", false, "warn", "info"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(config.GlobalPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.GlobalPath(), []byte(tc.cfg), 0600); err != nil {
				t.Fatal(err)
			}
			findMermaid = func(setting, name string) (string, error) {
				if tc.available {
					return "/tools/" + name, nil
				}
				if setting != "" {
					return "", exec.ErrNotFound
				}
				return "", nil
			}
			var r doctorReport
			r.checkMermaid()
			if d := diagnosticFor(t, r, "mermaid/merman"); d.Status != tc.primary {
				t.Fatal(d)
			}
			if d := diagnosticFor(t, r, "mermaid/mmdr"); d.Status != tc.fallback {
				t.Fatal(d)
			}
			if d := diagnosticFor(t, r, "mermaid/fonts"); d.Status != "info" {
				t.Fatal(d)
			}
		})
	}
}
