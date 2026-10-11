package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

type staticMultiplexerDetector struct {
	contexts map[int]reviewMultiplexerContext
}

func (d staticMultiplexerDetector) environmentPresent() bool {
	return false
}

func (d staticMultiplexerDetector) environmentContext() reviewMultiplexerContext {
	return nil
}

func (d staticMultiplexerDetector) processContexts() map[int]reviewMultiplexerContext {
	return d.contexts
}

func TestHerdrDetectorUsesEnvironment(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("HERDR_TAB_ID", "w1:t2")
	t.Setenv("HERDR_PANE_ID", "w1:p3")

	context := (herdrDetector{}).environmentContext()
	got, ok := context.(herdrContext)
	if !ok {
		t.Fatalf("environmentContext() = %T, want herdrContext", context)
	}
	if got != (herdrContext{workspace: "w1", tab: "w1:t2", pane: "w1:p3"}) {
		t.Errorf("environmentContext() = %+v", got)
	}
}

func TestFocusCurrentHerdrTab(t *testing.T) {
	var calls [][]string
	origRun, origLook := runCommand, lookPath
	runCommand = func(cmd *exec.Cmd) error {
		calls = append(calls, cmd.Args)
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	t.Setenv("HERDR_TAB_ID", "w1:t2")
	t.Cleanup(func() {
		runCommand, lookPath = origRun, origLook
	})

	focusCurrentHerdrTab()

	want := []string{"/usr/local/bin/herdr", "tab", "focus", "w1:t2"}
	if len(calls) != 1 || !slices.Equal(calls[0], want) {
		t.Fatalf("focus call = %v, want %v", calls, want)
	}
}

func TestHerdrProcessIDs(t *testing.T) {
	data := []byte(`{
  "result": {
    "process_info": {
      "pane_id": "w1:p1",
      "shell_pid": 101,
      "foreground_processes": [
        {"pid": 202, "name": "codex"},
        {"pid": 303, "name": "node"}
      ]
    }
  }
}`)

	got := herdrProcessIDs(data)
	for _, want := range []int{101, 202, 303} {
		if !slices.Contains(got, want) {
			t.Errorf("herdrProcessIDs() = %v, missing %d", got, want)
		}
	}
}

func TestResolveHerdrContextFromAncestorProcess(t *testing.T) {
	origOutput, origParent, origInspect, origLook := commandOutput, parentProcessID, inspectProcess, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		switch {
		case slices.Equal(cmd.Args[1:], []string{"pane", "list"}):
			return []byte(`{"result":{"panes":[{"workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1"},{"workspace_id":"w2","tab_id":"w2:t4","pane_id":"w2:p8"}]}}`), nil
		case slices.Equal(cmd.Args[1:], []string{"pane", "process-info", "--pane", "w1:p1"}):
			return []byte(`{"result":{"process_info":{"shell_pid":100,"foreground_processes":[{"pid":200}]}}}`), nil
		case slices.Equal(cmd.Args[1:], []string{"pane", "process-info", "--pane", "w2:p8"}):
			return []byte(`{"result":{"process_info":{"shell_pid":300,"foreground_processes":[{"pid":400}]}}}`), nil
		default:
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		}
	}
	parentProcessID = func() int { return 500 }
	inspectProcess = func(pid int) (int, error) {
		switch pid {
		case 500:
			return 400, nil
		default:
			return 0, fmt.Errorf("unexpected PID: %d", pid)
		}
	}
	lookPath = func(name string) (string, error) {
		if name != "herdr" {
			return "", fmt.Errorf("unexpected binary: %s", name)
		}
		return "/usr/local/bin/herdr", nil
	}
	t.Cleanup(func() {
		commandOutput, parentProcessID, inspectProcess, lookPath = origOutput, origParent, origInspect, origLook
	})

	context := walkAncestorContexts([]multiplexerDetector{herdrDetector{}})
	got, ok := context.(herdrContext)
	if !ok {
		t.Fatalf("walkAncestorContexts() = %T, want herdrContext", context)
	}
	if want := (herdrContext{workspace: "w2", tab: "w2:t4", pane: "w2:p8"}); got != want {
		t.Errorf("walkAncestorContexts() = %+v, want %+v", got, want)
	}
}

