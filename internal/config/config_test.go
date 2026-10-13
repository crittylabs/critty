package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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

	writeConfig(t, filepath.Join(configHome, "critty", "config.json"), `{
		"author": "Global Author",
		"quiet": true,
		"cleanup_on_approve": false,
		"ignore_patterns": ["*.lock"],
		"prompts": {"on_finish_approved": "inline:global", "on_finish_unresolved": "inline:global"}
	}`)
	writeConfig(t, ProjectPath(project), `{
		"author": "Project Author",
		"quiet": false,
		"ignore_patterns": ["*.min.js", "*.lock"],
		"prompts": {"on_finish_unresolved": "inline:project"}
	}`)

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
	if cfg.CleanupOnApprove {
		t.Error("global cleanup_on_approve=false should override the default true")
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

func TestLoadConfigPrecedence(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("%d layers", count), func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)
			project := t.TempDir()
			paths := []string{
				filepath.Join(configHome, "tcrit", "config.json"),
				filepath.Join(configHome, "critty", "config.json"),
				filepath.Join(project, ".tcrit.config.json"),
				filepath.Join(project, ".critty.config.json"),
			}
			for i, path := range paths[:count] {
				data, err := json.Marshal(map[string]any{
					"author":          fmt.Sprintf("layer %d", i),
					"quiet":           i%2 == 0,
					"ignore_patterns": []string{fmt.Sprintf("pattern %d", i)},
					"prompts": map[string]string{
						fmt.Sprintf("hook %d", i): "inline:kept",
						"shared":                  fmt.Sprintf("inline:layer %d", i),
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				writeConfig(t, path, string(data))
			}
			cfg, err := Load(project)
			if err != nil {
				t.Fatal(err)
			}
			last := count - 1
			if cfg.Author != fmt.Sprintf("layer %d", last) || cfg.Quiet != (last%2 == 0) || cfg.Prompts["shared"] != fmt.Sprintf("inline:layer %d", last) {
				t.Fatalf("last layer did not override earlier values: %+v", cfg)
			}
			active := []int{0}
			if count >= 2 {
				active = []int{1}
			}
			if count >= 3 {
				active = append(active, last)
			}
			if len(cfg.IgnorePatterns) != len(active) || len(cfg.Prompts) != len(active)+1 {
				t.Fatalf("unexpected merged values: %+v", cfg)
			}
			for index, layer := range active {
				if cfg.IgnorePatterns[index] != fmt.Sprintf("pattern %d", layer) || cfg.Prompts[fmt.Sprintf("hook %d", layer)] != "inline:kept" {
					t.Fatalf("layer %d values were lost: %+v", layer, cfg)
				}
			}
		})
	}
}

func TestCurrentConfigSkipsInvalidLegacyFiles(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	project := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "tcrit", "config.json"), "invalid JSON")
	writeConfig(t, filepath.Join(configHome, "critty", "config.json"), `{"author":"global"}`)
	writeConfig(t, filepath.Join(project, ".tcrit.config.json"), "invalid JSON")
	writeConfig(t, filepath.Join(project, ".critty.config.json"), `{}`)
	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Author != "global" {
		t.Fatalf("Author = %q, want global", cfg.Author)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !cfg.CleanupOnApprove {
		t.Error("cleanup_on_approve should default to true")
	}
}
