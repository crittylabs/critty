package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/tui"
	"github.com/spf13/cobra"
)

func TestDoctorTextColorsAndPlainJSON(t *testing.T) {
	r := doctorReport{}
	for _, status := range []string{"ok", "error", "info", "warn"} {
		r.add("test", status, "message", "")
	}
	var colored, plain, encoded bytes.Buffer
	if err := r.writeText(&colored, true); err != nil {
		t.Fatal(err)
	}
	if err := r.writeText(&plain, false); err != nil {
		t.Fatal(err)
	}
	if ansi.Strip(colored.String()) != plain.String() || !strings.Contains(colored.String(), "\x1b[32m") || !strings.Contains(colored.String(), "\x1b[31m") || !strings.Contains(colored.String(), "\x1b[33m") {
		t.Fatalf("wrong colors: %q", colored.String())
	}
	t.Setenv("NO_COLOR", "1")
	var disabled bytes.Buffer
	if err := r.write(&disabled, false); err != nil || disabled.String() != plain.String() {
		t.Fatalf("plain output changed: %q, %v", disabled.String(), err)
	}
	if err := r.write(&encoded, true); err != nil || !json.Valid(encoded.Bytes()) || strings.Contains(encoded.String(), "\x1b") {
		t.Fatalf("JSON contains formatting: %q, %v", encoded.String(), err)
	}
}

func TestDoctorMCPInstallAdvice(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	python := "/review/venv with 'quote'/python"
	requirements := []byte("mcp==2.3.0\n")
	if got := mcpInstallAdvice(python, "2.3.0", requirements); got != "Install into this environment: "+shellEscape(python)+" -m pip install 'mcp==2.3.0'" {
		t.Fatalf("fallback command = %q", got)
	}
	path := filepath.Join(".agents", "skills", "critty-ai-review", "scripts", "codex-review-requirements.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, requirements, 0600); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "Install into this environment: " + shellEscape(python) + " -m pip install -r " + shellEscape(abs)
	if got := mcpInstallAdvice(python, "2.3.0", requirements); got != want {
		t.Fatalf("requirements command = %q, want %q", got, want)
	}
	if err := os.WriteFile(path, []byte("mcp==old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := mcpInstallAdvice(python, "2.3.0", requirements); strings.Contains(got, " -r ") {
		t.Fatalf("recommended stale requirements: %q", got)
	}
}

func diagnosticFor(t *testing.T, r doctorReport, name string) diagnostic {
	t.Helper()
	for _, d := range r.Checks {
		if d.Check == name {
			return d
		}
	}
	t.Fatalf("missing diagnostic %s: %+v", name, r)
	return diagnostic{}
}

func TestDoctorIntegrations(t *testing.T) {
	for _, mode := range []string{"absent", "current", "missing", "stale", "unreadable", "global"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			project, global := filepath.Join(root, "project"), filepath.Join(root, "global")
			files := []integrationFile{}
			for _, name := range []string{"skill", "resource"} {
				files = append(files, integrationFile{dest: filepath.Join(project, name), globalDest: filepath.Join(global, name), content: func() ([]byte, error) { return []byte("current"), nil }})
			}
			dir := project
			if mode == "global" {
				dir = global
			}
			if mode != "absent" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "skill"), []byte("current"), 0600); err != nil {
					t.Fatal(err)
				}
				if mode != "missing" {
					data := "current"
					if mode == "stale" {
						data = "custom"
					}
					if mode == "unreadable" {
						if err := os.Mkdir(filepath.Join(dir, "resource"), 0700); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(filepath.Join(dir, "resource"), []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			r := doctorReport{}
			r.checkIntegrations(map[string][]integrationFile{"codex": files, "prompts": files})
			errors := 0
			for _, d := range r.Checks {
				if d.Status == "error" {
					errors++
				}
			}
			want := 0
			if mode == "missing" || mode == "stale" || mode == "unreadable" {
				want = 1
			}
			if errors != want {
				t.Fatalf("errors = %d, want %d: %+v", errors, want, r)
			}
			if mode == "global" && len(r.Checks) != 2 {
				t.Fatalf("global files omitted: %+v", r)
			}
		})
	}
}

func TestDoctorPythonRequirements(t *testing.T) {
	oldOutput, oldLook := commandOutput, lookPath
	t.Cleanup(func() { commandOutput, lookPath = oldOutput, oldLook })
	lookPath = func(name string) (string, error) { return name, nil }
	for _, tt := range []struct {
		minor                 int
		mcp                   string
		sdk                   bool
		claude, codex, status string
	}{
		{9, "unavailable", false, "warn", "warn", "warn"},
		{10, "unavailable", false, "ok", "warn", "warn"},
		{11, "2.3.0", true, "ok", "ok", "ok"},
		{15, "2.2.0", true, "ok", "ok", "warn"},
		{15, "2.3.0", false, "ok", "ok", "warn"},
	} {
		commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
			if cmd.Path != "/review/venv/python" || len(cmd.Args) != 4 || cmd.Args[1] != "-B" || cmd.Stdin != nil {
				t.Fatalf("unexpected probe: %+v", cmd)
			}
			return []byte(fmt.Sprintf(`{"major":3,"minor":%d,"mcp":%q,"sdk":%t}`, tt.minor, tt.mcp, tt.sdk)), nil
		}
		r := doctorReport{}
		r.checkAI("/review/venv/python")
		for name, want := range map[string]string{"ai/claude-python": tt.claude, "ai/codex-python": tt.codex, "ai/mcp": tt.status} {
			if got := diagnosticFor(t, r, name).Status; got != want {
				t.Fatalf("%s status = %s, want %s", name, got, want)
			}
		}
	}
}

