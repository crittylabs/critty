package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestZellijLaunchTargetsSourceTab(t *testing.T) {
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	t.Cleanup(func() { commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve })
	lookPath = func(string) (string, error) { return "/bin/zellij", nil }
	resolveExec = func() (string, error) { return "/path with spaces/critty", nil }
	t.Setenv("ZELLIJ_SESSION_NAME", "wrong-session")
	t.Setenv("ZELLIJ_PANE_ID", "99")
	t.Setenv("ZELLIJ_SOCKET_DIR", "/wrong/socket")
	t.Setenv("XDG_STATE_HOME", "/tmp/state with spaces")
	c := zellijContext{session: "right session", socket: "/right/socket", pane: "0"}
	for _, tc := range []struct {
		name, panes, created, wantError string
		exited                          bool
	}{
		{"success", `[{"id":0,"is_plugin":true,"tab_id":7},{"id":0,"is_plugin":false,"tab_id":3}]`, "terminal_8\n", "", false},
		{"already exited", `[{"id":0,"is_plugin":false,"tab_id":3}]`, "terminal_8\n", "", true},
		{"missing source", `[{"id":0,"is_plugin":true,"tab_id":7}]`, "", "no longer exists", false},
		{"invalid response", `[{"id":0,"is_plugin":false,"tab_id":3}]`, "plugin_8\n", "invalid pane ID", false},
		{"invalid JSON", `{}`, "", "locating Zellij pane", false},
		{"missing metadata", `[{"id":0}]`, "", "incomplete Zellij pane metadata", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			check := func(cmd *exec.Cmd) []string {
				t.Helper()
				if !slices.Equal(cmd.Args[1:3], []string{"--session", c.session}) {
					t.Fatalf("session not pinned: %v", cmd.Args)
				}
				for _, want := range []string{"ZELLIJ_SESSION_NAME=right session", "ZELLIJ_PANE_ID=0", "ZELLIJ_SOCKET_DIR=/right/socket"} {
					if !slices.Contains(cmd.Env, want) {
						t.Fatalf("missing %q in environment", want)
					}
				}
				return cmd.Args[3:]
			}
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				args := check(cmd)
				if slices.Contains(args, "list-panes") {
					if len(calls) == 0 {
						return []byte(tc.panes), nil
					}
					if tc.exited {
						return []byte(`[]`), nil
					}
					return []byte(`[{"id":8,"tab_id":3,"is_plugin":false}]`), nil
				}
				calls = append(calls, args)
				if !slices.Contains(args, "--tab-id") || args[slices.Index(args, "--tab-id")+1] != "3" || !slices.Contains(args, "--close-on-exit") {
					t.Fatalf("wrong destination or lifecycle: %v", args)
				}
				command := args[len(args)-1]
				if !strings.Contains(command, "exec env CRITTY_HOST='Zellij'") || !strings.Contains(command, "XDG_STATE_HOME='/tmp/state with spaces'") || !strings.Contains(command, "'/path with spaces/critty' _tui --session '0123456789ab'") {
					t.Fatalf("incorrect TUI command: %s", command)
				}
				return []byte(tc.created), nil
			}
			runCommand = func(cmd *exec.Cmd) error {
				calls = append(calls, check(cmd))
				if tc.exited && slices.Contains(cmd.Args, "close-pane") {
					return fmt.Errorf("pane not found")
				}
				return nil
			}
			launch, err := spawnTUIZellijPane(&reviewMode{sessionKey: "0123456789ab"}, c)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := launch.close(); err != nil {
				t.Fatal(err)
			}
			launch.restoreFocus()
			if !slices.Equal(calls[1], []string{"action", "close-pane", "--pane-id", "8"}) {
				t.Fatalf("close = %v", calls)
			}
			if !slices.Equal(calls[len(calls)-1], []string{"action", "focus-pane-id", "0"}) {
				t.Fatalf("focus = %v", calls)
			}
		})
	}
}

func TestZellijLauncherAvailability(t *testing.T) {
	origOutput, origLook := commandOutput, lookPath
	t.Cleanup(func() { commandOutput, lookPath = origOutput, origLook })
	lookPath = func(string) (string, error) { return "/bin/zellij", nil }
	terminal := zellijContext{session: "review", pane: "0"}
	for _, tc := range []struct {
		version string
		wantOK  bool
	}{
		{"zellij 0.45.1", true}, {"zellij 0.45.0", true}, {"zellij 1.0.0", true},
		{"zellij 0.43.1", false}, {"unknown", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			commandOutput = func(*exec.Cmd) ([]byte, error) { return []byte(tc.version), nil }
			candidates, err := launcherCandidates("mux", terminal, &reviewMode{})
			if err != nil || len(candidates) != 1 || candidates[0].name != "zellij" {
				t.Fatalf("candidates = %v, %v", candidates, err)
			}
			if err := candidates[0].check(); (err == nil) != tc.wantOK {
				t.Fatalf("availability = %v", err)
			}
		})
	}
	commandOutput = func(*exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("unavailable") }
}

func TestZellijCloseVerifiesFailedCleanup(t *testing.T) {
	origRun, origOutput := runCommand, commandOutput
	t.Cleanup(func() { runCommand, commandOutput = origRun, origOutput })
	runCommand = func(*exec.Cmd) error { return fmt.Errorf("close failed") }
	for _, tc := range []struct {
		name, response string
		wantError      bool
	}{
		{"already closed", `[]`, false},
		{"still present", `[{"id":8,"is_plugin":false,"tab_id":0}]`, true},
		{"unreadable", `{}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandOutput = func(*exec.Cmd) ([]byte, error) { return []byte(tc.response), nil }
			err := (zellijLaunch{context: zellijContext{session: "review", pane: "0"}, bin: "/bin/zellij", pane: "8"}).close()
			if (err != nil) != tc.wantError {
				t.Fatalf("close = %v", err)
			}
		})
	}
}
