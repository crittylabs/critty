package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestImageProbeRequiresSelectedTMUXPane(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tmux requires a Unix host")
	}
	dir := t.TempDir()
	const script = `#!/bin/sh
[ "$*" = '-S /socket,with,commas show-options -A -p -v -t %1 allow-passthrough ; display-message -p -t %1 #{window_id} ; list-clients -F #{pane_id} #{window_id} #{W:#{window_id} }' ] || exit 99
printf 'on\n@1\n%%1 @1 @1\n'
exit "$TMUX_TEST_STATUS"
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("TMUX", "/socket,with,commas,123,0")
	t.Setenv("TMUX_PANE", "%1")
	t.Setenv("TMUX_TEST_STATUS", "0")
	if !imageProbeAllowed() {
		t.Fatal("selected pane cannot probe")
	}
	t.Setenv("TMUX_PANE", "")
	if imageProbeAllowed() {
		t.Fatal("unknown pane allowed a probe")
	}
	t.Setenv("TMUX_PANE", "%1")
	t.Setenv("TMUX_TEST_STATUS", "1")
	if imageProbeAllowed() {
		t.Fatal("failed query allowed a probe")
	}
}

func TestTMUXProbeHasSingleReceiver(t *testing.T) {
	for _, tc := range []struct {
		name, mode, clients string
		want                bool
	}{
		{"selected", "on", "@1\n%1 @1 @1\n", true},
		{"selected with all", "all", "@1\n%1 @1 @1\n", true},
		{"disabled", "off", "@1\n%1 @1 @1\n", false},
		{"unknown mode", "unknown", "@1\n%1 @1 @1\n", false},
		{"missing mode", "", "@1\n%1 @1 @1\n", false},
		{"background", "on", "@1\n%2 @1 @1\n", false},
		{"no clients", "on", "@1\n", false},
		{"unrelated session", "all", "@1\n%1 @1 @1\n%2 @2 @2\n", true},
		{"multiple windows", "all", "@1\n%2 @2 @2 @3\n%1 @1 @3 @1\n", true},
		{"hidden linked window with on", "on", "@1\n%1 @1 @1\n%2 @2 @2 @1\n", true},
		{"hidden linked window with all", "all", "@1\n%1 @1 @1\n%2 @2 @2 @1\n", false},
		{"shared pane", "on", "@1\n%1 @1 @1\n%1 @1 @1\n", false},
		{"shared pane with all", "all", "@1\n%1 @1 @1\n%1 @1 @1\n", false},
		{"hidden window", "on", "@1\n%2 @2 @1 @2\n", false},
		{"hidden window with all", "all", "@1\n%2 @2 @1 @2\n", false},
		{"unattached window", "all", "@1\n%2 @2 @2\n", false},
		{"missing window", "on", "\n%1 @1 @1\n", false},
		{"missing client windows", "all", "@1\n%1 @1\n", false},
		{"window ID prefix", "on", "@1\n%1 @10 @10\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tmuxProbeHasSingleReceiver("%1", tc.mode+"\n"+tc.clients); got != tc.want {
				t.Fatalf("probe allowed = %t, want %t", got, tc.want)
			}
		})
	}
}
