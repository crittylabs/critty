package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLauncherConfigPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, project string
		want          []string
	}{
		{"inherit", "", []string{"mux", "custom:global"}},
		{"override", `launchers = ["custom:project", "mux"]`, []string{"custom:project", "mux"}},
		{"clear", `launchers = []`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, project := t.TempDir(), t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", home)
			writeConfig(t, GlobalPath(), `launchers = ["mux", "custom:global"]`)
			writeConfig(t, ProjectPath(project), tc.project)
			cfg, err := Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cfg.Launchers, tc.want) {
				t.Fatalf("Launchers = %v, want %v", cfg.Launchers, tc.want)
			}
		})
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergesGlobalAndProject(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	project := t.TempDir()

	writeConfig(t, GlobalPath(), `
		author = "Global Author"
		quiet = true
		cleanup_on_approve = true
		ignore_patterns = ["*.lock"]
		[prompts]
		on_finish_approved = "inline:global"
		on_finish_unresolved = "inline:global"
	`)
	writeConfig(t, ProjectPath(project), `
		author = "Project Author"
		quiet = false
		ignore_patterns = ["*.min.js", "*.lock"]
		[prompts]
		on_finish_unresolved = "inline:project"
	`)

	cfg, err := Load(project)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if cfg.Author != "Project Author" {
		t.Errorf("Author = %q, want project override", cfg.Author)
	}
	if cfg.Quiet {
		t.Error("project quiet=false should override global true")
	}
	if !cfg.CleanupOnApprove {
		t.Error("global cleanup_on_approve=true should enable cleanup")
	}
	want := []string{"*.lock", "*.min.js"}
	if len(cfg.IgnorePatterns) != len(want) {
		t.Fatalf("IgnorePatterns = %v, want union %v", cfg.IgnorePatterns, want)
	}
	if cfg.Prompts["on_finish_unresolved"] != "inline:project" {
		t.Errorf("project prompt should win, got %q", cfg.Prompts["on_finish_unresolved"])
	}
	if cfg.Prompts["on_finish_approved"] != "inline:global" {
		t.Errorf("global prompt should survive, got %q", cfg.Prompts["on_finish_approved"])
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if cfg.CleanupOnApprove {
		t.Error("cleanup_on_approve should default to false")
	}
	if cfg.Split {
		t.Fatal("split should default to false")
	}
	if cfg.History.Start != "auto" || cfg.History.End != "auto" || cfg.History.KeepDays != 30 || cfg.History.MaxBytes != 1<<30 {
		t.Fatalf("unexpected default history policy: %+v", cfg.History)
	}
}

func TestSplitConfig(t *testing.T) {
	for _, tc := range []struct {
		name, global, project string
		want                  bool
	}{
		{"global", "split = true", "", true},
		{"project disables", "split = true", "split = false", false},
		{"project enables", "split = false", "split = true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			project := t.TempDir()
			writeConfig(t, GlobalPath(), tc.global)
			writeConfig(t, ProjectPath(project), tc.project)
			cfg, err := Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Split != tc.want {
				t.Fatalf("split = %v, want %v", cfg.Split, tc.want)
			}
		})
	}
}

func TestHistoryPolicyIsGlobalOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	writeConfig(t, ProjectPath(project), `[history]
start = "delete"
end = "delete"
keep_days = 1
max_bytes = 1
`)
	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.History.Start != "auto" || cfg.History.End != "auto" {
		t.Fatalf("project enabled automatic deletion: %+v", cfg.History)
	}
	writeConfig(t, GlobalPath(), `[history]
start = "archive"
end = "ask"
keep_days = 7
max_bytes = 12345
`)
	cfg, err = Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.History.Start != "archive" || cfg.History.End != "ask" || cfg.History.KeepDays != 7 || cfg.History.MaxBytes != 12345 {
		t.Fatalf("global policy not respected: %+v", cfg.History)
	}
	writeConfig(t, GlobalPath(), `[history]
end = "typo"
`)
	if _, err := Load(project); err == nil {
		t.Fatal("invalid history policy accepted")
	}
}
