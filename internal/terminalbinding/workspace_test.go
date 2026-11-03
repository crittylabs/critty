package terminalbinding

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestWorkspaceCommandsPinIdentity(t *testing.T) {
	t.Setenv("ORCA_USER_DATA_PATH", "/wrong-profile")
	t.Setenv("ORCA_RUNTIME_SOURCE_ID", "wrong-source")
	t.Setenv("ORCA_ENVIRONMENT", "wrong-host")
	target := Target{Kind: "orca", Socket: "/profile", Workspace: "workspace1", Pane: "terminal1", SourceID: "source1", RuntimeID: "runtime1"}
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	cmd := target.Command(context.Background(), "status", "--json")
	for _, entry := range cmd.Env {
		if strings.Contains(entry, "wrong-") {
			t.Fatalf("unrelated host leaked: %s", entry)
		}
	}
	for _, value := range []string{"ORCA_USER_DATA_PATH=/profile", "ORCA_RUNTIME_SOURCE_ID=source1", "ORCA_RUNTIME_SOURCE_INCARNATION=runtime1"} {
		if !slices.Contains(cmd.Env, value) {
			t.Errorf("missing %s", value)
		}
	}
}

func TestWorkspaceSnapshotRequiresScreen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix host")
	}
	for _, tc := range []struct {
		name, host, reply string
		wantError         bool
	}{
		{"orca screen", "orca", `{"ok":true,"result":{"terminal":{"source":"screen","tail":["CRITTY-marker"]}}}`, false},
		{"orca history", "orca", `{"ok":true,"result":{"terminal":{"source":"stream","tail":["CRITTY-marker"]}}}`, true},
		{"orca incomplete", "orca", `{"ok":true,"result":{"terminal":{"source":"screen"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			data := filepath.Join(dir, "reply.json")
			if err := os.WriteFile(data, []byte(tc.reply), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, WorkspaceExecutable(tc.host)), []byte("#!/bin/sh\nexec /bin/cat '"+data+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			target := Target{Kind: tc.host, Socket: "/profile", Workspace: "workspace1", Pane: "terminal1", SourceID: "source1", RuntimeID: "runtime1"}
			text, err := Snapshot(context.Background(), target)
			if (err != nil) != tc.wantError || !tc.wantError && text != "CRITTY-marker" {
				t.Fatalf("snapshot = %q, %v", text, err)
			}
		})
	}
}
