package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func launcherScript(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLauncherSelection(t *testing.T) {
	originalPath, originalRequest := lookPath, terminalRequest
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	terminalRequest = "marker-bound"
	t.Cleanup(func() { lookPath, terminalRequest = originalPath, originalRequest })
	for _, tc := range []struct {
		name     string
		terminal reviewMultiplexerContext
		want     string
	}{
		{"mux", tmuxContext{pane: "%1"}, "tmux"},
		{"mux", herdrContext{pane: "p1"}, "herdr"},
		{"herdr", tmuxContext{pane: "%1"}, "tmux"},
		{"tmux", herdrContext{pane: "p1"}, "herdr"},
		{"custom:/missing/launcher", tmuxContext{pane: "%1"}, "tmux"},
		{"mux", nil, ""},
	} {
		t.Run(tc.name+tc.want, func(t *testing.T) {
			got, err := selectReviewLauncher(tc.name, tc.terminal, &reviewMode{sessionKey: "0123456789ab"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected inline review, got %s", got.name)
				}
			} else if got == nil || got.name != tc.want {
				t.Fatalf("launcher = %+v, want %s", got, tc.want)
			}
		})
	}
	for _, name := range []string{"unknown", "custom:"} {
		if _, err := selectReviewLauncher(name, nil, &reviewMode{}); err == nil {
			t.Fatalf("accepted invalid launcher %q", name)
		}
	}
}

func TestCustomLauncherPathSearch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	launcherScript(t, "local-launcher", "exit 0\n")
	for _, tc := range []struct {
		file, path string
		available  bool
	}{
		{"local-launcher", "/usr/bin:/bin", false},
		{"local-launcher", ".:/usr/bin:/bin", true},
		{"local-launcher", ":/usr/bin:/bin", true},
		{"./local-launcher", "/usr/bin:/bin", true},
		{filepath.Join(dir, "local-launcher"), "/usr/bin:/bin", true},
	} {
		t.Run(tc.file+tc.path, func(t *testing.T) {
			t.Setenv("PATH", tc.path)
			launcher, err := newCustomLauncher(tc.file, nil, &reviewMode{sessionKey: "0123456789ab"})
			if err != nil {
				t.Fatal(err)
			}
			if err := launcher.check(); (err == nil) != tc.available {
				t.Fatalf("availability = %v, want %t", err, tc.available)
			}
		})
	}
}

func TestCustomLauncherRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LAUNCHER_TEST_DIR", dir)
	t.Setenv("PATH", "/usr/bin:/bin")
	file := launcherScript(t, filepath.Join(dir, "launcher with 'quotes' $literal"), `
case "$1" in
check) cat > "$LAUNCHER_TEST_DIR/check" ;;
open) cat > "$LAUNCHER_TEST_DIR/open"; printf 'pane\000\n' ;;
close) cat > "$LAUNCHER_TEST_DIR/close" ;;
esac
`)
	launcher, err := newCustomLauncher(file, nil, &reviewMode{sessionKey: "0123456789ab"})
	if err != nil {
		t.Fatal(err)
	}
	if err := launcher.check(); err != nil {
		t.Fatal(err)
	}
	launch, err := launcher.open(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := launch.close(); err != nil {
		t.Fatal(err)
	}
	check, _ := os.ReadFile(filepath.Join(dir, "check"))
	open, _ := os.ReadFile(filepath.Join(dir, "open"))
	if !bytes.Equal(check, open) {
		t.Fatal("check and open received different requests")
	}
	var request launcherRequest
	if err := json.Unmarshal(open, &request); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if request.Session != "0123456789ab" || request.Cwd != cwd || !strings.Contains(request.Command, "_tui --session '0123456789ab'") {
		t.Fatalf("incorrect request: %+v", request)
	}
	token, _ := os.ReadFile(filepath.Join(dir, "close"))
	if !bytes.Equal(token, []byte("pane\x00\n")) {
		t.Fatalf("close token changed: %q", token)
	}
}

func TestCustomLauncherFailureStages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/bin")
	for _, operation := range []string{"check", "open", "close"} {
		t.Run(operation, func(t *testing.T) {
			file := launcherScript(t, filepath.Join(dir, operation), "cat > /dev/null\n[ \"$1\" != "+shellEscape(operation)+" ]\n")
			launcher, err := newCustomLauncher(file, nil, &reviewMode{sessionKey: "0123456789ab"})
			if err != nil {
				t.Fatal(err)
			}
			err = launcher.check()
			if operation == "check" {
				if err == nil {
					t.Fatal("check failure lost")
				}
				got, err := selectReviewLauncher("custom:"+file, nil, &reviewMode{sessionKey: "0123456789ab"})
				if err != nil || got != nil {
					t.Fatalf("check did not fall back to inline mux: %v %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			launch, err := launcher.open(nil)
			if operation == "open" {
				if err == nil || launch != nil {
					t.Fatal("open failure lost")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := launch.close(); err == nil {
				t.Fatal("close failure lost")
			}
		})
	}
}

func TestLauncherEnvironmentUsesDetectedContext(t *testing.T) {
	t.Setenv("TMUX", "stale")
	t.Setenv("TMUX_PANE", "%99")
	t.Setenv("HERDR_TAB_ID", "stale")
	t.Setenv("TTY", "stale")
	got := launcherEnvironment([]envVar{{"TMUX", "socket,100,1"}, {"TMUX_PANE", "%1"}, {"TTY", "/dev/tty1"}})
	for _, want := range []string{"TMUX=socket,100,1", "TMUX_PANE=%1", "TTY=/dev/tty1"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, value := range got {
		if strings.Contains(value, "=stale") || strings.HasPrefix(value, "HERDR_") {
			t.Errorf("stale terminal environment: %s", value)
		}
	}
}
