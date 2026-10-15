package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLauncherOrderAndFinalError(t *testing.T) {
	dir := t.TempDir()
	usable := "custom:" + launcherScript(t, filepath.Join(dir, "usable,launcher"), "echo checking >&2\nexit 0\n")
	failed := "custom:" + launcherScript(t, filepath.Join(dir, "failed"), "echo custom unavailable >&2\nexit 1\n")
	originalPath, originalRequest, originalStderr := lookPath, terminalRequest, os.Stderr
	lookPath = func(name string) (string, error) { return "", fmt.Errorf("%s executable missing", name) }
	terminalRequest = "marker-bound"
	stderr, err := os.CreateTemp(dir, "stderr")
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = stderr
	t.Cleanup(func() {
		lookPath, terminalRequest, os.Stderr = originalPath, originalRequest, originalStderr
		if err := stderr.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, tc := range []struct {
		name     string
		order    []string
		terminal reviewMultiplexerContext
		want     string
		error    string
	}{
		{"mux then custom", []string{"mux", usable}, nil, usable, ""},
		{"custom then mux", []string{usable, "mux"}, nil, usable, ""},
		{"failed then usable", []string{failed, usable}, nil, usable, ""},
		{"custom is final", []string{"mux", failed}, nil, "", "custom unavailable"},
		{"mux is final", []string{failed, "mux"}, tmuxContext{pane: "%1"}, "", "no usable review launcher"},
		{"implicit mux", []string{failed}, tmuxContext{pane: "%1"}, "", "no usable review launcher"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectReviewLauncher(tc.order, tc.terminal, &reviewMode{sessionKey: "0123456789ab"})
			if tc.error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.error) || got != nil {
					t.Fatalf("selection = %+v, %v; want %q", got, err, tc.error)
				}
				if !strings.Contains(err.Error(), "launcher mux:") && tc.terminal == nil {
					t.Fatalf("missing multiplexer detection failure: %v", err)
				}
				return
			}
			if err != nil || got == nil || got.name != tc.want {
				t.Fatalf("selection = %+v, %v; want %q", got, err, tc.want)
			}
		})
	}
	output, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 0 {
		t.Fatalf("availability checks wrote to stderr: %s", output)
	}
}

func launcherScript(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLauncherSelection(t *testing.T) {
	originalPath, originalRequest, originalOutput := lookPath, terminalRequest, commandOutput
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	terminalRequest = "marker-bound"
	commandOutput = func(*exec.Cmd) ([]byte, error) { return []byte("new-pane\t[-AdP] [-B border-lines] [-T title]\n"), nil }
	t.Cleanup(func() { lookPath, terminalRequest, commandOutput = originalPath, originalRequest, originalOutput })
	for _, tc := range []struct {
		name     string
		terminal reviewMultiplexerContext
		want     string
	}{
		{"mux", tmuxContext{pane: "%1"}, "tmux-floating"},
		{"mux", herdrContext{pane: "p1"}, "herdr"},
		{"herdr", tmuxContext{pane: "%1"}, "tmux-floating"},
		{"tmux", herdrContext{pane: "p1"}, "herdr"},
		{"custom:/missing/launcher", tmuxContext{pane: "%1"}, "tmux-floating"},
		{"tmux-split", tmuxContext{pane: "%1"}, "tmux-split"},
		{"tmux", tmuxContext{pane: "%1"}, "tmux-floating"},
		{"mux", nil, ""},
	} {
		t.Run(tc.name+tc.want, func(t *testing.T) {
			got, err := selectReviewLauncher([]string{tc.name}, tc.terminal, &reviewMode{sessionKey: "0123456789ab"})
			if tc.want == "" {
				if !errors.Is(err, errNoReviewLauncher) {
					t.Fatalf("missing unavailability error: %v", err)
				}
				if got != nil {
					t.Fatalf("expected inline review, got %s", got.name)
				}
			} else if err != nil || got == nil || got.name != tc.want {
				t.Fatalf("launcher = %+v, want %s", got, tc.want)
			}
		})
	}
	for _, name := range []string{"unknown", "custom:"} {
		if _, err := selectReviewLauncher([]string{name}, nil, &reviewMode{}); err == nil {
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

func TestTMUXLauncherFallback(t *testing.T) {
	for _, name := range []string{"mux", "tmux", "tmux-floating", "tmux-split"} {
		t.Run(name, func(t *testing.T) {
			calls := mockTMUXLaunch(t, "")
			original := commandOutput
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if slices.Contains(cmd.Args, "list-commands") {
					return []byte("new-pane\t[-dP] [-x width] [-y height]\n"), nil
				}
				return original(cmd)
			}
			launcher, err := selectReviewLauncher([]string{name}, tmuxContext{pane: "%7"}, &reviewMode{sessionKey: "0123456789ab"})
			if err != nil || launcher == nil || launcher.name != "tmux-split" {
				t.Fatalf("old server selection = %+v, %v", launcher, err)
			}
			if _, err := launcher.open(&reviewMode{sessionKey: "0123456789ab"}); err != nil {
				t.Fatal(err)
			}
			if tmuxCall(*calls, "new-pane") != nil || tmuxCall(*calls, "split-window") == nil {
				t.Fatalf("did not use split: %v", *calls)
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
				got, err := selectReviewLauncher([]string{"custom:" + file}, nil, &reviewMode{sessionKey: "0123456789ab"})
				if !errors.Is(err, errNoReviewLauncher) || got != nil {
					t.Fatalf("check failure lost: %v %v", got, err)
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
