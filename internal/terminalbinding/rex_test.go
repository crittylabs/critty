package terminalbinding

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRexTarget(t *testing.T) {
	session := "session:00000000-0000-0000-0000-000000000001"
	block := "block:00000000-0000-0000-0000-000000000002"
	for _, tc := range []struct {
		name, server, session, block string
		valid                        bool
	}{
		{"local", "unix:///tmp/rex.sock", session, block, true},
		{"remote", "https://example.test", session, block, false},
		{"label endpoint", "work", session, block, false},
		{"missing endpoint", "", session, block, false},
		{"unix host", "unix://other/tmp/rex.sock", session, block, false},
		{"session label", "unix:///tmp/rex.sock", "work", block, false},
		{"block suffix", "unix:///tmp/rex.sock", session, "0002", false},
		{"missing block", "unix:///tmp/rex.sock", session, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := Target{Kind: "rex", Socket: tc.server, Workspace: tc.session, Pane: tc.block}
			if err := target.Validate(); (err == nil) != tc.valid {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
	t.Setenv("REX_SERVER", "/tmp/rex.sock")
	t.Setenv("REX_SESSION", session)
	t.Setenv("REX_BLOCK", block)
	target := RexEnvironment()
	if target.Socket != "unix:///tmp/rex.sock" {
		t.Fatal(target)
	}
	t.Setenv("REX_CLIENT", "unrelated-client")
	t.Setenv("REX_SERVER", "wrong-server")
	cmd := target.Command(context.Background(), "focus", "-s", session, block)
	if !slices.Equal(cmd.Args[1:6], []string{"--server", target.Socket, "--autostart=false", "--timeout", "2s"}) {
		t.Fatal(cmd.Args)
	}
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "REX_") {
			t.Fatalf("inherited context leaked: %s", entry)
		}
	}
	if _, err := Snapshot(context.Background(), target); err == nil {
		t.Fatal("scrollback must not be used as a visible marker screen")
	}
}

func TestRexDefaultEndpointAndOwners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI")
	}
	clearTerminalEnv(t)
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *whoami*) printf '%s' '{"endpoint":"unix:///tmp/rex.sock"}' ;;
  *process*) printf '%s' '{"child":{"pid":123}}' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "rex"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("REX_SERVER", "")
	t.Setenv("REX_SESSION", "session:00000000-0000-0000-0000-000000000001")
	t.Setenv("REX_BLOCK", "block:00000000-0000-0000-0000-000000000002")
	target, err := FromEnvironment()
	if err != nil || target.Socket != "unix:///tmp/rex.sock" {
		t.Fatalf("target = %+v, %v", target, err)
	}
	if owners := RexOwners(context.Background(), target); owners[123] != target {
		t.Fatal(owners)
	}
	s := testStore(t)
	request, err := s.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Notify(request.ID, target); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resolved, issues, err := s.ResolveWithDiagnostics(ctx, request.ID, func(context.Context, Target) (string, error) {
		t.Fatal("notification must not read scrollback")
		return "", nil
	})
	if err != nil || len(issues) != 0 || resolved != target {
		t.Fatalf("resolved = %+v, issues = %+v, error = %v", resolved, issues, err)
	}
}
