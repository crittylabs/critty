package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func mockTMUXLaunch(t *testing.T, clientRows string) *[][]string {
	t.Helper()
	var calls [][]string
	originalOutput, originalRun, originalLook, originalResolve := commandOutput, runCommand, lookPath, resolveExec
	lookPath = func(string) (string, error) { return "tmux", nil }
	resolveExec = func() (string, error) { return "/critty", nil }
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		args := cmd.Args[1:]
		calls = append(calls, args)
		switch args[0] {
		case "list-commands":
			return []byte("new-pane\t[-AdP] [-B border-lines] [-T title] [-x width] [-y height]\n"), nil
		case "display-message":
			if args[len(args)-1] == "#{pane_active}" {
				return []byte("1\n"), nil
			}
			if args[len(args)-1] == "#{window_id}" {
				return []byte("@1\n"), nil
			}
			return []byte("$0\t@1\t%7\t180\t60\n"), nil
		case "new-pane", "split-window":
			return []byte("%99\n"), nil
		case "show-options":
			return []byte("on\n"), nil
		case "list-clients":
			return []byte(clientRows), nil
		}
		return nil, fmt.Errorf("unexpected command: %v", args)
	}
	runCommand = func(cmd *exec.Cmd) error {
		calls = append(calls, cmd.Args[1:])
		return nil
	}
	t.Cleanup(func() {
		commandOutput, runCommand, lookPath, resolveExec = originalOutput, originalRun, originalLook, originalResolve
	})
	return &calls
}

func tmuxCall(calls [][]string, name string) []string {
	for _, call := range calls {
		if call[0] == name {
			return call
		}
	}
	return nil
}

func TestBuiltinTMUXLaunch(t *testing.T) {
	calls := mockTMUXLaunch(t, "")
	launch, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, false)
	if err != nil {
		t.Fatal(err)
	}
	spawn := tmuxCall(*calls, "split-window")
	if !slices.Contains(spawn, "-d") || !strings.Contains(spawn[len(spawn)-1], "_tui --session '0123456789ab'") {
		t.Fatalf("incorrect TUI launch: %v", spawn)
	}
	if launch.pane != "%99" || launch.source.pane != "%7" {
		t.Fatalf("incorrect pane ownership: %+v", launch)
	}
}

func TestBuiltinTMUXFloatingGeometry(t *testing.T) {
	calls := mockTMUXLaunch(t, "")
	if _, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, true); err != nil {
		t.Fatal(err)
	}
	spawn := tmuxCall(*calls, "new-pane")
	for option, value := range map[string]string{"-x": "162", "-y": "51", "-X": "9", "-Y": "4", "-B": "single"} {
		i := slices.Index(spawn, option)
		if i < 0 || spawn[i+1] != value {
			t.Errorf("wrong window-relative geometry: %v", spawn)
		}
	}
	if !slices.Contains(spawn, "-A") || !slices.Contains(spawn, "-d") || slices.Contains(spawn, "-O") {
		t.Fatalf("float must remain detached and nonmodal: %v", spawn)
	}
}

