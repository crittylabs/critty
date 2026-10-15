package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
)

func TestEnvCommandPrintsTMUXEnvironment(t *testing.T) {
	for _, key := range []string{"TERMINAL_APP", "TERM_PROGRAM", "ITERM_SESSION_ID", "WT_SESSION", "SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(key, "")
	}
	previousOrigin := reviewOrigin
	t.Cleanup(func() { reviewOrigin = previousOrigin })
	t.Setenv("TMUX", "/tmp/tmux-501/default,100,2")
	t.Setenv("TMUX_PANE", "%7")
	for _, key := range []string{"HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(key, "")
	}
	origOutput, origInspect, origLook := commandOutput, inspectProcess, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[0] == "ps" {
			return nil, fmt.Errorf("ps must not run when the pane reports its terminal")
		}
		if cmd.Args[1] == "display-message" {
			if !slices.Equal(cmd.Args[2:5], []string{"-p", "-t", "%7"}) {
				return nil, fmt.Errorf("unexpected target: %v", cmd.Args)
			}
			return []byte("/tmp/tmux-501/default\t100\t$2\t%7\t/dev/ttys003\n"), nil
		}
		return nil, fmt.Errorf("no server listing")
	}
	inspectProcess = func(int) (int, error) { return 0, fmt.Errorf("no process info") }
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	t.Cleanup(func() {
		commandOutput, inspectProcess, lookPath = origOutput, origInspect, origLook
		envJSON = false
	})

	cmd, _, err := rootCmd.Find([]string{"env"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	t.Cleanup(func() { cmd.SetOut(nil) })
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	want := "TMUX=/tmp/tmux-501/default,100,2\nTMUX_PANE=%7\nTTY=/dev/ttys003\n"
	if output.String() != want {
		t.Errorf("env output = %q, want %q", output.String(), want)
	}

	output.Reset()
	envJSON = true
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var object map[string]string
	if err := json.Unmarshal(output.Bytes(), &object); err != nil {
		t.Fatalf("invalid JSON %q: %v", output.Bytes(), err)
	}
	if object["TMUX_PANE"] != "%7" || object["TTY"] != "/dev/ttys003" || len(object) != 3 {
		t.Errorf("env --json = %v", object)
	}
	if err := cmd.RunE(cmd, []string{"run", "true"}); err == nil {
		t.Error("--json with a command did not fail")
	}

	output.Reset()
	if err := cmd.RunE(cmd, []string{"tty"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "\"/dev/ttys003\"\n" {
		t.Errorf("env tty --json = %q", output.String())
	}
	envJSON = false
	output.Reset()
	if err := cmd.RunE(cmd, []string{"tty"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "/dev/ttys003\n" {
		t.Errorf("env tty = %q", output.String())
	}

	closed, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	cmd.SetOut(closed)
	for _, mode := range []string{"all", "tty"} {
		t.Run(mode+" write error", func(t *testing.T) {
			if err := cmd.RunE(cmd, []string{mode}); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("env %s error = %v, want closed output error", mode, err)
			}
		})
	}
}

func TestParseEnvArgs(t *testing.T) {
	cases := []struct {
		dash    int
		args    []string
		mode    string
		command []string
		fails   bool
	}{
		{-1, nil, "all", nil, false},
		{-1, []string{"all"}, "all", nil, false},
		{-1, []string{"tty"}, "tty", nil, false},
		{-1, []string{"run", "ls", "-l"}, "run", []string{"ls", "-l"}, false},
		{1, []string{"run", "ls", "-l"}, "run", []string{"ls", "-l"}, false},
		{0, []string{"ls", "-l"}, "run", []string{"ls", "-l"}, false},
		{0, []string{"run", "ls"}, "run", []string{"run", "ls"}, false},
		{0, nil, "", nil, true},
		{-1, []string{"run"}, "", nil, true},
		{-1, []string{"tty", "extra"}, "", nil, true},
		{-1, []string{"ls"}, "", nil, true},
	}
	for _, c := range cases {
		mode, command, err := parseEnvArgs(c.dash, c.args)
		if (err != nil) != c.fails || mode != c.mode || !slices.Equal(command, c.command) {
			t.Errorf("parseEnvArgs(%d, %v) = %q, %v, %v", c.dash, c.args, mode, command, err)
		}
	}
}

func TestTMUXEnvironmentWithoutPane(t *testing.T) {
	got, err := tmuxEnvironment(tmuxContext{session: "/tmp/tmux-501/default,100,3"})
	if err != nil || !slices.Equal(got, []envVar{{"TMUX", "/tmp/tmux-501/default,100,3"}}) {
		t.Errorf("tmuxEnvironment() = %v, %v", got, err)
	}
	if _, err := tmuxEnvironment(tmuxContext{}); err == nil {
		t.Error("empty context did not fail")
	}
}

func TestHerdrEnvironmentIncludesPaneTTY(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "")
	origOutput, origLook := commandOutput, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if !slices.Equal(cmd.Args[1:], []string{"pane", "process-info", "--pane", "w1:p1"}) {
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		}
		if !slices.Contains(cmd.Env, "HERDR_SOCKET_PATH=/run/herdr.sock") {
			return nil, fmt.Errorf("socket not pinned: %v", cmd.Env)
		}
		return []byte(`{"result":{"process_info":{"pane_id":"w1:p1","shell_pid":100,"tty":"/dev/ttys004"}}}`), nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	t.Cleanup(func() { commandOutput, lookPath = origOutput, origLook })

	got, err := herdrEnvironment(herdrContext{socket: "/run/herdr.sock", workspace: "w1", tab: "w1:t1", pane: "w1:p1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []envVar{
		{"HERDR_ENV", "1"},
		{"HERDR_SOCKET_PATH", "/run/herdr.sock"},
		{"HERDR_WORKSPACE_ID", "w1"},
		{"HERDR_TAB_ID", "w1:t1"},
		{"HERDR_PANE_ID", "w1:p1"},
		{"TTY", "/dev/ttys004"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("herdrEnvironment() = %v, want %v", got, want)
	}
}

func TestMergeEnvironmentOverridesExistingVariables(t *testing.T) {
	got := mergeEnvironment([]string{"PATH=/bin", "TMUX=stale", "TMUX_PANE=%1"}, []envVar{{"TMUX", "fresh"}, {"TTY", "/dev/ttys001"}})
	want := []string{"PATH=/bin", "TMUX_PANE=%1", "TMUX=fresh", "TTY=/dev/ttys001"}
	if !slices.Equal(got, want) {
		t.Errorf("mergeEnvironment() = %v, want %v", got, want)
	}
}

func TestTerminalEnvironmentFallsBackToAncestorTTY(t *testing.T) {
	for _, key := range []string{"TERMINAL_APP", "TERM_PROGRAM", "ITERM_SESSION_ID", "WT_SESSION", "SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(key, "")
	}
	previousOrigin := reviewOrigin
	t.Cleanup(func() { reviewOrigin = previousOrigin })
	for _, key := range []string{"TMUX", "TMUX_PANE", "HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("SSH_TTY", "/dev/pts/9")
	origOutput, origParent, origInspect, origLook := commandOutput, parentProcessID, inspectProcess, lookPath
	ttys := map[string]string{"400": "??", "300": "ttys005"}
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[0] != "ps" {
			return nil, fmt.Errorf("no multiplexer: %v", cmd.Args)
		}
		tty, ok := ttys[cmd.Args[4]]
		if !ok {
			tty = "??"
		}
		return []byte(tty + "\n"), nil
	}
	parentProcessID = func() int { return 400 }
	inspectProcess = func(pid int) (int, error) {
		if pid == 400 {
			return 300, nil
		}
		return 0, fmt.Errorf("unexpected PID: %d", pid)
	}
	lookPath = func(name string) (string, error) { return "", fmt.Errorf("%s not installed", name) }
	t.Cleanup(func() {
		commandOutput, parentProcessID, inspectProcess, lookPath = origOutput, origParent, origInspect, origLook
	})

	got, err := terminalEnvironment()
	if err != nil || !slices.Equal(got, []envVar{{"TTY", "/dev/ttys005"}, {"CRITTY_SSH", "1"}}) {
		t.Fatalf("terminalEnvironment() = %v, %v", got, err)
	}

	delete(ttys, "300")
	got, err = terminalEnvironment()
	if err != nil || !slices.Equal(got, []envVar{{"TTY", "/dev/pts/9"}, {"CRITTY_SSH", "1"}}) {
		t.Fatalf("SSH_TTY fallback = %v, %v", got, err)
	}

	t.Setenv("SSH_TTY", "")
	if _, err := terminalEnvironment(); err == nil {
		t.Fatal("missing terminal did not fail")
	}
}

func TestProcessTTYNormalizesDeviceNames(t *testing.T) {
	origOutput := commandOutput
	t.Cleanup(func() { commandOutput = origOutput })
	for input, want := range map[string]string{"ttys026\n": "/dev/ttys026", "pts/3\n": "/dev/pts/3", "?\n": "", "??\n": "", "-\n": "", "": ""} {
		commandOutput = func(*exec.Cmd) ([]byte, error) { return []byte(input), nil }
		if got := processTTY(42); got != want {
			t.Errorf("processTTY(%q) = %q, want %q", input, got, want)
		}
	}
}