func TestDoctorPythonProbeRuns(t *testing.T) {
	bin, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	oldLook := lookPath
	t.Cleanup(func() { lookPath = oldLook })
	lookPath = func(name string) (string, error) {
		if name == "python3" {
			return bin, nil
		}
		return "", exec.ErrNotFound
	}
	r := doctorReport{}
	r.checkAI("python3")
	if d := diagnosticFor(t, r, "ai/claude-python"); !strings.Contains(d.Message, "Python 3.") {
		t.Fatalf("probe failed: %+v", d)
	}
}

func TestDoctorTmuxInheritedSetting(t *testing.T) {
	stubDoctorTerminal(t)
	oldOutput, oldLook := commandOutput, lookPath
	t.Cleanup(func() { commandOutput, lookPath = oldOutput, oldLook })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Setenv("TMUX", "/tmp/socket,123,0")
	t.Setenv("TMUX_PANE", "%9")
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
	for _, value := range []string{"on", "all", "off", "unexpected"} {
		commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
			want := "tmux show-options -A -p -v -t %9 allow-passthrough"
			if got := strings.Join(cmd.Args, " "); got != want {
				t.Fatalf("command = %s", got)
			}
			return []byte(value + "\n"), nil
		}
		r := doctorReport{}
		r.checkImages()
		want := "warn"
		if value == "on" || value == "all" {
			want = "ok"
		}
		if d := diagnosticFor(t, r, "images/tmux"); d.Status != want {
			t.Fatalf("wrong diagnostic: %+v", d)
		}
	}
}

func TestDoctorHerdrConfig(t *testing.T) {
	for _, tt := range []struct{ config, status, message string }{
		{"", "info", "unset"},
		{"[experimental]\nkitty_graphics = false\n", "warn", "false"},
		{"[terminal]\nkitty_graphics = true\n[experimental]\nkitty_graphics = false\n", "info", "true"},
		{"terminal.kitty_graphics = false\n", "warn", "false"},
		{"[terminal]\nkitty_graphics = 'false'\n", "warn", "invalid"},
		{"[other]\ntext = '''\n[terminal]\nkitty_graphics = false\n'''\n", "info", "unset"},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		t.Setenv("HERDR_CONFIG_PATH", path)
		if err := os.WriteFile(path, []byte(tt.config), 0600); err != nil {
			t.Fatal(err)
		}
		r := doctorReport{}
		r.checkHerdrImages()
		d := diagnosticFor(t, r, "images/herdr")
		if d.Status != tt.status || !strings.Contains(d.Message, tt.message) {
			t.Fatalf("config %q: %+v", tt.config, d)
		}
	}
}

func TestDoctorAliasAndJSON(t *testing.T) {
	stubDoctorTerminal(t)
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
	oldLook := lookPath
	t.Cleanup(func() { lookPath = oldLook })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	for _, name := range []string{"doctor", "check"} {
		root := &cobra.Command{Use: "critty"}
		root.AddCommand(newDoctorCommand())
		root.SetArgs([]string{name, "--json"})
		var out bytes.Buffer
		root.SetOut(&out)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		var r doctorReport
		if err := json.Unmarshal(out.Bytes(), &r); err != nil || len(r.Checks) == 0 {
			t.Fatalf("invalid report %s: %v", out.String(), err)
		}
	}
	first := integrations()["codex"][0]
	if err := os.MkdirAll(filepath.Dir(first.dest), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := first.content()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.dest, data, 0600); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "critty", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newDoctorCommand())
	root.SetArgs([]string{"check", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("partial installation returned success")
	}
	var r doctorReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("error lost JSON report: %v", err)
	}
	if len(r.Checks) == 0 || r.Checks[1].Status != "error" {
		t.Fatalf("missing resources not reported: %+v", r)
	}
}

func stubDoctorTerminal(t *testing.T) {
	t.Helper()
	old := checkTerminalImages
	t.Cleanup(func() { checkTerminalImages = old })
	checkTerminalImages = func() (tui.ImageCapability, error) {
		return tui.ImageCapability{Reason: "no current TTY available"}, nil
	}
}

func TestDoctorTerminalImageStatus(t *testing.T) {
	stubDoctorTerminal(t)
	oldLook := lookPath
	t.Cleanup(func() { lookPath = oldLook })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	for _, name := range []string{"TMUX", "HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(name, "")
	}
	for _, tt := range []struct {
		result tui.ImageCapability
		err    error
		status string
	}{
		{tui.ImageCapability{Reason: "no current TTY available"}, nil, "info"},
		{tui.ImageCapability{Checked: true, Supported: true, Reason: "accepted"}, nil, "ok"},
		{tui.ImageCapability{Checked: true, Reason: "unconfirmed"}, nil, "warn"},
		{tui.ImageCapability{Checked: true}, fmt.Errorf("read failed"), "warn"},
	} {
		checkTerminalImages = func() (tui.ImageCapability, error) { return tt.result, tt.err }
		r := doctorReport{}
		r.checkImages()
		d := diagnosticFor(t, r, "images/terminal")
		if d.Status != tt.status {
			t.Fatalf("wrong status: %+v", d)
		}
		if !tt.result.Checked && !strings.HasPrefix(d.Message, "skipped:") {
			t.Fatalf("skip not reported: %+v", d)
		}
	}
}

func TestDoctorDeduplicatesHomeInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(home)
	if err := os.WriteFile("skill", []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	f := integrationFile{dest: "skill", globalDest: "skill", content: func() ([]byte, error) { return []byte("current"), nil }}
	r := doctorReport{}
	r.checkIntegrations(map[string][]integrationFile{"codex": {f}})
	if len(r.Checks) != 1 {
		t.Fatalf("home install checked twice: %+v", r)
	}
}
