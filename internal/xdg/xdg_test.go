package xdg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateHome(t *testing.T) {
	for _, mode := range []string{"fresh", "legacy", "alias", "collision", "blocked", "legacy symlink"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			t.Setenv("XDG_STATE_HOME", base)
			current, legacy := filepath.Join(base, "critty"), filepath.Join(base, "tcrit")
			want := current
			if mode != "fresh" {
				target := legacy
				if mode == "legacy symlink" {
					target = filepath.Join(base, "external")
				}
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if target != legacy {
					if err := os.Symlink(target, legacy); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(legacy, "saved"), []byte("review"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "alias":
				if err := os.Symlink("tcrit", current); err != nil {
					t.Fatal(err)
				}
			case "collision":
				if err := os.Mkdir(current, 0700); err != nil {
					t.Fatal(err)
				}
				want = legacy
			case "blocked":
				if err := os.WriteFile(current, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				want = legacy
			}
			for range 2 {
				if got := StateHome(); got != want {
					t.Fatalf("StateHome() = %q, want %q", got, want)
				}
			}
			if mode != "collision" && mode != "blocked" {
				old, err := os.Stat(legacy)
				if err != nil {
					t.Fatal(err)
				}
				currentInfo, err := os.Stat(current)
				if err != nil || !os.SameFile(old, currentInfo) {
					t.Fatalf("state directories differ: %v", err)
				}
			}
			if mode != "fresh" {
				data, err := os.ReadFile(filepath.Join(want, "saved"))
				if err != nil || string(data) != "review" {
					t.Fatalf("saved data = %q, %v", data, err)
				}
			}
			if mode == "blocked" {
				data, err := os.ReadFile(current)
				if err != nil || string(data) != "keep" {
					t.Fatalf("collision overwritten: %q, %v", data, err)
				}
			}
		})
	}
}

func TestStateHomeRelativeBase(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", "state")
	if err := os.MkdirAll("state/tcrit", 0700); err != nil {
		t.Fatal(err)
	}
	if got := StateHome(); got != filepath.Join("state", "critty") {
		t.Fatalf("StateHome() = %q", got)
	}
	old, err := os.Stat("state/tcrit")
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat("state/critty")
	if err != nil || !os.SameFile(old, current) {
		t.Fatalf("relative alias lost data: %v", err)
	}
}