func TestNearestMultiplexerContextChoosesClosestHerdr(t *testing.T) {
	origParent, origInspect := parentProcessID, inspectProcess
	parentProcessID = func() int { return 500 }
	inspectProcess = func(pid int) (int, error) {
		switch pid {
		case 500:
			return 400, nil
		case 400:
			return 300, nil
		default:
			return 1, nil
		}
	}
	t.Cleanup(func() {
		parentProcessID, inspectProcess = origParent, origInspect
	})

	got := walkAncestorContexts([]multiplexerDetector{
		staticMultiplexerDetector{contexts: map[int]reviewMultiplexerContext{300: tmuxContext{session: "outer-tmux", pane: "%1"}}},
		staticMultiplexerDetector{contexts: map[int]reviewMultiplexerContext{400: herdrContext{workspace: "w1", tab: "w1:t1", pane: "w1:p1"}}},
	})
	if _, ok := got.(herdrContext); !ok {
		t.Errorf("walkAncestorContexts() = %T, want herdrContext", got)
	}
}

func TestNearestMultiplexerContextChoosesClosestTMUX(t *testing.T) {
	origParent, origInspect := parentProcessID, inspectProcess
	parentProcessID = func() int { return 500 }
	inspectProcess = func(pid int) (int, error) {
		switch pid {
		case 500:
			return 400, nil
		case 400:
			return 300, nil
		default:
			return 1, nil
		}
	}
	t.Cleanup(func() {
		parentProcessID, inspectProcess = origParent, origInspect
	})

	got := walkAncestorContexts([]multiplexerDetector{
		staticMultiplexerDetector{contexts: map[int]reviewMultiplexerContext{400: tmuxContext{session: "inner-tmux", pane: "%1"}}},
		staticMultiplexerDetector{contexts: map[int]reviewMultiplexerContext{300: herdrContext{workspace: "w1", tab: "w1:t1", pane: "w1:p1"}}},
	})
	if _, ok := got.(tmuxContext); !ok {
		t.Errorf("walkAncestorContexts() = %T, want tmuxContext", got)
	}
}

