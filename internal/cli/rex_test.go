package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

const rexSource = "block:00000000-0000-0000-0000-000000000001"
const rexReview = "block:00000000-0000-0000-0000-000000000002"
const rexSession = "session:00000000-0000-0000-0000-000000000003"
const rexClient = "client:00000000-0000-0000-0000-000000000005"
const rexSourceWindow = "window:00000000-0000-0000-0000-000000000006"

func testRexContext() rexContext {
	return rexContext{terminalbinding.Target{Kind: "rex", Socket: "unix:///tmp/rex.sock", Workspace: rexSession, Pane: rexSource}}
}

func TestRexReviewLaunch(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		valid       bool
	}{
		{"created", `{"window_id":"window:00000000-0000-0000-0000-000000000004","block_ids":["` + rexReview + `"]}`, true},
		{"missing", `{}`, false},
		{"ambiguous", `{"window_id":"window:00000000-0000-0000-0000-000000000004","block_ids":["` + rexReview + `","` + rexSource + `"]}`, false},
		{"caller", `{"window_id":"window:00000000-0000-0000-0000-000000000004","block_ids":["` + rexSource + `"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldOutput, oldRun, oldResolve := commandOutput, runCommand, resolveExec
			t.Cleanup(func() { commandOutput, runCommand, resolveExec = oldOutput, oldRun, oldResolve })
			resolveExec = func() (string, error) { return "/build with spaces/critty", nil }
			t.Setenv("XDG_STATE_HOME", "/state with spaces")
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if slices.Contains(cmd.Args, "ls") {
					return []byte(`{"blocks":[{"block_id":"` + rexSource + `","creator_name":"com.superlogical.terminal","window_id":"` + rexSourceWindow + `","owner":"` + rexClient + `"}]}`), nil
				}
				for _, arg := range []string{"--server", "unix:///tmp/rex.sock", "--autostart=false", "-s", rexSession, "--shell=none", "--focus", "/bin/sh"} {
					if !slices.Contains(cmd.Args, arg) {
						t.Fatalf("missing %s: %v", arg, cmd.Args)
					}
				}
				command := cmd.Args[len(cmd.Args)-1]
				if !strings.Contains(command, "'/build with spaces/critty' _tui --session '0123456789ab'") || !strings.Contains(command, "XDG_STATE_HOME='/state with spaces'") {
					t.Fatal(command)
				}
				return []byte(tc.reply), nil
			}
			var calls [][]string
			runCommand = func(cmd *exec.Cmd) error { calls = append(calls, cmd.Args); return nil }
			launch, err := spawnRexReview(&reviewMode{sessionKey: "0123456789ab"}, testRexContext())
			if (err == nil) != tc.valid {
				t.Fatalf("launch = %+v, %v", launch, err)
			}
			if !tc.valid {
				return
			}
			if err := launch.close(); err != nil {
				t.Fatal(err)
			}
			launch.restoreFocus()
			wantCalls := [][]string{
				{"-C", rexClient, "do", "session.select", "session_id=" + rexSession, "window_id=window:00000000-0000-0000-0000-000000000004"},
				{"block", "close", "-s", rexSession, rexReview},
				{"focus", "-s", rexSession, rexSource},
				{"-C", rexClient, "do", "session.select", "session_id=" + rexSession, "window_id=" + rexSourceWindow},
			}
			if len(calls) != len(wantCalls) {
				t.Fatal(calls)
			}
			for i, want := range wantCalls {
				// Skip the executable and pinned server options.
				if got := calls[i][6:]; !slices.Equal(got, want) {
					t.Errorf("call %d = %v, want %v", i, got, want)
				}
			}
		})
	}
}

func TestRexCloseRequiresVerifiedAbsence(t *testing.T) {
	for _, state := range []string{"absent", "present", "unreadable", "incomplete", "invalid"} {
		t.Run(state, func(t *testing.T) {
			oldOutput, oldRun := commandOutput, runCommand
			t.Cleanup(func() { commandOutput, runCommand = oldOutput, oldRun })
			runCommand = func(*exec.Cmd) error { return fmt.Errorf("close failed") }
			commandOutput = func(*exec.Cmd) ([]byte, error) {
				switch state {
				case "absent":
					return []byte(`{"blocks":[]}`), nil
				case "present":
					return []byte(`{"blocks":[{"block_id":"` + rexReview + `"}]}`), nil
				case "invalid":
					return []byte(`{"blocks":[{}]}`), nil
				case "incomplete":
					return []byte(`{}`), nil
				default:
					return nil, fmt.Errorf("unavailable")
				}
			}
			err := (rexLaunch{context: testRexContext(), block: rexReview}).close()
			if (err == nil) != (state == "absent") {
				t.Fatalf("close = %v", err)
			}
		})
	}
}

func TestRexReviewFocusFailureKeepsTab(t *testing.T) {
	for _, owner := range []string{rexClient, ""} {
		t.Run("owner="+owner, func(t *testing.T) {
			oldOutput, oldRun, oldResolve := commandOutput, runCommand, resolveExec
			t.Cleanup(func() { commandOutput, runCommand, resolveExec = oldOutput, oldRun, oldResolve })
			resolveExec = func() (string, error) { return "/build/critty", nil }
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if slices.Contains(cmd.Args, "ls") {
					return []byte(`{"blocks":[{"block_id":"` + rexSource + `","creator_name":"com.superlogical.terminal","window_id":"` + rexSourceWindow + `","owner":"` + owner + `"}]}`), nil
				}
				return []byte(`{"window_id":"window:00000000-0000-0000-0000-000000000004","block_ids":["` + rexReview + `"]}`), nil
			}
			runCommand = func(cmd *exec.Cmd) error {
				if !slices.Contains(cmd.Args, "session.select") || !slices.Contains(cmd.Args, rexClient) {
					t.Fatalf("unexpected action after focus failure: %v", cmd.Args)
				}
				if _, err := fmt.Fprintln(cmd.Stderr, "Rex.app is not accepting actions; enable Remote Control"); err != nil {
					t.Fatal(err)
				}
				return fmt.Errorf("exit status 1")
			}
			launch, err := spawnRexReview(&reviewMode{sessionKey: "0123456789ab"}, testRexContext())
			if err != nil || launch.block != rexReview {
				t.Fatalf("review must remain usable: %+v, %v", launch, err)
			}
			if owner != "" {
				if err := launch.focusWindow(rexSourceWindow); err == nil || !strings.Contains(err.Error(), "enable Remote Control") {
					t.Fatalf("focus diagnostic lost: %v", err)
				}
			}
		})
	}
}

func TestRexLauncherRequiresRecordedTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		valid       bool
	}{
		{"caller", `{"blocks":[{"block_id":"` + rexSource + `","creator_name":"com.superlogical.terminal"}]}`, true},
		{"another block", `{"blocks":[{"block_id":"` + rexReview + `","creator_name":"com.superlogical.terminal"}]}`, false},
		{"another type", `{"blocks":[{"block_id":"` + rexSource + `","creator_name":"example"}]}`, false},
		{"missing inventory", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldOutput := commandOutput
			t.Cleanup(func() { commandOutput = oldOutput })
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Args[len(cmd.Args)-2] != rexSession {
					t.Fatal(cmd.Args)
				}
				return []byte(tc.reply), nil
			}
			if err := checkRexLauncher(testRexContext()); (err == nil) != tc.valid {
				t.Fatalf("check = %v", err)
			}
		})
	}
}
