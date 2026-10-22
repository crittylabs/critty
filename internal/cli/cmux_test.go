package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestSpawnTUICMUXTab(t *testing.T) {
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	t.Cleanup(func() { commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve })
	lookPath = func(string) (string, error) { return "/bin/cmux", nil }
	resolveExec = func() (string, error) { return "/path with spaces/critty", nil }
	t.Setenv("CMUX_WORKSPACE_ID", "stale")
	t.Setenv("CMUX_SOCKET_PATH", "/wrong/socket")
	t.Setenv("XDG_STATE_HOME", "/tmp/state with spaces")
	for _, tc := range []struct {
		name, location, created string
		failFocus               bool
		wantCalls               int
		wantError               string
	}{
		{"success", `{"caller":{"pane_id":"p1"},"focused":{"pane_id":"wrong"}}`, `{"surface_id":"s9"}`, false, 3, ""},
		{"focus failure", `{"caller":{"pane_id":"p1"}}`, `{"surface_id":"s9"}`, true, 3, "focusing cmux tab"},
		{"missing caller", `{"caller":null,"focused":{"pane_id":"wrong"}}`, "", false, 0, "missing caller pane"},
		{"bad create response", `{"caller":{"pane_id":"p1"}}`, `{}`, false, 0, "missing surface ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			outputs := 0
			check := func(cmd *exec.Cmd) []string {
				t.Helper()
				if !slices.Equal(cmd.Args[1:3], []string{"--socket", "/right/socket"}) {
					t.Fatalf("socket not pinned: %v", cmd.Args)
				}
				for _, e := range cmd.Env {
					if strings.HasPrefix(e, "CMUX_WORKSPACE_ID=") || strings.HasPrefix(e, "CMUX_SOCKET_PATH=") {
						t.Fatalf("stale context: %s", e)
					}
				}
				return cmd.Args[3:]
			}
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				args := check(cmd)
				outputs++
				if outputs == 1 {
					return []byte(tc.location), nil
				}
				for _, want := range []string{"new-surface", "--pane", "p1", "--working-directory", "--focus", "false"} {
					if !slices.Contains(args, want) {
						t.Fatalf("create missing %q: %v", want, args)
					}
				}
				command := args[len(args)-1]
				if !strings.Contains(command, "exec env CRITTY_HOST='cmux'") || !strings.Contains(command, "XDG_STATE_HOME='/tmp/state with spaces'") || !strings.Contains(command, "'/path with spaces/critty' _tui --session '0123456789ab'") {
					t.Fatalf("incorrect TUI command: %s", command)
				}
				return []byte(tc.created), nil
			}
			runCommand = func(cmd *exec.Cmd) error {
				args := check(cmd)
				calls = append(calls, args)
				if tc.failFocus && len(calls) == 1 {
					return fmt.Errorf("focus failed")
				}
				return nil
			}
			launch, err := spawnTUICMUXTab(&reviewMode{sessionKey: "0123456789ab"}, cmuxContext{socket: "/right/socket", workspace: "w1", surface: "s1"})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := launch.close(); err != nil {
					t.Fatal(err)
				}
				launch.restoreFocus()
			}
			if len(calls) != tc.wantCalls {
				t.Fatalf("calls: %v", calls)
			}
			if tc.wantCalls == 3 {
				want := [][]string{{"focus-panel", "--workspace", "w1", "--panel", "s9"}, {"close-surface", "--workspace", "w1", "--surface", "s9"}, {"focus-panel", "--workspace", "w1", "--panel", "s1"}}
				if !slices.EqualFunc(calls, want, slices.Equal) {
					t.Fatalf("calls = %v", calls)
				}
			}
		})
	}
}

func TestCMUXDetectorAndEnvironment(t *testing.T) {
	t.Setenv("CMUX_SOCKET_PATH", "/right/socket")
	t.Setenv("CMUX_WORKSPACE_ID", "w1")
	t.Setenv("CMUX_SURFACE_ID", "s1")
	c := (cmuxDetector{}).environmentContext()
	want := cmuxContext{socket: "/right/socket", workspace: "w1", surface: "s1"}
	if c != want {
		t.Fatalf("context = %+v", c)
	}
	vars, err := environmentForTerminal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []envVar{{"CMUX_SOCKET_PATH", want.socket}, {"CMUX_WORKSPACE_ID", want.workspace}, {"CMUX_SURFACE_ID", want.surface}} {
		if !slices.Contains(vars, v) {
			t.Errorf("missing %v in %v", v, vars)
		}
	}
	t.Setenv("CMUX_SURFACE_ID", "")
	if (cmuxDetector{}).environmentContext() != nil {
		t.Fatal("accepted incomplete context")
	}
}

func TestCMUXCloseHandlesExitedSurface(t *testing.T) {
	origRun, origOutput := runCommand, commandOutput
	t.Cleanup(func() { runCommand, commandOutput = origRun, origOutput })
	runCommand = func(*exec.Cmd) error { return fmt.Errorf("close failed") }
	for _, tc := range []struct {
		name, response string
		outputErr      error
		wantError      bool
	}{
		{"already closed", `{"surfaces":[{"id":"source"}]}`, nil, false},
		{"still exists", `{"surfaces":[{"id":"review"}]}`, nil, true},
		{"unreadable", "", fmt.Errorf("unavailable"), true},
		{"malformed", `{`, nil, true},
		{"missing surfaces", `{}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandOutput = func(*exec.Cmd) ([]byte, error) { return []byte(tc.response), tc.outputErr }
			err := (cmuxLaunch{context: cmuxContext{socket: "/socket", workspace: "w1", surface: "source"}, bin: "/bin/cmux", surface: "review"}).close()
			if (err != nil) != tc.wantError {
				t.Fatalf("close = %v", err)
			}
		})
	}
}

func TestTMUXWinsSharedCMUXProcess(t *testing.T) {
	origOutput, origParent, origLook := commandOutput, parentProcessID, lookPath
	t.Cleanup(func() { commandOutput, parentProcessID, lookPath = origOutput, origParent, origLook })
	t.Setenv("TMUX", "/tmux/socket,100,2")
	t.Setenv("TMUX_PANE", "%7")
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("CMUX_SOCKET_PATH", "/cmux/socket")
	t.Setenv("CMUX_WORKSPACE_ID", "w1")
	t.Setenv("CMUX_SURFACE_ID", "s1")
	parentProcessID = func() int { return 300 }
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Path == "/bin/cmux" {
			return []byte(`{"windows":[{"workspaces":[{"id":"w1","panes":[{"surfaces":[{"id":"s1","top_level_pids":[300]}]}]}]}]}`), nil
		}
		if slices.Contains(cmd.Args, "list-panes") {
			return []byte("300\t/tmux/socket\t100\t$2\t%7\n"), nil
		}
		return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
	}
	got := findMultiplexerContext()
	if got != (tmuxContext{session: "/tmux/socket,100,2", pane: "%7"}) {
		t.Fatalf("context = %+v", got)
	}
}
