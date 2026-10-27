package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

func TestReviewLauncherFallsBackAfterTerminalRequestFailure(t *testing.T) {
	for _, tc := range []struct {
		name, request, ssh, want string
		custom                   bool
	}{
		{name: "missing request", request: "missing", want: "rio-window"},
		{name: "ambiguous marker", request: "ambiguous", want: "rio-window"},
		{name: "marker timeout", request: "unmatched", custom: true},
		{name: "SSH custom fallback", request: "missing", ssh: "remote", custom: true},
		{name: "SSH rejects desktop", request: "missing", ssh: "remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.request == "unmatched" {
				// Storage tests cover marker polling; this checks fallback on expiry.
				previousTimeout := terminalRequestTimeout
				terminalRequestTimeout = 20 * time.Millisecond
				t.Cleanup(func() { terminalRequestTimeout = previousTimeout })
			}
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			previousStderr := os.Stderr
			os.Stderr = stderr
			t.Cleanup(func() {
				os.Stderr = previousStderr
				if err := stderr.Close(); err != nil {
					t.Error(err)
				}
			})
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("TMUX", "/unrelated/socket,123,0")
			t.Setenv("TMUX_PANE", "%99")
			for _, key := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY", "WSL_DISTRO_NAME"} {
				t.Setenv(key, "")
			}
			t.Setenv("SSH_CONNECTION", tc.ssh)
			t.Setenv("TERM_PROGRAM", "rio")
			t.Setenv("TERMINAL_APP", "rio")
			previous, origin, path := terminalRequest, reviewOrigin, lookPath
			t.Cleanup(func() { terminalRequest, reviewOrigin, lookPath = previous, origin, path })
			terminalRequest = strings.Repeat("0", 32)
			lookPath = func(name string) (string, error) {
				if name == "rio" || name == "tmux" {
					return "/bin/" + name, nil
				}
				return "", fmt.Errorf("%s unavailable", name)
			}
			if tc.request != "missing" {
				store := terminalbinding.DefaultStore()
				request, err := store.Prepare()
				if err != nil {
					t.Fatal(err)
				}
				terminalRequest = request.ID
				if tc.request == "ambiguous" {
					for _, pane := range []string{"%1", "%2"} {
						if err := store.Notify(request.ID, terminalbinding.Target{Kind: "tmux", Socket: "/recorded/socket", Pane: pane}); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			names := []string{"mux", "tmux-split", "rio-window"}
			want := tc.want
			if tc.custom {
				want = "custom:" + launcherScript(t, filepath.Join(t.TempDir(), "launcher"), "exit 0\n")
				names = append(names, want)
				// Force the local timeout case past the desktop candidate too.
				if tc.ssh == "" {
					names = []string{"mux", "tmux-split", want}
				}
			}
			mode := &reviewMode{sessionKey: "0123456789ab", launch: &launchSummary{Launcher: "previous-round"}}
			got, foreground, err := resolveReviewLauncher(names, mode)
			output, readErr := os.ReadFile(stderr.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(output) != 0 {
				t.Fatalf("binding failure wrote to stderr: %s", output)
			}
			if mode.launch.TerminalError == "" || len(mode.launch.Failures) == 0 || mode.launch.Failures[0].Launcher != "mux" || mode.launch.Launcher != "" {
				t.Fatalf("wrong binding failure diagnostics: %+v", mode.launch)
			}
			if foreground {
				t.Fatal("failed marker allowed foreground fallback")
			}
			if want == "" {
				if got != nil || !errors.Is(err, errNoReviewLauncher) || !strings.Contains(err.Error(), "terminal request unavailable") || !strings.Contains(err.Error(), "SSH") {
					t.Fatalf("selection = %+v, %v; want binding and launcher errors", got, err)
				}
				return
			}
			if err != nil || got == nil || got.name != want {
				t.Fatalf("selection = %+v, %v; want %s", got, err, want)
			}
			if reviewOrigin.Terminal != "rio" || reviewOrigin.SSH != (tc.ssh != "") {
				t.Fatalf("fallback lost caller hints: %+v", reviewOrigin)
			}
		})
	}
}

func TestTerminalNotificationRoutesReview(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "/recorded/socket,123,0")
	t.Setenv("TMUX_PANE", "%42")
	for _, key := range []string{"ZELLIJ", "ZELLIJ_SESSION_NAME", "ZELLIJ_PANE_ID", "ZELLIJ_SOCKET_DIR", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID", "CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID"} {
		t.Setenv(key, "")
	}
	prepare, _, err := rootCmd.Find([]string{"terminal", "prepare"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	prepare.SetOut(&output)
	t.Cleanup(func() { prepare.SetOut(nil) })
	if err := prepare.RunE(prepare, nil); err != nil {
		t.Fatal(err)
	}
	var request terminalbinding.Request
	if err := json.Unmarshal(output.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.ID == "" || request.Marker != "CRITTY-"+request.ID {
		t.Fatalf("invalid prepare response: %s", output.Bytes())
	}
	notify, _, err := rootCmd.Find([]string{"terminal", "notify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := notify.RunE(notify, []string{request.ID}); err != nil {
		t.Fatal(err)
	}
	// The daemon shell can have an unrelated context and no multiplexer binaries.
	t.Setenv("TMUX", "/stale/socket,987,0")
	t.Setenv("TMUX_PANE", "%99")
	t.Setenv("PATH", t.TempDir())
	previousOrigin := reviewOrigin
	t.Cleanup(func() { reviewOrigin = previousOrigin })
	previous := terminalRequest
	terminalRequest = request.ID
	t.Cleanup(func() { terminalRequest = previous })
	got, err := reviewTerminalContext()
	if err != nil || got != (tmuxContext{socket: "/recorded/socket", pane: "%42"}) {
		t.Fatalf("review context: %+v, %v", got, err)
	}
	if _, err := reviewTerminalContext(); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("consumed request fell back to stale daemon context: %v", err)
	}
	for _, cmd := range []string{"review", "sessions resume", "plan"} {
		command, _, err := rootCmd.Find(strings.Fields(cmd))
		if err != nil || command.Flags().Lookup("terminal-request") == nil {
			t.Fatalf("%s does not accept terminal request: %v", cmd, err)
		}
	}
}

func TestMultiplexerTerminalNotificationRoutesReview(t *testing.T) {
	for _, target := range []terminalbinding.Target{
		{Kind: "cmux", Socket: "/recorded/socket", Workspace: "w1", Pane: "s1"},
		{Kind: "zellij", Socket: "/recorded/socket", Workspace: "session", Pane: "0"},
	} {
		t.Run(target.Kind, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			store := terminalbinding.DefaultStore()
			request, err := store.Prepare()
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Notify(request.ID, target); err != nil {
				t.Fatal(err)
			}
			previous, origin := terminalRequest, reviewOrigin
			t.Cleanup(func() { terminalRequest, reviewOrigin = previous, origin })
			terminalRequest = request.ID
			t.Setenv("CMUX_SOCKET_PATH", "/wrong/socket")
			t.Setenv("CMUX_WORKSPACE_ID", "wrong-workspace")
			t.Setenv("CMUX_SURFACE_ID", "wrong-surface")
			got, err := reviewTerminalContext()
			if err != nil || terminalTarget(got) != target {
				t.Fatalf("context = %+v, %v", got, err)
			}
		})
	}
}
