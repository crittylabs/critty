package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFinishPromptDirectoryPrecedence(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	project := t.TempDir()
	dirs := []string{
		filepath.Join(configHome, "critty", "prompts"),
		filepath.Join(project, ".critty", "prompts"),
	}
	ctx := Context{Approved: true, Mode: "diff"}
	for i, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A newer generic prompt also overrides an older mode-specific one.
		if err := os.WriteFile(filepath.Join(dir, "on_finish_approved.diff.md"), []byte("mode-specific"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := RenderFinish(nil, project, ctx); got != "mode-specific" {
			t.Fatalf("layer %d mode-specific prompt = %q", i, got)
		}
		if err := os.Remove(filepath.Join(dir, "on_finish_approved.diff.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "on_finish_approved.md"), []byte(dir), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := RenderFinish(nil, project, ctx); got != dir {
			t.Fatalf("layer %d prompt = %q, want %q", i, got, dir)
		}
		if err := os.WriteFile(filepath.Join(dir, "on_finish_approved.diff.md"), []byte("older mode-specific"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := RenderFinish(map[string]string{"on_finish_approved": "inline:configured"}, project, ctx); got != "configured" {
		t.Fatalf("configured prompt = %q", got)
	}
}
