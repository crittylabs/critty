package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func toolLookup(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, installed := range names {
			if name == installed {
				return "/tools/" + name, nil
			}
		}
		return "", fmt.Errorf("%s not found", name)
	}
}

func TestSelectToolManager(t *testing.T) {
	for _, tc := range []struct {
		name, requested, want string
		installed             []string
		fail                  bool
	}{
		{"prefer mise", "auto", "mise", []string{"mise", "brew"}, false},
		{"brew fallback", "auto", "brew", []string{"brew"}, false},
		{"explicit brew", "brew", "brew", []string{"mise", "brew"}, false},
		{"missing explicit", "mise", "", []string{"brew"}, true},
		{"none installed", "auto", "", nil, true},
		{"invalid", "other", "", []string{"mise"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectToolManager(tc.requested, toolLookup(tc.installed...))
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("manager = %q, %v", got, err)
			}
		})
	}
}

func TestPreviewToolPlan(t *testing.T) {
	for _, tc := range []struct {
		name, manager, target, goos, goarch string
		installed                           []string
		ratex                               string
		want                                []string
		fail                                bool
	}{
		{"mise all shares Merman", "mise", "all", "linux", "amd64", nil, "", []string{"Merman", "RaTeX", "LaTeX and dvipng", "Update TinyTeX package manager", "AMS math and dvipng"}, false},
		{"brew all", "brew", "all", "darwin", "arm64", nil, "", []string{"Merman", "SVG converter", "Fontconfig", "LaTeX and dvipng"}, false},
		{"SVG already available", "mise", "svg", "darwin", "arm64", []string{"resvg"}, "", nil, false},
		{"existing Mermaid fallback", "mise", "mermaid", "linux", "arm64", []string{"mmdr"}, "", nil, false},
		{"existing math", "mise", "math", "darwin", "arm64", []string{"latex", "dvipng"}, "/tools/render", nil, false},
		{"partial TeX", "brew", "math", "linux", "amd64", []string{"latex"}, "", []string{"LaTeX and dvipng"}, false},
		{"unsupported TinyTeX", "mise", "all", "linux", "arm64", nil, "", nil, true},
		{"unsupported host with existing TeX", "mise", "math", "linux", "arm64", []string{"latex", "dvipng"}, "", []string{"RaTeX"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := previewToolPlan(tc.manager, tc.target, tc.goos, tc.goarch, toolLookup(tc.installed...), func(string) (string, error) { return tc.ratex, nil })
			if (err != nil) != tc.fail {
				t.Fatalf("plan error = %v", err)
			}
			var names []string
			for _, step := range plan {
				names = append(names, step.name)
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("steps = %v, want %v", names, tc.want)
			}
		})
	}

	plan, err := previewToolPlan("mise", "math", "darwin", "arm64", toolLookup(), func(string) (string, error) { return "", fmt.Errorf("unrelated render command") })
	if err != nil {
		t.Fatal(err)
	}
	if got := plan[len(plan)-1].args; !slices.Equal(got, []string{"exec", "tinytex@latest", "--", "tlmgr", "install", "amsmath", "amsfonts", "dvipng"}) {
		t.Fatalf("package setup argv = %q", got)
	}
}

func TestInstallToolsExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake manager uses a POSIX shell")
	}
	home, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(t.TempDir(), "manager.log")
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("TOOL_INSTALL_LOG", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" >> \"$TOOL_INSTALL_LOG\"\nexit \"${TOOL_INSTALL_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		fail bool
	}{
		{"dry run", []string{"svg", "--manager", "brew", "--dry-run"}, false},
		{"invalid target", []string{"unknown", "--manager", "brew"}, true},
		{"install", []string{"svg", "--manager", "brew"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newInstallToolsCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); (err != nil) != tc.fail {
				t.Fatalf("execute error = %v", err)
			}
			data, err := os.ReadFile(log)
			if tc.name != "install" {
				if !os.IsNotExist(err) {
					t.Fatalf("manager ran: %s, %v", data, err)
				}
			} else if err != nil || string(data) != home+"\ninstall\nlibrsvg\n" {
				t.Fatalf("manager log = %q, %v", data, err)
			}
		})
	}

	t.Setenv("TOOL_INSTALL_EXIT", "7")
	cmd := newInstallToolsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"all", "--manager", "brew"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "install Merman") {
		t.Fatalf("failure = %v", err)
	}
	data, err := os.ReadFile(log)
	if err != nil || strings.Contains(string(data), "texlive") || strings.Contains(string(data), "fontconfig") {
		t.Fatalf("continued after failure: %q, %v", data, err)
	}

	for _, name := range []string{"rsvg-convert", "merman-cli", "fc-match", "latex", "dvipng"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cmd = newInstallToolsCmd()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"all", "--manager", "brew"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(log)
	if err != nil || !bytes.Equal(data, unchanged) || !strings.Contains(out.String(), "already available") {
		t.Fatalf("existing tools were reinstalled: %q, %v; output %q", unchanged, err, out.String())
	}
}
