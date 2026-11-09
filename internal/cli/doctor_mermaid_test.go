package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
		tools             []string
		primary, fallback string
		install           bool
	}{
		{"missing", "", nil, "info", "info", true},
		{"auto", "", []string{"merman-cli", "mmdr"}, "ok", "ok", false},
		{"fallback available", "", []string{"mmdr"}, "info", "ok", false},
		{"off", "mermaid_merman='off'\nmermaid_mmdr='off'", nil, "info", "info", false},
		{"missing override", "mermaid_merman='wrapper'", nil, "warn", "info", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(config.GlobalPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.GlobalPath(), []byte(tc.cfg), 0600); err != nil {
				t.Fatal(err)
			}
			findMermaid = func(setting, name string) (string, error) {
				if slices.Contains(tc.tools, name) {
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
			} else if strings.Contains(d.Advice, "critty install tools mermaid") != tc.install {
				t.Fatalf("unexpected setup advice: %+v", d)
			}
			if d := diagnosticFor(t, r, "mermaid/mmdr"); d.Status != tc.fallback {
				t.Fatal(d)
			} else if strings.Contains(d.Advice, "critty install") {
				t.Fatalf("installer does not provide mmdr: %+v", d)
			}
			if d := diagnosticFor(t, r, "mermaid/fonts"); d.Status != "info" {
				t.Fatal(d)
			} else if !strings.Contains(d.Advice, "critty install tools mermaid --manager brew") {
				t.Fatalf("Fontconfig advice must select Homebrew: %+v", d)
			}
		})
	}
}
