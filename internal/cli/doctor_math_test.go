package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/crittylabs/critty/internal/config"
)

func TestDoctorMathTools(t *testing.T) {
	for _, tc := range []struct {
		name, settings string
		tools          map[string]bool
		want           [3]string
	}{
		{"absent", "", nil, [3]string{"info", "info", "info"}},
		{"latex only", "", map[string]bool{"latex": true, "dvipng": true}, [3]string{"info", "ok", "ok"}},
		{"partial latex", "", map[string]bool{"latex": true}, [3]string{"info", "ok", "info"}},
		{"auto ratex", "", map[string]bool{"render": true}, [3]string{"ok", "info", "info"}},
		{"explicit auto", "math_ratex = 'auto'\n", map[string]bool{"render": true}, [3]string{"ok", "info", "info"}},
		{"off", "math_ratex = 'off'\n", map[string]bool{"render": true, "latex": true, "dvipng": true}, [3]string{"info", "ok", "ok"}},
		{"command name", "math_ratex = 'render'\n", map[string]bool{"render": true}, [3]string{"ok", "info", "info"}},
		{"configured ratex", "math_ratex = '/tools/render'\n", map[string]bool{"/tools/render": true}, [3]string{"ok", "info", "info"}},
		{"unavailable ratex", "math_ratex = '/tools/missing'\n", map[string]bool{"latex": true, "dvipng": true}, [3]string{"warn", "ok", "ok"}},
		{"invalid setting", "math_ratex = './render'\n", nil, [3]string{"warn", "info", "info"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			if err := os.MkdirAll(filepath.Dir(config.GlobalPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.GlobalPath(), []byte(tc.settings), 0600); err != nil {
				t.Fatal(err)
			}
			old := lookPath
			oldFind := findMathRaTeX
			t.Cleanup(func() { lookPath, findMathRaTeX = old, oldFind })
			lookPath = func(name string) (string, error) {
				if tc.tools[name] {
					return filepath.Join("/tools", filepath.Base(name)), nil
				}
				return "", exec.ErrNotFound
			}
			findMathRaTeX = func(setting string) (string, error) {
				auto := setting == "" || setting == "auto"
				if auto {
					setting = "render"
				}
				path, err := lookPath(setting)
				if auto && err != nil {
					return "", nil
				}
				return path, err
			}
			r := doctorReport{}
			r.checkMath()
			for i, name := range []string{"ratex", "latex", "dvipng"} {
				if d := diagnosticFor(t, r, "math/"+name); d.Status != tc.want[i] {
					t.Fatalf("%s: %+v", name, d)
				}
			}
		})
	}
}
