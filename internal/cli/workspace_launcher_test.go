package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

func testWorkspaceContext(host string) workspaceContext {
	return workspaceContext{terminalbinding.Target{Kind: host, Socket: "/profile", Workspace: "workspace1", Pane: "source1", SourceID: "installation1", RuntimeID: "runtime1"}}
}

func TestWorkspaceReviewLaunch(t *testing.T) {
	for _, tc := range []struct {
		name, created string
		wantError     bool
	}{
		{"orca", `{"ok":true,"result":{"terminal":{"handle":"review1","worktreeId":"workspace1","executionHostId":"local"}}}`, false},
		{"orca missing handle", `{"ok":true,"result":{"terminal":{}}}`, true},
		{"orca wrong workspace", `{"ok":true,"result":{"terminal":{"handle":"review1","worktreeId":"other","executionHostId":"local"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldOutput, oldRun, oldLook, oldResolve := commandOutput, runCommand, lookPath, resolveExec
			t.Cleanup(func() { commandOutput, runCommand, lookPath, resolveExec = oldOutput, oldRun, oldLook, oldResolve })
			lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
			resolveExec = func() (string, error) { return "/build with spaces/critty", nil }
			t.Setenv("XDG_STATE_HOME", "/state with spaces")
			var calls [][]string
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				calls = append(calls, cmd.Args)
				i := slices.Index(cmd.Args, "--command")
				if i < 0 || !strings.Contains(cmd.Args[i+1], "'/build with spaces/critty' _tui --session '0123456789ab'") || !strings.Contains(cmd.Args[i+1], "XDG_STATE_HOME='/state with spaces'") {
					t.Fatalf("command lost quoted arguments: %v", cmd.Args)
				}
				return []byte(tc.created), nil
			}
			runCommand = func(cmd *exec.Cmd) error {
				calls = append(calls, cmd.Args)
				return nil
			}
			launch, err := spawnWorkspaceReview(&reviewMode{sessionKey: "0123456789ab"}, testWorkspaceContext("orca"))
			if (err != nil) != tc.wantError {
				t.Fatalf("launch = %+v, %v", launch, err)
			}
			if !tc.wantError {
				if launch.terminal != "review1" {
					t.Fatal("review terminal not retained")
				}
				if err := launch.close(); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Contains(tc.name, "wrong workspace") || !tc.wantError {
				last := calls[len(calls)-1]
				if !slices.Contains(last, "review1") || slices.Contains(last, "source1") {
					t.Fatalf("cleanup targeted another terminal: %v", last)
				}
			}
			before := len(calls)
			launch.restoreFocus()
			if len(calls) != before {
				t.Fatal("cleanup stole focus")
			}
		})
	}
}

func TestWorkspaceCloseRequiresVerifiedAbsence(t *testing.T) {
	for _, state := range []string{"absent", "present", "unreadable", "incomplete"} {
		t.Run(state, func(t *testing.T) {
			oldOutput, oldRun := commandOutput, runCommand
			t.Cleanup(func() { commandOutput, runCommand = oldOutput, oldRun })
			runCommand = func(*exec.Cmd) error { return fmt.Errorf("already exited or close failed") }
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if state == "unreadable" {
					return nil, fmt.Errorf("host unavailable")
				}
				if state == "incomplete" {
					return []byte(`{}`), nil
				}
				terminals := []orcaTerminal{}
				if state == "present" {
					terminals = append(terminals, orcaTerminal{Handle: "review1"})
				}
				data, err := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"terminals": terminals, "hostScope": map[string]any{"hostIds": []string{"local"}}}})
				if err != nil {
					t.Fatal(err)
				}
				return data, nil
			}
			err := (workspaceLaunch{context: testWorkspaceContext("orca"), bin: "/bin/orca", terminal: "review1"}).close()
			if (err == nil) != (state == "absent") {
				t.Fatalf("close = %v", err)
			}
		})
	}
}