func TestBuiltinTMUXOpeningFailureDoesNotSplit(t *testing.T) {
	calls := mockTMUXLaunch(t, "")
	original := commandOutput
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[1] == "new-pane" {
			return nil, fmt.Errorf("opening failed")
		}
		return original(cmd)
	}
	launcher, err := selectReviewLauncher([]string{"tmux"}, tmuxContext{pane: "%7"}, &reviewMode{sessionKey: "0123456789ab"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.open(&reviewMode{sessionKey: "0123456789ab"}); err == nil {
		t.Fatal("opening failure lost")
	}
	if tmuxCall(*calls, "split-window") != nil {
		t.Fatalf("opening failure retried another launcher: %v", *calls)
	}
}

func TestBuiltinTMUXFocusAndClose(t *testing.T) {
	for _, tc := range []struct {
		name, rows              string
		sourceActive, wantFocus bool
	}{
		{"source focused", "/dev/tty1\t$0\t%7\tattached,focused\n", true, true},
		{"other pane focused", "/dev/tty1\t$0\t%8\tattached,focused\n", false, false},
		{"other window focused", "/dev/tty1\t$0\t%8\tattached,focused\n", true, true},
		{"terminal not focused", "/dev/tty1\t$0\t%7\tattached\n", true, true},
		{"multiple clients", "/dev/tty1\t$0\t%7\tfocused\n/dev/tty2\t$0\t%7\tfocused\n", true, false},
		{"no client", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := mockTMUXLaunch(t, tc.rows)
			original := commandOutput
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Args[len(cmd.Args)-1] == "#{pane_active}" && !tc.sourceActive {
					return []byte("0\n"), nil
				}
				return original(cmd)
			}
			launch, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, false)
			if err != nil {
				t.Fatal(err)
			}
			if (tmuxCall(*calls, "select-pane") != nil) != tc.wantFocus {
				t.Fatalf("incorrect focus decision: %v", *calls)
			}
			*calls = nil
			// User selected another pane before closing: preserve their selection.
			launch.close()
			if tmuxCall(*calls, "select-pane") != nil || tmuxCall(*calls, "select-window") != nil ||
				!slices.Equal(tmuxCall(*calls, "kill-pane"), []string{"kill-pane", "-t", "%99"}) {
				t.Fatalf("close stole focus or killed the source: %v", *calls)
			}
		})
	}
	for _, floating := range []bool{false, true} {
		t.Run(fmt.Sprintf("restore from floating=%t", floating), func(t *testing.T) {
			calls := mockTMUXLaunch(t, "/dev/tty1\t$0\t%99\tattached,focused\n")
			launch, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, floating)
			if err != nil {
				t.Fatal(err)
			}
			*calls = nil
			launch.close()
			if !slices.Equal(tmuxCall(*calls, "select-pane"), []string{"select-pane", "-t", "%7"}) ||
				!slices.Equal(tmuxCall(*calls, "select-window"), []string{"select-window", "-t", "@1"}) {
				t.Fatalf("did not restore the source before cleanup: %v", *calls)
			}
		})
	}
}

func TestBuiltinTMUXFocusFailureCleansUp(t *testing.T) {
	calls := mockTMUXLaunch(t, "/dev/tty1\t$0\t%7\tattached,focused\n")
	originalRun := runCommand
	runCommand = func(cmd *exec.Cmd) error {
		if cmd.Args[1] == "select-pane" {
			return fmt.Errorf("focus failed")
		}
		return originalRun(cmd)
	}
	if _, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, false); err == nil ||
		!slices.Equal(tmuxCall(*calls, "kill-pane"), []string{"kill-pane", "-t", "%99"}) {
		t.Fatalf("failed startup leaked the review pane: calls=%v err=%v", *calls, err)
	}
}

func TestBuiltinTMUXSplitRetriesWithoutPercentage(t *testing.T) {
	calls := mockTMUXLaunch(t, "")
	original := commandOutput
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[1] == "split-window" && slices.Contains(cmd.Args, "-p") {
			*calls = append(*calls, cmd.Args[1:])
			return nil, fmt.Errorf("size unavailable")
		}
		return original(cmd)
	}
	if _, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, false); err != nil {
		t.Fatal(err)
	}
	var spawns [][]string
	for _, call := range *calls {
		if call[0] == "split-window" {
			spawns = append(spawns, call)
		}
	}
	if len(spawns) != 2 || slices.Contains(spawns[1], "-p") || !slices.Contains(spawns[1], "-d") {
		t.Fatalf("incorrect split retry: %v", spawns)
	}
}

func TestBuiltinTMUXCloseAfterSourceRemoved(t *testing.T) {
	calls := mockTMUXLaunch(t, "/dev/tty1\t$0\t%99\tfocused\n")
	launch, err := spawnTMUXPane(&reviewMode{sessionKey: "0123456789ab"}, tmuxContext{}, false)
	if err != nil {
		t.Fatal(err)
	}
	*calls = nil
	original := commandOutput
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[1] == "display-message" {
			return nil, fmt.Errorf("source removed")
		}
		return original(cmd)
	}
	launch.close()
	if tmuxCall(*calls, "select-window") != nil || tmuxCall(*calls, "select-pane") != nil || tmuxCall(*calls, "kill-pane") == nil {
		t.Fatalf("cleanup tried to restore a deleted source: %v", *calls)
	}
}
