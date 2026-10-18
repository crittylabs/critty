package xdg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateHome(t *testing.T) {
	for _, base := range []string{t.TempDir(), "relative-state"} {
		t.Run(base, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_STATE_HOME", base)
			if got := StateHome(); got != filepath.Join(base, "critty") {
				t.Fatalf("StateHome() = %q", got)
			}
			if _, err := os.Stat(StateHome()); !os.IsNotExist(err) {
				t.Fatalf("path lookup created state: %v", err)
			}
		})
	}
}
