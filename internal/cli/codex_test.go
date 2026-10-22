package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexWrapperHint(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", bin+string(os.PathSeparator), "../../cmd/...")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	self := filepath.Join(bin, "critty")
	other := filepath.Join(dir, "other")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(bin, alias); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		dirs []string
		want bool
	}{
		{"shadowed", []string{other, bin}, true},
		{"wrapper first", []string{bin, other}, false},
		{"wrapper absent", []string{other}, false},
		{"alias first", []string{alias, other, bin}, false},
		{"alias shadowed", []string{other, alias}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hint := codexWrapperHint(self, strings.Join(tt.dirs, string(os.PathListSeparator)))
			if (hint != "") != tt.want {
				t.Fatalf("hint = %q, want present: %v", hint, tt.want)
			}
			if tt.want && (!strings.Contains(hint, other) || !strings.Contains(hint, "[tools]")) {
				t.Fatalf("missing path or mise advice: %s", hint)
			}
		})
	}

	// Exercise the nonterminal review error with an isolated saved-state root.
	for _, key := range []string{"TMUX", "TMUX_PANE", "HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID", "CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID"} {
		t.Setenv(key, "")
	}
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, filepath.Join(dir, key))
	}
	t.Setenv("PATH", other+string(os.PathListSeparator)+bin)
	doc := filepath.Join(dir, "review.md")
	if err := os.WriteFile(doc, []byte("Review this document.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, doc)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "no tmux, Herdr, or cmux terminal detected") || !strings.Contains(string(out), "hint:") {
		t.Fatalf("review error = %v, output: %s", err, out)
	}

	failed := launcherScript(t, filepath.Join(dir, "failed,launcher"), "echo custom unavailable >&2\nexit 1\n")
	cmd = exec.Command(self, doc, "--launcher", "mux", "--launcher", "custom:"+failed)
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "launcher mux: no tmux, Herdr, or cmux terminal detected") ||
		!strings.Contains(string(out), "launcher custom:"+failed) || strings.Count(string(out), "custom unavailable") != 1 {
		t.Fatalf("combined launcher failure = %v, output: %s", err, out)
	}

	// Do not mistake an original Codex beside a critty-only install for our wrapper.
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if hint := codexWrapperHint(self, os.Getenv("PATH")); hint != "" {
		t.Fatalf("hint without bundled wrapper: %s", hint)
	}
}
