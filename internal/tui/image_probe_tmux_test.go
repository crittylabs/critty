package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestImageProbeRequiresSelectedTMUXPane(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tmux requires a Unix host")
	}
	for _, tc := range []struct {
		name, pane, clients string
		status              int
		want                bool
	}{
		{"selected", "%1", "@1\n%1 @1\n", 0, true},
		{"background", "%1", "@1\n%2 @1\n", 0, false},
		{"no clients", "%1", "@1\n", 0, false},
		{"unrelated session", "%1", "@1\n%1 @1\n%2 @2\n", 0, true},
		{"multiple windows", "%1", "@1\n%2 @2 @3\n%1 @3 @1\n", 0, true},
		{"linked window", "%1", "@1\n%1 @1\n%2 @2 @1\n", 0, false},
		{"shared pane", "%1", "@1\n%1 @1\n%1 @1\n", 0, false},
		{"hidden window", "%1", "@1\n%2 @1 @2\n", 0, false},
		{"unattached window", "%1", "@1\n%2 @2\n", 0, false},
		{"unknown pane", "", "@1\n%1 @1\n", 0, false},
		{"query failed", "%1", "@1\n%1 @1\n", 1, false},
		{"missing window", "%1", "\n%1 @1\n", 0, false},
		{"missing client windows", "%1", "@1\n%1\n", 0, false},
		{"window ID prefix", "%1", "@1\n%1 @10\n", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := fmt.Sprintf("#!/bin/sh\n[ \"$*\" = '-S /socket,with,commas display-message -p -t %s #{window_id} ; list-clients -F #{pane_id} #{W:#{window_id} }' ] || exit 99\nprintf '%%s' '%s'\nexit %d\n", tc.pane, tc.clients, tc.status)
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			t.Setenv("TMUX", "/socket,with,commas,123,0")
			t.Setenv("TMUX_PANE", tc.pane)
			if got := imageProbeAllowed(); got != tc.want {
				t.Fatalf("probe allowed = %t, want %t", got, tc.want)
			}
		})
	}
}
