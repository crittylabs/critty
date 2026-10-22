package terminalbinding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestZellijOwnersUsesNearestServer(t *testing.T) {
	target := Target{Kind: "zellij", Workspace: "inner", Pane: "0"}
	chain := map[int]processInfo{
		500: {400, "agent"}, 400: {300, "/bin/zsh"},
		300: {200, "/opt/bin/zellij"}, 200: {100, "/bin/zsh"},
		100: {1, "zellij"},
	}
	owners := zellijOwners(target, 500, func(pid int) (processInfo, error) {
		p, ok := chain[pid]
		if !ok {
			return processInfo{}, fmt.Errorf("unknown process")
		}
		return p, nil
	})
	if len(owners) != 1 || owners[400] != target {
		t.Fatalf("owners = %v", owners)
	}
}

func TestZellijEnvironmentAndSnapshot(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("ZELLIJ_SESSION_NAME", "review session")
	t.Setenv("ZELLIJ_PANE_ID", "0")
	t.Setenv("ZELLIJ_SOCKET_DIR", "/right/socket")
	target, err := FromEnvironment()
	if err != nil || target != (Target{Kind: "zellij", Socket: "/right/socket", Workspace: "review session", Pane: "0"}) {
		t.Fatalf("target = %+v, %v", target, err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --session ] && [ \"$2\" = 'review session' ] && [ \"$3\" = action ] && [ \"$4\" = dump-screen ] && [ \"$5\" = --pane-id ] && [ \"$6\" = 0 ] || exit 1\nprintf 'visible marker'\n"
	if err := os.WriteFile(filepath.Join(dir, "zellij"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ZELLIJ_SESSION_NAME", "stale")
	text, err := Snapshot(context.Background(), target)
	if err != nil || text != "visible marker" {
		t.Fatalf("snapshot = %q, %v", text, err)
	}
	cmd := target.Command(context.Background(), "action", "list-panes")
	if !slices.Contains(cmd.Env, "ZELLIJ_SESSION_NAME=review session") || slices.Contains(cmd.Env, "ZELLIJ_SESSION_NAME=stale") {
		t.Fatalf("stale session in command environment")
	}
	target.Socket = ""
	cmd = target.Command(context.Background(), "action", "list-panes")
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "ZELLIJ_SOCKET_DIR=") {
			t.Fatal("default socket inherited stale directory")
		}
	}
}

func TestZellijTargetValidation(t *testing.T) {
	for _, tc := range []struct {
		session, pane, socket string
		valid                 bool
	}{
		{"review", "0", "", true}, {"review session", "12", "/socket", true},
		{"", "0", "", false}, {"../other", "0", "", false}, {"-other", "0", "", false},
		{"review", "", "", false}, {"review", "plugin_0", "", false}, {"review", "-1", "", false},
		{"review", "4294967296", "", false}, {"review", "0", "relative", false},
	} {
		t.Run(tc.session+"/"+tc.pane+"/"+tc.socket, func(t *testing.T) {
			err := (Target{Kind: "zellij", Workspace: tc.session, Pane: tc.pane, Socket: tc.socket}).Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("validation = %v", err)
			}
		})
	}
}
