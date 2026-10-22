package cli

import (
	"encoding/base64"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

func TestDesktopLauncherOrder(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{nil, []string{"mux", "desktop-terminal"}},
		{[]string{"custom:review"}, []string{"custom:review", "mux", "desktop-terminal"}},
		{[]string{"mux", "custom:review"}, []string{"mux", "custom:review", "desktop-terminal"}},
		{[]string{"desktop-terminal", "custom:review"}, []string{"desktop-terminal", "custom:review", "mux"}},
	} {
		if got := defaultLauncherOrder(tc.input); !slices.Equal(got, tc.want) {
			t.Fatalf("order=%v want %v", got, tc.want)
		}
	}
	for _, tc := range []struct {
		platform, terminal, wsl string
		want                    []string
	}{
		{"darwin", "ghostty", "", []string{"ghostty-window", "terminal-app-window"}},
		{"darwin", "iterm", "", []string{"iterm-window", "terminal-app-window"}},
		{"darwin", "rio", "", []string{"rio-window", "terminal-app-window"}},
		{"darwin", "", "", []string{"terminal-app-window"}},
		{"linux", "", "", []string{"xdg-terminal"}},
		{"linux", "rio", "", []string{"rio-window", "xdg-terminal"}},
		{"linux", "rio", "Ubuntu", []string{"rio-window", "windows-terminal-wsl"}},
		{"linux", "windows-terminal", "Ubuntu", []string{"windows-terminal-wsl"}},
	} {
		got := desktopOrder(tc.platform, terminalbinding.Origin{Terminal: tc.terminal, WSLDistribution: tc.wsl})
		if !slices.Equal(got, tc.want) {
			t.Fatalf("desktop=%v want %v", got, tc.want)
		}
	}
}

func TestEveryDesktopLauncherRejectsKnownSSH(t *testing.T) {
	previous, request := reviewOrigin, terminalRequest
	reviewOrigin = terminalbinding.Origin{SSH: true}
	terminalRequest = "bound"
	t.Cleanup(func() { reviewOrigin, terminalRequest = previous, request })
	for _, name := range append(slices.Clone(desktopNames), "desktop-terminal") {
		candidates, err := desktopCandidates(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range candidates {
			if err := candidate.check(); err == nil {
				t.Fatalf("%s accepted SSH origin", candidate.name)
			}
		}
	}
}

func TestDesktopScriptsKeepCommandsOutOfWindowsSeparators(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	launcherScript(t, filepath.Join(dir, "wt.exe"), `printf '%s\000' "$@"`)
	command := "/bin/sh -c " + shellEscape("cd '/path with spaces;日本語' && exec '/binary name' _tui --session '0123456789ab'")
	encoded := base64.StdEncoding.EncodeToString([]byte(command))
	backend := desktopLaunch{name: "windows-terminal-wsl", origin: terminalbinding.Origin{WSLDistribution: "Ubuntu"}}
	output, err := backend.call("open", encoded, "/path with spaces;日本語")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	for _, arg := range args {
		if strings.Contains(arg, ";") {
			t.Fatalf("shell separator reached Windows Terminal: %q", arg)
		}
	}
	if args[len(args)-1] != encoded {
		t.Fatalf("command payload changed: %q", args)
	}
	decoded, err := base64.StdEncoding.DecodeString(args[len(args)-1])
	if err != nil || string(decoded) != command {
		t.Fatalf("command changed: %s,%v", decoded, err)
	}
}

func TestDesktopCleanupFailureDoesNotFailReview(t *testing.T) {
	// An unavailable script fails before invoking a GUI application. Cleanup is
	// best effort even if the application or its scripting interface has gone.
	launch := desktopLaunch{name: "unavailable-terminal"}
	if err := launch.close(); err != nil {
		t.Fatalf("cleanup invalidated the finished review: %v", err)
	}
}

func TestGhosttyRequiresWindowActivation(t *testing.T) {
	for _, tc := range []struct {
		commands string
		want     bool
	}{
		{`<command name="new window"/>`, false},
		{`<command name="new window"/><command name="activate window"/>`, true},
	} {
		dictionary := []byte(`<dictionary><suite>` + tc.commands + `<record-type name="surface configuration"/></suite></dictionary>`)
		if got := ghosttyScriptingAvailable(dictionary); got != tc.want {
			t.Fatalf("window activation support=%v, want %v", got, tc.want)
		}
	}
}

func TestRioCommandPreservesArguments(t *testing.T) {
	originalLook := lookPath
	t.Cleanup(func() { lookPath = originalLook })
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	value := "path with spaces;日本語 and 'quotes' $(not-a-command)"
	command := "printf '%s' " + shellEscape(value)
	cwd := "/path with spaces;日本語"
	for _, distribution := range []string{"", "Ubuntu"} {
		t.Run("wsl="+distribution, func(t *testing.T) {
			launch := desktopLaunch{name: "rio-window", origin: terminalbinding.Origin{WSLDistribution: distribution}}
			cmd, err := launch.rioCommand(command, cwd)
			if err != nil {
				t.Fatal(err)
			}
			if distribution == "" {
				want := []string{"/bin/rio", "--working-dir", cwd, "--title-placeholder", "Critty", "-e", "/bin/sh", "-c", command}
				if !slices.Equal(cmd.Args, want) {
					t.Fatalf("args = %q", cmd.Args)
				}
			} else {
				if cmd.Path != "/bin/rio.exe" || !slices.Contains(cmd.Args, distribution) || slices.Contains(cmd.Args, cwd) {
					t.Fatalf("WSL args = %q", cmd.Args)
				}
				payload := cmd.Args[len(cmd.Args)-1]
				decoded, err := base64.StdEncoding.DecodeString(payload)
				if err != nil || string(decoded) != command {
					t.Fatalf("payload = %q, %v", decoded, err)
				}
				shellIndex := slices.Index(cmd.Args, "-c")
				out, err := exec.Command("/bin/sh", "-c", cmd.Args[shellIndex+1], "critty", payload).Output()
				if err != nil || string(out) != value {
					t.Fatalf("nested command = %q, %v", out, err)
				}
			}
			if err := launch.close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
