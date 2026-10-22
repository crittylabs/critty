package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

func TestTerminalNotificationRoutesReview(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX", "/recorded/socket,123,0")
	t.Setenv("TMUX_PANE", "%42")
	for _, key := range []string{"HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID", "CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID"} {
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
	for _, cmd := range []string{"review", "plan"} {
		command, _, err := rootCmd.Find([]string{cmd})
		if err != nil || command.Flags().Lookup("terminal-request") == nil {
			t.Fatalf("%s does not accept terminal request: %v", cmd, err)
		}
	}
}

func TestCMUXTerminalNotificationRoutesReview(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := terminalbinding.DefaultStore()
	request, err := store.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	target := terminalbinding.Target{Kind: "cmux", Socket: "/recorded/socket", Workspace: "w1", Pane: "s1"}
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
}