func TestSpawnTUIHerdrTab(t *testing.T) {
	var outputCalls, runCalls [][]string
	checkServer := func(cmd *exec.Cmd) {
		t.Helper()
		if !slices.Contains(cmd.Env, "HERDR_SOCKET_PATH=/correct/server.sock") {
			t.Fatalf("command %v did not pin the socket", cmd.Args)
		}
		for _, e := range cmd.Env {
			if strings.HasPrefix(e, "HERDR_TAB_ID=") || strings.HasPrefix(e, "HERDR_CONFIG_PATH=") {
				t.Fatalf("inherited stale Herdr context: %s", e)
			}
		}
	}
	t.Setenv("HERDR_SOCKET_PATH", "/wrong/server.sock")
	t.Setenv("HERDR_TAB_ID", "wrong-tab")
	t.Setenv("HERDR_CONFIG_PATH", "/wrong/config")
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		checkServer(cmd)
		outputCalls = append(outputCalls, cmd.Args)
		return []byte(`{"result":{"tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}`), nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		checkServer(cmd)
		runCalls = append(runCalls, cmd.Args)
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Setenv("XDG_STATE_HOME", "/tmp/state with spaces")
	t.Cleanup(func() {
		commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve
	})

	launch, err := spawnTUIHerdrTab(
		&reviewMode{ref: "HEAD", sessionKey: "0123456789ab"},
		herdrContext{socket: "/correct/server.sock", workspace: "w1", tab: "w1:t1", pane: "w1:p1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if launch.tab != "w1:t9" || launch.sourceTab != "w1:t1" {
		t.Errorf("launch = %+v", launch)
	}
	if len(outputCalls) != 1 {
		t.Fatalf("expected one tab creation, got %d", len(outputCalls))
	}
	createArgs := outputCalls[0]
	for _, want := range []string{"tab", "create", "--workspace", "w1", "--label", "Critty", "--no-focus", "XDG_STATE_HOME=/tmp/state with spaces"} {
		if !slices.Contains(createArgs, want) {
			t.Errorf("tab create missing %q: %v", want, createArgs)
		}
	}
	if len(runCalls) != 2 {
		t.Fatalf("expected pane run and tab focus, got %d calls", len(runCalls))
	}
	if got := runCalls[0][3]; got != "w1:p9" {
		t.Errorf("pane run target = %q", got)
	}
	command := runCalls[0][4]
	for _, want := range []string{"exec env TCRIT_HOST='Herdr'", "XDG_STATE_HOME='/tmp/state with spaces'", "'/usr/local/bin/critty' _tui --session '0123456789ab'"} {
		if !strings.Contains(command, want) {
			t.Errorf("pane command missing %q: %s", want, command)
		}
	}
	if !slices.Equal(runCalls[1][1:], []string{"tab", "focus", "w1:t9"}) {
		t.Errorf("tab focus call = %v", runCalls[1])
	}
	launch.restoreFocus()
	launch.close()
}

func TestSpawnTUIHerdrTabClosesTabWhenPaneRunFails(t *testing.T) {
	var calls [][]string
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	commandOutput = func(*exec.Cmd) ([]byte, error) {
		return []byte(`{"result":{"tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}`), nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		calls = append(calls, cmd.Args)
		if len(calls) == 1 {
			return fmt.Errorf("pane busy")
		}
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Cleanup(func() {
		commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve
	})

	_, err := spawnTUIHerdrTab(
		&reviewMode{ref: "HEAD", sessionKey: "0123456789ab"},
		herdrContext{workspace: "w1", tab: "w1:t1", pane: "w1:p1"},
	)
	if err == nil {
		t.Fatal("expected pane run failure")
	}
	if len(calls) != 2 || !slices.Equal(calls[1][1:], []string{"tab", "close", "w1:t9"}) {
		t.Errorf("cleanup calls = %v", calls)
	}
}

func TestSpawnTUIHerdrTabClosesIncompleteTab(t *testing.T) {
	var calls [][]string
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	commandOutput = func(*exec.Cmd) ([]byte, error) {
		return []byte(`{"result":{"tab":{"tab_id":"w1:t9"},"root_pane":{}}}`), nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		calls = append(calls, cmd.Args)
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Cleanup(func() {
		commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve
	})

	_, err := spawnTUIHerdrTab(
		&reviewMode{ref: "HEAD", sessionKey: "0123456789ab"},
		herdrContext{workspace: "w1", tab: "w1:t1", pane: "w1:p1"},
	)
	if err == nil {
		t.Fatal("expected incomplete response error")
	}
	want := [][]string{
		{"/usr/local/bin/herdr", "tab", "close", "w1:t9"},
		{"/usr/local/bin/herdr", "tab", "focus", "w1:t1"},
	}
	if len(calls) != len(want) {
		t.Fatalf("cleanup calls = %v", calls)
	}
	for i := range want {
		if !slices.Equal(calls[i], want[i]) {
			t.Errorf("cleanup call %d = %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestSpawnTUIHerdrTabCleansUpWhenFocusFails(t *testing.T) {
	var calls [][]string
	origOutput, origRun, origLook, origResolve := commandOutput, runCommand, lookPath, resolveExec
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	commandOutput = func(*exec.Cmd) ([]byte, error) {
		return []byte(`{"result":{"tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}`), nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		calls = append(calls, cmd.Args)
		if len(calls) == 2 {
			return fmt.Errorf("no foreground client")
		}
		return nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Cleanup(func() {
		commandOutput, runCommand, lookPath, resolveExec = origOutput, origRun, origLook, origResolve
	})

	_, err := spawnTUIHerdrTab(
		&reviewMode{ref: "HEAD", sessionKey: "0123456789ab"},
		herdrContext{workspace: "w1", tab: "w1:t1", pane: "w1:p1"},
	)
	if err == nil {
		t.Fatal("expected focus failure")
	}
	want := [][]string{
		{"/usr/local/bin/herdr", "pane", "run", "w1:p9", "exec env TCRIT_HOST='Herdr' '/usr/local/bin/critty' _tui --session '0123456789ab'"},
		{"/usr/local/bin/herdr", "tab", "focus", "w1:t9"},
		{"/usr/local/bin/herdr", "tab", "close", "w1:t9"},
		{"/usr/local/bin/herdr", "tab", "focus", "w1:t1"},
	}
	if len(calls) != len(want) {
		t.Fatalf("cleanup calls = %v", calls)
	}
	for i := range want {
		if !slices.Equal(calls[i], want[i]) {
			t.Errorf("cleanup call %d = %v, want %v", i, calls[i], want[i])
		}
	}
}
