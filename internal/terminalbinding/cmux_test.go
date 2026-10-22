package terminalbinding

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCMUXProcessTargets(t *testing.T) {
	data := []byte(`{"windows":[{"workspaces":[{"id":"w1","panes":[{"surfaces":[{"id":"s1","top_level_pids":[100],"processes":[{"pid":200,"children":[{"pid":300}]}]},{"id":"s2","top_level_pids":[400]}]}]},{"id":"w2","panes":[{"surfaces":[{"id":"s3","top_level_pids":[500]}]}]}]}]}`)
	owners := CMUXProcessTargets(data, "/socket")
	for _, tc := range []struct {
		pid                int
		workspace, surface string
	}{{100, "w1", "s1"}, {200, "w1", "s1"}, {300, "w1", "s1"}, {400, "w1", "s2"}, {500, "w2", "s3"}} {
		if got := owners[tc.pid]; got != (Target{Kind: "cmux", Socket: "/socket", Workspace: tc.workspace, Pane: tc.surface}) {
			t.Errorf("pid %d = %+v", tc.pid, got)
		}
	}
	if got := CMUXProcessTargets([]byte(`{`), "/socket"); got != nil {
		t.Errorf("invalid JSON = %v", got)
	}
}

func TestNestedTMUXWinsSharedCMUXProcess(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("TMUX", "/tmux/socket,100,2")
	t.Setenv("TMUX_PANE", "%7")
	t.Setenv("CMUX_SOCKET_PATH", "/cmux/socket")
	t.Setenv("CMUX_WORKSPACE_ID", "w1")
	t.Setenv("CMUX_SURFACE_ID", "s1")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	pid := os.Getppid()
	scripts := map[string]string{
		"tmux": fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%d'\n", pid),
		"cmux": fmt.Sprintf("#!/bin/sh\nprintf '%%s' '{\"windows\":[{\"workspaces\":[{\"id\":\"w1\",\"panes\":[{\"surfaces\":[{\"id\":\"s1\",\"top_level_pids\":[%d]}]}]}]}]}'\n", pid),
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FromEnvironment()
	if err != nil || got != (Target{Kind: "tmux", Socket: "/tmux/socket", Pane: "%7"}) {
		t.Fatalf("target = %+v, %v", got, err)
	}
}

func TestCMUXEnvironmentTarget(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("CMUX_SOCKET_PATH", "/cmux/socket")
	t.Setenv("CMUX_WORKSPACE_ID", "w1")
	t.Setenv("CMUX_SURFACE_ID", "s1")
	got, err := FromEnvironment()
	if err != nil || got != (Target{Kind: "cmux", Socket: "/cmux/socket", Workspace: "w1", Pane: "s1"}) {
		t.Fatalf("target = %+v, %v", got, err)
	}
}
