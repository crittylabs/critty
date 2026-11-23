package cli

import (
	"os/exec"
	"slices"
	"testing"
)

func TestTmuxAdvertisesRGB(t *testing.T) {
	originalOutput := commandOutput
	t.Cleanup(func() { commandOutput = originalOutput })
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("TMUX", "/socket,1,0")
	t.Setenv("TMUX_PANE", "%42")
	t.Setenv("WT_SESSION", "")
	t.Setenv("GOOGLE_CLOUD_SHELL", "")
	for _, tc := range []struct {
		name, features, noColor string
		want, probe             bool
	}{
		{"RGB client", "256,mouse,RGB,sync\n", "", true, true},
		{"256-color client", "256,mouse,sync\n", "", false, true},
		{"color disabled", "256,RGB\n", "1", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.noColor)
			probed := false
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				probed = true
				want := []string{"tmux", "display-message", "-p", "-t", "%42", "#{client_termfeatures}"}
				if !slices.Equal(cmd.Args, want) {
					t.Fatalf("tmux command = %q, want %q", cmd.Args, want)
				}
				return []byte(tc.features), nil
			}
			if got := tmuxAdvertisesRGB(); got != tc.want || probed != tc.probe {
				t.Errorf("recognized RGB = %t, probed = %t, want %t and %t", got, probed, tc.want, tc.probe)
			}
		})
	}
}
