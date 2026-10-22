package terminalbinding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearTerminalEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"TMUX", "TMUX_PANE", "HERDR_SOCKET_PATH", "HERDR_CONFIG_PATH", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID", "CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID"} {
		t.Setenv(key, "")
	}
}

func TestEnvironmentTargets(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("TMUX", "/tmp/comma,socket,123,0")
	t.Setenv("TMUX_PANE", "%7")
	target, err := FromEnvironment()
	if err != nil || target.Socket != "/tmp/comma,socket" || target.Pane != "%7" {
		t.Fatalf("%+v %v", target, err)
	}
	clearTerminalEnv(t)
	t.Setenv("HERDR_SOCKET_PATH", "/custom/server.sock")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("HERDR_TAB_ID", "w1:t1")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	target, err = FromEnvironment()
	if err != nil || target.Socket != "/custom/server.sock" || target.Kind != "herdr" {
		t.Fatalf("%+v %v", target, err)
	}
}

func TestSnapshotUsesVisiblePaneAndPinnedSocket(t *testing.T) {
	dir := t.TempDir()
	for _, kind := range []string{"tmux", "herdr", "cmux"} {
		t.Run(kind, func(t *testing.T) {
			script := `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf '%s' "$HERDR_SOCKET_PATH" > "$SOCKET_FILE"
`
			script += "printf 'CRITTY-visible'\n"
			if err := os.WriteFile(filepath.Join(dir, kind), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			t.Setenv("ARGS_FILE", filepath.Join(dir, "args"))
			t.Setenv("SOCKET_FILE", filepath.Join(dir, "socket"))
			t.Setenv("HERDR_SOCKET_PATH", "/wrong/socket")
			target := testTarget("%7")
			want := "-S\n/tmp/test.sock\ncapture-pane\n-p\n-J\n-t\n%7\n"
			if kind == "herdr" {
				target = Target{Kind: "herdr", Socket: "/right/socket", Workspace: "w1", Tab: "w1:t1", Pane: "w1:p1"}
				want = "pane\nread\nw1:p1\n--source\nvisible\n--format\ntext\n"
			}
			if kind == "cmux" {
				target = Target{Kind: "cmux", Socket: "/right/socket", Workspace: "w1", Pane: "s1"}
				want = "--socket\n/right/socket\nread-screen\n--workspace\nw1\n--surface\ns1\n"
			}
			text, err := Snapshot(context.Background(), target)
			if err != nil || text != "CRITTY-visible" {
				t.Fatalf("snapshot %q %v", text, err)
			}
			args, err := os.ReadFile(filepath.Join(dir, "args"))
			if err != nil {
				t.Fatal(err)
			}
			if string(args) != want {
				t.Fatalf("args %q", args)
			}
			if kind == "herdr" {
				b, err := os.ReadFile(filepath.Join(dir, "socket"))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != "/right/socket" {
					t.Fatalf("socket: %s", b)
				}
			}
		})
	}
}

func TestInvalidTargets(t *testing.T) {
	for _, target := range []Target{{Kind: "tmux", Socket: "relative", Pane: "%1"}, {Kind: "tmux", Socket: "/s", Pane: "-a"}, {Kind: "herdr", Socket: "/s", Pane: "p"}, {Kind: "other", Socket: "/s", Pane: "p"}, {Kind: "tmux", Socket: "/s", Pane: "%" + strings.Repeat("9", 30)}} {
		if target.Validate() == nil {
			t.Fatalf("accepted %+v", target)
		}
	}
}
