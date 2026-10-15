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
		{"selected", "%1", "%1\n", 0, true},
		{"background", "%1", "%2\n", 0, false},
		{"no clients", "%1", "", 0, false},
		{"multiple clients", "%1", "%1\n%2\n", 0, false},
		{"shared pane", "%1", "%1\n%1\n", 0, false},
		{"unknown pane", "", "%1\n", 0, false},
		{"query failed", "%1", "%1\n", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := fmt.Sprintf("#!/bin/sh\n[ \"$*\" = '-S /socket,with,commas list-clients -F #{pane_id}' ] || exit 99\nprintf '%%s' '%s'\nexit %d\n", tc.clients, tc.status)
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
